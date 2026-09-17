// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// pinnedSpawnVersion is the spore.host spawn release strata launches build
// instances with. It is pinned for the same reason the cosign binary is
// (cosignReleaseDigests): a build must not silently pick up a different spawn
// than the one strata's launch contract was written and tested against. spawn
// manages instance lifecycle — TTL and auto-terminate — so a drifted spawn could
// change termination behaviour under strata. strata does not modify spore.host;
// bump this pin deliberately after testing against the new release, and file any
// needed change as an issue on the spawn project.
//
// Provenance: spore.host/spawn CHANGELOG, latest release v0.110.0 (2026-09).
const pinnedSpawnVersion = "v0.110.0"

// spawnBinary is the spawn executable, resolved on PATH. strata shells out to
// the CLI (rather than importing spawn as a library) so that what strata runs is
// visible to the operator and strata stays decoupled from spawn's Go API.
const spawnBinary = "spawn"

// commandRunner runs an external command and returns its combined output. It is
// an interface so the spawn integration is unit-tested without spawn installed.
type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// execRunner runs commands via os/exec, combining stdout and stderr.
type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// verifySpawnVersion refuses to proceed unless the spawn on PATH is exactly the
// pinned version. It runs "spawn version" and parses the "Version:" line. A
// mismatch — or a spawn that cannot report its version — is an error, not a
// warning: an unpinned lifecycle manager is the same weak link an unpinned
// cosign binary would be.
func verifySpawnVersion(ctx context.Context, runner commandRunner) error {
	out, err := runner.Run(ctx, spawnBinary, "version")
	if err != nil {
		return fmt.Errorf("build: running %q version (is spawn %s installed and on PATH?): %w", spawnBinary, pinnedSpawnVersion, err)
	}
	got := parseSpawnVersion(out)
	if got == "" {
		return fmt.Errorf("build: could not parse a version from %q version output", spawnBinary)
	}
	if got != pinnedSpawnVersion {
		return fmt.Errorf("build: spawn %s is not the pinned %s — strata's launch contract is tested against the pinned version; install it or bump the pin deliberately", got, pinnedSpawnVersion)
	}
	return nil
}

// parseSpawnVersion extracts the value from spawn's "Version:    <v>" line.
func parseSpawnVersion(out []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "Version:"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// SpawnLauncher launches strata build instances through the pinned spawn CLI
// instead of the AWS SDK, so every build instance gets spawn's TTL/auto-terminate
// lifecycle — no orphaned instance holding EBS after a failed or hung build,
// which the hand-rolled "stop on failure" path left behind (#236). It shells out
// via commandRunner, capturing the launched instance ID through spawn's
// --output-id file rather than parsing stdout.
type SpawnLauncher struct {
	cfg    EC2Config
	runner commandRunner
	// ttl is the auto-terminate window (e.g. "4h"): every instance terminates
	// after it regardless of build outcome — the hard cost ceiling #236 wants.
	ttl string
}

// NewSpawnLauncher builds a launcher that shells out to the real spawn CLI.
func NewSpawnLauncher(cfg EC2Config, ttl string) *SpawnLauncher {
	return &SpawnLauncher{cfg: cfg, runner: execRunner{}, ttl: ttl}
}

// spawnLaunchArgs builds the "spawn launch" argv from the build config. It is a
// pure function so the launch contract — which flags strata passes spawn — is
// unit-tested without invoking spawn. userDataFile holds the build user-data;
// outputIDFile is where spawn writes the launched instance ID for capture.
func (l *SpawnLauncher) spawnLaunchArgs(name, userDataFile, outputIDFile string) []string {
	args := []string{
		"launch", name,
		"--ami", l.cfg.AMIID,
		"--instance-type", l.cfg.InstanceType,
		"--region", l.cfg.Region,
		"--user-data-file", userDataFile,
		"--on-complete", "terminate",
		"--output-id", outputIDFile,
		"--quiet",
	}
	if l.ttl != "" {
		args = append(args, "--ttl", l.ttl)
	}
	if l.cfg.SubnetID != "" {
		args = append(args, "--subnet-id", l.cfg.SubnetID)
	}
	if l.cfg.SecurityGroupID != "" {
		args = append(args, "--security-group-ids", l.cfg.SecurityGroupID)
	}
	// spawn's --iam-role attaches (or creates) the instance role; strata passes
	// its pre-provisioned builder role, which the build needs for kms:Sign and
	// registry writes.
	if l.cfg.IAMProfile != "" {
		args = append(args, "--iam-role", l.cfg.IAMProfile)
	}
	if v := l.cfg.RootVolumeGB; v > 0 {
		args = append(args, "--volume-size", strconv.Itoa(int(v)))
	}
	return args
}

// Launch writes the user-data to a temp file, verifies the pinned spawn, invokes
// it to launch the instance with a TTL and on-complete=terminate, and returns
// the launched instance ID read from spawn's --output-id file. It is
// fire-and-forget: spawn owns the instance's lifecycle from here, so there is no
// stop-on-failure orphan for strata to sweep.
func (l *SpawnLauncher) Launch(ctx context.Context, name, userData string) (string, error) {
	if err := verifySpawnVersion(ctx, l.runner); err != nil {
		return "", err
	}
	udPath, cleanupUD, err := writeTempFile("strata-userdata-*.sh", userData)
	if err != nil {
		return "", fmt.Errorf("build: writing spawn user-data: %w", err)
	}
	defer cleanupUD()

	idPath, cleanupID, err := writeTempFile("strata-spawn-id-*", "")
	if err != nil {
		return "", fmt.Errorf("build: creating spawn id file: %w", err)
	}
	defer cleanupID()

	args := l.spawnLaunchArgs(name, udPath, idPath)
	if out, err := l.runner.Run(ctx, spawnBinary, args...); err != nil {
		return "", fmt.Errorf("build: spawn launch failed: %w\n%s", err, out)
	}

	id, err := os.ReadFile(idPath)
	if err != nil {
		return "", fmt.Errorf("build: reading spawn instance id: %w", err)
	}
	instanceID := strings.TrimSpace(string(id))
	if instanceID == "" {
		return "", fmt.Errorf("build: spawn launched but wrote no instance id to %s", idPath)
	}
	return instanceID, nil
}

// writeTempFile writes content to a new temp file matching pattern and returns
// its path plus a cleanup func. content may be empty (for an output file spawn
// will fill in).
func writeTempFile(pattern, content string) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", func() {}, err
	}
	path = f.Name()
	cleanup = func() { _ = os.Remove(path) }
	if content != "" {
		if _, err := f.WriteString(content); err != nil {
			_ = f.Close()
			cleanup()
			return "", func() {}, err
		}
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return path, cleanup, nil
}

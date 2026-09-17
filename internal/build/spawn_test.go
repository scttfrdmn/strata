// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// fakeSpawn is a commandRunner standing in for the spawn CLI. It answers
// "version" with versionOut, and for "launch" writes wroteID to the file named
// by --output-id (mimicking spawn --output-id) unless launchErr is set.
type fakeSpawn struct {
	versionOut string
	wroteID    string
	launchErr  error
}

func (f *fakeSpawn) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	if len(args) > 0 && args[0] == "version" {
		return []byte(f.versionOut), nil
	}
	// launch
	if f.launchErr != nil {
		return []byte("boom"), f.launchErr
	}
	for i, a := range args {
		if a == "--output-id" && i+1 < len(args) {
			_ = os.WriteFile(args[i+1], []byte(f.wroteID), 0600)
		}
	}
	return nil, nil
}

func spawnVersionOutput(v string) string {
	return "🌱 Spawn - EC2 Instance Lifecycle Manager\n\nVersion:    " + v + "\nGit Commit: abc123\nBuild Date: 2026-09-01\n"
}

func TestParseSpawnVersion(t *testing.T) {
	if got := parseSpawnVersion([]byte(spawnVersionOutput("v0.110.0"))); got != "v0.110.0" {
		t.Errorf("parseSpawnVersion = %q, want v0.110.0", got)
	}
	if got := parseSpawnVersion([]byte("no version here")); got != "" {
		t.Errorf("parseSpawnVersion of versionless output = %q, want empty", got)
	}
}

func TestVerifySpawnVersion(t *testing.T) {
	ctx := context.Background()

	// Exact pinned version passes.
	if err := verifySpawnVersion(ctx, &fakeSpawn{versionOut: spawnVersionOutput(pinnedSpawnVersion)}); err != nil {
		t.Errorf("verifySpawnVersion rejected the pinned version: %v", err)
	}

	// A different version is refused, and the error names both versions.
	err := verifySpawnVersion(ctx, &fakeSpawn{versionOut: spawnVersionOutput("v0.109.0")})
	if err == nil {
		t.Fatal("verifySpawnVersion accepted an unpinned spawn version")
	}
	if !strings.Contains(err.Error(), "v0.109.0") || !strings.Contains(err.Error(), pinnedSpawnVersion) {
		t.Errorf("error does not name the got and pinned versions: %v", err)
	}

	// Unparseable output is refused.
	if err := verifySpawnVersion(ctx, &fakeSpawn{versionOut: "garbage"}); err == nil {
		t.Error("verifySpawnVersion accepted output with no version line")
	}
}

// errRunner fails every command — models spawn not being installed.
type errRunner struct{}

func (errRunner) Run(context.Context, string, ...string) ([]byte, error) {
	return nil, errors.New("exec: spawn not found")
}

func TestVerifySpawnVersion_NotInstalled(t *testing.T) {
	if err := verifySpawnVersion(context.Background(), errRunner{}); err == nil {
		t.Error("verifySpawnVersion accepted a missing spawn binary")
	}
}

func TestSpawnLaunchArgs(t *testing.T) {
	l := &SpawnLauncher{
		cfg: EC2Config{
			AMIID:           "ami-0c421724a94bba6d6",
			InstanceType:    "c5.4xlarge",
			Region:          "us-east-1",
			SubnetID:        "subnet-123",
			SecurityGroupID: "sg-0fca02f58fafcdad1",
			IAMProfile:      "strata-builder",
			RootVolumeGB:    100,
		},
		ttl: "4h",
	}
	args := l.spawnLaunchArgs("strata-build-python-3.13-x86_64", "/tmp/ud.sh", "/tmp/id")

	// Flatten to "flag value" adjacency checks.
	want := map[string]string{
		"--ami":                "ami-0c421724a94bba6d6",
		"--instance-type":      "c5.4xlarge",
		"--region":             "us-east-1",
		"--user-data-file":     "/tmp/ud.sh",
		"--on-complete":        "terminate",
		"--output-id":          "/tmp/id",
		"--ttl":                "4h",
		"--subnet-id":          "subnet-123",
		"--security-group-ids": "sg-0fca02f58fafcdad1",
		"--iam-role":           "strata-builder",
		"--volume-size":        "100",
	}
	for flag, val := range want {
		if !hasFlagValue(args, flag, val) {
			t.Errorf("args missing %s %s; got %v", flag, val, args)
		}
	}
	if args[0] != "launch" || args[1] != "strata-build-python-3.13-x86_64" {
		t.Errorf("args do not start with 'launch <name>': %v", args[:2])
	}
	if !contains(args, "--quiet") {
		t.Errorf("args missing --quiet: %v", args)
	}

	// Optionals omitted when unset: no --ttl, --subnet-id, etc.
	bare := &SpawnLauncher{cfg: EC2Config{AMIID: "ami-x", InstanceType: "t3.micro", Region: "us-east-1"}}
	ba := bare.spawnLaunchArgs("n", "/tmp/u", "/tmp/i")
	for _, absent := range []string{"--ttl", "--subnet-id", "--security-group-ids", "--iam-role", "--volume-size"} {
		if contains(ba, absent) {
			t.Errorf("bare args should not contain %s: %v", absent, ba)
		}
	}
}

func TestSpawnLauncher_Launch(t *testing.T) {
	ctx := context.Background()
	cfg := EC2Config{AMIID: "ami-x", InstanceType: "c5.4xlarge", Region: "us-east-1", IAMProfile: "strata-builder"}

	// Happy path: version matches, spawn writes the id.
	fake := &fakeSpawn{versionOut: spawnVersionOutput(pinnedSpawnVersion), wroteID: "i-0abc123\n"}
	l := &SpawnLauncher{cfg: cfg, runner: fake, ttl: "4h"}
	id, err := l.Launch(ctx, "strata-build-x", "#!/bin/bash\necho hi\n")
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if id != "i-0abc123" {
		t.Errorf("Launch id = %q, want i-0abc123 (trimmed)", id)
	}

	// A version mismatch aborts before launch — the launch args are never sent.
	mismatch := &fakeSpawn{versionOut: spawnVersionOutput("v0.109.0"), wroteID: "i-should-not"}
	if _, err := (&SpawnLauncher{cfg: cfg, runner: mismatch}).Launch(ctx, "n", "ud"); err == nil {
		t.Error("Launch proceeded with an unpinned spawn")
	}

	// A launch failure surfaces.
	failing := &fakeSpawn{versionOut: spawnVersionOutput(pinnedSpawnVersion), launchErr: errors.New("capacity")}
	if _, err := (&SpawnLauncher{cfg: cfg, runner: failing}).Launch(ctx, "n", "ud"); err == nil {
		t.Error("Launch swallowed a spawn error")
	}

	// spawn returns success but writes no id → error, not an empty instance id.
	empty := &fakeSpawn{versionOut: spawnVersionOutput(pinnedSpawnVersion), wroteID: ""}
	if _, err := (&SpawnLauncher{cfg: cfg, runner: empty}).Launch(ctx, "n", "ud"); err == nil {
		t.Error("Launch accepted an empty instance id")
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func hasFlagValue(args []string, flag, val string) bool {
	for i, a := range args {
		if a == flag && i+1 < len(args) && args[i+1] == val {
			return true
		}
	}
	return false
}

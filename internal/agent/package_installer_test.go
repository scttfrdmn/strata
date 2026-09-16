// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

type recordedCall struct {
	name string
	args []string
}

// recordingRunner returns a cmdRunner that records every call instead of
// shelling out, so a test asserts the exact pip/conda invocation.
func recordingRunner(calls *[]recordedCall) cmdRunner {
	return func(_ context.Context, _, name string, args ...string) error {
		*calls = append(*calls, recordedCall{name: name, args: append([]string{}, args...)})
		return nil
	}
}

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// TestInstallPip_EnforcesRecordedHash: when an entry records a SHA256, pip is
// invoked with --require-hashes and --hash=sha256:<recorded>, pinned to the exact
// version. This is the fix for #98 — the recorded hash was ignored, so the same
// EnvironmentID could install different bytes.
func TestInstallPip_EnforcesRecordedHash(t *testing.T) {
	const sha = "abc123abc123abc123abc123abc123abc123abc123abc123abc123abc123abcd"
	var calls []recordedCall
	err := installPip(context.Background(),
		[]spec.ResolvedPackageEntry{{Name: "numpy", Version: "1.26.4", SHA256: sha}},
		"/bin", recordingRunner(&calls))
	if err != nil {
		t.Fatalf("installPip: %v", err)
	}
	if len(calls) != 1 || calls[0].name != "pip" {
		t.Fatalf("expected one pip call, got %+v", calls)
	}
	args := calls[0].args
	for _, want := range []string{"--require-hashes", "--no-deps", "numpy==1.26.4", "--hash=sha256:" + sha} {
		if !hasArg(args, want) {
			t.Errorf("pip args missing %q; got %v", want, args)
		}
	}
}

// TestInstallPip_PinsVersionWithoutHash is the contrast case that keeps the one
// above honest: with no recorded hash, pip still pins the version but must NOT be
// given a --hash (there is nothing to enforce). The two invocations must differ.
func TestInstallPip_PinsVersionWithoutHash(t *testing.T) {
	var calls []recordedCall
	err := installPip(context.Background(),
		[]spec.ResolvedPackageEntry{{Name: "numpy", Version: "1.26.4"}},
		"/bin", recordingRunner(&calls))
	if err != nil {
		t.Fatalf("installPip: %v", err)
	}
	args := calls[0].args
	if !hasArg(args, "numpy==1.26.4") {
		t.Errorf("version not pinned; got %v", args)
	}
	for _, unwanted := range []string{"--require-hashes", "--no-deps"} {
		if hasArg(args, unwanted) {
			t.Errorf("hashless install must not use %q; got %v", unwanted, args)
		}
	}
	for _, a := range args {
		if strings.HasPrefix(a, "--hash") {
			t.Errorf("hashless install passed a --hash flag: %q", a)
		}
	}
}

// TestInstallPip_RefusesFloatingVersion: an entry with no version cannot be
// pinned, so nothing is installed rather than floating.
func TestInstallPip_RefusesFloatingVersion(t *testing.T) {
	var calls []recordedCall
	err := installPip(context.Background(),
		[]spec.ResolvedPackageEntry{{Name: "numpy", Version: ""}},
		"/bin", recordingRunner(&calls))
	if err == nil {
		t.Fatal("installPip accepted an unversioned package")
	}
	if len(calls) != 0 {
		t.Errorf("a floating package was installed: %+v", calls)
	}
}

// TestInstallConda_PinsExactVersion: a pinned version installs name=version.
func TestInstallConda_PinsExactVersion(t *testing.T) {
	var calls []recordedCall
	err := installConda(context.Background(),
		[]spec.ResolvedPackageEntry{{Name: "mamba", Version: "1.5.8"}},
		"", "/bin", recordingRunner(&calls))
	if err != nil {
		t.Fatalf("installConda: %v", err)
	}
	if len(calls) != 1 || !hasArg(calls[0].args, "mamba=1.5.8") {
		t.Errorf("expected conda install of mamba=1.5.8; got %+v", calls)
	}
}

// TestInstallConda_RefusesFloatingVersion drives both floating forms the old code
// silently accepted — "latest" and "" — which installed whatever the channel
// served at boot. Both must now fail with nothing installed.
func TestInstallConda_RefusesFloatingVersion(t *testing.T) {
	for _, ver := range []string{"latest", ""} {
		var calls []recordedCall
		err := installConda(context.Background(),
			[]spec.ResolvedPackageEntry{{Name: "mamba", Version: ver}},
			"", "/bin", recordingRunner(&calls))
		if err == nil {
			t.Errorf("installConda accepted floating version %q", ver)
		}
		if len(calls) != 0 {
			t.Errorf("floating version %q installed anyway: %+v", ver, calls)
		}
	}
}

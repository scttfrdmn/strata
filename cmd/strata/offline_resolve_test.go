package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/testregistry"
	"github.com/scttfrdmn/strata/spec"
)

// This is the CLI half of #54: the property that closes the issue is not "the
// resolver package can resolve" but "someone with a fresh clone and no AWS
// credentials can run strata and get a lockfile". That path goes through
// buildRegistryClient and buildProbeClient, which is where it was broken —
// STRATA_REGISTRY_URL was handed to NewS3Client whatever its scheme, so a
// file:// URL failed the s3:// check and fell silently back to the embedded
// recipe catalog, which has no bundles and dies in stage 7.

// clearAWSEnv blanks the credentials the AWS SDK would otherwise discover, so
// that a test passing here cannot be passing because the machine running it
// happens to be able to reach AWS.
func clearAWSEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"AWS_PROFILE", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
		"AWS_REGION", "AWS_DEFAULT_REGION", "AWS_SHARED_CREDENTIALS_FILE", "AWS_CONFIG_FILE",
	} {
		t.Setenv(k, "")
	}
}

// TestResolveCmdOfflineWritesLockfile runs the resolve command exactly as the
// CLI does and asserts the artifact appears on disk.
func TestResolveCmdOfflineWritesLockfile(t *testing.T) {
	clearAWSEnv(t)

	work := t.TempDir()
	root, err := testregistry.Materialize(context.Background(), filepath.Join(work, "registry"))
	if err != nil {
		t.Fatalf("materializing fixture registry: %v", err)
	}
	t.Setenv("STRATA_REGISTRY_URL", testregistry.URI(root))

	profile := testregistry.WriteProfile(t, work, testregistry.ProfileMinimal)
	out := filepath.Join(work, "offline.lock.yaml")

	cmd := newResolveCmd()
	cmd.SetArgs([]string{profile, "-o", out})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("strata resolve: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("no lockfile written: %v", err)
	}
	lf, err := spec.ParseLockFileBytes(data)
	if err != nil {
		t.Fatalf("parsing written lockfile: %v", err)
	}
	if len(lf.Layers) != len(testregistry.LayerIDs) {
		t.Fatalf("lockfile has %d layers, want %d", len(lf.Layers), len(testregistry.LayerIDs))
	}
	if lf.Base.AMIID == "" {
		t.Error("lockfile has no base AMI ID")
	}
	for _, layer := range lf.Layers {
		if layer.Bundle == "" || layer.SHA256 == "" {
			t.Errorf("%s: bundle=%q sha256=%q — resolve should not emit an unattested layer",
				layer.ID, layer.Bundle, layer.SHA256)
		}
	}
}

// TestResolveCmdOfflineFormation covers the formation profile through the same
// CLI path.
func TestResolveCmdOfflineFormation(t *testing.T) {
	clearAWSEnv(t)

	work := t.TempDir()
	root, err := testregistry.Materialize(context.Background(), filepath.Join(work, "registry"))
	if err != nil {
		t.Fatalf("materializing fixture registry: %v", err)
	}
	t.Setenv("STRATA_REGISTRY_URL", testregistry.URI(root))

	profile := testregistry.WriteProfile(t, work, testregistry.ProfileFormation)
	out := filepath.Join(work, "formation.lock.yaml")

	cmd := newResolveCmd()
	cmd.SetArgs([]string{profile, "-o", out})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("strata resolve: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("no lockfile written: %v", err)
	}
}

// TestResolveCmdFileRegistryIsNotSilentlyIgnored is the negative control for the
// defect that was fixed. Pointed at an empty file:// registry, resolve must fail
// because that registry has no layers — not succeed against the embedded catalog
// and not fail with an S3 URL complaint. A silent fallback is the failure mode
// that hid this for a release: the warning went to stderr and the resolve
// carried on against a different registry than the one that was asked for.
func TestResolveCmdFileRegistryIsNotSilentlyIgnored(t *testing.T) {
	clearAWSEnv(t)

	work := t.TempDir()
	empty := filepath.Join(work, "empty-registry")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv("STRATA_REGISTRY_URL", "file://"+filepath.ToSlash(empty))

	profile := testregistry.WriteProfile(t, work, testregistry.ProfileMinimal)
	out := filepath.Join(work, "should-not-exist.lock.yaml")

	cmd := newResolveCmd()
	cmd.SetArgs([]string{profile, "-o", out})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("resolve succeeded against an empty file:// registry — it fell back to another catalog")
	}
	if strings.Contains(err.Error(), "invalid S3 URL") {
		t.Errorf("file:// URL was routed to the S3 client: %v", err)
	}
	if !strings.Contains(err.Error(), "LAYER_NOT_FOUND") {
		t.Errorf("want a layer-not-found failure from the empty registry, got: %v", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Error("a failed resolve wrote a lockfile")
	}
}

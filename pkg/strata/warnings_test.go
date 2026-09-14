package strata_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/registry"
	pkgstrata "github.com/scttfrdmn/strata/pkg/strata"
	"github.com/scttfrdmn/strata/spec"
)

// placeholderFormationStore builds a MemoryStore whose one formation carries the
// "pending-initial-build" placeholder rekor_entry — the input the resolver warns
// about at stage 2 (internal/resolver/stages.go). It reuses gccManifest from
// strata_test.go as the formation's single (properly attested) layer, so the
// placeholder is the formation's, not a layer's, and resolution still succeeds.
func placeholderFormationStore(t *testing.T) *registry.MemoryStore {
	t.Helper()
	store := registry.NewMemoryStore()
	store.AddLayer(gccManifest("x86_64"))
	store.AddFormation(&spec.Formation{
		Name:       "tools",
		Version:    "1.0",
		Bundle:     "s3://strata-registry/formations/tools@1.0/bundle.json",
		RekorEntry: "pending-initial-build",
		Layers:     []spec.SoftwareRef{{Name: "gcc", Version: "13.2.0"}},
	})
	return store
}

func placeholderProfile() *spec.Profile {
	return &spec.Profile{
		Name:     "test",
		Base:     spec.BaseRef{OS: "al2023", Arch: "x86_64"},
		Software: []spec.SoftwareRef{{Formation: "tools@1.0"}},
	}
}

// TestResolve_SurfacesPlaceholderAttestationWarning is #138: the pkg/strata
// library route built resolver.Config with no Warnings writer, so a caller
// resolving a formation with a placeholder attestation was never told the
// environment has no Rekor entry. The placeholder is not propagated into the
// lockfile (#46), so this warning is the only signal. With Options.Warnings
// supplied — here through the WithWarnings seam, since the public NewClient path
// needs a live S3 registry — the warning reaches the caller. If c.warnings were
// not threaded into resolver.Config, warn would be a no-op and buf would stay
// empty, so this test fails on the regression it guards.
func TestResolve_SurfacesPlaceholderAttestationWarning(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	c := pkgstrata.NewClientFromRegistry(placeholderFormationStore(t), "", pkgstrata.WithWarnings(&buf))

	if _, err := c.Resolve(ctx, placeholderProfile(), pkgstrata.ResolveOptions{AMI: "ami-test"}); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.Contains(buf.String(), "no Rekor attestation") {
		t.Errorf("warnings writer did not receive the placeholder-attestation warning; got %q", buf.String())
	}
}

// TestResolve_WarningsNilByDefaultDoesNotPanic is the control and the
// library-etiquette guarantee: with no Warnings writer, the same resolve still
// succeeds and writes nowhere. It is paired with the test above — that one fails
// if the writer is not threaded at all, this one fails if a nil writer is not
// tolerated — so a fix cannot satisfy one by breaking the other.
func TestResolve_WarningsNilByDefaultDoesNotPanic(t *testing.T) {
	ctx := context.Background()
	c := pkgstrata.NewClientFromRegistry(placeholderFormationStore(t), "")
	if _, err := c.Resolve(ctx, placeholderProfile(), pkgstrata.ResolveOptions{AMI: "ami-test"}); err != nil {
		t.Fatalf("Resolve with nil warnings writer: %v", err)
	}
}

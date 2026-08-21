package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/scttfrdmn/strata/internal/testregistry"
	"github.com/scttfrdmn/strata/spec"
)

// This is the CLI half of #53. Parsing the inline form is a spec-package
// property and is covered there; what a user actually asked for is that a
// profile written the way the documentation writes it resolves. So this runs the
// documented syntax through the whole resolver offline, with no AWS credentials,
// and asserts it produces the same environment as the mapping form.
//
// The two lockfiles are compared on layer identity, content digest and mount
// order rather than on EnvironmentID: the fixture's base carries no AMI SHA256,
// so the fixture lockfile is not frozen and EnvironmentID is the empty string
// for both. Asserting two empty strings are equal would look like a strong claim
// and prove nothing.
func TestResolveCmdOfflineInlineFormMatchesMappingForm(t *testing.T) {
	clearAWSEnv(t)

	work := t.TempDir()
	root, err := testregistry.Materialize(context.Background(), filepath.Join(work, "registry"))
	if err != nil {
		t.Fatalf("materializing fixture registry: %v", err)
	}
	t.Setenv("STRATA_REGISTRY_URL", testregistry.URI(root))

	resolve := func(profileName, outName string) *spec.LockFile {
		t.Helper()
		profile := testregistry.WriteProfile(t, work, profileName)
		out := filepath.Join(work, outName)

		cmd := newResolveCmd()
		cmd.SetArgs([]string{profile, "-o", out})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("strata resolve %s: %v", profileName, err)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatalf("%s: no lockfile written: %v", profileName, err)
		}
		lf, err := spec.ParseLockFileBytes(data)
		if err != nil {
			t.Fatalf("%s: parsing written lockfile: %v", profileName, err)
		}
		return lf
	}

	inline := resolve(testregistry.ProfileInline, "inline.lock.yaml")
	mapping := resolve(testregistry.ProfileMinimal, "mapping.lock.yaml")

	if len(inline.Layers) != len(testregistry.LayerIDs) {
		t.Fatalf("inline-form lockfile has %d layers, want %d", len(inline.Layers), len(testregistry.LayerIDs))
	}
	if len(inline.Layers) != len(mapping.Layers) {
		t.Fatalf("inline form resolved %d layers, mapping form %d", len(inline.Layers), len(mapping.Layers))
	}

	for i := range mapping.Layers {
		want, got := mapping.Layers[i], inline.Layers[i]
		if got.ID != want.ID {
			t.Errorf("layer[%d] ID: inline form gave %q, mapping form %q", i, got.ID, want.ID)
		}
		if got.SHA256 != want.SHA256 {
			t.Errorf("layer[%d] %s SHA256: inline form gave %q, mapping form %q", i, want.ID, got.SHA256, want.SHA256)
		}
		if got.MountOrder != want.MountOrder {
			t.Errorf("layer[%d] %s MountOrder: inline form gave %d, mapping form %d", i, want.ID, got.MountOrder, want.MountOrder)
		}
	}

	// The declared refs are recorded in the lockfile as the thing each layer
	// satisfies. They must survive the inline form, or provenance would report a
	// request the user never made.
	for _, layer := range inline.Layers {
		if layer.SatisfiedBy == "" {
			t.Errorf("layer %s has an empty satisfied_by after resolving the inline form", layer.ID)
		}
	}
}

// TestFixtureProfilesAllResolveOffline covers every fixture profile through the
// CLI, so adding one to ProfileNames without a test is not silently possible.
func TestFixtureProfilesAllResolveOffline(t *testing.T) {
	clearAWSEnv(t)

	for _, name := range testregistry.ProfileNames() {
		t.Run(name, func(t *testing.T) {
			work := t.TempDir()
			root, err := testregistry.Materialize(context.Background(), filepath.Join(work, "registry"))
			if err != nil {
				t.Fatalf("materializing fixture registry: %v", err)
			}
			t.Setenv("STRATA_REGISTRY_URL", testregistry.URI(root))

			profile := testregistry.WriteProfile(t, work, name)
			out := filepath.Join(work, "out.lock.yaml")

			cmd := newResolveCmd()
			cmd.SetArgs([]string{profile, "-o", out})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("strata resolve %s: %v", name, err)
			}
			if _, err := os.Stat(out); err != nil {
				t.Fatalf("%s: no lockfile written: %v", name, err)
			}
		})
	}
}

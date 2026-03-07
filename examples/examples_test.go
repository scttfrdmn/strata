// Package examples_test verifies that all example profiles parse and validate
// correctly. These tests act as a living contract: if the spec changes in a
// way that breaks real-world profiles, they fail here first.
package examples_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

// profiles lists every example profile that must parse and validate cleanly.
var profiles = []string{
	"alphafold3.yaml",
	"r-quarto-workstation.yaml",
	"pytorch-jupyter.yaml",
}

func TestExampleProfilesParse(t *testing.T) {
	for _, name := range profiles {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(".", name)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			p, err := spec.ParseProfileBytes(data)
			if err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}
			if p.Name == "" {
				t.Errorf("%s: Name is empty after parse", name)
			}
			if p.Base.OS == "" {
				t.Errorf("%s: Base.OS is empty after parse", name)
			}
			if len(p.Software) == 0 {
				t.Errorf("%s: Software list is empty after parse", name)
			}
		})
	}
}

// TestExampleProfilesRoundTrip verifies that every example profile
// marshals and re-parses to the same value.
func TestExampleProfilesRoundTrip(t *testing.T) {
	for _, name := range profiles {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(".", name))
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			p, err := spec.ParseProfileBytes(data)
			if err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}
			marshaled, err := spec.MarshalProfile(p)
			if err != nil {
				t.Fatalf("marshal %s: %v", name, err)
			}
			p2, err := spec.ParseProfileBytes(marshaled)
			if err != nil {
				t.Fatalf("re-parse %s after marshal: %v", name, err)
			}
			if p.Name != p2.Name {
				t.Errorf("%s: Name mismatch after round-trip: %q != %q", name, p.Name, p2.Name)
			}
			if len(p.Software) != len(p2.Software) {
				t.Errorf("%s: Software count mismatch after round-trip: %d != %d", name, len(p.Software), len(p2.Software))
			}
			for i := range p.Software {
				if p.Software[i].String() != p2.Software[i].String() {
					t.Errorf("%s: Software[%d] mismatch: %q != %q", name, i, p.Software[i].String(), p2.Software[i].String())
				}
			}
		})
	}
}

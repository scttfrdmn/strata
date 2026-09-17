// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package spec

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestNewBuildEnvironment_SortsAndDedups: the recorded package set is
// order-stable and duplicate-free regardless of how rpm -qa emitted it, so two
// builds that installed the same packages record an identical BuildEnvironment.
func TestNewBuildEnvironment_SortsAndDedups(t *testing.T) {
	a := NewBuildEnvironment("ami-x", "2023.6.20260101",
		[]string{"zlib-devel-1.2.11-x86_64", "gcc-11.4.1-x86_64", "", "gcc-11.4.1-x86_64", "openssl-devel-3.0-x86_64"})
	want := []string{"gcc-11.4.1-x86_64", "openssl-devel-3.0-x86_64", "zlib-devel-1.2.11-x86_64"}
	if len(a.Packages) != len(want) {
		t.Fatalf("Packages = %v, want %v", a.Packages, want)
	}
	for i := range want {
		if a.Packages[i] != want[i] {
			t.Fatalf("Packages = %v, want %v (sorted, deduped, empties dropped)", a.Packages, want)
		}
	}

	// Same packages in a different order → identical record.
	b := NewBuildEnvironment("ami-x", "2023.6.20260101",
		[]string{"openssl-devel-3.0-x86_64", "gcc-11.4.1-x86_64", "zlib-devel-1.2.11-x86_64"})
	if strings.Join(a.Packages, ",") != strings.Join(b.Packages, ",") {
		t.Errorf("package order changed the record: %v vs %v", a.Packages, b.Packages)
	}
}

func TestBuildEnvironment_Recorded(t *testing.T) {
	cases := []struct {
		name string
		env  *BuildEnvironment
		want bool
	}{
		{"nil", nil, false},
		{"empty", &BuildEnvironment{}, false},
		{"ami only", &BuildEnvironment{AMIID: "ami-x"}, false},
		{"packages only", &BuildEnvironment{Packages: []string{"gcc-11-x86_64"}}, false},
		{"ami + packages", NewBuildEnvironment("ami-x", "2023.6", []string{"gcc-11-x86_64"}), true},
	}
	for _, tc := range cases {
		if got := tc.env.Recorded(); got != tc.want {
			t.Errorf("%s: Recorded() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestBuildEnvironment_ManifestRoundTrip: a manifest carrying a BuildEnvironment
// serialises and parses back intact, and a manifest without one omits the field
// entirely (omitempty) rather than emitting an empty block.
func TestBuildEnvironment_ManifestRoundTrip(t *testing.T) {
	m := &LayerManifest{
		ID:               "python-3.13-linux-gnu-2.34-x86_64",
		Name:             "python",
		Version:          "3.13",
		BuildEnvironment: NewBuildEnvironment("ami-0c421724a94bba6d6", "2023.6.20260101", []string{"gcc-11.4.1-x86_64", "glibc-2.34-x86_64"}),
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got LayerManifest
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.BuildEnvironment.Recorded() {
		t.Fatal("round-tripped manifest lost its BuildEnvironment")
	}
	if got.BuildEnvironment.AMIID != "ami-0c421724a94bba6d6" || got.BuildEnvironment.Releasever != "2023.6.20260101" {
		t.Errorf("BuildEnvironment fields not preserved: %+v", got.BuildEnvironment)
	}

	// Absent when unset.
	bare, err := yaml.Marshal(&LayerManifest{ID: "x", Name: "x", Version: "1"})
	if err != nil {
		t.Fatalf("marshal bare: %v", err)
	}
	if strings.Contains(string(bare), "build_environment") {
		t.Errorf("nil BuildEnvironment was serialised: %s", bare)
	}
}

package main

import (
	"testing"
)

func TestParseNameVersion(t *testing.T) {
	tests := []struct {
		arg     string
		name    string
		version string
	}{
		{"python@3.11.11", "python", "3.11.11"},
		{"gcc@13.2.0", "gcc", "13.2.0"},
		{"python", "python", ""},
		{"r@4.4.3", "r", "4.4.3"},
	}
	for _, tt := range tests {
		n, v := parseNameVersion(tt.arg)
		if n != tt.name || v != tt.version {
			t.Errorf("parseNameVersion(%q) = (%q, %q), want (%q, %q)",
				tt.arg, n, v, tt.name, tt.version)
		}
	}
}

func TestNewRemoveCmd_MissingRegistry(t *testing.T) {
	// Verify the command returns an error when --registry is not set.
	t.Setenv("STRATA_REGISTRY_URL", "")
	cmd := newRemoveCmd()
	cmd.SetArgs([]string{"python@3.11.11"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --registry, got nil")
	}
}

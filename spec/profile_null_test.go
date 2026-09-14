package spec_test

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/spec"
)

// TestProfile_NullSoftwareEntryRejected is #79: a null entry in the software list
// (a bare "-" or a leftover dash) must be an error, not silently dropped.
func TestProfile_NullSoftwareEntryRejected(t *testing.T) {
	for name, y := range map[string]string{
		"bare dash":     "name: x\nbase:\n  os: al2023\nsoftware:\n  - python@3.13\n  -\n",
		"leading null":  "name: x\nbase:\n  os: al2023\nsoftware:\n  -\n  - python@3.13\n",
		"explicit null": "name: x\nbase:\n  os: al2023\nsoftware:\n  - python@3.13\n  - null\n",
		"tilde null":    "name: x\nbase:\n  os: al2023\nsoftware:\n  - ~\n",
	} {
		t.Run(name, func(t *testing.T) {
			var p spec.Profile
			err := yaml.Unmarshal([]byte(y), &p)
			if err == nil {
				t.Fatalf("null software entry accepted; parsed %d entries (#79)", len(p.Software))
			}
			if !strings.Contains(err.Error(), "empty entry") {
				t.Errorf("want an empty-entry error, got: %v", err)
			}
		})
	}
}

// TestProfile_ValidSoftwareStillParses is the control: a well-formed software
// list (scalar and mapping forms) is unaffected.
func TestProfile_ValidSoftwareStillParses(t *testing.T) {
	y := "name: x\nbase:\n  os: al2023\nsoftware:\n  - python@3.13\n  - formation: ml@2026.03\n  - name: quarto\n    version: \"1.4\"\n"
	var p spec.Profile
	if err := yaml.Unmarshal([]byte(y), &p); err != nil {
		t.Fatalf("valid profile rejected: %v", err)
	}
	if len(p.Software) != 3 {
		t.Errorf("want 3 software entries, got %d", len(p.Software))
	}
}

package lintschema

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGolangciConfigValidatesAgainstVendoredSchema is the CI enforcement that
// replaces the networked `golangci-lint config verify`: the shipped
// .golangci.yml must satisfy the vendored schema. A typo that would silently
// disable a linter — a misspelled key, a wrong-typed block — fails here instead
// of passing a gate that fetched a schema from a host we do not control (#107).
func TestGolangciConfigValidatesAgainstVendoredSchema(t *testing.T) {
	cfg, err := os.ReadFile(filepath.Join("..", "..", ".golangci.yml"))
	if err != nil {
		t.Fatalf("read .golangci.yml: %v", err)
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("the shipped .golangci.yml does not satisfy the vendored schema: %v", err)
	}
}

// TestSchemaRejectsInvalidConfig is the control: a validator that accepts
// everything looks exactly like a clean config, so it must be shown to reject.
// Each case violates the schema in a distinct way.
func TestSchemaRejectsInvalidConfig(t *testing.T) {
	cases := map[string]string{
		"linters is not an object":   "version: \"2\"\nlinters: 123\n",
		"enable is not a list":       "version: \"2\"\nlinters:\n  enable: \"govet\"\n",
		"unknown top-level property": "version: \"2\"\nnot_a_real_section: true\n",
		"version is the wrong type":  "version: [2]\n",
		"not valid yaml at all":      "version: \"2\"\n\tenable: [oops]\n", // tab indent → parse error
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Validate([]byte(cfg)); err == nil {
				t.Errorf("schema accepted an invalid config (%s); the validation is vacuous", name)
			}
		})
	}
}

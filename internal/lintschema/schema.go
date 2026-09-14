// Package lintschema vendors the golangci-lint configuration JSONSchema and
// validates .golangci.yml against it offline.
//
// CI's Lint gate previously fetched this schema over the network at run time
// (golangci-lint-action's `verify: true` default), so a host this repository
// does not control could make the gate that certifies every other trust claim
// fail on clean code, or — worse — pass while enforcing less (#107). The gate now
// runs with `verify: false`, and this package holds the schema instead: it moves
// only in a diff, and validation happens against something the repository holds
// rather than something it fetches. Refresh the embedded file when the pinned
// linter version bumps (see .github/workflows/ci.yml); it is pinned to v2.11.
package lintschema

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

//go:embed golangci.v2.11.jsonschema.json
var schemaJSON []byte

// Validate checks a golangci-lint YAML configuration against the vendored
// schema, returning a non-nil error describing the first violation. The config
// is round-tripped through JSON so its values are the native JSON types the
// validator expects, rather than the YAML decoder's Go types.
func Validate(configYAML []byte) error {
	schemaDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return fmt.Errorf("lintschema: decoding vendored schema: %w", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("golangci.json", schemaDoc); err != nil {
		return fmt.Errorf("lintschema: adding schema resource: %w", err)
	}
	sch, err := c.Compile("golangci.json")
	if err != nil {
		return fmt.Errorf("lintschema: compiling schema: %w", err)
	}

	var raw any
	if err := yaml.Unmarshal(configYAML, &raw); err != nil {
		return fmt.Errorf("lintschema: parsing config YAML: %w", err)
	}
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("lintschema: normalising config to JSON: %w", err)
	}
	config, err := jsonschema.UnmarshalJSON(bytes.NewReader(jsonBytes))
	if err != nil {
		return fmt.Errorf("lintschema: decoding normalised config: %w", err)
	}
	if err := sch.Validate(config); err != nil {
		return fmt.Errorf("lintschema: config does not satisfy the golangci-lint schema: %w", err)
	}
	return nil
}

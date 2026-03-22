package scan

import (
	"context"
	"errors"
	"testing"
)

// mockRunner is a test ExecRunner.
type mockRunner struct {
	outputs map[string][]byte
	errs    map[string]error
}

func (m *mockRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	key := name
	for _, a := range args {
		key += " " + a
	}
	if err, ok := m.errs[key]; ok {
		return nil, err
	}
	if out, ok := m.outputs[key]; ok {
		return out, nil
	}
	return nil, errors.New("command not found: " + key)
}

func TestDetectPip_JSON(t *testing.T) {
	runner := &mockRunner{
		outputs: map[string][]byte{
			"python3 -c import sys; print(sys.prefix)": []byte("/usr/local\n"),
			"python3 -m pip list --format=json": []byte(`[
				{"name": "numpy", "version": "1.26.4"},
				{"name": "scipy", "version": "1.12.0"}
			]`),
		},
	}

	pkgs, err := DetectPip(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("got %d packages, want 2", len(pkgs))
	}
	if pkgs[0].Name != "numpy" {
		t.Errorf("pkgs[0].Name = %q, want numpy", pkgs[0].Name)
	}
}

func TestDetectPip_NoPython(t *testing.T) {
	runner := &mockRunner{
		errs: map[string]error{
			"python3 -c import sys; print(sys.prefix)": errors.New("not found"),
		},
	}

	pkgs, err := DetectPip(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 0 {
		t.Errorf("expected empty result, got %d", len(pkgs))
	}
}

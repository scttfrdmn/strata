package spec

import "testing"

func TestValidateLayerID(t *testing.T) {
	// Realistic resolver-produced IDs must pass — a validator that rejected
	// these would break every real lockfile, so they are the non-vacuity anchor
	// for the rejections below.
	valid := []string{
		"python-3.13.2-linux-gnu-2.34-x86_64",
		"gcc-13.2.0-linux-gnu-2.34-aarch64",
		"openmpi-5.0.1-linux-gnu-2.34-x86_64",
		"R-4.4.3-linux-gnu-2.34-x86_64",
		"a", // a single character is a valid component
	}
	for _, id := range valid {
		if err := ValidateLayerID(id); err != nil {
			t.Errorf("ValidateLayerID(%q) = %v, want nil (a real resolver id must pass)", id, err)
		}
	}

	// Each of these would, joined to a base dir and Cleaned by filepath.Join,
	// either escape that dir or name the dir itself.
	invalid := []struct {
		name string
		id   string
	}{
		{"empty", ""},
		{"dot", "."},
		{"dotdot", ".."},
		{"parent traversal", "../escape"},
		{"deep traversal", "../../etc/cron.d/payload"},
		{"leading slash (absolute)", "/etc/passwd"},
		{"embedded separator", "a/b"},
		{"traversal after a component", "foo/../../bar"},
		{"backslash separator", `a\b`},
		{"NUL byte", "a\x00b"},
	}
	for _, tc := range invalid {
		if err := ValidateLayerID(tc.id); err == nil {
			t.Errorf("ValidateLayerID(%q) = nil, want error (%s)", tc.id, tc.name)
		}
	}
}

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// firstYAMLBlock is the first ```yaml fenced block in a markdown file.
var firstYAMLBlock = regexp.MustCompile("(?s)```yaml\\n(.*?)```")

// TestReadmeExampleResolves is #71's closing condition: the README's flagship
// profile must resolve against the shipped catalog, not merely look plausible.
// The previous example did not parse (a `formation:r-research@2024.03` scalar and
// a version the catalog did not contain), and nothing noticed. This extracts the
// exact fenced block the README shows and resolves it, so the example cannot drift
// out of parseability without turning CI red.
func TestReadmeExampleResolves(t *testing.T) {
	clearAWSEnv(t)
	t.Setenv("STRATA_REGISTRY_URL", "")

	md, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	m := firstYAMLBlock.FindSubmatch(md)
	if m == nil {
		t.Fatal("no ```yaml block found in README.md; the flagship example is gone")
	}
	profileYAML := m[1]
	if !strings.Contains(string(profileYAML), "software:") {
		t.Fatalf("the first README yaml block is not a profile:\n%s", profileYAML)
	}

	work := t.TempDir()
	profile := filepath.Join(work, "readme.yaml")
	if err := os.WriteFile(profile, profileYAML, 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	out := filepath.Join(work, "readme.lock.yaml")

	cmd := newResolveCmd()
	cmd.SetArgs([]string{profile, "-o", out})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("the README example does not resolve against the shipped catalog: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("no lockfile written: %v", err)
	}
}

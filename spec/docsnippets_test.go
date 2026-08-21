package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestDocumentedProfileSnippetsParse is the acceptance test #53 names: no
// documented software: snippet in the repository fails to parse. Seven
// documentation sites taught the inline form while the parser rejected it, and
// nothing mechanical noticed, because the only profiles under test lived in
// examples/ and were written in the one form that worked.
//
// The check is deliberately narrow. It unmarshals; it does not Validate. A
// documentation snippet is a fragment and is entitled to omit name: or base:,
// so requiring a valid Profile would make this test a documentation style guide
// instead of a parser invariant.
func TestDocumentedProfileSnippetsParse(t *testing.T) {
	snippets := harvestProfileSnippets(t, "..")
	if len(snippets) == 0 {
		t.Fatal("harvested no profile snippets — the harvester is broken, not the docs")
	}
	t.Logf("checking %d profile snippets across the repository's markdown", len(snippets))

	for _, s := range snippets {
		t.Run(s.where, func(t *testing.T) {
			var p Profile
			if err := yaml.Unmarshal([]byte(s.body), &p); err != nil {
				t.Errorf("documented snippet does not parse: %v\n\n%s", err, s.body)
			}
		})
	}
}

// snippet is one fenced YAML block from a markdown file.
type snippet struct {
	where string // "README.md:17" — file and the line the fence opens on
	body  string
}

// harvestProfileSnippets returns every fenced yaml block under root whose
// top-level keys make it a profile. Blocks are matched by structure rather than
// by filename so that a new document is covered the day it is written.
func harvestProfileSnippets(t *testing.T, root string) []snippet {
	t.Helper()

	var out []snippet
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "bin", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		out = append(out, profileBlocks(rel, string(data))...)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return out
}

// profileBlocks extracts the fenced yaml blocks in one markdown document that
// are profile-shaped: they carry a top-level software: or defaults: key, the two
// keys whose values are software refs. A block that merely mentions software
// nested inside some other structure is not a profile and is not checked here.
func profileBlocks(name, content string) []snippet {
	var out []snippet

	lines := strings.Split(content, "\n")
	inFence := false
	fenceLang := ""
	fenceStart := 0
	var body []string

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inFence {
			if strings.HasPrefix(trimmed, "```") {
				inFence = true
				fenceLang = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, "```")))
				fenceStart = i + 1
				body = nil
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") {
			inFence = false
			if (fenceLang == "yaml" || fenceLang == "yml") && isProfileShaped(body) {
				out = append(out, snippet{
					where: name + ":" + itoa(fenceStart),
					body:  strings.Join(body, "\n") + "\n",
				})
			}
			continue
		}
		body = append(body, line)
	}
	return out
}

// isProfileShaped reports whether a block has a top-level software: or defaults:
// key — the profile fields whose entries are software refs.
func isProfileShaped(body []string) bool {
	for _, line := range body {
		if line == "software:" || line == "defaults:" ||
			strings.HasPrefix(line, "software: ") || strings.HasPrefix(line, "defaults: ") {
			return true
		}
	}
	return false
}

// itoa avoids pulling strconv in for one call site in a test helper.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

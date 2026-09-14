package propdoc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoFetcher resolves a citation's file against the working tree (empty sha) or
// against a commit via `git show`, and — for a pinned citation — requires the
// commit to be an ancestor of HEAD so the reference cannot point at code that was
// never merged (#105). Tests run from internal/propdoc, so the repo root is two
// levels up; git resolves its own paths from the repo root regardless of cwd.
func repoFetcher(t *testing.T) LineFetcher {
	t.Helper()
	return func(path, sha string) ([]string, bool) {
		var data []byte
		var err error
		if sha == "" {
			data, err = os.ReadFile(filepath.Join("..", "..", path))
		} else {
			if exec.Command("git", "merge-base", "--is-ancestor", sha, "HEAD").Run() != nil {
				return nil, false // a pin that is not an ancestor of HEAD is not usable
			}
			data, err = exec.Command("git", "show", sha+":"+path).Output()
		}
		if err != nil {
			return nil, false
		}
		return strings.Split(string(data), "\n"), true
	}
}

// TestPropertiesCitationsResolve is the CI enforcement: every machine-resolvable
// `path:line` citation in PROPERTIES.md must resolve — the path exists, the cited
// lines are within the file, and a named symbol appears within the cited span —
// against the working tree, or against a commit the citation's line pins to.
// Editing any such citation to something false makes this fail.
func TestPropertiesCitationsResolve(t *testing.T) {
	src, err := os.ReadFile(propertiesPath())
	if err != nil {
		t.Fatalf("read PROPERTIES.md: %v", err)
	}
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse PROPERTIES.md: %v", err)
	}
	stats := doc.CheckCitations(repoFetcher(t))
	for _, d := range stats.Defects {
		t.Errorf("PROPERTIES.md:%d: unresolvable citation %s — %s", d.LineIdx+1, d.Raw, d.Reason)
	}
	if stats.Checked == 0 {
		t.Fatal("no resolvable citations checked; the parser or fixture is broken")
	}
	// The count of citations that cannot be resolved without a search (basename
	// only) is the notation gap #105 records; logged, not enforced, until the
	// normalisation pass. Bare `:NNN` (no path) are not matched at all.
	t.Logf("citations: %d checked, %d defects, %d not machine-resolvable (basename-only)",
		stats.Checked, len(stats.Defects), stats.NotResolvable)
}

// TestCitationCheckCanFail is the control: a checker that resolves nothing
// reports no defects and looks exactly like a clean document, so it must be shown
// to go red. A fixture with one honest citation and three broken ones — a missing
// path, a symbol that has moved off its line, and a line past EOF — must yield
// exactly the three defects, and the honest one must not. (Two offenders differing
// in kind, per the non-vacuity rule.)
func TestCitationCheckCanFail(t *testing.T) {
	const fixture = "Prose. " +
		"Honest: `pkg/thing.go:2 Foo`. " +
		"Missing: `pkg/gone.go:1`. " +
		"Moved: `pkg/thing.go:1 Foo`. " +
		"PastEOF: `pkg/thing.go:99`.\n"
	doc := &Doc{lines: strings.Split(fixture, "\n")}
	fetch := func(path, sha string) ([]string, bool) {
		if path == "pkg/thing.go" {
			return []string{"package thing", "func Foo() {}"}, true // Foo is on line 2
		}
		return nil, false
	}
	stats := doc.CheckCitations(fetch)
	got := map[string]string{}
	for _, d := range stats.Defects {
		got[d.Path] = d.Reason
	}
	if len(stats.Defects) != 3 {
		t.Fatalf("expected 3 defects (missing, moved, past-eof), got %d: %v", len(stats.Defects), stats.Defects)
	}
	// The honest citation resolved (Foo is on line 2), so it is not among the
	// defects — otherwise the check is a blanket failure, not a citation checker.
	honest := 0
	for _, d := range stats.Defects {
		if d.Raw == "`pkg/thing.go:2 Foo`" {
			honest++
		}
	}
	if honest != 0 {
		t.Error("the honest citation `pkg/thing.go:2 Foo` was reported as a defect; the check fails indiscriminately")
	}
}

// TestCitationPinnedToPastCommit covers the tense constraint: a citation whose
// working-tree resolution fails resolves against a commit token on its line, so
// a discharged refutation's record survives the code moving.
func TestCitationPinnedToPastCommit(t *testing.T) {
	// The doc line carries the pin `deadbee` alongside a citation whose symbol is
	// no longer at the cited line in the working tree.
	const fixture = "It was `pkg/thing.go:1 Old` at `deadbee`, now moved.\n"
	doc := &Doc{lines: strings.Split(fixture, "\n")}
	fetch := func(path, sha string) ([]string, bool) {
		switch {
		case path == "pkg/thing.go" && sha == "":
			return []string{"func New() {}"}, true // Old is gone in the working tree
		case path == "pkg/thing.go" && sha == "deadbee":
			return []string{"func Old() {}"}, true // Old was on line 1 at deadbee
		}
		return nil, false
	}
	if stats := doc.CheckCitations(fetch); len(stats.Defects) != 0 {
		t.Errorf("pinned citation reported as a defect: %v", stats.Defects)
	}
}

// TestCitationSpanParsing pins the range/list distinction: a symbol anywhere in a
// range resolves, and a discrete list checks each listed line.
func TestCitationSpanParsing(t *testing.T) {
	cases := map[string][][2]int{
		"82-98":       {{82, 98}},
		"264,350,519": {{264, 264}, {350, 350}, {519, 519}},
		"1-3,7":       {{1, 3}, {7, 7}},
		"42":          {{42, 42}},
	}
	for in, want := range cases {
		got := parseSpans(in)
		if len(got) != len(want) {
			t.Errorf("parseSpans(%q) = %v, want %v", in, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("parseSpans(%q)[%d] = %v, want %v", in, i, got[i], want[i])
			}
		}
	}
}

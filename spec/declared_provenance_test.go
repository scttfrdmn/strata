package spec

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// This file enforces the "declared provenance" route out of envHashInput
// (spec/lockfile_hash.go). LockFile.RekorEntry is exported into the assembled
// environment and is deliberately not hashed, on the argument that it is a
// statement *about* the environment rather than part of it. That argument holds
// only while nothing in-tree reads the exported value back: a program that
// branched on $STRATA_REKOR_ENTRY would make the field content, and the identity
// would be under-determined for exactly as long as nobody noticed.
//
// The obligation was originally written as a comment. A comment asserting an
// absence is a check waiting to be written, so it is written here.
//
// # What is checked, and which check is the fence
//
// Two detectors, in decreasing order of strength:
//
//  1. Enumeration. Every mention of an exported name in a scanned file must
//     appear in allowedProvenanceMentions, with the expected count, and every
//     entry in that list must be observed. A new mention anywhere fails,
//     whatever it does — including forms detector 2 cannot spell, such as
//     indexing a map built from os.Environ(), a read split across lines, or a
//     local wrapper named getenv. This is the fence.
//  2. Read-function proximity. A mention on a line that also names an
//     environment read (os.Getenv, os.LookupEnv, os.Environ, syscall.Getenv,
//     syscall.Environ) fails with a specific message, and a shell expansion
//     ($NAME or ${NAME}) fails likewise. This adds nothing detector 1 misses
//     inside scanned files; it exists so the failure names the defect instead of
//     reporting an unexplained count.
//
// # Scope, stated rather than left to the reader
//
// Scanned: non-test .go files, and .sh/.bash files. Out of scope, by name:
//
//   - _test.go files — internal/overlay/overlay_test.go:76 asserts the export is
//     present, which is a test of the writer, not a program reading the value.
//     This file is itself a test and names the variables throughout.
//   - .md files — docs/userspace.md:103 publishes the variable as part of the
//     userspace contract and PROPERTIES.md cites it as evidence. Prose cannot
//     read an environment variable.
//   - .yml/.yaml — CI workflows do not assemble a Strata environment, so a
//     mention there could not make the field content.
//   - the YAML key rekor_entry (spec/lockfile.go:27) — that is the lockfile's
//     own serialisation, not a name invented for export. Reading it back means
//     reading the lockfile, which is the scope question #121 tracks, not this
//     one.
//
// The names checked are the two the value is exported under, which is *not* the
// same as the set of sites that export RekorEntry: internal/fold/eject.go
// exports STRATA_PROFILE and does not export the Rekor entry at all.

// declaredProvenanceNames are the names LockFile.RekorEntry's value is published
// under. Both are strings invented for export, so any in-tree occurrence of one
// is either a write site or a consumer.
func declaredProvenanceNames() []string {
	return []string{
		// Written to /etc/profile.d/strata.sh, /etc/strata/environment, and the
		// child process environment.
		"STRATA_REKOR_ENTRY",
		// The OCI image label written by internal/export/oci.go.
		"strata.lockfile.rekor_entry",
	}
}

// allowedProvenanceMentions maps each exported name to the scanned files that
// may mention it, and how many lines in each may do so. The counts are
// deliberately exact in both directions: an extra mention fails as a possible
// reader, and a missing one fails as either a moved export site or a broken
// scanner. A one-directional list would report "no readers found" when the walk
// reached nothing at all.
func allowedProvenanceMentions() map[string]map[string]int {
	return map[string]map[string]int{
		"STRATA_REKOR_ENTRY": {
			// :143 export line in /etc/profile.d/strata.sh, :198 line in
			// /etc/strata/environment. Both writes.
			"internal/overlay/overlay.go": 2,
			// :372 sets it in the child process environment. A write.
			"cmd/strata/run.go": 1,
			// The route's documentation, which names this test.
			"spec/lockfile_hash.go": 1,
		},
		"strata.lockfile.rekor_entry": {
			// :407 sets the OCI label. A write.
			"internal/export/oci.go": 1,
		},
	}
}

// envReadPattern matches the ways in-tree Go reads the process environment.
// Built per call because this package declares no globals.
func envReadPattern() *regexp.Regexp {
	return regexp.MustCompile(`\b(os\.Getenv|os\.LookupEnv|os\.Environ|syscall\.Getenv|syscall\.Environ)\b`)
}

// provenanceMention is one line of one file that names an exported name.
type provenanceMention struct {
	Path string
	Line int
	Text string
	// Read records that the same line also names an environment read.
	Read bool
}

// scanMentions returns one entry per line of content that contains name.
// Counting by line, not by occurrence, is what allowedProvenanceMentions
// records; two occurrences on one line would count as one and fail the exact
// count, which is the safe direction.
func scanMentions(path, content, name string, readPattern *regexp.Regexp) []provenanceMention {
	var out []provenanceMention
	for i, line := range strings.Split(content, "\n") {
		if !strings.Contains(line, name) {
			continue
		}
		out = append(out, provenanceMention{
			Path: path,
			Line: i + 1,
			Text: strings.TrimSpace(line),
			Read: readPattern.MatchString(line),
		})
	}
	return out
}

// scanShellExpansions returns the lines of content that expand name as a shell
// variable, in either the bare or braced form.
func scanShellExpansions(path, content, name string) []provenanceMention {
	pattern := regexp.MustCompile(`\$\{?` + regexp.QuoteMeta(name) + `\b`)
	var out []provenanceMention
	for i, line := range strings.Split(content, "\n") {
		if !pattern.MatchString(line) {
			continue
		}
		out = append(out, provenanceMention{Path: path, Line: i + 1, Text: strings.TrimSpace(line), Read: true})
	}
	return out
}

// moduleRoot walks up from the test's working directory to the directory
// holding go.mod, and confirms it is this module. Without the confirmation a
// stray go.mod above the checkout would silently redirect the whole scan.
func moduleRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		modPath := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(modPath); err == nil {
			if !strings.Contains(string(data), "module github.com/scttfrdmn/strata\n") {
				t.Fatalf("%s is not this module's go.mod; the scan would cover the wrong tree", modPath)
			}
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod found above the test working directory")
		}
		dir = parent
	}
}

// collectFiles reads every file under root whose repo-relative path satisfies
// want, keyed by that relative path.
func collectFiles(t *testing.T, root string, want func(rel string) bool) map[string]string {
	t.Helper()

	out := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules", "bin":
				return filepath.SkipDir
			}
			return nil
		}
		if !want(filepath.ToSlash(rel)) {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return out
}

func isGoNonTest(rel string) bool {
	return strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go")
}

func isShellScript(rel string) bool {
	return strings.HasSuffix(rel, ".sh") || strings.HasSuffix(rel, ".bash")
}

func TestDeclaredProvenance_NoInTreeReader(t *testing.T) {
	root := moduleRoot(t)
	readPattern := envReadPattern()

	goFiles := collectFiles(t, root, isGoNonTest)
	shellFiles := collectFiles(t, root, isShellScript)

	// Non-vacuity, before any result is read. A scan that reached no files
	// reports the same clean zero as a scan that found no readers.
	if len(goFiles) == 0 {
		t.Fatalf("scanned 0 non-test .go files under %s; the walk found nothing, so a "+
			"zero-readers result below would mean nothing", root)
	}
	if len(shellFiles) == 0 {
		t.Fatalf("scanned 0 shell scripts under %s; the shell detector below would be "+
			"reading an empty domain", root)
	}
	t.Logf("scanned %d non-test .go files and %d shell scripts under %s",
		len(goFiles), len(shellFiles), root)

	allowed := allowedProvenanceMentions()

	for _, name := range declaredProvenanceNames() {
		observed := make(map[string]int)

		for _, files := range []map[string]string{goFiles, shellFiles} {
			for rel, content := range files {
				for _, m := range scanMentions(rel, content, name, readPattern) {
					observed[rel]++
					if m.Read {
						t.Errorf("%s:%d reads the exported provenance name %q:\n"+
							"    %s\n"+
							"  The declared-provenance route in spec/lockfile_hash.go rests on "+
							"nothing in-tree reading this value back. With a reader, the field is "+
							"content and must be hashed into envHashInput (#120).",
							m.Path, m.Line, name, m.Text)
					}
				}
			}
		}

		for rel, content := range shellFiles {
			for _, m := range scanShellExpansions(rel, content, name) {
				t.Errorf("%s:%d expands $%s in a shell script:\n"+
					"    %s\n"+
					"  Same consequence as a Go read: the field becomes content (#120).",
					m.Path, m.Line, name, m.Text)
			}
		}

		want := allowed[name]
		for rel, count := range observed {
			switch wantCount, ok := want[rel]; {
			case !ok:
				t.Errorf("%s mentions %q on %d line(s) and is not in "+
					"allowedProvenanceMentions.\n"+
					"  Every mention of an exported provenance name is either a write site or a "+
					"consumer. If it is a write, add it to the list with its purpose. If it "+
					"reads the value, the declared-provenance route no longer applies and the "+
					"field must move into envHashInput (#120).", rel, name, count)
			case count != wantCount:
				t.Errorf("%s mentions %q on %d line(s), expected %d.\n"+
					"  A new line naming this value is a possible consumer. Classify it, then "+
					"update the count.", rel, name, count, wantCount)
			}
		}
		for rel, wantCount := range want {
			if observed[rel] == 0 {
				t.Errorf("expected %d mention(s) of %q in %s and found none.\n"+
					"  Either the export site moved — update allowedProvenanceMentions — or "+
					"this scan is broken, in which case every zero it reported above is "+
					"meaningless.", wantCount, name, rel)
			}
		}
	}
}

// TestDeclaredProvenanceDetectorsFire is the positive control for both
// detectors. Without it, TestDeclaredProvenance_NoInTreeReader passing is
// consistent with a scanner that cannot recognise a reader at all: a repository
// with no readers and a broken matcher produce identical output.
//
// Each detector class gets its own case, and the last two cases carry two
// offenders differing in kind, because a check that returns a collection can
// pass a single-offender fixture by stopping at the first one.
func TestDeclaredProvenanceDetectorsFire(t *testing.T) {
	readPattern := envReadPattern()
	const name = "STRATA_REKOR_ENTRY"

	goCases := []struct {
		label     string
		content   string
		wantLines int
		wantReads int
	}{
		{
			label:     "os.Getenv",
			content:   "package p\n\nfunc f() string { return os.Getenv(\"STRATA_REKOR_ENTRY\") }\n",
			wantLines: 1,
			wantReads: 1,
		},
		{
			label:     "os.LookupEnv",
			content:   "package p\n\nfunc f() (string, bool) { return os.LookupEnv(\"STRATA_REKOR_ENTRY\") }\n",
			wantLines: 1,
			wantReads: 1,
		},
		{
			label:     "syscall.Getenv",
			content:   "package p\n\nfunc f() (string, bool) { return syscall.Getenv(\"STRATA_REKOR_ENTRY\") }\n",
			wantLines: 1,
			wantReads: 1,
		},
		{
			label: "map index built from os.Environ — enumeration only",
			// The mention line names no read function, so detector 2 stays
			// silent and detector 1 is the only thing standing between this and
			// a silent pass. That is the case this fixture exists to pin.
			content:   "package p\n\nfunc f(env map[string]string) string {\n\treturn env[\"STRATA_REKOR_ENTRY\"]\n}\n",
			wantLines: 1,
			wantReads: 0,
		},
		{
			label: "two offenders differing in kind",
			content: "package p\n\nfunc f() string {\n" +
				"\tif v := os.Getenv(\"STRATA_REKOR_ENTRY\"); v != \"\" {\n" +
				"\t\treturn v\n\t}\n" +
				"\treturn lookup[\"STRATA_REKOR_ENTRY\"]\n}\n",
			wantLines: 2,
			wantReads: 1,
		},
	}

	for _, tc := range goCases {
		t.Run("go/"+tc.label, func(t *testing.T) {
			got := scanMentions("fixture.go", tc.content, name, readPattern)
			if len(got) != tc.wantLines {
				t.Fatalf("scanMentions found %d mention(s), want %d: %+v", len(got), tc.wantLines, got)
			}
			reads := 0
			for _, m := range got {
				if m.Read {
					reads++
				}
			}
			if reads != tc.wantReads {
				t.Errorf("scanMentions flagged %d read(s), want %d: %+v", reads, tc.wantReads, got)
			}
		})
	}

	shellCases := []struct {
		label     string
		content   string
		wantLines int
	}{
		{
			label:     "braced expansion",
			content:   "#!/bin/sh\necho \"${STRATA_REKOR_ENTRY}\"\n",
			wantLines: 1,
		},
		{
			label:     "bare expansion",
			content:   "#!/bin/sh\ntest -n \"$STRATA_REKOR_ENTRY\" && exit 1\n",
			wantLines: 1,
		},
		{
			label:     "two offenders differing in kind",
			content:   "#!/bin/sh\necho \"${STRATA_REKOR_ENTRY}\"\nif [ -n \"$STRATA_REKOR_ENTRY\" ]; then :; fi\n",
			wantLines: 2,
		},
		{
			label: "a write is not an expansion",
			// The write form must not be reported, or every export site would
			// fail the shell detector and the check would be 100% false
			// positives on the population it runs against.
			content:   "#!/bin/sh\nexport STRATA_REKOR_ENTRY=abc\n",
			wantLines: 0,
		},
	}

	for _, tc := range shellCases {
		t.Run("shell/"+tc.label, func(t *testing.T) {
			got := scanShellExpansions("fixture.sh", tc.content, name)
			if len(got) != tc.wantLines {
				t.Fatalf("scanShellExpansions found %d expansion(s), want %d: %+v",
					len(got), tc.wantLines, got)
			}
		})
	}
}

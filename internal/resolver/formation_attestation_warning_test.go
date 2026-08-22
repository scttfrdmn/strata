package resolver_test

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/resolver"
	"github.com/scttfrdmn/strata/internal/testregistry"
	"github.com/scttfrdmn/strata/spec"
)

// These tests re-derive the counterexample tracked by #49 — "the resolver
// expanded unattested formations without warning" — against today's code,
// because §2.1 rule 11 says a register row may be marked Discharged: Yes only on
// re-derived evidence at coverage `chosen` or stronger with subject
// `implementation`, and the row it belongs to said "Yes — closed completed"
// (#137). A closed issue is a fact about the tracker.
//
// What they establish, and the bound: stage 2 emits the warning for a formation
// whose rekor_entry carries the placeholder, and it emits it through
// resolver.Config.Warnings. The values are chosen, not enumerated — one
// formation, one placeholder spelling — so this is `chosen/implementation` and
// nothing here claims a bound on other placeholder spellings.
//
// What they deliberately do NOT establish is that every caller receives it.
// resolver.warn writes to cfg.Warnings and returns silently when it is nil
// (internal/resolver/resolver.go:65-70), and pkg/strata builds a resolver.Config
// without that field (pkg/strata/strata.go:103), so the public library route is
// silent on the same input. That half is why the row is Partially rather than
// Yes, and TestPlaceholderWarningIsSilentWhenNoWriterIsSet below asserts the
// mechanism so that closing the gap fails a test rather than passing quietly.

// placeholderRekorEntry is the value stage 2 warns about
// (internal/resolver/stages.go:71). It is repeated here rather than exported
// from the resolver: a test that reads the constant it is testing agrees with
// the implementation by construction, and would keep passing if the
// implementation started warning about a different string.
const placeholderRekorEntry = "pending-initial-build"

// warningFixture materialises the offline fixture registry and rewrites its one
// formation manifest to carry the placeholder rekor_entry. It returns a resolver
// whose warnings land in the returned buffer.
//
// The rewrite is what makes the premise satisfiable: every fixture manifest
// carries testregistry.SentinelRekorEntry, so an unmodified fixture resolves
// through stage 2 without reaching the warning at all — the test would pass on
// an implementation that never warns.
func warningFixture(t *testing.T, warnings *bytes.Buffer) *resolver.Resolver {
	t.Helper()
	root, client := testregistry.New(t)

	rewritten := 0
	err := filepath.WalkDir(filepath.Join(root, "formations"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "manifest.yaml" {
			return nil
		}
		data, readErr := os.ReadFile(p) //nolint:gosec // p comes from WalkDir over t.TempDir()
		if readErr != nil {
			return readErr
		}
		var f spec.Formation
		if unmarshalErr := yaml.Unmarshal(data, &f); unmarshalErr != nil {
			return unmarshalErr
		}
		f.RekorEntry = placeholderRekorEntry
		out, marshalErr := yaml.Marshal(&f)
		if marshalErr != nil {
			return marshalErr
		}
		if writeErr := os.WriteFile(p, out, 0o600); writeErr != nil {
			return writeErr
		}
		rewritten++
		return nil
	})
	if err != nil {
		t.Fatalf("rewriting fixture formation manifests: %v", err)
	}
	// The premise assertion: a rewrite that matched nothing leaves a fixture
	// that cannot reach the warning, and the resulting silence would read as a
	// finding about the implementation.
	if rewritten != 1 {
		t.Fatalf("rewrote %d formation manifests, want exactly 1 — the fixture registry's shape changed", rewritten)
	}

	probeClient, err := testregistry.Probe()
	if err != nil {
		t.Fatalf("fixture probe client: %v", err)
	}
	cfg := resolver.Config{
		Registry:      client,
		Probe:         probeClient,
		StrataVersion: "0.0.0-test",
	}
	if warnings != nil {
		cfg.Warnings = warnings
	}
	r, err := resolver.New(cfg)
	if err != nil {
		t.Fatalf("resolver.New: %v", err)
	}
	return r
}

func warningFixtureProfile(t *testing.T) *spec.Profile {
	t.Helper()
	data, err := testregistry.ProfileBytes(testregistry.ProfileFormation)
	if err != nil {
		t.Fatalf("%v", err)
	}
	p, err := spec.ParseProfileBytes(data)
	if err != nil {
		t.Fatalf("parsing %s: %v", testregistry.ProfileFormation, err)
	}
	return p
}

// TestStage2AnnouncesAPlaceholderAttestation is the discharge evidence for the
// #49 register row: expanding a formation whose attestation is a placeholder
// announces it at use time, which is the repaired form of T7 (a weaker check
// announces itself rather than being documented).
func TestStage2AnnouncesAPlaceholderAttestation(t *testing.T) {
	var warnings bytes.Buffer
	r := warningFixture(t, &warnings)

	if _, err := r.Resolve(context.Background(), warningFixtureProfile(t)); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	got := warnings.String()
	// Three separate assertions, because a warning that fires without naming
	// the formation or the reason displaces the operator's attention exactly as
	// silence does.
	for _, want := range []string{
		testregistry.FormationRef,
		"no Rekor attestation",
		placeholderRekorEntry,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("stage 2 warning does not mention %q; warnings were: %q", want, got)
		}
	}
}

// TestStage2IsSilentOnAnAttestedFormation is the control that makes the test
// above non-vacuous: without it, an implementation that warns on every
// formation would pass. The fixture's own SentinelRekorEntry is not the
// placeholder, so this is the unmodified path.
func TestStage2IsSilentOnAnAttestedFormation(t *testing.T) {
	_, client := testregistry.New(t)
	probeClient, err := testregistry.Probe()
	if err != nil {
		t.Fatalf("fixture probe client: %v", err)
	}
	var warnings bytes.Buffer
	r, err := resolver.New(resolver.Config{
		Registry:      client,
		Probe:         probeClient,
		StrataVersion: "0.0.0-test",
		Warnings:      &warnings,
	})
	if err != nil {
		t.Fatalf("resolver.New: %v", err)
	}
	if _, err := r.Resolve(context.Background(), warningFixtureProfile(t)); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if strings.Contains(warnings.String(), "no Rekor attestation") {
		t.Errorf("stage 2 warned about a formation carrying %q; warnings were: %q",
			testregistry.SentinelRekorEntry, warnings.String())
	}
}

// TestPlaceholderWarningIsSilentWhenNoWriterIsSet asserts the live half of the
// #49 row rather than the discharged half: the warning reaches nobody when
// Config.Warnings is unset, which is the state pkg/strata resolves in
// (pkg/strata/strata.go:103, and pkg/strata.Options exposes no way to set it).
//
// It is an exclusion control in the sense §4 requires: it asserts today's
// behaviour, so wiring the warning through — or defaulting it to os.Stderr —
// fails here and forces the register row to be revisited on purpose, instead of
// a scope limit that was honest when written quietly becoming a lie.
func TestPlaceholderWarningIsSilentWhenNoWriterIsSet(t *testing.T) {
	r := warningFixture(t, nil)

	var err error
	stderr := captureStderrDuring(t, func() {
		_, err = r.Resolve(context.Background(), warningFixtureProfile(t))
	})

	// Resolution must still succeed: the gap is that the operator is not told,
	// not that resolution breaks.
	if err != nil {
		t.Fatalf("Resolve with no Warnings writer: %v", err)
	}
	// And the assertion that gives this test its content. Asserting only that
	// Resolve returned nil would pass on an implementation that defaulted the
	// warning to os.Stderr — i.e. on the fix — so it would certify the gap
	// without being able to observe it.
	if strings.Contains(stderr, "no Rekor attestation") {
		t.Errorf("the placeholder warning reached os.Stderr with Config.Warnings unset, "+
			"so pkg/strata's route is no longer silent and the #49 register row must be "+
			"revisited; stderr was: %q", stderr)
	}
}

// captureStderrDuring swaps os.Stderr for a pipe while fn runs. cmd/strata has
// the same helper; it is not importable from a test in another package, and
// duplicating twenty lines is cheaper than exporting a test seam from a main
// package.
func captureStderrDuring(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stderr
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, readErr := r.Read(buf)
			sb.Write(buf[:n])
			if readErr != nil {
				break
			}
		}
		done <- sb.String()
	}()
	fn()
	os.Stderr = saved
	w.Close() //nolint:errcheck
	out := <-done
	r.Close() //nolint:errcheck
	return out
}

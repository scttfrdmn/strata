package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/trust"
)

// Tests for the agent's verifier construction (#56).
//
// The defect was that "cosign is not installed" and "the operator accepted the
// risk" were the same value — a nil trust.Verifier — so an EC2 instance
// downgraded from authenticity checking to hash checking without anyone asking
// it to. Before this change `newCosignVerifier` had 0.0% coverage: it was called
// only from main(), so nothing in CI executed the most security-relevant branch
// in the binary.
//
// The property under test is therefore not "a verifier is built". It is that a
// nil verifier is returned if and only if the operator explicitly opted out.

func cosignPresent(string) (string, error) { return "/usr/local/bin/cosign", nil }

// cosignAbsent reproduces exec.LookPath's failure shape.
func cosignAbsent(string) (string, error) {
	return "", errors.New(`exec: "cosign": executable file not found in $PATH`)
}

func keyFetched(context.Context) string     { return "/tmp/strata-cosign-test.pub" }
func keyUnavailable(context.Context) string { return "" }

// TestNewCosignVerifier_NamesTheMissingPrerequisite covers the two refusal
// directions and the one success direction.
//
// Both refusals must be reachable without cosign installed and without AWS
// credentials, which is the state of CI — that reachability is most of what this
// issue was about.
func TestNewCosignVerifier_NamesTheMissingPrerequisite(t *testing.T) {
	tests := []struct {
		name       string
		prereqs    verifierPrereqs
		wantReason string
	}{
		{
			name:       "cosign missing",
			prereqs:    verifierPrereqs{lookPath: cosignAbsent, fetchKey: keyFetched},
			wantReason: "cosign not found on PATH",
		},
		{
			// Reported as the key, not as cosign: both prerequisites are
			// consulted in order and this row proves the second one is
			// reached rather than masked by the first.
			name:       "cosign present, key unavailable",
			prereqs:    verifierPrereqs{lookPath: cosignPresent, fetchKey: keyUnavailable},
			wantReason: "could not fetch the cosign public key",
		},
		{
			name:    "both prerequisites present",
			prereqs: verifierPrereqs{lookPath: cosignPresent, fetchKey: keyFetched},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := newCosignVerifier(context.Background(), tt.prereqs)

			if tt.wantReason == "" {
				if err != nil {
					t.Fatalf("want a verifier, got error: %v", err)
				}
				if v == nil {
					t.Fatal("returned (nil, nil): a nil Verifier is a panic in trust.VerifyLayer, not a skip")
				}
				cv, ok := v.(*trust.CosignVerifier)
				if !ok {
					t.Fatalf("want *trust.CosignVerifier, got %T", v)
				}
				if cv.KeyRef != keyFetched(context.Background()) {
					t.Errorf("verifier does not use the fetched key: KeyRef = %q", cv.KeyRef)
				}
				return
			}

			if err == nil {
				t.Fatalf("want a refusal, got verifier %+v", v)
			}
			if v != nil {
				t.Errorf("returned a verifier alongside an error: %+v", v)
			}
			if !strings.Contains(err.Error(), tt.wantReason) {
				t.Errorf("refusal names the wrong prerequisite:\n got: %v\nwant substring: %s",
					err, tt.wantReason)
			}
		})
	}
}

// TestNewCosignVerifier_ReportsCosignBeforeTheKey: when both prerequisites are
// missing, the operator must be sent to the binary and not to the key, because
// without the binary the key is useless.
//
// This is the row that the two-refusal table above cannot express — it needs
// both defects present at once, and it asserts which one wins.
func TestNewCosignVerifier_ReportsCosignBeforeTheKey(t *testing.T) {
	_, err := newCosignVerifier(context.Background(),
		verifierPrereqs{lookPath: cosignAbsent, fetchKey: keyUnavailable})
	if err == nil {
		t.Fatal("want a refusal")
	}
	if !strings.Contains(err.Error(), "cosign not found on PATH") {
		t.Errorf("want the cosign prerequisite reported first, got: %v", err)
	}
	if strings.Contains(err.Error(), "public key") {
		t.Errorf("key fetch was attempted or reported despite cosign being absent: %v", err)
	}
}

// TestResolveVerifier_NilVerifierOnlyWithAnExplicitOptOut is the invariant.
//
// A nil verifier is not an error value here — it is the opt-out result — so the
// assertion cannot be "never nil". It is that the pairing (nil verifier, nil
// error) occurs only when allowUnverified is set, and that every other
// prerequisite failure is an error the boot stops on.
func TestResolveVerifier_NilVerifierOnlyWithAnExplicitOptOut(t *testing.T) {
	failing := []struct {
		name    string
		prereqs verifierPrereqs
	}{
		{"cosign missing", verifierPrereqs{lookPath: cosignAbsent, fetchKey: keyFetched}},
		{"key unavailable", verifierPrereqs{lookPath: cosignPresent, fetchKey: keyUnavailable}},
	}

	for _, f := range failing {
		t.Run(f.name+", opt-out not set: refuses", func(t *testing.T) {
			v, err := resolveVerifier(context.Background(), f.prereqs)
			if err == nil {
				t.Fatalf("boot was allowed to continue without verification (verifier %+v)", v)
			}
			if v != nil {
				t.Errorf("returned a verifier alongside an error: %+v", v)
			}
			if !strings.Contains(err.Error(), allowUnverifiedEnv) {
				t.Errorf("refusal does not name the opt-out an operator would need: %v", err)
			}
		})

		t.Run(f.name+", opt-out set: proceeds without a verifier", func(t *testing.T) {
			p := f.prereqs
			p.allowUnverified = true
			v, err := resolveVerifier(context.Background(), p)
			if err != nil {
				t.Fatalf("explicit opt-out must not fail the boot: %v", err)
			}
			if v != nil {
				t.Errorf("opt-out returned a verifier: %+v", v)
			}
		})
	}

	t.Run("prerequisites present: a verifier, regardless of the opt-out", func(t *testing.T) {
		// The opt-out is permission to boot unverified, not an instruction to
		// stop verifying. An operator who sets it on a fleet where cosign is
		// present should still get verification.
		for _, allow := range []bool{false, true} {
			v, err := resolveVerifier(context.Background(), verifierPrereqs{
				lookPath: cosignPresent, fetchKey: keyFetched, allowUnverified: allow,
			})
			if err != nil {
				t.Fatalf("allowUnverified=%v: unexpected error: %v", allow, err)
			}
			if v == nil {
				t.Errorf("allowUnverified=%v: verification was skipped although both prerequisites were present", allow)
			}
		}
	})
}

// TestAllowUnverified_DefaultsToClosed drives the opt-out parser through the
// values an operator can actually produce, including the ones that look
// affirmative and are not.
//
// The unset case is first because it is the one production depends on and the
// one a table of hand-set booleans would never reach.
func TestAllowUnverified_DefaultsToClosed(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"", false}, // unset, and the production default
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"  yes  ", true}, // whitespace is trimmed

		{"on", true},
		{"0", false},
		{"false", false},
		{"no", false},
		// Anything unrecognised leaves verification on. A typo in the value is
		// not consent, and this is the direction of error a security default
		// should have.
		{"maybe", false},
		{"ture", false},
		{"2", false},
		{"disabled", false},
	}
	for _, tt := range tests {
		got := allowUnverified(func(string) string { return tt.value })
		if got != tt.want {
			t.Errorf("allowUnverified(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

// TestAllowUnverified_ReadsTheRightVariable: a parser that ignored its key would
// pass every row above, because they all return the same value for any key.
func TestAllowUnverified_ReadsTheRightVariable(t *testing.T) {
	getenv := func(k string) string {
		if k == allowUnverifiedEnv {
			return "1"
		}
		return ""
	}
	if !allowUnverified(getenv) {
		t.Errorf("allowUnverified does not read %s", allowUnverifiedEnv)
	}

	other := func(k string) string {
		if k == "STRATA_AGENT_ALLOW_UNVERIFIED_LAYERS" {
			return "1"
		}
		return ""
	}
	if allowUnverified(other) {
		t.Error("a similarly-named variable enabled the opt-out")
	}
}

// TestProductionPrereqs_ClosedByDefault reaches the default through the wiring
// production uses, rather than by setting the field.
//
// This test exists because of a predicted failure recorded on #56 before the
// code was written: that the closed default would be correct by construction and
// asserted by nothing, since every other test here sets allowUnverified
// explicitly. Reading the literal `false` back with one's eyes is not a check.
func TestProductionPrereqs_ClosedByDefault(t *testing.T) {
	// A getenv that reports every variable as unset — the state of a fresh
	// instance whose user-data says nothing about verification.
	unset := func(string) string { return "" }

	p := productionPrereqs(unset)
	if p.allowUnverified {
		t.Error("the production default opts out of verification")
	}
	if p.lookPath == nil || p.fetchKey == nil {
		t.Fatal("production prereqs are incompletely wired")
	}

	// And the consequence, through resolveVerifier rather than by inspecting
	// the field: with cosign unavailable and nothing set, the boot stops.
	p.lookPath = cosignAbsent
	if _, err := resolveVerifier(context.Background(), p); err == nil {
		t.Error("with nothing set and cosign absent, the boot was allowed to continue")
	}
}

// TestProductionPrereqs_RefusesWithTheRealLookPath closes the gap the injected
// lookPath leaves: that the production wiring actually consults the real one.
//
// Every other test here substitutes cosignAbsent, which proves the decision
// logic and nothing about what production calls. Stripping PATH makes the real
// exec.LookPath fail — deterministically, whether or not cosign is installed on
// the machine running this — and no S3 call happens, because the key fetch is
// never reached.
func TestProductionPrereqs_RefusesWithTheRealLookPath(t *testing.T) {
	t.Setenv("PATH", "/nonexistent-strata-agent-test")

	v, err := resolveVerifier(context.Background(), productionPrereqs(os.Getenv))
	if err == nil {
		t.Fatalf("cosign is unreachable and nothing was opted out, but the boot was allowed to continue (verifier %+v)", v)
	}
	if v != nil {
		t.Errorf("returned a verifier alongside an error: %+v", v)
	}
	if !strings.Contains(err.Error(), "cosign not found on PATH") {
		t.Errorf("refusal does not name the real prerequisite: %v", err)
	}
}

// TestProductionPrereqs_UsesTheRealEnvironment checks the wiring against
// os.Getenv, which is what main passes. os.Setenv is safe here because the
// variable is read once, synchronously, by the call under test.
func TestProductionPrereqs_UsesTheRealEnvironment(t *testing.T) {
	if productionPrereqs(os.Getenv).allowUnverified {
		t.Fatalf("%s is set in this test environment; the default cannot be observed", allowUnverifiedEnv)
	}

	t.Setenv(allowUnverifiedEnv, "1")
	if !productionPrereqs(os.Getenv).allowUnverified {
		t.Errorf("productionPrereqs does not read %s from the environment", allowUnverifiedEnv)
	}
}

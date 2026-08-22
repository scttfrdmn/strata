package agent_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/scttfrdmn/strata/internal/agent"
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// This file enumerates the boot sequence's verification decision surface instead
// of sampling it. The space is finite and small, so enumeration is available —
// and it is strictly stronger than sampling, because a sample cannot report which
// cells it missed. Issue #128 has the derivation; the short version:
//
//	Config.Verifier   nil | accepts-anything | refuses-anything | content-bound   (4)
//	bundle bytes      no fetcher | valid-for-this | valid-for-other | (nil,nil)
//	                  | non-JSON | wrong media type | fetch error                 (7)
//	layer.Bundle      "" | "s3://..."                                             (2)
//	content vs SHA256 match | mismatch                                            (2)
//
// Two corrections came out of deriving that list rather than estimating it.
//
// `cached` is deliberately absent. LayerFetcher.Fetch returns a path; a cache hit
// and a fresh download are indistinguishable to Run, and verifyBundles iterates
// lf.Layers rather than fetch provenance, so a cache hit cannot skip verification
// by construction. That question belongs to cmd/strata-agent.s3LayerFetcher as one
// assertion, not as a doubling of this table.
//
// The bundle *bytes* are their own dimension, separate from the verifier's
// disposition. Splitting them makes the substitution case — a valid bundle for
// different content — an ordinary cell of the product rather than a special case
// somebody has to think of.
//
// The expected outcome per cell comes from the precedence ladder in wantFor(),
// authored from the contract (#92, #93, and the interface documentation) rather
// than from reading verifyBundles. The evidence that it is not a paraphrase of the
// implementation is that it *disagrees* with it: the ladder says a boot with no
// verifier is a refusal, and today it boots. Those cells carry knownOpen and
// assert today's behaviour, so they fail loudly when the hole closes instead of
// silently certifying it.

// bootOutcome is the reason the boot sequence gave, or reasonBoots.
//
// Each value carries the substring the error must contain, so a cell asserts
// *which* check fired rather than merely that one did. A test that only asserts
// "Run returned an error" passes when the fixture fails an earlier step for an
// unrelated reason, which is the defect #92's probe 3 was built to expose.
type bootOutcome string

const (
	reasonBoots        bootOutcome = ""
	reasonDigest       bootOutcome = "SHA256 mismatch"
	reasonNoVerifier   bootOutcome = "no way to verify" // intended by #93(a); not implemented
	reasonAbsentBundle bootOutcome = "no attestation bundle"
	reasonFetchBundle  bootOutcome = "fetching bundle"
	reasonEmptyBytes   bootOutcome = "no bundle bytes"
	reasonParse        bootOutcome = "parsing bundle"
	reasonVerify       bootOutcome = "verifying layer"
)

// ladderOutcomes is the manifest of arms wantFor() can return. The table asserts
// that every one of these fires for at least one cell: an expectation arm no cell
// reaches is an unsearched branch wearing a green table, the same defect class as
// a fuzz floor nothing can trip (#125). Asserting the *set* rather than the count
// is deliberate — a count matches while the members differ.
var ladderOutcomes = []bootOutcome{
	reasonBoots,
	reasonDigest,
	reasonNoVerifier,
	reasonAbsentBundle,
	reasonFetchBundle,
	reasonEmptyBytes,
	reasonParse,
	reasonVerify,
}

// knownOpenCells is the number of cells that reach the mount with no authenticity
// verification because Verifier or BundleFetcher is nil (#93(a)). It is asserted
// as a literal so the size of the hole is a number in the suite, and so the
// expectation table cannot shrink it silently: check (5) below compares it against
// wantFor()'s own output, which means it catches an edit to the *table*.
//
// It is not the tripwire against the code, and #148 measured that: fixing #93(a)
// leaves this number at 20 and check (5) green, because both sides are derived from
// wantFor(). What goes red is the knownOpen branch in runCell (:526-530), once per
// cell — Run refuses where the cell says it does not — and that failure carries the
// instruction to lower this constant. Twenty cells fail, not one.
const knownOpenCells = 20

// --- dimension 1: the verifier's disposition -------------------------------

type verifierKind int

const (
	verifierNil verifierKind = iota
	verifierAcceptsAll
	verifierRefusesAll
	verifierContentBound // trust.FakeVerifier: accepts iff the bundle signs this artifact
)

func (k verifierKind) String() string {
	switch k {
	case verifierNil:
		return "verifier=nil"
	case verifierAcceptsAll:
		return "verifier=accepts"
	case verifierRefusesAll:
		return "verifier=refuses"
	case verifierContentBound:
		return "verifier=content-bound"
	}
	return fmt.Sprintf("verifier=?%d", int(k))
}

// errRefuseAll is what verifierRefusesAll returns. A verifier that refuses
// everything is the configuration #92's probe 3 used: it is the strongest
// possible verifier, so any cell that boots past it boots past every verifier.
var errRefuseAll = errors.New("matrixVerifier: refusing every artifact")

// matrixVerifier counts calls and delegates to the disposition under test.
// The call count is load-bearing: "Run returned nil" does not distinguish
// verification succeeding from verification never happening, and the whole of
// #92 is that second case.
type matrixVerifier struct {
	kind verifierKind

	mu    sync.Mutex
	calls int
}

func (v *matrixVerifier) Verify(ctx context.Context, artifactPath string, bundle *trust.Bundle) error {
	v.mu.Lock()
	v.calls++
	v.mu.Unlock()

	switch v.kind {
	case verifierAcceptsAll:
		return nil
	case verifierRefusesAll:
		return errRefuseAll
	case verifierContentBound:
		fv := &trust.FakeVerifier{}
		return fv.Verify(ctx, artifactPath, bundle)
	}
	return fmt.Errorf("matrixVerifier: Verify called with kind %v", v.kind)
}

func (v *matrixVerifier) callCount() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.calls
}

// --- dimension 2: what the bundle fetch yields ----------------------------

type bundleKind int

const (
	bundleNoFetcher bundleKind = iota // Config.BundleFetcher is nil
	bundleValidForThis
	bundleValidForOther // a valid bundle, signed over different content
	bundleEmptyBytes    // (nil, nil) — what the shipped s3 fetcher does for an empty Bundle
	bundleNonJSON
	bundleWrongMediaType // well-formed JSON, wrong mediaType: ParseBundle's second gate
	bundleFetchErr
)

func (k bundleKind) String() string {
	switch k {
	case bundleNoFetcher:
		return "bytes=no-fetcher"
	case bundleValidForThis:
		return "bytes=valid"
	case bundleValidForOther:
		return "bytes=valid-for-other"
	case bundleEmptyBytes:
		return "bytes=empty"
	case bundleNonJSON:
		return "bytes=non-json"
	case bundleWrongMediaType:
		return "bytes=wrong-media-type"
	case bundleFetchErr:
		return "bytes=fetch-error"
	}
	return fmt.Sprintf("bytes=?%d", int(k))
}

var errBundleFetch = errors.New("matrixBundleFetcher: registry unreachable")

// matrixBundleFetcher answers every layer the same way, per its kind, and counts
// how many times it was asked. The ask count distinguishes "refused before the
// fetch" from "fetched and then refused", which is the difference between the two
// doors #92 documents.
type matrixBundleFetcher struct {
	kind  bundleKind
	bytes []byte // for the two valid kinds

	mu    sync.Mutex
	asked int
}

func (f *matrixBundleFetcher) FetchBundleJSON(_ context.Context, _ spec.ResolvedLayer) ([]byte, error) {
	f.mu.Lock()
	f.asked++
	f.mu.Unlock()

	switch f.kind {
	case bundleValidForThis, bundleValidForOther:
		return f.bytes, nil
	case bundleEmptyBytes:
		return nil, nil
	case bundleNonJSON:
		return []byte("this is not a cosign bundle"), nil
	case bundleWrongMediaType:
		return []byte(`{"mediaType":"application/vnd.example.not-a-bundle+json"}`), nil
	case bundleFetchErr:
		return nil, errBundleFetch
	}
	return nil, fmt.Errorf("matrixBundleFetcher: asked with kind %v", f.kind)
}

func (f *matrixBundleFetcher) askedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asked
}

// --- the precedence ladder ------------------------------------------------

// wantFor returns the outcome the contract requires for a cell, and whether the
// implementation is known to disagree.
//
// Authored from the contract, not from verifyBundles. Rule 2 is the disagreement:
// #93(a) says nil wiring must be a refusal (or an explicit opt-out), and today it
// returns nil and boots.
func wantFor(v verifierKind, b bundleKind, namesBundle, digestMatches bool) (want bootOutcome, knownOpen bool) {
	// 1. Content integrity is checked before authenticity, so a digest mismatch
	//    pre-empts every other cell value. Asserting this for all 56 mismatched
	//    cells is what forbids a wiring combination that inverts the order.
	if !digestMatches {
		return reasonDigest, false
	}
	// 2. No verifier, or no way to fetch what it would verify.
	if v == verifierNil || b == bundleNoFetcher {
		return reasonNoVerifier, true
	}
	// 3. A layer naming no bundle cannot be verified, and with a verifier
	//    configured that is a refusal rather than a skip (#92, closed).
	if !namesBundle {
		return reasonAbsentBundle, false
	}
	// 4-6. Absent or unusable material is treated as invalid material.
	switch b {
	case bundleFetchErr:
		return reasonFetchBundle, false
	case bundleEmptyBytes:
		return reasonEmptyBytes, false
	case bundleNonJSON, bundleWrongMediaType:
		return reasonParse, false
	}
	// 7-8. Real material: the verifier decides.
	switch v {
	case verifierAcceptsAll:
		return reasonBoots, false
	case verifierRefusesAll:
		return reasonVerify, false
	case verifierContentBound:
		if b == bundleValidForThis {
			return reasonBoots, false
		}
		// A valid bundle over different content: the substitution case.
		return reasonVerify, false
	}
	panic(fmt.Sprintf("wantFor: unhandled cell v=%v b=%v", v, b))
}

// --- the table -----------------------------------------------------------

type matrixCell struct {
	name          string
	verifier      verifierKind
	bundle        bundleKind
	namesBundle   bool
	digestMatches bool
	want          bootOutcome
	knownOpen     bool
}

func buildMatrix() []matrixCell {
	verifiers := []verifierKind{verifierNil, verifierAcceptsAll, verifierRefusesAll, verifierContentBound}
	bundles := []bundleKind{
		bundleNoFetcher, bundleValidForThis, bundleValidForOther,
		bundleEmptyBytes, bundleNonJSON, bundleWrongMediaType, bundleFetchErr,
	}
	naming := []bool{false, true}
	digests := []bool{true, false}

	cells := make([]matrixCell, 0, len(verifiers)*len(bundles)*len(naming)*len(digests))
	for _, v := range verifiers {
		for _, b := range bundles {
			for _, names := range naming {
				for _, digestOK := range digests {
					want, open := wantFor(v, b, names, digestOK)
					cells = append(cells, matrixCell{
						name: fmt.Sprintf("%v/%v/bundle=%s/digest=%s",
							v, b, namedLabel(names), digestLabel(digestOK)),
						verifier:      v,
						bundle:        b,
						namesBundle:   names,
						digestMatches: digestOK,
						want:          want,
						knownOpen:     open,
					})
				}
			}
		}
	}
	return cells
}

func namedLabel(named bool) string {
	if named {
		return "named"
	}
	return "empty"
}

func digestLabel(ok bool) string {
	if ok {
		return "ok"
	}
	return "bad"
}

// TestBootMatrix_Shape checks the table before the table checks anything. A
// generated matrix can be short, unbalanced, or exercise only some arms of its
// own expectation function, and all three are indistinguishable from a clean pass
// once the cells run.
func TestBootMatrix_Shape(t *testing.T) {
	cells := buildMatrix()

	// (1) Cardinality against a literal, deliberately. Deriving it from the same
	// slices buildMatrix iterates would make a dropped dimension value shrink
	// both sides of the comparison and pass — the derivation would be a
	// consistency check against itself. So the count is stated, and changing a
	// dimension means editing this line on purpose. Check (2) is the derived one:
	// it compares each value's share against the cell total, which no single
	// declaration controls.
	wantCount := 4 * 7 * 2 * 2
	if len(cells) != wantCount {
		t.Fatalf("matrix has %d cells, want %d (4 verifiers x 7 bundle kinds x 2 bundle fields x 2 digests)",
			len(cells), wantCount)
	}

	// (2) Balance: every value of every dimension appears in exactly
	// len(cells)/n cells. This is what catches a dropped or duplicated value,
	// which a bare cardinality check cannot.
	byVerifier := map[verifierKind]int{}
	byBundle := map[bundleKind]int{}
	byNaming := map[bool]int{}
	byDigest := map[bool]int{}
	for _, c := range cells {
		byVerifier[c.verifier]++
		byBundle[c.bundle]++
		byNaming[c.namesBundle]++
		byDigest[c.digestMatches]++
	}
	for _, d := range []struct {
		what  string
		size  int
		count map[string]int
	}{
		{"verifier", 4, stringifyKeys(byVerifier)},
		{"bundle", 7, stringifyKeys(byBundle)},
		{"names bundle", 2, stringifyKeys(byNaming)},
		{"digest matches", 2, stringifyKeys(byDigest)},
	} {
		if len(d.count) != d.size {
			t.Errorf("dimension %q has %d distinct values in the table, want %d",
				d.what, len(d.count), d.size)
			continue
		}
		want := len(cells) / d.size
		for value, n := range d.count {
			if n != want {
				t.Errorf("dimension %q value %s appears in %d cells, want %d",
					d.what, value, n, want)
			}
		}
	}

	// (3) Every arm of the ladder fires. Asserted as a set both ways: an arm no
	// cell reaches is an unexercised branch, and a cell reaching an arm not on
	// the manifest means the manifest is stale.
	fired := map[bootOutcome]int{}
	for _, c := range cells {
		fired[c.want]++
	}
	manifest := map[bootOutcome]bool{}
	for _, r := range ladderOutcomes {
		manifest[r] = true
		if fired[r] == 0 {
			t.Errorf("ladder outcome %q fires for no cell — the arm is unexercised", r)
		}
	}
	for r := range fired {
		if !manifest[r] {
			t.Errorf("cells expect outcome %q, which is not in ladderOutcomes", r)
		}
	}

	// (4) Both classes of outcome occur, and cell names are unique — a duplicate
	// name silently merges two cells under go test's subtest naming.
	if fired[reasonBoots] == 0 {
		t.Error("no cell boots — the table would pass by refusing everything")
	}
	refusals := len(cells) - fired[reasonBoots] - fired[reasonNoVerifier]
	if refusals == 0 {
		t.Error("no cell refuses — the table asserts nothing about refusal")
	}
	seen := map[string]bool{}
	for _, c := range cells {
		if seen[c.name] {
			t.Errorf("duplicate cell name %q", c.name)
		}
		seen[c.name] = true
	}

	// (5) The size of the open hole, as a literal, checked against the expectation
	// table it is a claim about. Both sides come from wantFor(), so this catches the
	// table being edited and not the code being fixed — see knownOpenCells.
	open := fired[reasonNoVerifier]
	if open != knownOpenCells {
		t.Errorf("%d cells are #93(a) fail-open, knownOpenCells says %d — "+
			"if the hole moved, say so here", open, knownOpenCells)
	}
	t.Logf("%d cells; %d reach the mount unverified (#93(a)); outcome distribution: %s",
		len(cells), open, describe(fired))
}

func stringifyKeys[K comparable](m map[K]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[fmt.Sprint(k)] = v
	}
	return out
}

func describe(fired map[bootOutcome]int) string {
	parts := make([]string, 0, len(ladderOutcomes))
	for _, r := range ladderOutcomes {
		label := string(r)
		if r == reasonBoots {
			label = "boots"
		}
		parts = append(parts, fmt.Sprintf("%s=%d", label, fired[r]))
	}
	return strings.Join(parts, " ")
}

// TestBootMatrix_DecisionSurface runs every cell.
func TestBootMatrix_DecisionSurface(t *testing.T) {
	for _, c := range buildMatrix() {
		c := c
		t.Run(c.name, func(t *testing.T) {
			runMatrixCell(t, c)
		})
	}
}

func runMatrixCell(t *testing.T, c matrixCell) {
	t.Helper()
	ctx := context.Background()

	layer, path := makeLayer(t, "python-3.11", []byte("squashfs content alpha"), 1)
	if c.namesBundle {
		layer.Bundle = "s3://strata-registry/bundles/python-3.11.json"
	}
	if !c.digestMatches {
		// Declare a digest the file does not have. The file is untouched, so the
		// only difference from the matching cell is the manifest claim.
		other := sha256.Sum256([]byte("some other content entirely"))
		layer.SHA256 = hex.EncodeToString(other[:])
	}

	verifier := &matrixVerifier{kind: c.verifier}
	fetcher := &matrixBundleFetcher{kind: c.bundle}
	switch c.bundle {
	case bundleValidForThis:
		fetcher.bytes = signedBundleJSON(t, path)
	case bundleValidForOther:
		fetcher.bytes = signedBundleJSON(t, writeOtherArtifact(t))
	}

	cfg := agent.Config{
		Source:   &agent.FakeLockfileSource{Lockfile: &spec.LockFile{ProfileName: "ml-env", Layers: []spec.ResolvedLayer{layer}}},
		Fetcher:  &agent.FakeLayerFetcher{Paths: map[string]string{layer.ID: path}},
		Signaler: &agent.FakeReadySignaler{},
		Mounter:  &recordingMounter{},
	}
	signaler := cfg.Signaler.(*agent.FakeReadySignaler)
	mounter := cfg.Mounter.(*recordingMounter)
	if c.verifier != verifierNil {
		cfg.Verifier = verifier
	}
	if c.bundle != bundleNoFetcher {
		cfg.BundleFetcher = fetcher
	}

	_, err := newAgent(t, cfg).Run(ctx)

	if c.knownOpen {
		// #93(a): this cell should refuse and does not. Assert what actually
		// happens, including that the mount was reached with nothing verified —
		// stating the consequence, not just the tolerance.
		if err != nil {
			t.Fatalf("this cell is marked #93(a) fail-open but Run refused: %v\n"+
				"If #93(a) is closed, change wantFor()'s rule 2 to return the refusal, "+
				"drop the knownOpen flag, and lower knownOpenCells.", err)
		}
		if !mounter.wasCalled() {
			t.Error("Run succeeded without reaching Mount — the fail-open cell's premise is wrong")
		}
		if !signaler.ReadyCalled {
			t.Error("Run succeeded without signalling ready")
		}
		if got := verifier.callCount(); got != 0 {
			t.Errorf("Verify call count = %d, want 0 — nothing was verified on this path", got)
		}
		return
	}

	if c.want == reasonBoots {
		if err != nil {
			t.Fatalf("Run: want boot, got error: %v", err)
		}
		if !mounter.wasCalled() {
			t.Error("Mount was not reached on a booting cell")
		}
		if !signaler.ReadyCalled {
			t.Error("SignalReady was not called on a booting cell")
		}
		if signaler.FailedCalled {
			t.Errorf("SignalFailed was called on a booting cell: %v", signaler.FailedReason)
		}
		if got := verifier.callCount(); got != 1 {
			t.Errorf("Verify call count = %d, want 1 — a boot that verified nothing is the #92 defect", got)
		}
		return
	}

	// Refusal cells.
	if err == nil {
		t.Fatalf("Run: want refusal %q, got nil", c.want)
	}
	if !strings.Contains(err.Error(), string(c.want)) {
		t.Errorf("Run error should name %q, got: %v", c.want, err)
	}
	if mounter.wasCalled() {
		t.Errorf("Mount was reached despite %q", c.want)
	}
	if signaler.ReadyCalled {
		t.Errorf("SignalReady was called despite %q", c.want)
	}
	if !signaler.FailedCalled {
		t.Errorf("SignalFailed was not called on refusal %q", c.want)
	}

	// Which checks ran before the refusal. This is the half that stops a refusal
	// from passing for the wrong reason: reasonVerify must have called the
	// verifier, and everything earlier must not have.
	wantVerifyCalls := 0
	if c.want == reasonVerify {
		wantVerifyCalls = 1
	}
	if got := verifier.callCount(); got != wantVerifyCalls {
		t.Errorf("Verify call count = %d, want %d for refusal %q", got, wantVerifyCalls, c.want)
	}

	// The bundle fetch happens only for a layer that names a bundle and only
	// once the wiring is complete: an ask on an absent-bundle cell would mean the
	// refusal came after the fetch, reopening #92's second door.
	wantAsked := 0
	switch c.want {
	case reasonFetchBundle, reasonEmptyBytes, reasonParse, reasonVerify:
		wantAsked = 1
	}
	if got := fetcher.askedCount(); got != wantAsked {
		t.Errorf("BundleFetcher asked %d times, want %d for refusal %q", got, wantAsked, c.want)
	}
}

// writeOtherArtifact returns the path of a file whose content differs from the
// layer under test, so a bundle signed over it is valid and wrong.
func writeOtherArtifact(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "other.sqfs")
	if err := os.WriteFile(path, []byte("squashfs content from a different layer"), 0o600); err != nil {
		t.Fatalf("writing substitute artifact: %v", err)
	}
	return path
}

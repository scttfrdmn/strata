package resolver

// Evidence for PROPERTIES.md R4 (provider soundness) and R5 (provider
// completeness), both of which stood SOUND with a Basis of `none` — asserted by
// the document and executed by nothing.
//
// Both were refuted by issue #67, whose two halves lived in different stages:
//
//	stage 4 (R5) `spec.BaseCapabilities.SatisfiesRequirement` walked Provides and
//	  decided on the FIRST entry whose name matched, so a satisfiable profile was
//	  rejected when a non-satisfying provider of the same capability name was
//	  merged in first.
//	stage 6 (R4) `stage6TopoSort` built `capProviderIdx[cap.Name] = i` in a
//	  nested loop with no guard, so the LAST provider of a capability won the
//	  dependency edge irrespective of version.
//
// Both are fixed as of #67, and the counts below are now zero. That changes what
// this file is for without changing a line of its structure: it was a
// reproduction, and it is now a regression barrier. The distinction matters when
// reading a failure — a nonzero count here no longer means "the defect is where
// we said it was", it means the defect is back.
//
// These tests enumerate a declared, finite domain of the shipping resolver
// rather than sampling it, so what they establish is the exact boundary: which
// cells violate and which hold. That is what made the refutation reproducible,
// which is what #67's register row needed in order to be dischargeable at all —
// and it makes the tests fail in BOTH directions. A regression makes a count
// nonzero; a change in the enumeration makes the cardinality mismatch. Either
// way the stated numbers stop matching and someone has to look.
//
// One assertion is deliberately NOT a count: stage 4's verdict is now asserted
// to be the same for both provider orders of the same pair. A count of known
// defects has to be maintained by hand and says nothing once it reaches zero,
// whereas order-independence is the property the fix actually established and it
// stays falsifiable forever.
//
// The version oracle here is deliberately NOT the implementation's semverGTE and
// semverLT. Expectations computed with the code under test would hold by
// construction, so satisfaction is decided by comparing major versions as
// integers over a domain chosen to make that sufficient.

import (
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

// capName is the capability both providers in these tables provide, and the one
// the consumer requires.
const capName = "mpi"

// matrixVersions is the provider version domain. Majors only, so this test's own
// integer comparison is a complete oracle for it without borrowing the semver
// implementation it is measuring.
func matrixVersions() []string { return []string{"1.0.0", "2.0.0", "3.0.0"} }

// matrixRequirements is the constraint domain: unconstrained, min only, max only
// (exclusive), and both.
func matrixRequirements() []spec.Requirement {
	return []spec.Requirement{
		{Name: capName},
		{Name: capName, MinVersion: "2.0.0"},
		{Name: capName, MaxVersion: "3.0.0"},
		{Name: capName, MinVersion: "2.0.0", MaxVersion: "3.0.0"},
	}
}

// major is the oracle's parser. It fails the test rather than guessing, so a
// version added to the domain without extending the oracle cannot pass silently.
func major(t *testing.T, version string) int {
	t.Helper()
	switch version {
	case "1.0.0":
		return 1
	case "2.0.0":
		return 2
	case "3.0.0":
		return 3
	default:
		t.Fatalf("oracle has no major for %q; extend major() with the domain", version)
		return 0
	}
}

// satisfiesOracle decides satisfaction independently of spec's semver helpers.
// MaxVersion is exclusive, matching Requirement's documented meaning.
func satisfiesOracle(t *testing.T, version string, req spec.Requirement) bool {
	t.Helper()
	v := major(t, version)
	if req.MinVersion != "" && v < major(t, req.MinVersion) {
		return false
	}
	if req.MaxVersion != "" && v >= major(t, req.MaxVersion) {
		return false
	}
	return true
}

// provider is a layer providing capName at one version and requiring nothing.
func provider(id, version string) resolvedLayer {
	return resolvedLayer{manifest: &spec.LayerManifest{
		ID:       id,
		Name:     id,
		Version:  version,
		Provides: []spec.Capability{{Name: capName, Version: version}},
	}}
}

// consumer is a layer requiring capName under req and providing nothing.
func consumer(req spec.Requirement) resolvedLayer {
	return resolvedLayer{manifest: &spec.LayerManifest{
		ID:       "consumer",
		Name:     "consumer",
		Version:  "1.0.0",
		Requires: []spec.Requirement{req},
	}}
}

// TestStage4ProviderCompleteness enumerates R5's domain: two providers of one
// capability at every pair of versions, every constraint, both slice orders.
//
// R5: if some layer in the resolved set satisfies the constraint, resolution does
// not fail for want of a provider. So a cell violates R5 when the oracle says
// some provider satisfies and stage 4 rejects anyway.
//
// The converse is asserted too, and it is what stops R5 being "fixed" by never
// rejecting anything: when NO provider satisfies, stage 4 must reject.
//
// Since the fix, the load-bearing assertion is neither of those counts but
// order-independence: the same pair of providers must produce the same verdict in
// either slice order. A count of known defects stops discriminating once it is
// zero — every implementation that rejects nothing scores zero too — whereas the
// order comparison has no such fixed point.
func TestStage4ProviderCompleteness(t *testing.T) {
	// Cardinality is stated rather than derived from the same slices the loops
	// iterate: computing it from len(versions)*len(versions)*len(reqs)*2 would
	// let a dropped dimension value shrink both sides and pass.
	const wantCells = 72
	// #67's stage-4 half, fixed: SatisfiesRequirement now scans every provider
	// instead of deciding on the first name match. This was 12 — the cells where a
	// satisfiable set was rejected because the first provider of the name did not
	// satisfy — and it is a count of KNOWN DEFECTS, so zero is the post-fix value
	// and any nonzero is a regression.
	const wantViolations = 0
	// Non-vacuity bound for the order-independence assertion, which compares two
	// verdicts and therefore asserts nothing about a pair whose providers agree.
	// A pair discriminates only when exactly one of the two satisfies: 4 for each
	// of the three constrained requirements (satisfying set size s of 3 gives
	// 2*s*(3-s) discordant ordered pairs, so 4, 4, 4) and 0 for the unconstrained
	// one, where all three satisfy. That is the same 12 wantViolations used to
	// hold, and not by coincidence — a discordant pair has exactly one order whose
	// first provider fails, which is exactly what produced one violating cell. The
	// arithmetic is why the assertion can REPLACE the count rather than sit
	// alongside it.
	const wantDiscriminating = 12

	versions := matrixVersions()
	reqs := matrixRequirements()
	base := &spec.BaseCapabilities{} // Provides nothing, so the layer path decides.

	cells, violations, satisfiableCells, discriminating := 0, 0, 0, 0
	for _, vA := range versions {
		for _, vB := range versions {
			for _, req := range reqs {
				// Stage 4's verdict must not depend on which of the two
				// providers comes first in the slice. This is the property the
				// stage-4 half actually broke, and it is asserted instead of
				// merely counted because it survives the fix: a count of known
				// defects says nothing once it reaches zero, whereas this stays
				// falsifiable forever. On the first-match implementation the two
				// orders disagree in exactly the 12 cells wantViolations used to
				// hold, so this assertion subsumes that count rather than
				// duplicating it.
				accepted := make(map[bool]bool, 2)
				if satisfiesOracle(t, vA, req) != satisfiesOracle(t, vB, req) {
					discriminating++
				}

				for _, aFirst := range []bool{true, false} {
					cells++

					pA, pB := provider("provider-a", vA), provider("provider-b", vB)
					layers := []resolvedLayer{pA, pB, consumer(req)}
					firstVersion := vA
					if !aFirst {
						layers = []resolvedLayer{pB, pA, consumer(req)}
						firstVersion = vB
					}

					satisfiable := satisfiesOracle(t, vA, req) || satisfiesOracle(t, vB, req)
					if satisfiable {
						satisfiableCells++
					}
					err := (&Resolver{}).stage4ValidateGraph(base, layers)
					accepted[aFirst] = err == nil
					name := req.String() + " providers=" + firstVersion + " then " +
						map[bool]string{true: vB, false: vA}[aFirst]

					switch {
					case satisfiable && err != nil:
						violations++
						// The claim about WHY this cell violates, stated as a
						// property of the domain rather than copied from the
						// implementation: only a non-satisfying first provider
						// can produce this, because nothing else in stage 4
						// looks at order.
						if satisfiesOracle(t, firstVersion, req) {
							t.Errorf("%s: stage 4 rejected a satisfiable set whose FIRST provider satisfies (%v); "+
								"R5 is violated by a mechanism other than the old first-match", name, err)
						}
					case !satisfiable && err == nil:
						t.Errorf("%s: no provider satisfies the constraint and stage 4 accepted; "+
							"totality is broken, or R5 was 'fixed' by never rejecting", name)
					}
				}

				if accepted[true] != accepted[false] {
					t.Errorf("%s providers={%s,%s}: stage 4 accepts with one provider first and "+
						"rejects with the other (a-first accepted=%v, b-first accepted=%v); "+
						"the verdict depends on slice order",
						req.String(), vA, vB, accepted[true], accepted[false])
				}
			}
		}
	}

	if cells != wantCells {
		t.Errorf("enumerated %d cells, want %d", cells, wantCells)
	}
	if violations != wantViolations {
		t.Errorf("R5 violated in %d cells, want %d. #67's stage-4 half is fixed, so nonzero "+
			"here is a regression: a satisfiable requirement is being rejected. Do not restate "+
			"this constant to match — SatisfiesRequirement must scan every provider",
			violations, wantViolations)
	}
	// Non-vacuity: an oracle that called nothing satisfiable would make every
	// R5 assertion above unreachable while the table still passed.
	if satisfiableCells == 0 || satisfiableCells == cells {
		t.Errorf("%d of %d cells are satisfiable; a table with no mix of both cannot "+
			"exercise R5 and its converse", satisfiableCells, cells)
	}
	// Non-vacuity for the order-independence assertion specifically. Stated as an
	// exact count rather than >0 because both directions are informative: fewer
	// discriminating pairs means the version domain or the constraint domain lost
	// the asymmetry the assertion needs, and more means the domain grew and the
	// arithmetic in the constant's derivation no longer describes it.
	if discriminating != wantDiscriminating {
		t.Errorf("%d of %d (vA,vB,req) combinations have exactly one satisfying provider, want %d; "+
			"only those can falsify order-independence, so a change here changes what that "+
			"assertion covers", discriminating, len(versions)*len(versions)*len(reqs), wantDiscriminating)
	}
}

// permutations returns every ordering of items. Used to enumerate the input
// order of a four-layer set, because slice order is the whole axis both halves of
// #67 turn on.
func permutations(items []string) [][]string {
	if len(items) <= 1 {
		return [][]string{append([]string(nil), items...)}
	}
	var out [][]string
	for i := range items {
		rest := make([]string, 0, len(items)-1)
		rest = append(rest, items[:i]...)
		rest = append(rest, items[i+1:]...)
		for _, p := range permutations(rest) {
			out = append(out, append([]string{items[i]}, p...))
		}
	}
	return out
}

// TestStage6ProviderSoundness enumerates R4's domain.
//
// R4 is about an edge, and stage 6 does not expose its edges — so the property is
// asserted through the one consequence that IS observable: a consumer whose only
// satisfying provider is P must sort after P. Four layers are needed to make that
// falsifiable. With only a consumer and two providers, the consumer always has
// in-degree 1 and both providers have in-degree 0, so the consumer sorts last
// whichever provider owns the edge, and a wrong edge is invisible.
//
//	yy-provider  provides yy
//	mpi-3        provides mpi@3.0.0, requires yy   <- the only satisfying provider
//	mpi-1        provides mpi@1.0.0                <- does not satisfy mpi>=2.0.0
//	consumer     requires mpi>=2.0.0
//
// Because mpi-3 is itself blocked behind yy-provider, an edge wrongly pointing at
// mpi-1 releases the consumer early and it mounts BEFORE the provider it needs.
//
// Two reachability facts are asserted rather than assumed, because either would
// make this table vacuous:
//
//	stage 5 must not reject the two same-name providers (it does not: both have
//	  the default InstallLayout, so canCoexist exempts them), and
//	stage 4 must accept, which before the fix it only did when the first mpi
//	  provider in slice order satisfied — so the stage-4 half MASKED the stage-6
//	  half in half the permutations. Fixing stage 4 unmasks them, which is why the
//	  reachable count below went from 12 to 24 in the same change that took
//	  violations to zero. The two halves were coupled through this number, and
//	  that coupling is the reason they had to be fixed together: closing stage 4
//	  alone would have doubled the exposure of stage 6.
func TestStage6ProviderSoundness(t *testing.T) {
	const wantCells = 24 // 4! input orders.
	// Every order now reaches stage 6, because stage 4 no longer rejects on the
	// basis of which provider comes first. This was 12 — the orders with mpi-3
	// ahead of mpi-1 — and the change from 12 to 24 is the masking being removed,
	// not the domain growing.
	const wantReachable = 24
	// #67's stage-6 half, fixed: the edge is chosen by highest satisfying version
	// (ties by lowest layer ID) instead of by whichever provider happened to be
	// assigned last. This was 3.
	const wantViolations = 0

	build := func(name string) resolvedLayer {
		switch name {
		case "yy-provider":
			return resolvedLayer{manifest: &spec.LayerManifest{
				ID: name, Name: name, Version: "1.0.0",
				Provides: []spec.Capability{{Name: "yy", Version: "1.0.0"}},
			}}
		case "mpi-3":
			l := provider(name, "3.0.0")
			l.manifest.Requires = []spec.Requirement{{Name: "yy"}}
			return l
		case "mpi-1":
			return provider(name, "1.0.0")
		case "consumer":
			return consumer(spec.Requirement{Name: capName, MinVersion: "2.0.0"})
		default:
			t.Fatalf("no fixture for %q", name)
			return resolvedLayer{}
		}
	}

	base := &spec.BaseCapabilities{}
	cells, reachable, violations := 0, 0, 0
	for _, order := range permutations([]string{"yy-provider", "mpi-3", "mpi-1", "consumer"}) {
		cells++
		layers := make([]resolvedLayer, 0, len(order))
		for _, name := range order {
			layers = append(layers, build(name))
		}
		label := ""
		for i, name := range order {
			if i > 0 {
				label += " "
			}
			label += name
		}

		r := &Resolver{}
		if err := r.stage4ValidateGraph(base, layers); err != nil {
			// Before #67 this was the masking path and was recorded rather than
			// asserted clean, the rejection being an R5 violation enumerated by
			// the test above. It is now a fault in its own right: mpi-3 satisfies
			// mpi>=2.0.0 in every one of these orders, so a rejection here means
			// stage 4 has regressed to deciding on slice order. Asserted per-order
			// as well as counted below, because "12 reached stage 6, want 24" does
			// not say WHICH orders stopped or why.
			t.Errorf("%s: stage 4 rejected a set in which mpi-3 satisfies mpi>=2.0.0 (%v); "+
				"the stage-4 half has regressed and is masking this table again", label, err)
			continue
		}
		reachable++

		// The premise: these two providers must be allowed to coexist, or R4's
		// domain is empty here and the table below asserts nothing.
		if err := r.stage5DetectConflicts(layers); err != nil {
			t.Fatalf("%s: stage 5 rejected two coexisting providers (%v); R4's domain is "+
				"unreachable and this table would be vacuous", label, err)
		}

		ordered, err := r.stage6TopoSort(layers)
		if err != nil {
			t.Fatalf("%s: stage 6 failed: %v", label, err)
		}
		if len(ordered) != len(layers) {
			t.Fatalf("%s: stage 6 returned %d layers, want %d", label, len(ordered), len(layers))
		}

		pos := make(map[string]int, len(ordered))
		for i, rl := range ordered {
			pos[rl.manifest.ID] = i
		}
		if len(pos) != len(layers) {
			t.Fatalf("%s: stage 6 output names %d distinct layers, want %d", label, len(pos), len(layers))
		}

		if pos["consumer"] < pos["mpi-3"] {
			violations++
			// The harm, spelled out so the failure is legible without the
			// issue: the consumer requires mpi>=2.0.0, mpi-3 is the only
			// provider satisfying that, and the consumer mounts first.
			t.Errorf("%s: consumer (requires mpi>=2.0.0) sorts at %d, before its only satisfying "+
				"provider mpi-3 at %d; the dependency edge points at the wrong layer",
				label, pos["consumer"], pos["mpi-3"])
			if pos["mpi-1"] >= pos["consumer"] {
				t.Errorf("%s: and NOT after mpi-1 either; R4 is violated by a mechanism other "+
					"than the old last-wins index", label)
			}
		}
	}

	if cells != wantCells {
		t.Errorf("enumerated %d input orders, want %d", cells, wantCells)
	}
	if reachable != wantReachable {
		t.Errorf("%d cells reached stage 6, want %d; every input order must reach it now that "+
			"stage 4 is order-independent, so fewer means the stage-4 half regressed and is "+
			"masking this table", reachable, wantReachable)
	}
	if violations != wantViolations {
		t.Errorf("R4 violated in %d of %d reachable cells, want %d. #67's stage-6 half is fixed, "+
			"so nonzero here is a regression: the edge is being drawn to a provider that does not "+
			"satisfy. Do not restate this constant to match", violations, reachable, wantViolations)
	}
	// Non-vacuity, stated separately from the count above: a table where nothing
	// reached stage 6 would report zero violations and look like a clean pass.
	if reachable == 0 {
		t.Error("no cell reached stage 6; every R4 assertion above was unreachable")
	}
}

// TestStage6SelectsHighestSatisfyingProvider pins the selection rule, which R4
// does not constrain and which #67's fix therefore had to choose.
//
// R4 only requires the edge to point at SOME satisfying provider. When two
// providers both satisfy, that leaves the choice open, and the two candidates are
// not equivalent: "first satisfying in slice order" is a function of the order the
// profile author listed the layers, while "highest satisfying version" is a
// function of the capabilities alone. Since the edge determines mount order and
// mount order goes into the lockfile, the first rule would make the lockfile
// depend on an author-visible ordering — the same class of defect as #67 itself.
// So the rule is highest version, ties broken by lowest layer ID, and this test
// is what holds it in place.
//
// TestStage6ProviderSoundness cannot do that job: there, mpi-1 does not satisfy
// mpi>=2.0.0, so every rule that picks a satisfying provider picks mpi-3 and the
// two candidate rules are indistinguishable. Here BOTH providers satisfy, and
// that premise is asserted rather than assumed.
//
//	yy-provider  provides yy
//	mpi-3        provides mpi@3.0.0, requires yy   <- highest; blocked behind yy
//	mpi-2        provides mpi@2.0.0                <- also satisfies mpi>=2.0.0
//	consumer     requires mpi>=2.0.0
//
// mpi-3 is blocked behind yy-provider for the same reason as in the soundness
// table, and the reason is worth restating because without it this test passes
// under both rules: with no prerequisite, the consumer is the only layer with any
// in-degree, so Kahn's algorithm emits it last whichever provider owns the edge
// and the wrong choice is invisible. The block gives the wrong edge a way to
// release the consumer early.
func TestStage6SelectsHighestSatisfyingProvider(t *testing.T) {
	const wantCells = 24 // 4! input orders.

	req := spec.Requirement{Name: capName, MinVersion: "2.0.0"}
	if !satisfiesOracle(t, "2.0.0", req) || !satisfiesOracle(t, "3.0.0", req) {
		t.Fatalf("both providers must satisfy %s, or this table cannot distinguish "+
			"highest-version selection from first-in-slice selection", req)
	}

	build := func(name string) resolvedLayer {
		switch name {
		case "yy-provider":
			return resolvedLayer{manifest: &spec.LayerManifest{
				ID: name, Name: name, Version: "1.0.0",
				Provides: []spec.Capability{{Name: "yy", Version: "1.0.0"}},
			}}
		case "mpi-3":
			l := provider(name, "3.0.0")
			l.manifest.Requires = []spec.Requirement{{Name: "yy"}}
			return l
		case "mpi-2":
			return provider(name, "2.0.0")
		case "consumer":
			return consumer(req)
		default:
			t.Fatalf("no fixture for %q", name)
			return resolvedLayer{}
		}
	}

	base := &spec.BaseCapabilities{}
	cells := 0
	for _, order := range permutations([]string{"yy-provider", "mpi-3", "mpi-2", "consumer"}) {
		cells++
		layers := make([]resolvedLayer, 0, len(order))
		for _, name := range order {
			layers = append(layers, build(name))
		}
		label := ""
		for i, name := range order {
			if i > 0 {
				label += " "
			}
			label += name
		}

		r := &Resolver{}
		// Both premises asserted per-order, because either one failing would make
		// the assertion below unreachable for that order while the loop still ran
		// to completion and the test still passed.
		if err := r.stage4ValidateGraph(base, layers); err != nil {
			t.Errorf("%s: stage 4 rejected a set where both mpi providers satisfy (%v)", label, err)
			continue
		}
		if err := r.stage5DetectConflicts(layers); err != nil {
			t.Fatalf("%s: stage 5 rejected two coexisting mpi providers (%v); the selection "+
				"rule's domain is unreachable and this table would be vacuous", label, err)
		}

		ordered, err := r.stage6TopoSort(layers)
		if err != nil {
			t.Fatalf("%s: stage 6 failed: %v", label, err)
		}
		pos := make(map[string]int, len(ordered))
		for i, rl := range ordered {
			pos[rl.manifest.ID] = i
		}
		if len(pos) != len(layers) {
			t.Fatalf("%s: stage 6 output names %d distinct layers, want %d", label, len(pos), len(layers))
		}

		// The observable consequence of selecting mpi-3: the consumer inherits
		// mpi-3's own prerequisite and cannot mount before it. Under a
		// first-satisfying rule the edge points at mpi-2 in the orders where mpi-2
		// precedes mpi-3, and the consumer is released as soon as mpi-2 is emitted
		// — ahead of mpi-3 in some of them.
		if pos["consumer"] < pos["mpi-3"] {
			t.Errorf("%s: consumer sorts at %d, before mpi-3 at %d; the edge was drawn to "+
				"mpi-2 (also satisfying, lower version), so selection is following slice "+
				"order rather than version", label, pos["consumer"], pos["mpi-3"])
		}
	}

	if cells != wantCells {
		t.Errorf("enumerated %d input orders, want %d", cells, wantCells)
	}
}

package resolver

// Evidence for PROPERTIES.md R4 (provider soundness) and R5 (provider
// completeness), both of which stood SOUND with a Basis of `none` — asserted by
// the document and executed by nothing.
//
// Both are refuted by issue #67, whose two halves live in different stages:
//
//	stage 4 (R5) `spec.BaseCapabilities.SatisfiesRequirement` walks Provides and
//	  decides on the FIRST entry whose name matches, so a satisfiable profile is
//	  rejected when a non-satisfying provider of the same capability name is
//	  merged in first.
//	stage 6 (R4) `stage6TopoSort` builds `capProviderIdx[cap.Name] = i` in a
//	  nested loop with no guard, so the LAST provider of a capability wins the
//	  dependency edge irrespective of version.
//
// These tests enumerate a declared, finite domain of the shipping resolver
// rather than sampling it, so what they establish is the exact boundary of the
// defect: which cells violate and which hold. That makes the refutation
// reproducible, which is what #67's register row needs in order to be
// dischargeable at all — and it makes the tests fail in BOTH directions. A
// regression widens the violating set; a fix empties it. Either way the stated
// counts stop matching and someone has to look.
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
func TestStage4ProviderCompleteness(t *testing.T) {
	// Cardinality is stated rather than derived from the same slices the loops
	// iterate: computing it from len(versions)*len(versions)*len(reqs)*2 would
	// let a dropped dimension value shrink both sides and pass.
	const wantCells = 72
	// #67's stage-4 half. This is a count of KNOWN DEFECTS: it must go to zero
	// when #67 is fixed, and this test must then be updated on purpose.
	const wantViolations = 12

	versions := matrixVersions()
	reqs := matrixRequirements()
	base := &spec.BaseCapabilities{} // Provides nothing, so the layer path decides.

	cells, violations, satisfiableCells := 0, 0, 0
	for _, vA := range versions {
		for _, vB := range versions {
			for _, req := range reqs {
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
								"R5 is violated by a mechanism other than #67's first-match", name, err)
						}
					case !satisfiable && err == nil:
						t.Errorf("%s: no provider satisfies the constraint and stage 4 accepted; "+
							"totality is broken, or R5 was 'fixed' by never rejecting", name)
					case satisfiable && err == nil:
						// R5 holds here, and it must be for the right reason.
						if !satisfiesOracle(t, firstVersion, req) {
							t.Errorf("%s: stage 4 accepted although the first provider does not satisfy; "+
								"#67's stage-4 half no longer reproduces — update wantViolations", name)
						}
					}
				}
			}
		}
	}

	if cells != wantCells {
		t.Errorf("enumerated %d cells, want %d", cells, wantCells)
	}
	if violations != wantViolations {
		t.Errorf("R5 violated in %d cells, want %d (#67's stage-4 half). "+
			"Fewer means the defect is being fixed and this test needs updating on purpose; "+
			"more means a regression", violations, wantViolations)
	}
	// Non-vacuity: an oracle that called nothing satisfiable would make every
	// R5 assertion above unreachable while the table still passed.
	if satisfiableCells == 0 || satisfiableCells == cells {
		t.Errorf("%d of %d cells are satisfiable; a table with no mix of both cannot "+
			"exercise R5 and its converse", satisfiableCells, cells)
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
//	stage 4 must accept, which it only does when the first mpi provider in slice
//	  order satisfies — so #67's stage-4 half MASKS its stage-6 half in half the
//	  permutations, and the reachable count is stated below.
func TestStage6ProviderSoundness(t *testing.T) {
	const wantCells = 24     // 4! input orders.
	const wantReachable = 12 // Those stage 4 accepts: mpi-3 before mpi-1.
	const wantViolations = 3 // #67's stage-6 half, at min-index tie-breaking.

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
			// Masked by #67's other half. Recorded, not asserted clean: the
			// rejection is itself an R5 violation, enumerated by the test above.
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
			if pos["mpi-1"] >= pos["consumer"] {
				t.Errorf("%s: consumer sorts before mpi-3 and NOT after mpi-1 either; "+
					"R4 is violated by a mechanism other than #67's last-wins index", label)
			}
		}
	}

	if cells != wantCells {
		t.Errorf("enumerated %d input orders, want %d", cells, wantCells)
	}
	if reachable != wantReachable {
		t.Errorf("%d cells reached stage 6, want %d; #67's stage-4 half masks the rest, "+
			"so a change here means one of the two halves moved", reachable, wantReachable)
	}
	if violations != wantViolations {
		t.Errorf("R4 violated in %d of %d reachable cells, want %d (#67's stage-6 half). "+
			"Zero means the defect is fixed and this test needs updating on purpose",
			violations, reachable, wantViolations)
	}
	// Non-vacuity, stated separately from the count above: a table where nothing
	// reached stage 6 would report zero violations and look like a clean pass.
	if reachable == 0 {
		t.Error("no cell reached stage 6; every R4 assertion above was unreachable")
	}
}

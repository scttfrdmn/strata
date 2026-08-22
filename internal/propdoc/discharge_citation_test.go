package propdoc

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// Tests for the §2.1 rule 11 parse guard: a register row may be marked
// `Discharged: Yes` only on evidence it names and cites, and "closed completed"
// is neither.
//
// The rows are constructed here rather than taken from PROPERTIES.md on purpose.
// A guard measured only against the current document says nothing about what it
// would do to the next row — and the whole reason this guard exists is that rule
// 11 was written and never run against the rows already in the table (#137).
// TestDischargeGuardAcceptsTheCurrentRegister below adds the corpus check as a
// separate claim.

// TestCheckDischargeCitation is the must-reject / must-accept table. Each
// rejection asserts *which* reason fired: a cell missing both halves must be
// reported as missing the basis, not accepted because some other check caught
// it, and a rejection from the wrong reason is not evidence for the right one.
func TestCheckDischargeCitation(t *testing.T) {
	for _, tc := range []struct {
		name string
		cell string
		want error // nil means the cell must be accepted
	}{
		{
			name: "the closure string, which is what rule 11 names",
			cell: "Yes — closed completed",
			want: ErrDischargeNoBasis,
		},
		{
			name: "a bare Yes offers nothing at all",
			cell: "Yes",
			want: ErrDischargeNoBasis,
		},
		{
			name: "a basis with no artifact is a claim about strength only",
			cell: "Yes — E1, verified by hand on the release branch",
			want: ErrDischargeNoCitation,
		},
		{
			name: "an issue reference is not an artifact citation",
			cell: "Yes — E1, fixed by #104 and confirmed",
			want: ErrDischargeNoCitation,
		},
		{
			name: "a backticked command is not an artifact citation",
			cell: "Yes — chosen/implementation, `go test ./internal/agent/`",
			want: ErrDischargeNoCitation,
		},
		{
			name: "an artifact with no basis leaves rule 11's floor unstated",
			cell: "Yes — see `internal/agent/verify_bundles_test.go:291`",
			want: ErrDischargeNoBasis,
		},
		{
			name: "a path outside backticks is prose, not a citation",
			cell: "Yes — E1, internal/agent/verify_bundles_test.go:291",
			want: ErrDischargeNoCitation,
		},
		{
			name: "legacy basis name with a Go path",
			cell: "Yes — E1, `cmd/strata/run_verify_test.go:264,350,519`",
			want: nil,
		},
		{
			name: "pair spelling with a Go path",
			cell: "Yes — exhaustive/implementation, `internal/agent/boot_matrix_test.go:88`",
			want: nil,
		},
		{
			name: "a non-Go artifact is a legitimate citation",
			cell: "Yes — E1, `.github/workflows/ci.yml:90` pins the version the run installs",
			want: nil,
		},
		{
			name: "two citations and prose between them",
			cell: "Yes — E1, `internal/agent/verify_bundles_test.go:291` (empty bytes refused) and " +
				"`cmd/strata-agent/s3_bundle_contract_test.go:22` (the shipped fetcher)",
			want: nil,
		},
		// The two cells below are why the guard is keyed on Discharged rather
		// than applied to every cell: No and Partially rows carry no discharge
		// to justify. Partially is a deliberate, measured exclusion (#142), not
		// an oversight — see checkDischargeCitation's doc comment.
		{
			name: "No needs no citation",
			cell: "No",
			want: nil,
		},
		{
			name: "Partially is out of scope today even with nothing cited",
			cell: "Partially — validated out of band; the install still ignores the pin",
			want: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			discharged, rest, _ := strings.Cut(tc.cell, " ")
			err := checkDischargeCitation(discharged, rest, 0)
			switch {
			case tc.want == nil && err != nil:
				t.Errorf("cell %q was rejected: %v", tc.cell, err)
			case tc.want != nil && err == nil:
				t.Errorf("cell %q was accepted; want %v", tc.cell, tc.want)
			case tc.want != nil && !errors.Is(err, tc.want):
				t.Errorf("cell %q rejected for the wrong reason: got %v, want %v", tc.cell, err, tc.want)
			}
		})
	}
}

// TestDischargeGuardReportsItsLine checks the part of the error a reader acts
// on. A guard that rejects the document without saying where costs more than it
// saves, and the line number is off-by-one-able: parseRefutation counts from
// zero and the message must count from one.
func TestDischargeGuardReportsItsLine(t *testing.T) {
	err := checkDischargeCitation(DischargedYes, "— closed completed", 684)
	if err == nil {
		t.Fatal("want a rejection")
	}
	if !strings.Contains(err.Error(), "line 685") {
		t.Errorf("error does not name line 685 (0-based 684): %v", err)
	}
	if !strings.Contains(err.Error(), "closed completed") {
		t.Errorf("error does not quote the offending cell: %v", err)
	}
}

// TestPropertiesRegisterMeetsRule11 is the enforcement: it is what makes rule 11
// a check rather than a paragraph. The rule was written in an earlier session and
// nothing ran it against the rows already in the table, which is how two
// `Yes — closed completed` cells survived the change that forbade exactly that
// string (#137). This test is the arrow pointed the other way.
//
// It is deliberately the *weaker* of the two claims in this file: it says only
// that today's rows pass. TestDischargeReportRejectsTheRegisterAsItWas below is
// the one that shows the report can fire at all.
func TestPropertiesRegisterMeetsRule11(t *testing.T) {
	src, err := os.ReadFile(propertiesPath())
	if err != nil {
		t.Fatalf("read PROPERTIES.md: %v", err)
	}
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse PROPERTIES.md: %v", err)
	}

	// The premise, asserted before the absence of defects is read: a register with
	// no Yes rows would pass this test without exercising the check once, and so
	// would a register the parser had stopped reading partway through.
	yes := 0
	for _, r := range doc.Register() {
		if r.Discharged == DischargedYes {
			yes++
		}
	}
	if yes == 0 {
		t.Fatal("the register has no Discharged: Yes rows, so this test exercised nothing")
	}

	for _, def := range doc.DischargeDefects() {
		t.Errorf("PROPERTIES.md:%d: %s is marked %q but %v\n    cell: %s",
			def.Line, def.Tracking, DischargedYes, def.Err, def.Cell)
	}
	t.Logf("rule 11 checked %d Discharged: %s rows of %d", yes, DischargedYes, len(doc.Register()))
}

// TestDischargeReportRejectsTheRegisterAsItWas is the check with teeth. The two
// cells this report was built for are gone from the document — #141 replaced
// them — so a clean report on today's register is equally consistent with the
// report working and with DischargeDefects returning nil unconditionally. This
// reconstructs a register containing the T7 row as it read before #141 and
// requires exactly that row to be named.
//
// The fixture carries four rows for three separate reasons, none of which a
// one-row document could establish: the compliant Yes row must be absent from the
// report, the No row must be absent for a different reason, and there are **two**
// offending rows so that a report which stopped at the first is distinguishable
// from one that reports them all. The two offend differently, one per half of
// rule 11, so the sentinels are read off a single report.
func TestDischargeReportRejectsTheRegisterAsItWas(t *testing.T) {
	const head = "## 3. Propositions\n\n| # | P | Verdict | Basis | Status | Evidence |\n" +
		"|---|---|---|---|---|---|\n" +
		"| **T7** | the resolver expanded unattested formations | SOUND | none | REFUTED | e |\n" +
		"| **I1** | bundles are verified before use | SOUND | none | REFUTED | e |\n" +
		"| **P4** | stage 7 refuses profiles resolved offline | SOUND | none | REFUTED | e |\n" +
		"| **R1** | resolution is deterministic | SOUND | none | REFUTED | e |\n"
	const regHead = "\n## 4. Refutation register\n\n" +
		"| Proposition | Counterexample | Capability | Tracking | Discharged |\n" +
		"|---|---|---|---|---|\n"
	// The first offender is verbatim what T7/#49's Discharged cell read at
	// 6555716: no basis and no citation.
	const noBasis = "| T7 | expanded an unattested formation silently | H1 | #49 | Yes — closed completed |\n"
	// The second names a basis and still cites nothing, which is the near miss
	// either side of "closed completed" — a stronger-looking cell that offers the
	// reader no artifact to check.
	const noCitation = "| P4 | resolved offline against the shipped catalog | H1 | #54 | Yes — E1, confirmed by hand against the fixture registry |\n"
	const compliant = "| I1 | accepted an empty bundle | H2 | #59 | Yes — E1, `internal/agent/verify_bundles_test.go:291` |\n"
	const undischarged = "| R1 | ResolvedAt is wall-clock | H1 | #61 | No |\n"

	src := head + regHead + compliant + noBasis + undischarged + noCitation
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse the reconstructed register: %v", err)
	}
	if got := len(doc.Register()); got != 4 {
		t.Fatalf("the fixture parsed %d register rows, want 4 — it no longer represents the "+
			"case this test is about", got)
	}

	// Line numbers derived from the fixture rather than counted by hand, so
	// editing it cannot silently point an assertion at the wrong row.
	lineOf := func(row string) int {
		return strings.Count(src[:strings.Index(src, row)], "\n") + 1
	}
	want := []DischargeDefect{
		{Line: lineOf(noBasis), Tracking: "#49", Err: ErrDischargeNoBasis},
		{Line: lineOf(noCitation), Tracking: "#54", Err: ErrDischargeNoCitation},
	}

	defects := doc.DischargeDefects()
	if len(defects) != len(want) {
		t.Fatalf("want the %d offending rows reported and no others, got %d: %+v",
			len(want), len(defects), defects)
	}
	for i, w := range want {
		got := defects[i]
		if got.Tracking != w.Tracking {
			t.Errorf("defect %d names %q, want the offending row %s", i, got.Tracking, w.Tracking)
		}
		if !errors.Is(got.Err, w.Err) {
			t.Errorf("defect %d (%s) was reported for the wrong reason: %v", i, w.Tracking, got.Err)
		}
		if got.Line != w.Line {
			t.Errorf("defect %d (%s) points at line %d, want %d", i, w.Tracking, got.Line, w.Line)
		}
	}
	if !strings.Contains(defects[0].Cell, "closed completed") {
		t.Errorf("the report does not quote the offending cell: %q", defects[0].Cell)
	}
}

// TestDischargeCitationPatternIsNotSatisfiedByProse guards the regexp itself.
// A citation pattern loose enough to match ordinary prose would accept every
// cell, which is the failure mode where the guard reports green forever.
func TestDischargeCitationPatternIsNotSatisfiedByProse(t *testing.T) {
	for _, prose := range []string{
		"closed completed",
		"`the resolver` refuses it now",
		"confirmed against the shipped catalog",
		"`strata verify --packages` validates against PyPI out of band",
		"E1, no longer reachable",
	} {
		if dischargeCitation.MatchString(prose) {
			t.Errorf("the citation pattern matches prose %q, so it cannot distinguish a citation "+
				"from a claim", prose)
		}
	}
	// And the converse, so a pattern that matches nothing does not pass this
	// test by being useless.
	for _, cite := range []string{
		"`internal/agent/agent.go:310-323`",
		"`spec/docsnippets_test.go`",
		"`.github/workflows/ci.yml:90`",
		"`cmd/strata/formations/hpc-mpi@2026.03.yaml`",
	} {
		if !dischargeCitation.MatchString(cite) {
			t.Errorf("the citation pattern does not match %q, which is a citation the register uses", cite)
		}
	}
}

// TestDischargeBasisPatternCoversEveryBasis ties the guard's basis pattern to
// section 2's grid rather than to a hand-written list. Bases() is the parser's
// registry and basis_pair_test.go already holds it to the document's table in
// both directions, so iterating it means the guard accepts exactly the bases the
// document defines — including the three that have no legacy name, which a
// hand-written list of E-names would silently omit.
//
// Both spellings are checked because a cell may be authored either way: the
// parser normalises a Basis cell's tier, but a Discharged cell is prose and this
// guard reads it as written.
func TestDischargeBasisPatternCoversEveryBasis(t *testing.T) {
	for _, b := range Bases() {
		for _, spelling := range []string{b.Spelling(), b.Canonical()} {
			if !dischargeBasis.MatchString(spelling) {
				t.Errorf("the basis pattern does not match %q, a basis section 2 defines", spelling)
			}
		}
	}
	// Non-vacuity: the loop above is over a registry, so assert it was not empty
	// and that it did carry all seven — a Bases() returning one element would
	// pass every assertion above.
	if got := len(Bases()); got != 7 {
		t.Fatalf("Bases() returned %d bases, want the 7 of section 2; the loop above checked %d", got, got)
	}
	// A pattern matching any capital-E-digit would accept "E9"; any
	// slash-separated pair would accept "guessed/implementation"; and a bare
	// coverage is not a basis, since section 2 gives every coverage but asserted
	// a subject.
	for _, notABasis := range []string{"E4", "E9", "guessed/implementation", "chosen/guess", "exhaustive", "chosen"} {
		if dischargeBasis.MatchString(notABasis) {
			t.Errorf("the basis pattern matches %q, which section 2 does not define", notABasis)
		}
	}
}

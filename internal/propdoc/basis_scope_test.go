package propdoc

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// Tests for per-scope bases and the meet (#135).
//
// The defect being fixed is that a Basis cell held one basis for a proposition
// quantifying over several execution routes, and section 3 defined that basis as
// the strongest claimed — a **max**. A max over unevenly covered routes reports
// the best-covered route's strength as the proposition's, which overstates and
// does so silently, because nothing in the document says which routes the number
// came from.
//
// The replacement is a meet. Two things make it more than a sign flip:
//
//   - Coverage is totally ordered and Subject is not, so a meet exists only where
//     the covered scopes share a Subject. Where they do not there is no answer to
//     report and the set is reported instead.
//   - The reduction is over the scopes the cell *names*. A part of the domain no
//     entry names is not covered by the number, which is why an entry may declare
//     a scope uncovered and why that collapses the whole cell.

// maxByRank is what the old definition computed, and the tests below use it to
// assert their own fixtures are discriminating: a row whose meet equals its max
// cannot tell a meet implementation from a max one.
func maxByRank(kinds []BasisKind) BasisKind {
	out := kinds[0]
	for _, k := range kinds[1:] {
		if k.Rank() > out.Rank() {
			out = k
		}
	}
	return out
}

func TestReduceIsAMeetNotAMax(t *testing.T) {
	impl, model := SubjectImplementation, SubjectModel
	entry := func(tier, scope string) ScopedBasis {
		return ScopedBasis{Tier: tier, Scope: scope, Citation: "`x_test.go:1`"}
	}
	for _, tc := range []struct {
		name string
		in   []ScopedBasis
		// wantMeet is the pair the entries reduce to, spelled as Spelling().
		wantMeet   string
		wantSubj   Subject
		comparable bool
		ok         bool
		// coincides marks a row where the meet and the max are the same basis, so
		// the row is deliberately not discriminating and the assertion below that
		// they differ is skipped for it.
		coincides bool
	}{
		{
			name:       "one entry reduces to itself",
			in:         []ScopedBasis{entry("E1", "the only route")},
			wantMeet:   "chosen/implementation",
			wantSubj:   impl,
			comparable: true, ok: true, coincides: true,
		},
		{
			name:       "T1's shape: exhaustive on one route, chosen on the other",
			in:         []ScopedBasis{entry("E1", "strata run"), entry("exhaustive/implementation", "agent boot")},
			wantMeet:   "chosen/implementation",
			wantSubj:   impl,
			comparable: true, ok: true,
		},
		{
			name: "the same two the other way round, because a meet is not the last entry either",
			in: []ScopedBasis{
				entry("exhaustive/implementation", "agent boot"), entry("E1", "strata run"),
			},
			wantMeet:   "chosen/implementation",
			wantSubj:   impl,
			comparable: true, ok: true,
		},
		{
			name: "T5's shape: three routes, two of them at the same basis",
			in: []ScopedBasis{
				entry("E1", "strata run"), entry("E1", "verifier construction"),
				entry("exhaustive/implementation", "agent boot"),
			},
			wantMeet:   "chosen/implementation",
			wantSubj:   impl,
			comparable: true, ok: true,
		},
		{
			name: "sampled between the two, so the meet is not simply the first or the extreme",
			in: []ScopedBasis{
				entry("E2", "b"), entry("exhaustive/implementation", "a"), entry("E2", "c"),
			},
			wantMeet:   "sampled/implementation",
			wantSubj:   impl,
			comparable: true, ok: true,
		},
		{
			name:       "asserted is the bottom of the coverage order, so it wins the meet",
			in:         []ScopedBasis{entry("E0", "documented only"), entry("exhaustive/implementation", "agent boot")},
			wantMeet:   "asserted",
			wantSubj:   SubjectNone,
			comparable: true, ok: true,
		},
		{
			name:       "a model scope and an implementation scope have no meet",
			in:         []ScopedBasis{entry("chosen/model", "the state machine"), entry("exhaustive/implementation", "agent boot")},
			comparable: false, ok: true,
		},
		{
			name:       "two model scopes do have one, since they share a Subject",
			in:         []ScopedBasis{entry("chosen/model", "a"), entry("E3", "b")},
			wantMeet:   "chosen/model",
			wantSubj:   model,
			comparable: true, ok: true,
		},
		{
			name: "an uncovered scope collapses the cell however well covered the rest is",
			in: []ScopedBasis{
				{Tier: TierNone, Scope: "the lockfile route"},
				entry("exhaustive/implementation", "agent boot"),
			},
			ok: false,
		},
		{
			name: "no entry names a basis at all",
			in:   []ScopedBasis{},
			ok:   false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			red, ok := Reduce(tc.in)
			if ok != tc.ok {
				t.Fatalf("Reduce ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if red.Comparable != tc.comparable {
				t.Fatalf("Comparable = %v, want %v (set %v)", red.Comparable, tc.comparable, red.Set)
			}
			if !tc.comparable {
				if len(red.Set) < 2 {
					t.Errorf("an incomparable reduction reported %d bases; the set is the whole answer "+
						"here and must name every basis claimed", len(red.Set))
				}
				return
			}
			if got := red.Meet.Spelling(); got != tc.wantMeet {
				t.Errorf("Meet = %q, want %q", got, tc.wantMeet)
			}
			if red.Meet.Subject != tc.wantSubj {
				t.Errorf("Meet subject = %q, want %q", red.Meet.Subject, tc.wantSubj)
			}

			// The row's own teeth. A Reduce that took the max would satisfy every
			// assertion above on any row where the two coincide, so each row states
			// which it is and the discriminating ones are checked to discriminate.
			kinds := make([]BasisKind, 0, len(tc.in))
			for _, e := range tc.in {
				if k, known := lookupBasis(e.Tier); known {
					kinds = append(kinds, k)
				}
			}
			max := maxByRank(kinds)
			switch {
			case tc.coincides && max != red.Meet:
				t.Errorf("row is marked as one where the meet and the max coincide, but they are %q and %q",
					red.Meet.Spelling(), max.Spelling())
			case !tc.coincides && max == red.Meet:
				t.Errorf("the meet and the max are both %q, so this row cannot tell a meet from a max; "+
					"either mark it coincides or give it entries that differ", max.Spelling())
			}
		})
	}
}

// TestReduceRefusesSection2sIncomparablePairs is the guard section 2 promised in
// advance and this change makes due.
//
// The ranking note there ends: "nothing in this repository consumes it. No tool
// sorts propositions by basis, so the list above is a note, and notes do not fire.
// The first time something does sort by basis … the list acquires the guard that
// makes it non-vacuous: every pair named here must be one the order actually
// ranks." Reduce is the first consumer. It consumes the Coverage order only, so
// each of the four pairs gets the disposition it actually has, and one of them is
// not expressible as a pair of bases at all — which is stated rather than skipped.
func TestReduceRefusesSection2sIncomparablePairs(t *testing.T) {
	cited := func(tier string, i int) ScopedBasis {
		return ScopedBasis{Tier: tier, Scope: fmt.Sprintf("scope %d", i), Citation: "`x_test.go:1`"}
	}
	for _, tc := range []struct {
		item int
		pair [2]string
		// wantRefused is whether Reduce declines to compare the pair.
		wantRefused bool
		// wantMeet is checked only where the pair is not refused.
		wantMeet string
		why      string
	}{
		{
			item: 1, pair: [2]string{"sampled/implementation", "exhaustive/model"}, wantRefused: true,
			why: "legacy E2 vs E3: sampling the code that ships against exhausting an abstraction of it",
		},
		{
			item: 2, pair: [2]string{"exhaustive/implementation", "exhaustive/model"}, wantRefused: true,
			why: "incomparable on domain size; the pair #129 could not hold in one number",
		},
		{
			item: 4, pair: [2]string{"chosen/model", "asserted"}, wantRefused: false, wantMeet: "asserted",
			why: "not refused, and the reason is narrow: asserted is the bottom of the *coverage* order, " +
				"so a floor over coverage is well defined. Section 2's complaint about this pair is about " +
				"usefulness — whether something that ran but was not about the code beats a bare assertion — " +
				"and Reduce does not compute a usefulness ranking and does not claim to.",
		},
	} {
		t.Run(fmt.Sprintf("item %d: %s vs %s", tc.item, tc.pair[0], tc.pair[1]), func(t *testing.T) {
			red, ok := Reduce([]ScopedBasis{cited(tc.pair[0], 1), cited(tc.pair[1], 2)})
			if !ok {
				t.Fatalf("Reduce declined to read the pair at all: %v", tc.why)
			}
			if refused := !red.Comparable; refused != tc.wantRefused {
				t.Fatalf("Reduce refused = %v, want %v — %s", refused, tc.wantRefused, tc.why)
			}
			if tc.wantRefused {
				// Non-vacuity for this row: a refusal must still name both bases, or
				// the caveat is enforced by discarding the information it is about.
				if len(red.Set) != 2 {
					t.Errorf("a refused pair reported %d bases, want both named", len(red.Set))
				}
				return
			}
			if got := red.Meet.Spelling(); got != tc.wantMeet {
				t.Errorf("Meet = %q, want %q — %s", got, tc.wantMeet, tc.why)
			}
		})
	}
	// Item 3 of the same list — "any two bases with different declared bounds" — is
	// deliberately absent from the table, and this is the record of why rather than
	// an omission. It is not a pair of bases: it says that comparing the bases of two
	// propositions whose bounds differ compares nothing, which is a statement about
	// two *cells* and has no encoding as two BasisKinds. Reduce's answer to it is
	// structural instead of a verdict — every entry carries its own scope, and the
	// reduction is a floor over the union of those scopes rather than a ranking of
	// one scope against another. TestParseBasisRejectsUnscopedMultiEntryCells is
	// where that structural answer is enforced.
	t.Log("section 2 ranking note item 3 is not expressible as a pair of bases; see the comment above")
}

func TestParseBasisScopedGrammar(t *testing.T) {
	for _, tc := range []struct {
		name string
		cell string
		want error // nil means the cell must parse
		// tier is checked on the cells that parse.
		tier   string
		scopes int
	}{
		{
			name:   "the single-entry grammar is untouched",
			cell:   "E1 — `cmd/strata/run_verify_test.go:466`",
			tier:   "E1",
			scopes: 0,
		},
		{
			name:   "a single entry may still name its bound",
			cell:   "E1 @ the `strata run` route — `cmd/strata/run_verify_test.go:466`",
			tier:   "E1",
			scopes: 1,
		},
		{
			name: "T1's cell reduces to the weaker route",
			cell: "chosen/implementation @ the `strata run` route — `cmd/strata/run_verify_test.go:370,519`; " +
				"exhaustive/implementation @ the agent boot route — `internal/agent/boot_matrix_test.go:464`",
			tier:   "E1",
			scopes: 2,
		},
		{
			name: "an @ inside a citation is a citation, not a bound",
			cell: "E1 — `cmd/strata/formations/hpc-mpi@2026.03.yaml`",
			tier: "E1",
		},
		{
			name: "scopes with no meet reduce to incomparable rather than to the better one",
			cell: "chosen/model @ the state machine — `m_test.go:1`; " +
				"exhaustive/implementation @ the agent boot route — `b_test.go:2`",
			tier:   TierIncomparable,
			scopes: 2,
		},
		{
			name: "an uncovered scope collapses the cell and cites nothing for itself",
			cell: "none @ the lockfile route; exhaustive/implementation @ the agent boot route — `b_test.go:2`",
			tier: TierNone, scopes: 2,
		},
		{
			name: "an unnamed scope in a multi-entry cell",
			cell: "E1 — `a_test.go:1`; exhaustive/implementation @ agent boot — `b_test.go:2`",
			want: ErrBasisScopeUnnamed,
		},
		{
			name: "the same scope claimed twice, which puts strongest-wins back on the reader",
			cell: "E1 @ agent boot — `a_test.go:1`; exhaustive/implementation @ agent boot — `b_test.go:2`",
			want: ErrBasisScopeRepeated,
		},
		{
			name: "an entry claiming a basis and citing nothing",
			cell: "E1 @ strata run — ; exhaustive/implementation @ agent boot — `b_test.go:2`",
			want: ErrBasisScopeUncited,
		},
		{
			name: "leading with the strongest scope, which is the overstatement this grammar removes",
			cell: "exhaustive/implementation @ agent boot — `b_test.go:2`; E1 @ strata run — `a_test.go:1`",
			want: ErrBasisNotWeakestFirst,
		},
		{
			name: "leading with a covered scope while another is declared uncovered",
			cell: "exhaustive/implementation @ agent boot — `b_test.go:2`; none @ the lockfile route",
			want: ErrBasisNotWeakestFirst,
		},
		{
			name: "withdrawn is a fact about a proposition, not about one of its scopes",
			cell: "withdrawn @ agent boot — I6; E1 @ strata run — `a_test.go:1`",
			want: ErrBasisWithdrawnScoped,
		},
		{
			name: "a scope opened and never closed",
			cell: "E1 @ the agent boot route `b_test.go:2`",
			want: ErrBasisScopeUnterminated,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := parseBasis(tc.cell, 0)
			if tc.want != nil {
				switch {
				case err == nil:
					t.Fatalf("cell parsed to %+v; want %v", b, tc.want)
				case !errors.Is(err, tc.want):
					t.Fatalf("cell rejected for the wrong reason: got %v, want %v", err, tc.want)
				}
				return
			}
			if err != nil {
				t.Fatalf("cell was rejected: %v", err)
			}
			if b.Tier != tc.tier {
				t.Errorf("Tier = %q, want %q", b.Tier, tc.tier)
			}
			if len(b.Scoped) != tc.scopes {
				t.Errorf("len(Scoped) = %d, want %d", len(b.Scoped), tc.scopes)
			}
		})
	}
}

// TestParseBasisRejectsUnscopedMultiEntryCells is the structural answer to section
// 2's ranking-note item 3, asserted on its own rather than as one row of a table:
// a cell claiming several bases cannot decline to say which domain each covers.
// Without this the reduction would be a floor over an unstated union, which is the
// same defect one level down from the one #135 was filed about.
func TestParseBasisRejectsUnscopedMultiEntryCells(t *testing.T) {
	for _, cell := range []string{
		"E1 — `a_test.go:1`; exhaustive/implementation — `b_test.go:2`",
		"E1 @ strata run — `a_test.go:1`; exhaustive/implementation — `b_test.go:2`",
		"E1 — `a_test.go:1`; exhaustive/implementation @ agent boot — `b_test.go:2`",
	} {
		if _, err := parseBasis(cell, 0); !errors.Is(err, ErrBasisScopeUnnamed) {
			t.Errorf("cell %q: got %v, want %v", cell, err, ErrBasisScopeUnnamed)
		}
	}
}

func TestDeriveStatusRendersTheReduction(t *testing.T) {
	scoped := func(tiers ...string) []ScopedBasis {
		out := make([]ScopedBasis, 0, len(tiers))
		for i, tier := range tiers {
			out = append(out, ScopedBasis{
				Tier: tier, Scope: fmt.Sprintf("scope %d", i), Citation: "`x_test.go:1`",
			})
		}
		return out
	}
	for _, tc := range []struct {
		name     string
		basis    Basis
		want     string
		wantKind string
	}{
		{
			name:     "a single-scope cell renders as it always did",
			basis:    Basis{Tier: "E1", Citation: "`x_test.go:1`"},
			want:     "ENFORCED E1",
			wantKind: "ENFORCED E1",
		},
		{
			name: "a multi-scope cell says what its basis is the weakest of",
			basis: Basis{Tier: "E1", Citation: "`x_test.go:1`",
				Scoped: scoped("E1", "exhaustive/implementation")},
			want: "ENFORCED E1 (weakest of 2 scopes)",
			// The count is parenthesised detail, so the distribution the document
			// quotes does not grow a bucket for it.
			wantKind: "ENFORCED E1",
		},
		{
			name: "an incomparable cell names the set rather than choosing from it",
			basis: Basis{Tier: TierIncomparable, Citation: "`x_test.go:1`",
				Scoped: scoped("chosen/model", "exhaustive/implementation")},
			want:     "ENFORCED (no meet over 2 scopes: chosen/model, exhaustive/implementation)",
			wantKind: "ENFORCED",
		},
		{
			name: "a multi-scope asserted cell keeps the ASSERTED class",
			basis: Basis{Tier: "E0", Citation: "`p.go:1`",
				Scoped: scoped("E0", "exhaustive/implementation")},
			want:     "ASSERTED (E0, weakest of 2 scopes)",
			wantKind: "ASSERTED (E0)",
		},
		{
			name: "an uncovered scope reports why it is unpopulated",
			basis: Basis{Tier: TierNone, Citation: "`x_test.go:1`",
				Scoped: scoped(TierNone, "exhaustive/implementation")},
			want:     "UNPOPULATED (a scope of 2 is uncovered)",
			wantKind: "UNPOPULATED",
		},
		{
			name: "a live refutation still outranks every basis, however many scopes",
			basis: Basis{Tier: "E1", Citation: "`x_test.go:1`",
				Scoped: scoped("E1", "exhaustive/implementation")},
			want:     "REFUTED",
			wantKind: "REFUTED",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			live, total := 0, 0
			if tc.want == "REFUTED" {
				live, total = 1, 1
			}
			got := DeriveStatus(tc.basis, live, total)
			if got != tc.want {
				t.Errorf("DeriveStatus = %q, want %q", got, tc.want)
			}
			if k := Kind(got); k != tc.wantKind {
				t.Errorf("Kind(%q) = %q, want %q", got, k, tc.wantKind)
			}
		})
	}
}

// TestPropertiesScopedCellsReduceToTheirWeakestRoute is the corpus check, and it
// is deliberately about the parsed reduction rather than the Status column.
//
// Neither scoped proposition's Status shows its basis: both are REFUTED, and a live
// refutation outranks any basis (proof-standard rule 5). So this change moves no
// Status cell and the generated column is byte-identical before and after it —
// which means the document's own green says nothing at all about whether the
// reduction is right. This test reads the reduction where it exists.
func TestPropertiesScopedCellsReduceToTheirWeakestRoute(t *testing.T) {
	src, err := os.ReadFile(propertiesPath())
	if err != nil {
		t.Fatalf("read PROPERTIES.md: %v", err)
	}
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse PROPERTIES.md: %v", err)
	}

	want := map[string]struct {
		tier   string
		scopes int
		// wasMax is what the old definition reported for this cell, and asserting
		// it differs from the reduction is what makes the row evidence that the
		// change moved a number rather than renamed one.
		wasMax string
	}{
		"T1": {tier: "E1", scopes: 2, wasMax: "exhaustive/implementation"},
		"T5": {tier: "E1", scopes: 3, wasMax: "exhaustive/implementation"},
	}

	scoped := map[string]*Proposition{}
	single := 0
	for _, p := range doc.Propositions() {
		if len(p.Basis.Scoped) > 1 {
			scoped[p.ID] = p
			continue
		}
		single++
	}
	if len(scoped) != len(want) {
		t.Fatalf("the document has %d multi-scope Basis cells, want %d (%v) — if a cell was added, "+
			"add it here with the basis it reduces to", len(scoped), len(want), keysOf(scoped))
	}
	// Cardinality stated, so a parser that silently stopped reading section 3 cannot
	// pass by finding both scoped rows and nothing else.
	if got, wantTotal := single+len(scoped), len(doc.Propositions()); got != wantTotal {
		t.Fatalf("counted %d propositions, want %d", got, wantTotal)
	}
	t.Logf("%d propositions: %d single-scope cells, %d multi-scope", len(doc.Propositions()), single, len(scoped))

	for id, w := range want {
		p, ok := scoped[id]
		if !ok {
			t.Errorf("%s is no longer a multi-scope cell", id)
			continue
		}
		if p.Basis.Tier != w.tier {
			t.Errorf("%s reduces to %q, want %q", id, p.Basis.Tier, w.tier)
		}
		if len(p.Basis.Scoped) != w.scopes {
			t.Errorf("%s names %d scopes, want %d", id, len(p.Basis.Scoped), w.scopes)
		}
		red, ok := Reduce(p.Basis.Scoped)
		if !ok || !red.Comparable {
			t.Errorf("%s does not reduce: ok=%v comparable=%v", id, ok, red.Comparable)
			continue
		}
		// The teeth on the live document: the reduction must differ from the max the
		// old definition took, or this row is consistent with nothing having changed.
		kinds := make([]BasisKind, 0, len(p.Basis.Scoped))
		for _, e := range p.Basis.Scoped {
			if k, known := lookupBasis(e.Tier); known {
				kinds = append(kinds, k)
			}
		}
		max := maxByRank(kinds)
		if max.Spelling() != w.wasMax {
			t.Errorf("%s: the strongest scope is %q, want %q — the row records what the old "+
				"definition reported and that no longer matches the cell", id, max.Spelling(), w.wasMax)
		}
		if max == red.Meet {
			t.Errorf("%s: meet and max are both %q, so this row does not witness the correction",
				id, max.Spelling())
		}
		// And the bound, recorded where the green is read: the rendering path that
		// prints the reduction is not reached by this document.
		if live, _ := doc.counts(id); live == 0 {
			t.Errorf("%s is no longer refuted, so its Status now renders the reduction; "+
				"this test's premise that the corpus cannot exercise that path has expired", id)
		}
	}
	t.Log("neither scoped proposition's Status shows its basis (both REFUTED), so the multi-scope " +
		"renderings are covered by TestDeriveStatusRendersTheReduction and not by this document")
}

func keysOf(m map[string]*Proposition) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestScopedGrammarRejectsTheCellsAsTheyWere records that the migration was
// forced rather than chosen. Both cells carried their second route in prose, and
// the reader had to notice a semicolon and a sentence to learn that the leading
// `exhaustive/implementation` covered one route of several. Under the new grammar
// those cells do not parse, so no document can be half-migrated.
func TestScopedGrammarRejectsTheCellsAsTheyWere(t *testing.T) {
	for _, cell := range []string{
		// Verbatim from PROPERTIES.md at 041e50f.
		"exhaustive/implementation — `internal/agent/boot_matrix_test.go:464`, **the agent boot route only**; " +
			"the `strata run` route stands at chosen/implementation, `cmd/strata/run_verify_test.go:370,519`",
		"exhaustive/implementation — `internal/agent/boot_matrix_test.go:464`, **the agent boot route only**; " +
			"the `strata run` and verifier-construction routes stand at chosen/implementation, " +
			"`cmd/strata/run_verify_test.go:264`, `cmd/strata-agent/cosign_verifier_test.go:127,187,249`",
	} {
		_, err := parseBasis(cell, 0)
		if err == nil {
			t.Errorf("the pre-migration cell still parses, so a cell can claim a strongest basis "+
				"over unstated routes: %q", cell)
			continue
		}
		if !strings.Contains(err.Error(), "unknown evidence tier") {
			t.Logf("rejected for a different reason than the tier of its second entry: %v", err)
		}
	}
}

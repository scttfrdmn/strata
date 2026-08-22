// Package propdoc derives the Status column of PROPERTIES.md from that
// document's own refutation register, so that one fact has one source.
//
// PROPERTIES.md states propositions in section 3 and refutations in section 4.
// Status is a function of the two — see DeriveStatus — and the function is
// written down here rather than applied by hand. Maintaining status beside the
// register is what this package exists to prevent: two sources of truth for one
// fact drift apart, and the drift is invisible because both sources look
// authoritative. Discharging one of a proposition's two counterexamples used to
// leave its Status cell untouched, which made a real movement unreportable.
//
// The authored inputs are the Basis cell (one or more scoped bases, each with its
// citation) and the register rows. Everything in the Status column is output.
//
// A Basis cell claiming several scopes reduces to the **weakest** of them, not the
// strongest — see Reduce. A proposition quantifying over several execution routes
// is rarely covered equally on all of them, and reporting the best-covered route's
// strength as the proposition's overstates silently (#135).
package propdoc

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Tier values accepted in a proposition's Basis cell, in addition to the seven
// bases defined in section 2 of the document — see Bases, and note that a basis
// is a (Coverage, Subject) pair rather than a rung on a ladder.
const (
	// TierNone means no evidence is cited for the proposition.
	TierNone = "none"
	// TierWithdrawn means the proposition has been superseded and is no longer
	// measured; the citation names what replaced it.
	TierWithdrawn = "withdrawn"
	// TierIncomparable is the reduction of a multi-scope Basis cell whose scopes
	// do not share a Subject, so no meet exists. It is not a tier an author may
	// write: parseBasis derives it, and Reduce's Set names the bases it stands for.
	TierIncomparable = "incomparable"
)

// Discharge values accepted in a register row's Discharged cell.
const (
	DischargedYes       = "Yes"
	DischargedNo        = "No"
	DischargedPartially = "Partially"
)

// Basis is the authored half of a proposition's status: what the cell claims, and
// the citation carrying the claim.
//
// Where the cell claims one basis, Tier is it. Where the cell claims a basis per
// scope, Tier is what those scopes **reduce** to — the meet, per Reduce — and
// Scoped holds the entries the reduction was taken over. Reading Tier therefore
// cannot overstate, which reading "the strongest claimed" did (#135). "Strongest"
// was never a total order either: section 2 states the ranking convention and the
// pairs it ranks that the evidence does not.
type Basis struct {
	// Tier is the canonical spelling of one of the seven bases — a legacy name
	// E0 through E3, or pair notation such as "exhaustive/implementation" for a
	// pair that has none — or TierNone, TierWithdrawn or TierIncomparable. Pair
	// decodes it.
	Tier string
	// Citation is whatever the Basis cell says after the tier, or for a
	// multi-scope cell every entry's citation joined with "; ". For
	// TierWithdrawn it names the replacement propositions.
	Citation string
	// Scoped holds the cell's entries when it names any scope at all, in cell
	// order. Nil for a Basis built by hand and for a single-entry cell that names
	// no scope, whose Tier and Citation are the whole claim.
	//
	// len(Scoped) > 1 is what makes Tier a reduction rather than a transcription,
	// and DeriveStatus says so in the status it renders.
	Scoped []ScopedBasis
}

// Proposition is one row of a section 3 group table.
type Proposition struct {
	// ID is the proposition identifier, such as R1, I1' or T5.
	ID string
	// Verdict is the result of attacking the proposition's text.
	Verdict string
	// Basis is the authored evidence claim.
	Basis Basis
	// Written is the Status cell as it appears in the file.
	Written string

	lineIdx int
	cells   []string
	statusI int
}

// Refutation is one row of the section 4 register.
type Refutation struct {
	// Props are the proposition IDs the counterexample breaks.
	Props []string
	// Capability is the adversary capability used, or H1 for none.
	Capability string
	// Tracking names the artifact tracking the discharge.
	Tracking string
	// Discharged is DischargedYes, DischargedNo or DischargedPartially.
	Discharged string

	// evidence is the rest of the Discharged cell after that token: for a
	// discharged row, the basis and citation rule 11 requires. It is kept
	// unexported because DischargeDefects is the only reader, and a second
	// exported spelling of the cell would be a second thing to keep in step.
	evidence string

	lineIdx int
}

// Live reports whether the refutation is still standing. Partially discharged
// counts as live: a counterexample half-answered still refutes.
func (r Refutation) Live() bool { return r.Discharged != DischargedYes }

// Rejection reasons for a Discharged: Yes cell, exported so a test can assert
// which check fired rather than that some check did. A rejection from the wrong
// reason is not evidence for the right one.
var (
	// ErrDischargeNoBasis is returned when a Yes cell names no basis, so the
	// reader cannot tell whether the evidence meets rule 11's floor.
	ErrDischargeNoBasis = errors.New("propdoc: a Discharged: Yes cell must name the basis of its evidence")
	// ErrDischargeNoCitation is returned when a Yes cell cites no artifact —
	// the case "Yes — closed completed" is, where the only thing offered is a
	// fact about the tracker.
	ErrDischargeNoCitation = errors.New("propdoc: a Discharged: Yes cell must cite the evidence, not the closure")
)

// Rejection reasons for a multi-scope Basis cell, exported for the same reason as
// the discharge sentinels above: a test asserts which authoring mistake was
// caught, not merely that one was.
//
// All but ErrBasisScopeUnterminated apply only to a cell claiming more than one
// scope, that one being a malformed entry wherever it appears. A single-entry cell
// naming no scope parses exactly as it did before per-scope bases existed, which is
// why the twelve such cells in the document needed no edit (#135).
var (
	// ErrBasisScopeUnnamed is returned when a multi-scope cell leaves an entry's
	// bound unnamed. A set of bases over unnamed scopes says nothing: the pair
	// says *how* a domain was covered and only the bound says *which* domain
	// (§2.1 rules 2 and 10).
	ErrBasisScopeUnnamed = errors.New("propdoc: every entry of a multi-scope Basis cell must name its scope with @")
	// ErrBasisScopeRepeated is returned when two entries name the same scope.
	// Two bases for one scope puts the choice between them back on the reader,
	// and the strongest-wins reading is the one this grammar exists to remove.
	ErrBasisScopeRepeated = errors.New("propdoc: two entries of a Basis cell name the same scope; merge them")
	// ErrBasisScopeUncited is returned when an entry of a multi-scope cell cites
	// nothing. A basis with no citation is not a basis, per section 3, and
	// per-scope bases mean per-scope evidence.
	ErrBasisScopeUncited = errors.New("propdoc: every entry of a multi-scope Basis cell must cite its evidence")
	// ErrBasisNotWeakestFirst is returned when a multi-scope cell does not lead
	// with the entry it reduces to. The reduction is not written in the cell — it
	// is derived — so the only thing a reader's eye lands on is the first entry,
	// and a cell leading with its strongest scope reproduces the overstatement
	// even though every derived number is right.
	ErrBasisNotWeakestFirst = errors.New("propdoc: a multi-scope Basis cell must lead with the scope it reduces to")
	// ErrBasisWithdrawnScoped is returned when withdrawn appears as one entry of
	// a multi-scope cell. Withdrawal is a fact about the whole proposition — it
	// is no longer measured — so it cannot be true of one scope and not another.
	ErrBasisWithdrawnScoped = errors.New("propdoc: withdrawn describes a whole proposition and cannot be one scope of a Basis cell")
	// ErrBasisScopeUnterminated is returned when an entry opens a scope with @ and
	// never closes it with an em dash, so the citation cannot be told from the
	// bound.
	ErrBasisScopeUnterminated = errors.New("propdoc: a Basis entry opening a scope with @ must close it with an em dash before its citation")
)

// dischargeCitation matches a backtick-quoted span naming a file, optionally
// with line numbers. The extension list is wider than the current corpus needs
// (every citation today is a .go path) so that citing a workflow, a recipe or a
// document is not a rejection.
var dischargeCitation = regexp.MustCompile("`[^`]*[\\w./-]+\\.(?:go|md|ya?ml|sh|json)(?::[\\d,-]+)?[^`]*`")

// dischargeBasis matches any of the seven bases of section 2, in either
// spelling. Legacy E0-E3 name four of them; the other three have pair
// spellings only.
var dischargeBasis = regexp.MustCompile(`\b(?:E[0-3]|asserted|(?:chosen|sampled|exhaustive)/(?:model|implementation))\b`)

// checkDischargeCitation is the per-cell predicate behind DischargeDefects: it
// applies section 2.1 rule 11 to one Discharged cell. A row may be marked Yes
// only on re-derived evidence, and it cites that evidence rather than the
// closure. Rule 11 makes two demands — the evidence is at coverage `chosen` or
// stronger with subject `implementation`, and the row cites it — so the cell has
// to say both which basis and which artifact, and each missing half gets its own
// error.
//
// # What this cannot catch
//
// A cell can name a basis, cite a real path, track a genuinely closed issue, and
// still be false, because the closed issue can be about a *different claim* than
// the counterexample beside it. Two instances are known — P4/#54 (the row was
// about the embedded catalog, #54 fixed file:// scheme dispatch) and T7/#48 (the
// row says `packages:` entries are unattested, #48 added a not-installed
// warning). Every component of such a row is true. Nothing here, and no parser,
// can see it: finding it means reading the tracked issue and comparing subjects.
// This function's green is therefore not evidence about that class.
//
// Scope, measured on 1a7646f: the check runs on Yes only. Of the three
// Partially cells, one (I6/#51) names neither a basis nor a path for its
// discharged half and would be reported; that row needs a re-derivation, and a
// Discharged change travels alone, so it is #142 rather than a change here.
func checkDischargeCitation(discharged, rest string, lineIdx int) error {
	if discharged != DischargedYes {
		return nil
	}
	if !dischargeBasis.MatchString(rest) {
		return fmt.Errorf("propdoc: line %d: Discharged cell is %q with no basis named: %w",
			lineIdx+1, strings.TrimSpace(discharged+" "+rest), ErrDischargeNoBasis)
	}
	if !dischargeCitation.MatchString(rest) {
		return fmt.Errorf("propdoc: line %d: Discharged cell is %q with no artifact cited: %w",
			lineIdx+1, strings.TrimSpace(discharged+" "+rest), ErrDischargeNoCitation)
	}
	return nil
}

// Doc is a parsed PROPERTIES.md.
type Doc struct {
	lines []string
	props []*Proposition
	reg   []Refutation
}

// Propositions returns the parsed section 3 rows in document order.
func (d *Doc) Propositions() []*Proposition { return d.props }

// Register returns the parsed section 4 rows in document order.
func (d *Doc) Register() []Refutation { return d.reg }

// DeriveStatus returns the status a proposition must carry, given its authored
// basis and its register rows: live is the number of rows not discharged, total
// the number of rows naming it at all.
//
// The definition, in the order the tests apply it:
//
//   - A withdrawn proposition is not measured.
//   - Any live refutation makes it REFUTED, whatever else is cited for it.
//     This is proof-standard rule 5: a refutation outranks any tier. The count
//     is reported when there is more than one row, so that discharging one of
//     several counterexamples is visible in this column rather than only in
//     prose.
//   - Otherwise the authored tier decides, and a tier with no citation is not
//     a tier.
//
// Where the Basis cell claims more than one scope, the tier that decides is the
// **meet** over those scopes and the rendered status says how many scopes it is the
// weakest of, so that a number derived from unevenly covered routes cannot be read
// as a claim about the whole domain (#135). Where the scopes do not share a Subject
// there is no meet, and the status names the set instead of picking from it.
//
// The function is total: every input yields a status, so no proposition can be
// left without one.
func DeriveStatus(b Basis, live, total int) string {
	if b.Tier == TierWithdrawn {
		if b.Citation == "" {
			return "WITHDRAWN"
		}
		return "WITHDRAWN (superseded by " + b.Citation + ")"
	}
	if live > 0 {
		if total > 1 {
			return fmt.Sprintf("REFUTED (%d of %d live)", live, total)
		}
		return "REFUTED"
	}
	if b.Tier == TierNone || b.Tier == "" || b.Citation == "" {
		if n := len(b.Scoped); n > 1 && b.Tier == TierNone {
			return fmt.Sprintf("UNPOPULATED (a scope of %d is uncovered)", n)
		}
		return "UNPOPULATED"
	}
	// A cell claiming several scopes reports what its tier is the weakest of.
	// Parenthesised so that Kind tallies it with the single-scope statuses: this
	// is detail about the claim, not a different class of claim.
	scopes := ""
	if n := len(b.Scoped); n > 1 {
		scopes = fmt.Sprintf(", weakest of %d scopes", n)
	}
	if b.Tier == TierIncomparable {
		red, _ := Reduce(b.Scoped)
		names := make([]string, 0, len(red.Set))
		for _, k := range red.Set {
			names = append(names, k.Spelling())
		}
		return fmt.Sprintf("ENFORCED (no meet over %d scopes: %s)", len(b.Scoped), strings.Join(names, ", "))
	}
	// Asserted coverage enforces nothing, so it renders as what it is. Naming the
	// basis in parentheses rather than hard-coding "E0" keeps the rendering keyed
	// to the coverage; today asserted is the one basis whose pair has no subject
	// and therefore exactly one spelling, so this can only print "ASSERTED (E0)".
	if k, ok := lookupBasis(b.Tier); ok {
		if k.Coverage == CoverageAsserted {
			return "ASSERTED (" + k.Canonical() + scopes + ")"
		}
		if scopes != "" {
			return "ENFORCED " + k.Canonical() + " (" + strings.TrimPrefix(scopes, ", ") + ")"
		}
		return "ENFORCED " + k.Canonical()
	}
	return "ENFORCED " + b.Tier
}

// Statuses returns the derived status of every proposition, keyed by ID.
func (d *Doc) Statuses() map[string]string {
	out := make(map[string]string, len(d.props))
	for _, p := range d.props {
		live, total := d.counts(p.ID)
		out[p.ID] = DeriveStatus(p.Basis, live, total)
	}
	return out
}

// counts returns the number of live and total register rows naming id.
func (d *Doc) counts(id string) (live, total int) {
	for _, r := range d.reg {
		for _, p := range r.Props {
			if p != id {
				continue
			}
			total++
			if r.Live() {
				live++
			}
			break
		}
	}
	return live, total
}

// Distribution returns the number of propositions carrying each derived status,
// with the count stripped from REFUTED so that "REFUTED (1 of 2 live)" and
// "REFUTED" are tallied together. The document quotes this distribution, and a
// quoted number that nothing derives is a number that drifts.
func (d *Doc) Distribution() map[string]int {
	out := map[string]int{}
	for _, status := range d.Statuses() {
		out[Kind(status)]++
	}
	return out
}

// Kind reduces a status to the class it belongs to, discarding any parenthesised
// detail: "REFUTED (3 of 4 live)" and "WITHDRAWN (superseded by I6)" become
// "REFUTED" and "WITHDRAWN". "ASSERTED (E0)" is left whole, because the tier is
// the point of that status rather than detail about it.
func Kind(status string) string {
	if strings.HasPrefix(status, "ASSERTED") {
		return "ASSERTED (E0)"
	}
	if i := strings.Index(status, " ("); i >= 0 {
		return status[:i]
	}
	return status
}

// Drift describes one proposition whose written status disagrees with the
// status derived from the register.
type Drift struct {
	// ID is the proposition identifier.
	ID string
	// Line is the 1-indexed line of the proposition's row.
	Line int
	// Written is the status in the file; Derived is what the register implies.
	Written, Derived string
}

// Drifts returns every proposition whose Status cell is not what the register
// implies, in document order.
func (d *Doc) Drifts() []Drift {
	statuses := d.Statuses()
	var out []Drift
	for _, p := range d.props {
		if got := statuses[p.ID]; got != p.Written {
			out = append(out, Drift{ID: p.ID, Line: p.lineIdx + 1, Written: p.Written, Derived: got})
		}
	}
	return out
}

// Render returns the document with every Status cell set to its derived value.
// Nothing else is altered: cells are rejoined as they were parsed, so a
// document with no drift renders byte-identically to its input.
func (d *Doc) Render() []byte {
	lines := make([]string, len(d.lines))
	copy(lines, d.lines)
	statuses := d.Statuses()
	for _, p := range d.props {
		cells := make([]string, len(p.cells))
		copy(cells, p.cells)
		cells[p.statusI] = " " + statuses[p.ID] + " "
		lines[p.lineIdx] = strings.Join(cells, "|")
	}
	return []byte(strings.Join(lines, "\n"))
}

// Unknown returns register rows naming a proposition that section 3 does not
// state, which is a typo or a deleted proposition rather than a refutation.
func (d *Doc) Unknown() []string {
	known := make(map[string]bool, len(d.props))
	for _, p := range d.props {
		known[p.ID] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, r := range d.reg {
		for _, p := range r.Props {
			if !known[p] && !seen[p] {
				seen[p] = true
				out = append(out, fmt.Sprintf("%s (register line %d)", p, r.lineIdx+1))
			}
		}
	}
	sort.Strings(out)
	return out
}

// DischargeDefect describes a register row marked Discharged: Yes whose cell does
// not meet section 2.1 rule 11.
type DischargeDefect struct {
	// Line is the 1-indexed line of the register row.
	Line int
	// Tracking is the row's Tracking cell, so the reader can go to the artifact.
	Tracking string
	// Cell is the Discharged cell as written.
	Cell string
	// Err wraps ErrDischargeNoBasis or ErrDischargeNoCitation, so a caller can
	// tell which of rule 11's two demands the row fails.
	Err error
}

// DischargeDefects returns every register row marked Discharged: Yes that names
// no basis or cites no artifact, in document order — the "closed completed" case
// rule 11 was written against, and the near misses either side of it.
//
// This is a report rather than a parse error, and the distinction is the whole
// design. Rule 11 is a policy about what a row may claim, not a statement about
// whether the row can be read; making a violation a parse failure would mean a
// document with one unjustified discharge could not be read *at all* — not by
// propgen, which is the tool that would regenerate its Status column, and not by
// a test constructing a hypothetical register to prove some other check can fail.
// Unknown has the same shape for the same reason. The enforcement lives where it
// belongs, in a CI test asserting this slice is empty, so a rule-11 violation
// fails the build with a message naming the row rather than aborting every reader
// of the document.
//
// The predicate is checkDischargeCitation, whose doc comment states what it
// cannot see.
func (d *Doc) DischargeDefects() []DischargeDefect {
	var out []DischargeDefect
	for _, r := range d.reg {
		if err := checkDischargeCitation(r.Discharged, r.evidence, r.lineIdx); err != nil {
			out = append(out, DischargeDefect{
				Line:     r.lineIdx + 1,
				Tracking: r.Tracking,
				Cell:     strings.TrimSpace(r.Discharged + " " + r.evidence),
				Err:      err,
			})
		}
	}
	return out
}

// section identifiers used while scanning.
type section int

const (
	sectionOther section = iota
	sectionProps
	sectionRegister
)

// Parse reads a PROPERTIES.md and returns its propositions and register.
// Malformed rows are errors rather than skipped rows: a row this package cannot
// read is a row whose status nothing would check.
func Parse(src []byte) (*Doc, error) {
	d := &Doc{lines: strings.Split(strings.TrimSuffix(string(src), "\n"), "\n")}
	cur := sectionOther
	for i, line := range d.lines {
		switch {
		case strings.HasPrefix(line, "## 3."):
			cur = sectionProps
			continue
		case strings.HasPrefix(line, "## 4."):
			cur = sectionRegister
			continue
		case strings.HasPrefix(line, "### Group "):
			cur = sectionProps
			continue
		// Any other subsection ends the tables: section 3.1 and 4.1 both hold
		// tables of their own, and reading those as propositions or as register
		// rows is how a parser silently measures the wrong thing.
		case strings.HasPrefix(line, "### "), strings.HasPrefix(line, "## "):
			cur = sectionOther
			continue
		}
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := splitCells(line)
		switch cur {
		case sectionProps:
			if !strings.HasPrefix(strings.TrimSpace(cells[1]), "**") {
				continue // Header or separator row.
			}
			p, err := parseProposition(cells, i)
			if err != nil {
				return nil, err
			}
			d.props = append(d.props, p)
		case sectionRegister:
			head := strings.TrimSpace(cells[1])
			if head == "Proposition" || strings.HasPrefix(head, "---") || head == "" {
				continue
			}
			r, err := parseRefutation(cells, i)
			if err != nil {
				return nil, err
			}
			d.reg = append(d.reg, r)
		case sectionOther:
		}
	}
	if len(d.props) == 0 {
		return nil, fmt.Errorf("propdoc: no propositions found in section 3")
	}
	// A repeated ID means either the document states one proposition twice or
	// the scan has wandered into a table that is not a group table. Both are
	// silent measurement errors, so neither is tolerated.
	seen := make(map[string]int, len(d.props))
	for _, p := range d.props {
		if prev, dup := seen[p.ID]; dup {
			return nil, fmt.Errorf("propdoc: proposition %s appears at lines %d and %d",
				p.ID, prev, p.lineIdx+1)
		}
		seen[p.ID] = p.lineIdx + 1
	}
	if len(d.reg) == 0 {
		return nil, fmt.Errorf("propdoc: no register rows found in section 4")
	}
	return d, nil
}

// propositionCells is the number of pipe-delimited fields in a section 3 row,
// counting the empty leading and trailing fields.
const propositionCells = 8

// registerCells is the same count for a section 4 row.
const registerCells = 7

func parseProposition(cells []string, lineIdx int) (*Proposition, error) {
	if len(cells) != propositionCells {
		return nil, fmt.Errorf("propdoc: line %d: proposition row has %d fields, want %d",
			lineIdx+1, len(cells), propositionCells)
	}
	basis, err := parseBasis(cells[4], lineIdx)
	if err != nil {
		return nil, err
	}
	return &Proposition{
		ID:      strings.Trim(strings.TrimSpace(cells[1]), "*"),
		Verdict: strings.Trim(strings.TrimSpace(cells[3]), "*"),
		Basis:   basis,
		Written: strings.TrimSpace(cells[5]),
		lineIdx: lineIdx,
		cells:   cells,
		statusI: 5,
	}, nil
}

// parseBasis reads a Basis cell: one or more entries separated by semicolons,
// each a tier token, an optional "@ scope" closed by an em dash, and a citation.
//
//	E1 — `x_test.go:1`
//	chosen/implementation @ the `strata run` route — `a_test.go:1`; exhaustive/implementation @ the agent boot route — `b_test.go:2`
//
// A cell with one entry and no scope is the whole of the old grammar, and parses
// to the same Basis it always did — Scoped stays nil. A cell with several entries
// reduces to their meet (Reduce), and the returned Tier is that reduction rather
// than any one entry's claim.
func parseBasis(cell string, lineIdx int) (Basis, error) {
	text := strings.TrimSpace(cell)
	if text == "" {
		return Basis{}, fmt.Errorf("propdoc: line %d: empty Basis cell", lineIdx+1)
	}
	parts := strings.Split(text, ";")
	entries := make([]ScopedBasis, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return Basis{}, fmt.Errorf("propdoc: line %d: Basis cell has an empty entry; "+
				"a semicolon separates two scoped bases", lineIdx+1)
		}
		e, err := parseBasisEntry(part, lineIdx)
		if err != nil {
			return Basis{}, err
		}
		entries = append(entries, e)
	}

	if len(entries) == 1 {
		e := entries[0]
		if e.Tier == TierNone && e.Citation != "" {
			return Basis{}, fmt.Errorf("propdoc: line %d: tier %q carries a citation %q; "+
				"cite a tier or claim none", lineIdx+1, TierNone, e.Citation)
		}
		b := Basis{Tier: e.Tier, Citation: e.Citation}
		if e.Scope != "" {
			b.Scoped = entries
		}
		return b, nil
	}
	return reduceCell(entries, lineIdx)
}

// parseBasisEntry reads one entry of a Basis cell. The scope is recognised only
// when @ is the first thing after the tier token, so a citation containing an @ —
// a formation filename, say — is not mistaken for a bound.
func parseBasisEntry(text string, lineIdx int) (ScopedBasis, error) {
	tier, rest, _ := strings.Cut(strings.TrimSpace(text), " ")
	switch tier {
	case TierNone, TierWithdrawn:
	default:
		// Either spelling is accepted and normalised to the canonical one, so a
		// cell written as "chosen/implementation" derives the same Status as one
		// written "E1" and the column stays uniform.
		canonical, err := parseTier(tier, lineIdx)
		if err != nil {
			return ScopedBasis{}, err
		}
		tier = canonical
	}
	rest = strings.TrimSpace(rest)
	var scope string
	if strings.HasPrefix(rest, "@") {
		bound, cited, ok := strings.Cut(rest[1:], "—")
		if !ok {
			// TierNone is exempt, and only TierNone: a scope declared uncovered has
			// nothing to cite, so there is nothing for the em dash to separate. Every
			// other tier claims evidence, and an entry that opens a bound and never
			// closes it leaves the bound and the citation indistinguishable.
			if tier != TierNone {
				return ScopedBasis{}, fmt.Errorf("propdoc: line %d: entry %q: %w",
					lineIdx+1, strings.TrimSpace(text), ErrBasisScopeUnterminated)
			}
			bound, cited = rest[1:], ""
		}
		scope, rest = strings.TrimSpace(bound), cited
		if scope == "" {
			return ScopedBasis{}, fmt.Errorf("propdoc: line %d: entry %q names an empty scope: %w",
				lineIdx+1, strings.TrimSpace(text), ErrBasisScopeUnnamed)
		}
	}
	citation := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(rest), "—:-"))
	return ScopedBasis{Tier: tier, Scope: scope, Citation: citation}, nil
}

// reduceCell applies the rules a multi-scope Basis cell must satisfy and returns
// the Basis its entries reduce to.
//
// The rules are not stylistic. Unnamed or repeated scopes make the reduction
// meaningless or ambiguous; an uncited entry contributes a tier on no evidence; and
// a cell not leading with its reduction leaves the overstatement in the one place a
// reader actually looks, since the reduction itself is derived and appears only in
// the generated Status column.
//
// TierNone is the one tier exempt from the citation requirement, and required to
// carry no citation: it declares a scope *uncovered*, and an uncovered scope has
// nothing to cite. That is the same rule the single-entry path applies to a bare
// "none" cell.
func reduceCell(entries []ScopedBasis, lineIdx int) (Basis, error) {
	seen := make(map[string]bool, len(entries))
	var citations []string
	for _, e := range entries {
		if e.Tier == TierWithdrawn {
			return Basis{}, fmt.Errorf("propdoc: line %d: %w", lineIdx+1, ErrBasisWithdrawnScoped)
		}
		if e.Scope == "" {
			return Basis{}, fmt.Errorf("propdoc: line %d: the entry claiming %q names no scope: %w",
				lineIdx+1, e.Tier, ErrBasisScopeUnnamed)
		}
		if seen[e.Scope] {
			return Basis{}, fmt.Errorf("propdoc: line %d: scope %q is claimed twice: %w",
				lineIdx+1, e.Scope, ErrBasisScopeRepeated)
		}
		seen[e.Scope] = true
		switch {
		case e.Tier == TierNone && e.Citation != "":
			return Basis{}, fmt.Errorf("propdoc: line %d: scope %q is declared uncovered and cites "+
				"%q; cite a tier or claim none", lineIdx+1, e.Scope, e.Citation)
		case e.Tier != TierNone && e.Citation == "":
			return Basis{}, fmt.Errorf("propdoc: line %d: scope %q claims %q and cites nothing: %w",
				lineIdx+1, e.Scope, e.Tier, ErrBasisScopeUncited)
		}
		if e.Citation != "" {
			citations = append(citations, e.Citation)
		}
	}

	// Every case below leads with the entry the cell reduces to, and the check is
	// the same check: an uncovered scope is the weakest thing a cell can say, a
	// meet is the weakest of what it claims, and an incomparable cell has no
	// weakest entry to lead with — so that case carries no ordering rule and
	// DeriveStatus prints the whole set instead.
	b := Basis{Citation: strings.Join(citations, "; "), Scoped: entries}
	red, ok := Reduce(entries)
	if !ok {
		if entries[0].Tier != TierNone {
			return Basis{}, fmt.Errorf("propdoc: line %d: the cell leads with %q while a scope is "+
				"declared uncovered: %w", lineIdx+1, entries[0].Tier, ErrBasisNotWeakestFirst)
		}
		b.Tier = TierNone
		return b, nil
	}
	if !red.Comparable {
		b.Tier = TierIncomparable
		return b, nil
	}
	if first, known := lookupBasis(entries[0].Tier); !known || first != red.Meet {
		return Basis{}, fmt.Errorf("propdoc: line %d: the cell leads with %q but reduces to %q: %w",
			lineIdx+1, entries[0].Tier, red.Meet.Canonical(), ErrBasisNotWeakestFirst)
	}
	b.Tier = red.Meet.Canonical()
	return b, nil
}

func parseRefutation(cells []string, lineIdx int) (Refutation, error) {
	if len(cells) != registerCells {
		return Refutation{}, fmt.Errorf("propdoc: line %d: register row has %d fields, want %d",
			lineIdx+1, len(cells), registerCells)
	}
	discharged, rest, _ := strings.Cut(strings.TrimSpace(cells[5]), " ")
	switch discharged {
	case DischargedYes, DischargedNo, DischargedPartially:
	default:
		return Refutation{}, fmt.Errorf("propdoc: line %d: Discharged is %q, want one of %s/%s/%s",
			lineIdx+1, discharged, DischargedYes, DischargedNo, DischargedPartially)
	}
	var props []string
	for _, p := range strings.Split(cells[1], ",") {
		id := strings.Trim(strings.TrimSpace(p), "`*")
		if id != "" {
			props = append(props, id)
		}
	}
	if len(props) == 0 {
		return Refutation{}, fmt.Errorf("propdoc: line %d: register row names no proposition", lineIdx+1)
	}
	return Refutation{
		Props:      props,
		Capability: strings.Trim(strings.TrimSpace(cells[3]), "`*"),
		Tracking:   strings.TrimSpace(cells[4]),
		Discharged: discharged,
		evidence:   rest,
		lineIdx:    lineIdx,
	}, nil
}

// splitCells splits a markdown table row on unescaped pipes. Evidence cells in
// this document contain shell pipelines written as "\|", and splitting on every
// pipe would mangle them.
func splitCells(line string) []string {
	var cells []string
	var cur strings.Builder
	escaped := false
	for _, r := range line {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\':
			cur.WriteRune(r)
			escaped = true
		case r == '|':
			cells = append(cells, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	return append(cells, cur.String())
}

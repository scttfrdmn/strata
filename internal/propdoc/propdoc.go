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
// The authored inputs are the Basis cell (the highest evidence tier claimed,
// with its citation) and the register rows. Everything in the Status column is
// output.
package propdoc

import (
	"fmt"
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
)

// Discharge values accepted in a register row's Discharged cell.
const (
	DischargedYes       = "Yes"
	DischargedNo        = "No"
	DischargedPartially = "Partially"
)

// Basis is the authored half of a proposition's status: the strongest basis
// claimed for it, and the citation carrying that claim. "Strongest" is not a
// total order — section 2 states the ranking convention and the pairs it ranks
// that the evidence does not.
type Basis struct {
	// Tier is the canonical spelling of one of the seven bases — a legacy name
	// E0 through E3, or pair notation such as "exhaustive/implementation" for a
	// pair that has none — or TierNone or TierWithdrawn. Pair decodes it.
	Tier string
	// Citation is whatever the Basis cell says after the tier. For
	// TierWithdrawn it names the replacement propositions.
	Citation string
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

	lineIdx int
}

// Live reports whether the refutation is still standing. Partially discharged
// counts as live: a counterexample half-answered still refutes.
func (r Refutation) Live() bool { return r.Discharged != DischargedYes }

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
		return "UNPOPULATED"
	}
	// Asserted coverage enforces nothing, so it renders as what it is. Naming the
	// basis in parentheses rather than hard-coding "E0" keeps the rendering keyed
	// to the coverage; today asserted is the one basis whose pair has no subject
	// and therefore exactly one spelling, so this can only print "ASSERTED (E0)".
	if k, ok := lookupBasis(b.Tier); ok {
		if k.Coverage == CoverageAsserted {
			return "ASSERTED (" + k.Canonical() + ")"
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

// parseBasis reads a Basis cell: a tier token, then an optional citation
// separated by an em dash, a colon or whitespace.
func parseBasis(cell string, lineIdx int) (Basis, error) {
	text := strings.TrimSpace(cell)
	if text == "" {
		return Basis{}, fmt.Errorf("propdoc: line %d: empty Basis cell", lineIdx+1)
	}
	tier, rest, _ := strings.Cut(text, " ")
	switch tier {
	case TierNone, TierWithdrawn:
	default:
		// Either spelling is accepted and normalised to the canonical one, so a
		// cell written as "chosen/implementation" derives the same Status as one
		// written "E1" and the column stays uniform.
		canonical, err := parseTier(tier, lineIdx)
		if err != nil {
			return Basis{}, err
		}
		tier = canonical
	}
	citation := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(rest), "—:-"))
	if tier == TierNone && citation != "" {
		return Basis{}, fmt.Errorf("propdoc: line %d: tier %q carries a citation %q; "+
			"cite a tier or claim none", lineIdx+1, TierNone, citation)
	}
	return Basis{Tier: tier, Citation: strings.TrimSpace(citation)}, nil
}

func parseRefutation(cells []string, lineIdx int) (Refutation, error) {
	if len(cells) != registerCells {
		return Refutation{}, fmt.Errorf("propdoc: line %d: register row has %d fields, want %d",
			lineIdx+1, len(cells), registerCells)
	}
	discharged, _, _ := strings.Cut(strings.TrimSpace(cells[5]), " ")
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

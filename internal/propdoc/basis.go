package propdoc

import (
	"fmt"
	"strings"
)

// Coverage is how much of a declared domain a proposition's evidence reached.
// Coverage is totally ordered — asserted, chosen, sampled, exhaustive — which is
// why "chosen or stronger" is a well-formed phrase about coverage alone.
type Coverage string

// The four coverage values defined in section 2 of the document.
const (
	// CoverageAsserted means nothing was executed: the property is stated in
	// documentation or follows from the structure of the code.
	CoverageAsserted Coverage = "asserted"
	// CoverageChosen means example tests exercised the property on inputs the
	// author picked, and the site the property is about ran.
	CoverageChosen Coverage = "chosen"
	// CoverageSampled means a generated run drew inputs from a declared domain.
	CoverageSampled Coverage = "sampled"
	// CoverageExhaustive means every member of a declared bounded domain ran.
	CoverageExhaustive Coverage = "exhaustive"
)

// Subject is what a proposition's evidence was about. Subject is not an
// ordering: a model result does not become an implementation result, with or
// without a faithfulness argument (section 2.1 rule 1). The pair of a coverage
// and a subject is therefore not totally ordered, which is the whole reason
// Basis records a pair rather than a rung.
type Subject string

// The subject values defined in section 2 of the document.
const (
	// SubjectNone is the subject of CoverageAsserted, which has none: where
	// nothing was executed there is no artifact the evidence was about.
	SubjectNone Subject = ""
	// SubjectModel means the evidence executed an abstraction of the code.
	SubjectModel Subject = "model"
	// SubjectImplementation means the evidence executed the code that ships.
	SubjectImplementation Subject = "implementation"
)

// BasisKind is one of the seven bases section 2 defines: a coverage, the subject
// it applies to, and the legacy E-name if that pair has one.
type BasisKind struct {
	Coverage Coverage
	Subject  Subject
	// Legacy is the E0-E3 name for this pair, or empty where the pair has none.
	// Three of the seven have no legacy name, which is the defect that the
	// one-dimensional ladder had and this pair does not: the strongest of the
	// three, exhaustive/implementation, had to be filed below a model check.
	Legacy string
}

// Spelling returns the pair notation for the kind: "coverage/subject", or just
// the coverage where the subject is SubjectNone.
func (k BasisKind) Spelling() string {
	if k.Subject == SubjectNone {
		return string(k.Coverage)
	}
	return string(k.Coverage) + "/" + string(k.Subject)
}

// Canonical returns the spelling a Basis cell normalises to: the legacy name
// where one exists, so that the Status column reads the same whichever spelling
// the author used, and the pair notation otherwise.
func (k BasisKind) Canonical() string {
	if k.Legacy != "" {
		return k.Legacy
	}
	return k.Spelling()
}

// Bases returns the seven bases, in the order section 2 tabulates them. It is a
// function rather than a package variable both because this package has no
// globals and because a caller cannot then mutate the registry the parser reads.
//
// The count is 1 + 3*2 rather than 4*2: CoverageAsserted takes no subject, since
// "asserted about the implementation" and "asserted about a model" distinguish
// nothing measurable when nothing was executed.
func Bases() []BasisKind {
	return []BasisKind{
		{CoverageAsserted, SubjectNone, "E0"},
		{CoverageChosen, SubjectModel, ""},
		{CoverageChosen, SubjectImplementation, "E1"},
		{CoverageSampled, SubjectModel, ""},
		{CoverageSampled, SubjectImplementation, "E2"},
		{CoverageExhaustive, SubjectModel, "E3"},
		{CoverageExhaustive, SubjectImplementation, ""},
	}
}

// Rank returns the position of a basis's coverage in the coverage order:
// asserted 0, chosen 1, sampled 2, exhaustive 3.
//
// This is the ordering on Coverage alone, which section 2 states is total and
// means it. It is *not* the seven-basis sorting convention, which ranks pairs the
// evidence does not — Reduce refuses to compare two bases differing in Subject
// rather than consulting a rank for them.
func (k BasisKind) Rank() int {
	switch k.Coverage {
	case CoverageAsserted:
		return 0
	case CoverageChosen:
		return 1
	case CoverageSampled:
		return 2
	case CoverageExhaustive:
		return 3
	}
	// An unrecognised coverage ranks at the bottom rather than panicking: a
	// hand-built BasisKind carrying a typo should weaken a meet, never strengthen
	// one, and Reduce's whole purpose is to not overstate.
	return 0
}

// ScopedBasis is one entry of a Basis cell: a basis, the part of the
// proposition's domain that basis covers, and the citation carrying it.
//
// A proposition quantifying over several execution routes is not covered equally
// on all of them, and one basis for the whole cell cannot say so — which is the
// defect this type exists to remove (#135).
type ScopedBasis struct {
	// Tier is the canonical spelling of one of the seven bases, or TierNone for a
	// scope declared uncovered.
	Tier string
	// Scope is the declared bound: which part of the proposition's domain this
	// entry covers. Empty only for a single-entry cell, whose scope is whatever
	// the proposition's own text quantifies over.
	Scope string
	// Citation is the evidence for this scope.
	Citation string
}

// Reduction is what a Basis cell's entries reduce to: the honest floor over the
// scopes the cell covers.
//
// The reduction is a **meet** (greatest lower bound), not a max. A max over
// unevenly covered scopes reports the best-covered scope's strength as the
// proposition's, which overstates silently and is what #135 was filed about. A
// meet reports the weakest, which is the most that holds everywhere the cell
// claims to cover.
//
// But a meet needs an order, and the seven bases do not have one: Coverage is
// totally ordered and Subject is not (§2.1 rule 1). So the meet exists only where
// all covered scopes share a Subject, and where they do not there is no single
// answer to report — hence Set, and hence Comparable.
type Reduction struct {
	// Meet is the greatest lower bound of the covered scopes' bases. Valid only
	// when Comparable; the zero BasisKind otherwise.
	Meet BasisKind
	// Set is every distinct basis the cell claims, in the order section 2
	// tabulates them. Always populated, and it is the whole of the answer when
	// Comparable is false.
	Set []BasisKind
	// Comparable reports whether a meet exists — that is, whether every covered
	// scope whose evidence executed something shares a Subject with every other.
	Comparable bool
	// Scopes is the union of the declared bounds, in cell order. It is part of the
	// claim, not decoration: the reduction is a floor over *these* scopes and says
	// nothing about a part of the domain no entry names (§2.1 rule 2 and rule 10).
	Scopes []string
}

// Reduce returns what a set of scoped bases reduces to, and whether any entry
// named a basis at all.
//
// asserted is the bottom of the coverage order and is treated as comparable with
// every other basis, because the incomparability of Subject arises from *what was
// executed* and asserted executed nothing: there is no model-versus-implementation
// disagreement to have. Note what this does and does not claim. It is a floor on
// **coverage**. It is not a claim that a `model` result outranks asserted in
// usefulness — section 2's ranking note says it does not, and this function does
// not consult that ranking.
//
// TierNone short-circuits the whole reduction: a scope declared uncovered means
// nothing is established over the union, however well covered the other scopes
// are. That is the mechanism by which a cell can state the bound honestly instead
// of leaving an uncovered route to be inferred from prose.
func Reduce(entries []ScopedBasis) (Reduction, bool) {
	var r Reduction
	var kinds []BasisKind
	uncovered := false
	for _, e := range entries {
		if e.Scope != "" {
			r.Scopes = append(r.Scopes, e.Scope)
		}
		if e.Tier == TierNone {
			uncovered = true
			continue
		}
		if k, ok := lookupBasis(e.Tier); ok {
			kinds = append(kinds, k)
		}
	}
	if uncovered || len(kinds) == 0 {
		return r, false
	}

	// Distinct, in section 2's order, so two cells claiming the same bases report
	// the same set whatever order they were authored in.
	for _, b := range Bases() {
		for _, k := range kinds {
			if k == b {
				r.Set = append(r.Set, b)
				break
			}
		}
	}

	// Subject agreement, checked over the entries that executed something.
	r.Comparable = true
	subject := SubjectNone
	for _, k := range kinds {
		if k.Coverage == CoverageAsserted {
			continue
		}
		if subject == SubjectNone {
			subject = k.Subject
			continue
		}
		if k.Subject != subject {
			r.Comparable = false
		}
	}
	if !r.Comparable {
		return r, true
	}

	r.Meet = kinds[0]
	for _, k := range kinds[1:] {
		if k.Rank() < r.Meet.Rank() {
			r.Meet = k
		}
	}
	return r, true
}

// lookupBasis finds the kind a Basis cell's tier token names, accepting either
// the legacy name or the pair notation.
func lookupBasis(token string) (BasisKind, bool) {
	for _, k := range Bases() {
		if token == k.Legacy || token == k.Spelling() {
			return k, true
		}
	}
	return BasisKind{}, false
}

// Pair returns the coverage and subject a basis claims, and whether it claims
// one at all: TierNone, TierWithdrawn and an unrecognised tier claim no basis.
// The tier may be spelled either way, so a Basis built by hand rather than
// parsed still decodes.
func (b Basis) Pair() (Coverage, Subject, bool) {
	k, ok := lookupBasis(b.Tier)
	return k.Coverage, k.Subject, ok
}

// parseTier resolves one Basis cell's tier token to its canonical spelling.
// Errors distinguish the three ways a pair can be malformed from an
// unrecognised token, because the first three are an author following section 2
// imprecisely and the fourth is a typo.
func parseTier(token string, lineIdx int) (string, error) {
	if k, ok := lookupBasis(token); ok {
		return k.Canonical(), nil
	}
	cov, subj, hasSlash := strings.Cut(token, "/")
	known := false
	for _, k := range Bases() {
		if string(k.Coverage) == cov {
			known = true
			break
		}
	}
	if !known {
		// Keeps the wording that section 2's legacy names are checked under, so
		// a stray "E9" still reads as what it is.
		return "", fmt.Errorf("propdoc: line %d: unknown evidence tier %q", lineIdx+1, token)
	}
	if Coverage(cov) == CoverageAsserted && hasSlash {
		return "", fmt.Errorf("propdoc: line %d: coverage %q takes no subject "+
			"(nothing was executed, so there is no artifact the evidence was about); write %q",
			lineIdx+1, CoverageAsserted, CoverageAsserted)
	}
	if !hasSlash {
		return "", fmt.Errorf("propdoc: line %d: coverage %q needs a subject: write %q or %q",
			lineIdx+1, cov, cov+"/"+string(SubjectImplementation), cov+"/"+string(SubjectModel))
	}
	return "", fmt.Errorf("propdoc: line %d: unknown evidence subject %q in %q: want %q or %q",
		lineIdx+1, subj, token, SubjectImplementation, SubjectModel)
}

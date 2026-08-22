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

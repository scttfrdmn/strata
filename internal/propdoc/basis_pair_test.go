package propdoc

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// The seven bases of section 2 are authored twice on purpose: once as a markdown
// table a reader consults, once as the registry Bases returns and the parser
// reads. That is not the two-copies-agreeing defect this package exists to
// prevent, because the two are not derived from each other and the failure they
// guard against is asymmetric and real: a spelling the document defines and the
// tool rejects breaks an author who followed the document, and a spelling the
// tool accepts and the document never defines is a basis nobody can look up.
// Only one of those two directions is loud on its own, so both are checked here.

// documentedBases parses section 2's basis table. It is bounded by the table's
// header rather than by matching rows against the registry, because recognising
// rows by what the registry already knows would make the "every documented row
// is accepted" direction vacuous.
func documentedBases(t *testing.T) []BasisKind {
	t.Helper()
	src, err := os.ReadFile(propertiesPath())
	if err != nil {
		t.Fatalf("read PROPERTIES.md: %v", err)
	}
	const header = "| Spelling in a Basis cell | Coverage | Subject | Legacy name |"
	lines := strings.Split(string(src), "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == header {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("PROPERTIES.md has no basis table: expected a row %q", header)
	}
	var out []BasisKind
	for _, line := range lines[start+2:] { // Skip the header and its separator.
		if !strings.HasPrefix(strings.TrimSpace(line), "|") {
			break
		}
		cells := splitCells(line)
		if len(cells) != 6 {
			t.Fatalf("basis table row has %d fields, want 6: %q", len(cells), line)
		}
		clean := func(i int) string {
			return strings.Trim(strings.TrimSpace(cells[i]), "`*")
		}
		subject := clean(3)
		if subject == "—" {
			subject = string(SubjectNone)
		}
		legacy := clean(4)
		if legacy == "none" {
			legacy = ""
		}
		out = append(out, BasisKind{
			Coverage: Coverage(clean(2)),
			Subject:  Subject(subject),
			Legacy:   legacy,
		})
		// The spelling column is derivable from the other two, so it is checked
		// rather than read: a row whose spelling does not follow from its pair
		// would teach an author a token the parser cannot resolve.
		if got, want := clean(1), out[len(out)-1].Spelling(); got != want {
			t.Errorf("basis table row %q spells the pair %q", got, want)
		}
	}
	return out
}

// TestBasisTableMatchesRegistry checks section 2's table against Bases in both
// directions, and states its own cardinality rather than deriving it from either
// side — a count taken from the registry would shrink with the registry, and one
// taken from the table would shrink with the table.
func TestBasisTableMatchesRegistry(t *testing.T) {
	const wantBases = 7 // 1 subjectless + 3 coverages x 2 subjects.

	documented := documentedBases(t)
	if len(documented) != wantBases {
		t.Fatalf("section 2 tabulates %d bases, want %d", len(documented), wantBases)
	}
	registry := Bases()
	if len(registry) != wantBases {
		t.Fatalf("Bases returns %d kinds, want %d", len(registry), wantBases)
	}

	bySpelling := func(ks []BasisKind) map[string]BasisKind {
		m := make(map[string]BasisKind, len(ks))
		for _, k := range ks {
			m[k.Spelling()] = k
		}
		return m
	}
	doc, reg := bySpelling(documented), bySpelling(registry)
	if len(doc) != wantBases {
		t.Errorf("section 2 tabulates a spelling twice: %d rows, %d distinct", len(documented), len(doc))
	}
	if len(reg) != wantBases {
		t.Errorf("Bases returns a spelling twice: %d kinds, %d distinct", len(registry), len(reg))
	}
	for spelling, want := range doc {
		got, ok := reg[spelling]
		if !ok {
			t.Errorf("section 2 defines %q and Bases does not: an author who follows the document writes a cell the parser rejects", spelling)
			continue
		}
		if got != want {
			t.Errorf("basis %q: section 2 says %+v, Bases says %+v", spelling, want, got)
		}
	}
	for spelling := range reg {
		if _, ok := doc[spelling]; !ok {
			t.Errorf("Bases accepts %q and section 2 does not define it: a basis nobody can look up", spelling)
		}
	}

	// The structural claims section 2 makes about the grid, which the row-by-row
	// comparison above would pass even if both sides were wrong together.
	legacy := map[string]string{}
	subjectless := 0
	for _, k := range registry {
		if k.Subject == SubjectNone {
			subjectless++
			if k.Coverage != CoverageAsserted {
				t.Errorf("basis %+v has no subject; only %q may", k, CoverageAsserted)
			}
		}
		if k.Legacy == "" {
			continue
		}
		if prev, dup := legacy[k.Legacy]; dup {
			t.Errorf("legacy name %q names both %q and %q", k.Legacy, prev, k.Spelling())
		}
		legacy[k.Legacy] = k.Spelling()
	}
	if subjectless != 1 {
		t.Errorf("%d bases take no subject, want exactly 1", subjectless)
	}
	if len(legacy) != 4 {
		t.Errorf("%d bases carry a legacy name, want 4 (E0-E3); three of the seven have none", len(legacy))
	}
	for _, name := range []string{"E0", "E1", "E2", "E3"} {
		if _, ok := legacy[name]; !ok {
			t.Errorf("legacy name %q names no basis", name)
		}
	}
}

// TestBothSpellingsDeriveTheSameStatus is the point of canonicalising at parse:
// the Basis column may be written either way, and the generated Status column
// must not record which way the author chose.
func TestBothSpellingsDeriveTheSameStatus(t *testing.T) {
	for _, k := range Bases() {
		if k.Legacy == "" {
			continue
		}
		for _, spelling := range []string{k.Legacy, k.Spelling()} {
			b, err := parseBasis(spelling+" `x_test.go:1`", 0)
			if err != nil {
				t.Fatalf("parseBasis(%q): %v", spelling, err)
			}
			if b.Tier != k.Canonical() {
				t.Errorf("parseBasis(%q).Tier = %q, want the canonical %q", spelling, b.Tier, k.Canonical())
			}
			if got, want := DeriveStatus(b, 0, 0), DeriveStatus(Basis{Tier: k.Legacy, Citation: "x"}, 0, 0); got != want {
				t.Errorf("%q derives %q; %q derives %q", spelling, got, k.Legacy, want)
			}
			// And the same for a Basis built by hand rather than parsed, which is
			// how every caller outside this package constructs one. Parse
			// canonicalises; a struct literal does not, so DeriveStatus has to
			// resolve the spelling itself or the two paths disagree.
			unparsed := DeriveStatus(Basis{Tier: spelling, Citation: "x"}, 0, 0)
			if want := DeriveStatus(Basis{Tier: k.Canonical(), Citation: "x"}, 0, 0); unparsed != want {
				t.Errorf("unparsed Basis{Tier: %q} derives %q, want %q", spelling, unparsed, want)
			}
		}
	}
}

// TestBasisWithNoLegacyName covers the case the one-dimensional ladder had no
// rung for, and which #129 produced: an exhaustive walk of a declared domain of
// the shipping implementation. Under the ladder it had to be filed as E1 or E2 —
// beneath an exhaustive check of a model — so the assertion here is that it is
// expressible at all, and renders as itself.
func TestBasisWithNoLegacyName(t *testing.T) {
	b, err := parseBasis("exhaustive/implementation — `internal/agent/boot_matrix_test.go`", 0)
	if err != nil {
		t.Fatalf("parseBasis: %v", err)
	}
	if b.Tier != "exhaustive/implementation" {
		t.Errorf("Tier = %q, want the pair notation kept", b.Tier)
	}
	cov, subj, ok := b.Pair()
	if !ok || cov != CoverageExhaustive || subj != SubjectImplementation {
		t.Errorf("Pair() = (%q, %q, %v), want (exhaustive, implementation, true)", cov, subj, ok)
	}
	if got, want := DeriveStatus(b, 0, 0), "ENFORCED exhaustive/implementation"; got != want {
		t.Errorf("DeriveStatus = %q, want %q", got, want)
	}
	// A live refutation still outranks it — proof-standard rule 5 is about the
	// register, and adding a stronger basis must not create an exemption.
	if got, want := DeriveStatus(b, 1, 2), "REFUTED (1 of 2 live)"; got != want {
		t.Errorf("with a live row, DeriveStatus = %q, want %q", got, want)
	}
}

func TestParseTierRejections(t *testing.T) {
	tests := []struct {
		token, wantErr string
	}{
		// The control for the inherited unknown-tier case: a stray E-name must
		// still read as an unknown tier rather than as a malformed pair.
		{"E9", "unknown evidence tier"},
		{"E4", "unknown evidence tier"},
		{"witnessed/implementation", "unknown evidence tier"},
		{"asserted/implementation", "takes no subject"},
		{"asserted/model", "takes no subject"},
		{"chosen", "needs a subject"},
		{"exhaustive", "needs a subject"},
		{"exhaustive/code", "unknown evidence subject"},
		{"chosen/", "unknown evidence subject"},
	}
	for _, tc := range tests {
		t.Run(tc.token, func(t *testing.T) {
			got, err := parseTier(tc.token, 0)
			if err == nil {
				t.Fatalf("parseTier(%q) = %q, want an error containing %q", tc.token, got, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("parseTier(%q) error = %v, want it to contain %q", tc.token, err, tc.wantErr)
			}
		})
	}
	// The control: every documented spelling resolves, so the rejections above
	// are rejecting something other than everything.
	for _, k := range documentedBases(t) {
		if _, err := parseTier(k.Spelling(), 0); err != nil {
			t.Errorf("parseTier(%q): %v", k.Spelling(), err)
		}
		if k.Legacy == "" {
			continue
		}
		if _, err := parseTier(k.Legacy, 0); err != nil {
			t.Errorf("parseTier(%q): %v", k.Legacy, err)
		}
	}
}

// TestEveryDerivedKindIsStated closes the hole that pair notation opens in the
// document's own distribution sentence. The inherited
// TestPropertiesDistributionMatchesHeader checks five kinds by name; a bare list
// of names reads as the complete set, and it is not — a Basis cell spelled as a
// pair with no legacy name derives a Status kind that is in no such list, so the
// prose would omit it and nothing would object. This asserts the derived
// direction: whatever kinds the register produces, the prose states them.
func TestEveryDerivedKindIsStated(t *testing.T) {
	src, err := os.ReadFile(propertiesPath())
	if err != nil {
		t.Fatalf("read PROPERTIES.md: %v", err)
	}
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	dist := doc.Distribution()
	if len(dist) == 0 {
		t.Fatal("the register derives no statuses at all; this check would pass on an empty document")
	}
	flat := strings.Join(strings.Fields(string(src)), " ")
	claim := func(kind string, n int) string {
		verb := "are"
		if n == 1 {
			verb = "is"
		}
		return fmt.Sprintf("%d %s `%s`", n, verb, kind)
	}
	for kind, n := range dist {
		if c := claim(kind, n); !strings.Contains(flat, c) {
			t.Errorf("the register derives %q for %d propositions and PROPERTIES.md does not state %q",
				kind, n, c)
		}
	}
	// The control: the search can fail. Without this, a prose sentence that
	// stated nothing would pass every assertion above the moment dist emptied.
	if absent := claim("ENFORCED exhaustive/implementation", len(doc.Propositions())); strings.Contains(flat, absent) {
		t.Errorf("PROPERTIES.md states %q, which no proposition claims; the control is wrong, not the document", absent)
	}
}

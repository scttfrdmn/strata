package propdoc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// propertiesPath is the document this package exists to keep honest.
func propertiesPath() string { return filepath.Join("..", "..", "PROPERTIES.md") }

func TestDeriveStatus(t *testing.T) {
	e1 := Basis{Tier: "E1", Citation: "`x_test.go:1`"}
	none := Basis{Tier: TierNone}

	tests := []struct {
		name        string
		basis       Basis
		live, total int
		want        string
	}{
		// The case this package was written for: a proposition with two
		// counterexamples, one discharged. The count moves even though the
		// verdict does not, which is what a hand-maintained column could not
		// show.
		{"one of two discharged", e1, 1, 2, "REFUTED (1 of 2 live)"},
		{"both live", e1, 2, 2, "REFUTED (2 of 2 live)"},
		{"all discharged, evidence cited", e1, 0, 2, "ENFORCED E1"},
		{"single counterexample omits the count", e1, 1, 1, "REFUTED"},
		{"refutation outranks a cited tier", e1, 1, 3, "REFUTED (1 of 3 live)"},
		{"never refuted, evidence cited", e1, 0, 0, "ENFORCED E1"},
		{"structure only", Basis{Tier: "E0", Citation: "`p.go:1`"}, 0, 0, "ASSERTED (E0)"},
		{"quantified", Basis{Tier: "E2", Citation: "`f_test.go:1`"}, 0, 0, "ENFORCED E2"},
		{"no evidence, no refutation", none, 0, 0, "UNPOPULATED"},
		// A tier with nothing cited is not a tier: the rule is "no citation ->
		// UNPOPULATED", so claiming E1 and citing nothing must not read as
		// enforcement.
		{"tier without citation", Basis{Tier: "E1"}, 0, 0, "UNPOPULATED"},
		{"withdrawn names its replacement", Basis{Tier: TierWithdrawn, Citation: "I1′, I6"}, 0, 0,
			"WITHDRAWN (superseded by I1′, I6)"},
		// Withdrawal outranks a live refutation: a proposition no longer
		// measured is not reported as failing.
		{"withdrawn with a live row", Basis{Tier: TierWithdrawn, Citation: "I6"}, 1, 1,
			"WITHDRAWN (superseded by I6)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeriveStatus(tc.basis, tc.live, tc.total); got != tc.want {
				t.Errorf("DeriveStatus(%+v, live=%d, total=%d) = %q, want %q",
					tc.basis, tc.live, tc.total, got, tc.want)
			}
		})
	}
}

// TestPropertiesStatusColumnIsDerived is the enforcement. It fails if any Status
// cell in PROPERTIES.md disagrees with the register, which is the drift this
// package removes. CI runs it: .github/workflows/ci.yml runs ./... with no -run
// filter.
func TestPropertiesStatusColumnIsDerived(t *testing.T) {
	src, err := os.ReadFile(propertiesPath())
	if err != nil {
		t.Fatalf("read PROPERTIES.md: %v", err)
	}
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse PROPERTIES.md: %v", err)
	}
	for _, d := range doc.Drifts() {
		t.Errorf("PROPERTIES.md:%d: %s status is %q, register implies %q",
			d.Line, d.ID, d.Written, d.Derived)
	}
	if unknown := doc.Unknown(); len(unknown) > 0 {
		t.Errorf("register names propositions section 3 does not state: %s",
			strings.Join(unknown, ", "))
	}
}

// TestPropertiesRenderIsIdempotent pins the claim that Render only touches the
// Status column. A document with no drift must render byte-identically, or
// regenerating it would produce diff noise that hides a real change.
func TestPropertiesRenderIsIdempotent(t *testing.T) {
	src, err := os.ReadFile(propertiesPath())
	if err != nil {
		t.Fatalf("read PROPERTIES.md: %v", err)
	}
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := string(doc.Render())
	want := strings.TrimSuffix(string(src), "\n")
	if got != want {
		for i, line := range strings.Split(want, "\n") {
			gotLines := strings.Split(got, "\n")
			if i < len(gotLines) && gotLines[i] != line {
				t.Fatalf("Render differs at line %d:\n got: %s\nwant: %s", i+1, gotLines[i], line)
			}
		}
		t.Fatalf("Render differs in length: got %d lines, want %d",
			len(strings.Split(got, "\n")), len(strings.Split(want, "\n")))
	}
}

// TestPropertiesGuardCanFail is the control for the two tests above. Without it,
// a parser that silently found no propositions would report no drift and read
// exactly like a clean document. Discharging a live register row must move the
// status of the propositions it names.
func TestPropertiesGuardCanFail(t *testing.T) {
	src, err := os.ReadFile(propertiesPath())
	if err != nil {
		t.Fatalf("read PROPERTIES.md: %v", err)
	}
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Propositions()) < 30 {
		t.Fatalf("parsed %d propositions, want at least 30 — the parser is not reading section 3",
			len(doc.Propositions()))
	}
	if len(doc.Register()) < 30 {
		t.Fatalf("parsed %d register rows, want at least 30 — the parser is not reading section 4",
			len(doc.Register()))
	}
	// Section 3.1 tabulates six proposition IDs and section 4.1 discusses
	// several more. Neither is a group table, and reading either would inflate
	// the counts above while leaving every status plausible.
	for _, p := range doc.Propositions() {
		if !strings.HasPrefix(p.Verdict, "SOUND") && !strings.Contains(p.Verdict, "TOO") &&
			!strings.Contains(p.Verdict, "ILL-FORMED") {
			t.Errorf("%s has verdict %q, which is not a verdict — the parser is reading a "+
				"table that is not a group table", p.ID, p.Verdict)
		}
	}

	// Discharge every live row and require each to move a status. Iterating all
	// of them (not just the first) makes this a check that no live row is inert
	// in the derivation, and the mutation is by column position, not text — the
	// counterexample column is free prose and can begin with "No"/"Yes"/
	// "Partially", so a search for "| "+Discharged could land on it (#111).
	before := doc.Statuses()
	liveRows := 0
	for _, r := range doc.Register() {
		if !r.Live() {
			continue
		}
		liveRows++

		mutated := dischargeRow(string(src), doc.lines[r.lineIdx])
		if mutated == string(src) {
			t.Fatalf("discharging %s changed nothing; the mutation missed the Discharged cell", r.Tracking)
		}
		mutatedDoc, err := Parse([]byte(mutated))
		if err != nil {
			t.Fatalf("Parse after discharging %s: %v", r.Tracking, err)
		}
		after := mutatedDoc.Statuses()
		moved := 0
		for _, id := range r.Props {
			if before[id] != after[id] {
				moved++
			}
		}
		if moved == 0 {
			t.Errorf("discharging %s moved no status among %v; that live row is inert in the derivation",
				r.Tracking, r.Props)
		}
		if len(mutatedDoc.Drifts()) == 0 {
			t.Errorf("discharging %s produced no drift against the written Status column; the guard cannot fail on it",
				r.Tracking)
		}
	}
	if liveRows == 0 {
		t.Fatal("no live register row found; this control cannot run")
	}
}

// dischargeRow rewrites a register row's Discharged cell — the last cell before
// the trailing pipe — to a synthetic valid discharge (a basis and a cited
// artifact, so it passes rule 11's guard, #143). It locates the cell by position
// rather than by searching for the Discharged token: the counterexample column
// is free prose and can begin with "No"/"Yes"/"Partially", so a text search
// could rewrite it instead and leave the Discharged cell untouched (#111 — the
// failure observed when #54 was reopened with a "No profile…" counterexample).
func dischargeRow(src, rowLine string) string {
	lastPipe := strings.LastIndex(rowLine, "|")
	if lastPipe < 0 {
		return src
	}
	secondLast := strings.LastIndex(rowLine[:lastPipe], "|")
	if secondLast < 0 {
		return src
	}
	mutatedLine := rowLine[:secondLast+1] +
		" Yes — E1, `internal/propdoc/guard_probe_test.go:1` " +
		rowLine[lastPipe:]
	return strings.Replace(src, rowLine, mutatedLine, 1)
}

// TestDischargeRowTargetsDischargedNotCounterexample is the #111 regression: a
// register row whose *counterexample* begins with "No" (the shape #54 took when
// reopened) must have its Discharged cell flipped, not its counterexample. The
// old text-based mutation rewrote the counterexample and left the row live.
func TestDischargeRowTargetsDischargedNotCounterexample(t *testing.T) {
	const head = "## 3. Propositions\n\n| # | P | Verdict | Basis | Status | Evidence |\n" +
		"|---|---|---|---|---|---|\n" +
		"| **R1** | p | SOUND | none | REFUTED | e |\n"
	const reg = "\n## 4. Refutation register\n\n| Proposition | C | Capability | Tracking | Discharged |\n" +
		"|---|---|---|---|---|\n" +
		"| R1 | No profile could resolve offline | H1 | #1 | No |\n"
	src := head + reg

	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var row Refutation
	for _, r := range doc.Register() {
		if r.Live() {
			row = r
			break
		}
	}
	if row.Tracking == "" {
		t.Fatal("no live row in fixture")
	}

	mutated := dischargeRow(src, doc.lines[row.lineIdx])
	if !strings.Contains(mutated, "No profile could resolve offline") {
		t.Fatal("mutation rewrote the counterexample cell instead of the Discharged cell (#111)")
	}

	mdoc, err := Parse([]byte(mutated))
	if err != nil {
		t.Fatalf("Parse mutated: %v", err)
	}
	for _, r := range mdoc.Register() {
		if r.Tracking == "#1" && r.Live() {
			t.Error("row #1 is still live after discharge; the mutation did not reach the Discharged cell")
		}
	}
	if before, after := doc.Statuses()["R1"], mdoc.Statuses()["R1"]; before == after {
		t.Errorf("R1 status did not move off %q after its only live row was discharged", after)
	}
}

func TestParseRejectsMalformedRows(t *testing.T) {
	const head = "## 3. Propositions\n\n| # | P | Verdict | Basis | Status | Evidence |\n" +
		"|---|---|---|---|---|---|\n"
	const reg = "\n## 4. Refutation register\n\n| Proposition | C | Capability | Tracking | Discharged |\n" +
		"|---|---|---|---|---|\n| R1 | x | H1 | #1 | No |\n"

	tests := []struct {
		name, src, wantErr string
	}{
		{
			name:    "unknown tier",
			src:     head + "| **R1** | p | SOUND | E9 `x` | REFUTED | e |\n" + reg,
			wantErr: "unknown evidence tier",
		},
		{
			name:    "tier none with a citation",
			src:     head + "| **R1** | p | SOUND | none `x_test.go:1` | REFUTED | e |\n" + reg,
			wantErr: "cite a tier or claim none",
		},
		{
			name:    "wrong field count",
			src:     head + "| **R1** | p | SOUND | none | REFUTED |\n" + reg,
			wantErr: "want 8",
		},
		{
			name: "unknown discharge value",
			src: head + "| **R1** | p | SOUND | none | REFUTED | e |\n" +
				"\n## 4. Refutation register\n\n| Proposition | C | Capability | Tracking | Discharged |\n" +
				"|---|---|---|---|---|\n| R1 | x | H1 | #1 | Mostly |\n",
			wantErr: "want one of Yes/No/Partially",
		},
		{
			name:    "no propositions",
			src:     "## 2. Proof standard\n\n| a | b |\n" + reg,
			wantErr: "no propositions found",
		},
		// A duplicate ID is how the scan announces that it has wandered out of
		// the group tables — section 3.1 tabulates proposition IDs too, and
		// reading it would double every ID it names.
		{
			name: "duplicate proposition",
			src: head + "| **R1** | p | SOUND | none | REFUTED | e |\n" +
				"| **R1** | p | SOUND | none | REFUTED | e |\n" + reg,
			wantErr: "appears at lines",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse([]byte(tc.src)); err == nil {
				t.Fatalf("Parse(%s): expected an error containing %q, got nil", tc.name, tc.wantErr)
			} else if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Parse(%s) error = %v, want it to contain %q", tc.name, err, tc.wantErr)
			}
		})
	}

	// The control: the same shape, well formed, parses and derives.
	good := head + "| **R1** | p | SOUND | none | REFUTED | e |\n" + reg
	doc, err := Parse([]byte(good))
	if err != nil {
		t.Fatalf("Parse(well-formed): %v", err)
	}
	if got := doc.Statuses()["R1"]; got != "REFUTED" {
		t.Errorf("well-formed R1 status = %q, want REFUTED", got)
	}
}

func TestKind(t *testing.T) {
	tests := map[string]string{
		"REFUTED (3 of 4 live)":             "REFUTED",
		"REFUTED":                           "REFUTED",
		"WITHDRAWN (superseded by I1′, I6)": "WITHDRAWN",
		"ENFORCED E1":                       "ENFORCED E1",
		"UNPOPULATED":                       "UNPOPULATED",
		// The tier is the content of this status, not detail about it, so it
		// survives the reduction that strips "(3 of 4 live)".
		"ASSERTED (E0)": "ASSERTED (E0)",
	}
	for in, want := range tests {
		if got := Kind(in); got != want {
			t.Errorf("Kind(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestPropertiesDistributionMatchesHeader pins the numbers PROPERTIES.md quotes
// about itself. The header states the distribution in prose; without this, that
// sentence is a hand count that goes stale the first time a register row is
// discharged — the same defect the Status column had.
func TestPropertiesDistributionMatchesHeader(t *testing.T) {
	src, err := os.ReadFile(propertiesPath())
	if err != nil {
		t.Fatalf("read PROPERTIES.md: %v", err)
	}
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	dist := doc.Distribution()
	total := 0
	for _, n := range dist {
		total += n
	}
	if total != len(doc.Propositions()) {
		t.Errorf("distribution totals %d, but there are %d propositions",
			total, len(doc.Propositions()))
	}

	// Collapse whitespace so the assertion does not depend on where the prose
	// happens to wrap, and state the total the same way.
	flat := strings.Join(strings.Fields(string(src)), " ")
	claims := []string{fmt.Sprintf("Of the %d propositions", total)}
	for _, kind := range []string{"REFUTED", "ENFORCED E1", "ASSERTED (E0)", "UNPOPULATED", "WITHDRAWN"} {
		n := dist[kind]
		verb := "are"
		if n == 1 {
			verb = "is"
		}
		claims = append(claims, fmt.Sprintf("%d %s `%s`", n, verb, kind))
	}
	for _, claim := range claims {
		if !strings.Contains(flat, claim) {
			t.Errorf("PROPERTIES.md does not state %q, which is what the register derives", claim)
		}
	}
}

// TestSplitCellsKeepsEscapedPipes pins the parsing detail that a naive split
// would get wrong: evidence cells quote shell pipelines as "\|".
func TestSplitCellsKeepsEscapedPipes(t *testing.T) {
	cells := splitCells(`| **T6** | p | SOUND | none | REFUTED | grep x . \| grep -v y |`)
	if len(cells) != propositionCells {
		t.Fatalf("splitCells returned %d fields, want %d: %q", len(cells), propositionCells, cells)
	}
	if want := ` grep x . \| grep -v y `; cells[6] != want {
		t.Errorf("evidence cell = %q, want %q", cells[6], want)
	}
}

package propdoc

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// citationRe matches a `path:line[s] [symbol...]` reference: a backticked path
// to a .go/.md/.yaml/.sh file, a line number or comma/range list, and optional
// trailing identifiers (a test or symbol name). The path may be repo-relative
// (`internal/agent/agent.go:299`) or a bare basename (`agent.go:299`); only the
// former is resolvable without a search.
var citationRe = regexp.MustCompile(
	"`([A-Za-z0-9_./-]+\\.(?:go|md|ya?ml|sh)):(\\d+(?:[,-]\\d+)*)((?:\\s+[A-Za-z_][A-Za-z0-9_]*)*)`")

// shaRe matches a backticked 7–40-char hex token on a document line. A citation
// that fails against the working tree is retried against each such token, so a
// citation pinned in prose to a past commit ("reproduced at `339329fb`")
// resolves against the code as it was. Layer/environment digests are also hex and
// will match; that is harmless — a `git show <digest>:<path>` simply fails and
// the retry moves on.
var shaRe = regexp.MustCompile("`([0-9a-f]{7,40})`")

// Citation is a `path:line[s] [symbol...]` reference found in the document.
type Citation struct {
	LineIdx  int      // 0-based line index in the document
	Path     string   // the path exactly as written
	Spans    [][2]int // cited line spans; "a-b" → {a,b} (inclusive), "a" → {a,a}
	Symbols  []string // trailing identifiers, e.g. a test name
	LineSHAs []string // commit-like hex tokens on the same document line
	Raw      string
}

// Resolvable reports whether the file can be located without a search — i.e. the
// path carries a directory. A basename-only citation (`agent.go:299`) is not
// resolvable here, and a bare `:299` (no path) is not matched at all; the caller
// counts those as the notation gap rather than checking them.
func (c Citation) Resolvable() bool { return strings.Contains(c.Path, "/") }

// Citations returns every `path:line` reference in the document, in order.
func (d *Doc) Citations() []Citation {
	var out []Citation
	for i, line := range d.lines {
		var shas []string
		for _, m := range shaRe.FindAllStringSubmatch(line, -1) {
			shas = append(shas, m[1])
		}
		for _, m := range citationRe.FindAllStringSubmatch(line, -1) {
			out = append(out, Citation{
				LineIdx:  i,
				Path:     m[1],
				Spans:    parseSpans(m[2]),
				Symbols:  strings.Fields(m[3]),
				LineSHAs: shas,
				Raw:      m[0],
			})
		}
	}
	return out
}

// parseSpans turns "82-98", "264,350,519" or "1-3,7" into inclusive line spans.
// A range is a contiguous span the cited symbol may sit anywhere within; a
// comma-list is discrete single-line spans.
func parseSpans(s string) [][2]int {
	var spans [][2]int
	for _, part := range strings.Split(s, ",") {
		if lo, hi, ok := strings.Cut(part, "-"); ok {
			a, e1 := strconv.Atoi(strings.TrimSpace(lo))
			b, e2 := strconv.Atoi(strings.TrimSpace(hi))
			if e1 == nil && e2 == nil {
				spans = append(spans, [2]int{a, b})
			}
			continue
		}
		if a, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			spans = append(spans, [2]int{a, a})
		}
	}
	return spans
}

// LineFetcher returns the lines of path at commit sha (empty sha = working tree)
// and whether it could be read. cmd/propgen supplies one backed by the working
// tree and `git show`; tests supply a fake.
type LineFetcher func(path, sha string) ([]string, bool)

// CitationDefect is a resolvable citation that does not resolve.
type CitationDefect struct {
	Citation
	Reason string
}

// CitationStats summarises a citation check: the count of citations examined,
// the count skipped as not machine-resolvable (the notation gap), and the
// defects found among the resolvable ones.
type CitationStats struct {
	Checked       int
	NotResolvable int
	Defects       []CitationDefect
}

// CheckCitations resolves every citation against fetch. A resolvable citation is
// a defect if its path does not resolve, any cited line is past end of file, or
// (when the citation names symbols) none of those symbols appears on any cited
// line. A citation that fails against the working tree is retried against each
// commit token on its document line before being reported, so a citation pinned
// to a past commit — the record of a discharged refutation — resolves against the
// code as it was and is not a defect. Non-resolvable citations are counted, not
// checked: normalising them is the notation half of the work (#105).
func (d *Doc) CheckCitations(fetch LineFetcher) CitationStats {
	var stats CitationStats
	for _, c := range d.Citations() {
		if !c.Resolvable() {
			stats.NotResolvable++
			continue
		}
		stats.Checked++
		reason := checkCitationAt(c, "", fetch)
		if reason == "" {
			continue
		}
		// Retry against any commit the document line pins to.
		resolved := false
		for _, sha := range c.LineSHAs {
			if checkCitationAt(c, sha, fetch) == "" {
				resolved = true
				break
			}
		}
		if !resolved {
			stats.Defects = append(stats.Defects, CitationDefect{Citation: c, Reason: reason})
		}
	}
	return stats
}

// checkCitationAt returns "" if the citation resolves at revision sha (empty =
// working tree), or a reason it does not.
func checkCitationAt(c Citation, sha string, fetch LineFetcher) string {
	lines, ok := fetch(c.Path, sha)
	if !ok {
		return fmt.Sprintf("path %q does not resolve%s", c.Path, atSuffix(sha))
	}
	for _, sp := range c.Spans {
		if sp[0] < 1 || sp[0] > sp[1] || sp[1] > len(lines) {
			return fmt.Sprintf("line span %d-%d is out of range for %q (%d lines)%s",
				sp[0], sp[1], c.Path, len(lines), atSuffix(sha))
		}
	}
	if len(c.Symbols) > 0 && len(c.Spans) > 0 {
		if !symbolInAnySpan(c, lines) {
			return fmt.Sprintf("none of %v appears within the cited span(s) %v of %q%s",
				c.Symbols, c.Spans, c.Path, atSuffix(sha))
		}
	}
	return ""
}

// symbolInAnySpan reports whether any named symbol appears on any line within any
// cited span. It is intentionally lenient across a multi-site citation: the check
// that bites is a citation whose named symbol has moved off *every* span it points
// at, which is the drift class #105 is about (a `path:line` that resolves but is
// the wrong line). A tighter per-symbol rule waits on the notation pass that makes
// one citation name one site.
func symbolInAnySpan(c Citation, lines []string) bool {
	for _, sp := range c.Spans {
		for n := sp[0]; n <= sp[1] && n <= len(lines); n++ {
			if n < 1 {
				continue
			}
			for _, s := range c.Symbols {
				if strings.Contains(lines[n-1], s) {
					return true
				}
			}
		}
	}
	return false
}

func atSuffix(sha string) string {
	if sha == "" {
		return ""
	}
	return " at " + sha
}

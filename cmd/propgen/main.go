// Command propgen derives the Status column of PROPERTIES.md from that
// document's refutation register.
//
// Status is a function of the register and the authored Basis cell, defined in
// internal/propdoc.DeriveStatus. Run with no flags to check for drift; run with
// -write to regenerate the column in place.
//
//	go run ./cmd/propgen            # exit 1 if any Status cell disagrees
//	go run ./cmd/propgen -write     # rewrite the Status column
//
// The same check runs in CI as internal/propdoc's
// TestPropertiesStatusColumnIsDerived, so a drifting document fails the build
// whether or not anyone runs this command.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/scttfrdmn/strata/internal/propdoc"
)

func main() {
	write := flag.Bool("write", false, "rewrite the Status column in place")
	path := flag.String("file", "PROPERTIES.md", "path to the properties document")
	flag.Parse()

	if err := run(*path, *write); err != nil {
		fmt.Fprintf(os.Stderr, "propgen: %v\n", err)
		os.Exit(1)
	}
}

func run(path string, write bool) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	doc, err := propdoc.Parse(src)
	if err != nil {
		return err
	}
	if unknown := doc.Unknown(); len(unknown) > 0 {
		return fmt.Errorf("register names propositions section 3 does not state: %v", unknown)
	}
	// Reported before the Status column is touched: a row whose discharge is not
	// justified makes every count derived from it wrong, so rewriting the column
	// on top of one would publish a number the register does not support.
	if defects := doc.DischargeDefects(); len(defects) > 0 {
		for _, def := range defects {
			fmt.Fprintf(os.Stderr, "%s:%d: %s: %v\n", path, def.Line, def.Tracking, def.Err)
		}
		return fmt.Errorf("%d register row(s) marked %q without naming and citing their evidence "+
			"(section 2.1 rule 11)", len(defects), propdoc.DischargedYes)
	}

	drifts := doc.Drifts()
	if len(drifts) == 0 {
		fmt.Printf("%s: %d propositions, %d register rows, no drift\n",
			path, len(doc.Propositions()), len(doc.Register()))
		printDistribution(doc)
		return nil
	}
	for _, d := range drifts {
		fmt.Fprintf(os.Stderr, "%s:%d: %s written %q, register implies %q\n",
			path, d.Line, d.ID, d.Written, d.Derived)
	}
	if !write {
		return fmt.Errorf("%d proposition(s) drifted; rerun with -write", len(drifts))
	}
	out := append(doc.Render(), '\n')
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return err
	}
	fmt.Printf("%s: rewrote %d Status cell(s)\n", path, len(drifts))
	printDistribution(doc)
	return nil
}

// printDistribution reports how many propositions carry each status. The
// document's header quotes these numbers, so they are printed rather than
// counted by hand.
func printDistribution(doc *propdoc.Doc) {
	dist := doc.Distribution()
	kinds := make([]string, 0, len(dist))
	for k := range dist {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Printf("  %-14s %d\n", k, dist[k])
	}
}

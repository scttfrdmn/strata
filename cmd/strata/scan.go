package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/scan"
)

func newScanCmd() *cobra.Command {
	var (
		noLmod   bool
		noConda  bool
		noPip    bool
		fsFlag   bool
		outProf  string
		outLock  string
		jsonOut  bool
		regURL   string
		archFlag string
		abiFlag  string
	)

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Detect installed software and match against the Strata registry",
		Long: `Scan the current system for installed software (Lmod modules, conda packages,
pip packages, filesystem heuristics) and match each against the Strata layer
registry. Outputs a table showing which packages are available as layers and
which would need to be captured.

Examples:

  # Scan all sources:
  strata scan

  # Scan Lmod only, skip conda/pip:
  strata scan --no-conda --no-pip

  # Output a profile from matched layers:
  strata scan --output-profile current.yaml

  # Machine-readable JSON:
  strata scan --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			lmod := !noLmod
			conda := !noConda
			pip := !noPip
			return runScan(cmd.Context(), lmod, conda, pip, fsFlag, outProf, outLock, jsonOut, regURL, archFlag, abiFlag)
		},
	}

	cmd.Flags().BoolVar(&noLmod, "no-lmod", false, "disable Lmod module detection")
	cmd.Flags().BoolVar(&noConda, "no-conda", false, "disable conda package detection")
	cmd.Flags().BoolVar(&noPip, "no-pip", false, "disable pip package detection")
	cmd.Flags().BoolVar(&fsFlag, "fs", false, "enable filesystem heuristics (slower)")
	cmd.Flags().StringVar(&outProf, "output-profile", "", "write matched layers as profile.yaml to FILE")
	cmd.Flags().StringVar(&outLock, "output-lockfile", "", "write matched layers as lock.yaml to FILE")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output results as JSON")
	cmd.Flags().StringVar(&regURL, "registry", os.Getenv("STRATA_REGISTRY_URL"),
		"registry URL (s3:// or file://); default: STRATA_REGISTRY_URL or embedded catalog")
	cmd.Flags().StringVar(&archFlag, "arch", "", "override detected architecture")
	cmd.Flags().StringVar(&abiFlag, "abi", "", "override detected ABI")

	return cmd
}

// scanJSONResult is the JSON output format.
type scanJSONResult struct {
	Package     string `json:"package"`
	Version     string `json:"version"`
	Status      string `json:"status"`
	NearVersion string `json:"near_version,omitempty"`
	Source      string `json:"source"`
	Prefix      string `json:"prefix,omitempty"`
}

func runScan(ctx context.Context, lmod, conda, pip, fs bool, outProf, outLock string, jsonOut bool, regURL, archFlag, abiFlag string) error {
	// Detect arch and ABI.
	arch := archFlag
	if arch == "" {
		arch = scan.CurrentArch()
	}
	abi := abiFlag
	if abi == "" {
		var err error
		abi, err = scan.CurrentABI()
		if err != nil {
			// Non-fatal: use empty ABI (works on macOS, non-Linux).
			abi = ""
		}
	}

	// Build scanner.
	s := &scan.Scanner{
		LmodEnabled:  lmod,
		CondaEnabled: conda,
		PipEnabled:   pip,
		FSEnabled:    fs,
		FSScanPaths:  scan.DefaultFSScanPaths,
	}

	pkgs, err := s.Scan(ctx)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	// Build registry client.
	var reg interface {
		ListLayers(ctx context.Context, name, arch, abi string) (interface{}, error)
	}
	_ = reg

	regClient := buildFederatedClient(nil)
	if regURL != "" {
		c, err := newClientForURL(regURL)
		if err != nil {
			return fmt.Errorf("scan: registry: %w", err)
		}
		regClient = c
	}

	// Match packages.
	results, err := scan.MatchAll(ctx, regClient, pkgs, arch, abi)
	if err != nil {
		return fmt.Errorf("scan: matching: %w", err)
	}

	// Output.
	if jsonOut {
		return printScanJSON(results)
	}
	printScanTable(results)

	// Write profile/lockfile if requested.
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "scanned"
	}

	if outProf != "" {
		profile := scan.ProfileFromMatches(results, hostname, "", arch)
		data, err := yaml.Marshal(profile)
		if err != nil {
			return fmt.Errorf("scan: marshaling profile: %w", err)
		}
		if err := os.WriteFile(outProf, data, 0644); err != nil {
			return fmt.Errorf("scan: writing profile: %w", err)
		}
		fmt.Fprintf(os.Stderr, "profile written: %s\n", outProf)
	}

	if outLock != "" {
		lf := scan.LockFileFromMatches(results, hostname, "", arch, abi)
		data, err := yaml.Marshal(lf)
		if err != nil {
			return fmt.Errorf("scan: marshaling lockfile: %w", err)
		}
		if err := os.WriteFile(outLock, data, 0644); err != nil {
			return fmt.Errorf("scan: writing lockfile: %w", err)
		}
		fmt.Fprintf(os.Stderr, "lockfile written: %s\n", outLock)
	}

	return nil
}

func printScanTable(results []scan.MatchResult) {
	matched, near, unmatched := 0, 0, 0
	for _, r := range results {
		switch r.Status {
		case scan.StatusMatched:
			matched++
		case scan.StatusNearMatch:
			near++
		case scan.StatusUnmatched:
			unmatched++
		}
	}

	fmt.Printf("Scanned: %d packages  [matched: %d  near: %d  unmatched: %d]\n\n",
		len(results), matched, near, unmatched)

	if len(results) == 0 {
		return
	}

	fmt.Printf("%-8s %-20s %-12s %-20s %s\n", "STATUS", "NAME", "DETECTED", "REGISTRY", "SOURCE")
	fmt.Printf("%-8s %-20s %-12s %-20s %s\n",
		"------", "----", "--------", "--------", "------")

	for _, r := range results {
		p := r.Package
		var status, regVer string
		switch r.Status {
		case scan.StatusMatched:
			status = "matched"
			regVer = p.Version
		case scan.StatusNearMatch:
			status = "near"
			regVer = r.NearVersion + " (near)"
		case scan.StatusUnmatched:
			status = "unmatched"
			regVer = "-"
		}
		fmt.Printf("%-8s %-20s %-12s %-20s %s\n",
			status, p.Name, p.Version, regVer, p.Source)
	}
}

func printScanJSON(results []scan.MatchResult) error {
	out := make([]scanJSONResult, 0, len(results))
	for _, r := range results {
		out = append(out, scanJSONResult{
			Package:     r.Package.Name,
			Version:     r.Package.Version,
			Status:      string(r.Status),
			NearVersion: r.NearVersion,
			Source:      string(r.Package.Source),
			Prefix:      r.Package.Prefix,
		})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

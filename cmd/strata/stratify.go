package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/strata/internal/capture"
	"github.com/scttfrdmn/strata/internal/scan"
	"github.com/scttfrdmn/strata/internal/trust"
)

func newStratifyCmd() *cobra.Command {
	var (
		noLmod    bool
		noConda   bool
		noPip     bool
		fsFlag    bool
		abiFlag   string
		archFlag  string
		normalize bool
		noSign    bool
		key       string
		reg       string
		provides  string
		requires  string
		dryRun    bool
	)

	cmd := &cobra.Command{
		Use:   "stratify",
		Short: "Scan installed software and capture unmatched layers into the registry",
		Long: `Stratify combines strata scan and strata capture into a single workflow.
It detects installed software, matches it against the registry, and captures
any unmatched or near-matched packages that have a discoverable install prefix.

Near-matched packages (name in registry but different version) are also
captured so the registry has a record of the exact version in use.

Examples:

  # Dry-run: show what would be captured
  strata stratify --dry-run

  # Capture all unmatched Lmod modules to local registry:
  strata stratify --no-conda --no-pip --no-sign

  # Capture to S3 registry with signing:
  strata stratify --registry s3://my-strata-registry \
    --key awskms:///alias/strata-signing-key`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			lmod := !noLmod
			conda := !noConda
			pip := !noPip
			return runStratify(cmd.Context(), lmod, conda, pip, fsFlag,
				abiFlag, archFlag, normalize, noSign, key, reg, provides, requires, dryRun)
		},
	}

	cmd.Flags().BoolVar(&noLmod, "no-lmod", false, "disable Lmod module detection")
	cmd.Flags().BoolVar(&noConda, "no-conda", false, "disable conda package detection")
	cmd.Flags().BoolVar(&noPip, "no-pip", false, "disable pip package detection")
	cmd.Flags().BoolVar(&fsFlag, "fs", false, "enable filesystem heuristics (slower)")
	cmd.Flags().StringVar(&abiFlag, "abi", "", "override detected ABI")
	cmd.Flags().StringVar(&archFlag, "arch", "", "override detected arch")
	cmd.Flags().BoolVar(&normalize, "normalize", false, "rewrite absolute paths in captured layers")
	cmd.Flags().BoolVar(&noSign, "no-sign", false, "skip cosign signing")
	cmd.Flags().StringVar(&key, "key", "awskms:///alias/strata-signing-key", "cosign key or KMS URI")
	cmd.Flags().StringVar(&reg, "registry", os.Getenv("STRATA_REGISTRY_URL"),
		"destination registry (s3:// or file://)")
	cmd.Flags().StringVar(&provides, "provides", "", "comma-separated capability=version pairs for captured layers")
	cmd.Flags().StringVar(&requires, "requires", "", "comma-separated requirement strings for captured layers")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be captured without running")

	return cmd
}

func runStratify(ctx context.Context, lmod, conda, pip, fs bool,
	abiFlag, archFlag string, normalize, noSign bool, key, reg, provides, requires string, dryRun bool) error {

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
			abi = ""
		}
	}

	// Scan installed software.
	s := &scan.Scanner{
		LmodEnabled:  lmod,
		CondaEnabled: conda,
		PipEnabled:   pip,
		FSEnabled:    fs,
		FSScanPaths:  scan.DefaultFSScanPaths,
	}

	pkgs, err := s.Scan(ctx)
	if err != nil {
		return fmt.Errorf("stratify: scan: %w", err)
	}

	// Match against registry.
	regClient := buildFederatedClient(nil)
	if reg != "" {
		c, err := newClientForURL(reg)
		if err != nil {
			return fmt.Errorf("stratify: registry: %w", err)
		}
		regClient = c
	}

	results, err := scan.MatchAll(ctx, regClient, pkgs, arch, abi)
	if err != nil {
		return fmt.Errorf("stratify: matching: %w", err)
	}

	// Identify candidates for capture: unmatched or near-match with a known prefix.
	var candidates []scan.MatchResult
	for _, r := range results {
		if r.Package.Prefix == "" {
			continue // pip packages have no prefix; skip
		}
		if r.Status == scan.StatusMatched {
			continue // already in registry at exact version
		}
		candidates = append(candidates, r)
	}

	if len(candidates) == 0 {
		fmt.Println("stratify: all detected packages are already in the registry")
		return nil
	}

	fmt.Printf("stratify: %d package(s) to capture\n\n", len(candidates))

	// Parse provides/requires.
	layerProvides, err := parseCapabilities(provides)
	if err != nil {
		return fmt.Errorf("stratify: --provides: %w", err)
	}
	layerRequires, err := parseRequirements(requires)
	if err != nil {
		return fmt.Errorf("stratify: --requires: %w", err)
	}

	// Build signer.
	var signer trust.Signer
	if !noSign {
		signer = &trust.CosignSigner{KeyRef: key}
	} else {
		fmt.Fprintln(os.Stderr, "WARNING: capturing without signing; use strata run --no-verify to use unsigned layers")
	}

	// Build registry client for capture.
	var pushReg capture.PushRegistry
	if !dryRun {
		regURL := reg
		if regURL == "" {
			regURL = "file://" + defaultCacheDir() + "/../captured"
		}
		c, err := newClientForURL(regURL)
		if err != nil {
			return fmt.Errorf("stratify: capture registry: %w", err)
		}
		pr, ok := c.(capture.PushRegistry)
		if !ok {
			return fmt.Errorf("stratify: registry does not support PushLayer")
		}
		pushReg = pr
	}

	// Capture each candidate.
	var (
		captured int
		failed   int
	)
	for _, r := range candidates {
		p := r.Package
		fmt.Printf("  capturing %s@%s (source: %s, prefix: %s)\n", p.Name, p.Version, p.Source, p.Prefix)

		captureSource := string(p.Source)
		cfg := capture.Config{
			Name:           p.Name,
			Version:        p.Version,
			Prefix:         p.Prefix,
			ABI:            abi,
			Arch:           arch,
			Normalize:      normalize,
			CaptureSource:  captureSource,
			OriginalPrefix: p.Prefix,
			Signer:         signer,
			Registry:       pushReg,
			DryRun:         dryRun,
			Provides:       layerProvides,
			Requires:       layerRequires,
		}

		result, err := capture.Capture(ctx, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  error capturing %s@%s: %v\n", p.Name, p.Version, err)
			failed++
			continue
		}
		if !dryRun && result.Manifest != nil {
			fmt.Printf("  captured: %s@%s (%s/%s)\n", result.Manifest.Name, result.Manifest.Version, result.Manifest.Arch, result.Manifest.ABI)
		}
		captured++
	}

	fmt.Printf("\nstratify: %d captured, %d failed\n", captured, failed)
	if failed > 0 {
		return fmt.Errorf("stratify: %d capture(s) failed", failed)
	}
	return nil
}

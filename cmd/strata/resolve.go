package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/probe"
	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/internal/resolver"
	"github.com/scttfrdmn/strata/spec"
)

func newResolveCmd() *cobra.Command {
	var output, strataVer string

	cmd := &cobra.Command{
		Use:   "resolve <profile.yaml>",
		Short: "Resolve a profile to a lockfile",
		Long: `Resolve a profile through the full 8-stage pipeline and write the
resulting lockfile to disk. Set STRATA_REGISTRY_URL to use the S3-backed
registry; otherwise the embedded Tier 0 catalog is used.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			profile := loadProfile(args[0])
			reg := buildRegistryClient()
			probeClient := buildProbeClient()

			r, err := resolver.New(resolver.Config{
				Registry:      reg,
				Probe:         probeClient,
				StrataVersion: strataVer,
			})
			if err != nil {
				return fmt.Errorf("resolve: %w", err)
			}

			lf, err := r.Resolve(context.Background(), profile)
			if err != nil {
				return fmt.Errorf("resolve: %w", err)
			}

			outPath := resolveOutputPath(args[0], output, ".lock.yaml")
			if err := writeYAML(outPath, lf); err != nil {
				return err
			}
			fmt.Printf("resolved: %s\n", outPath)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "o", "o", "", "output lockfile path (default: <profile-basename>.lock.yaml)")
	cmd.Flags().StringVar(&strataVer, "strata-version", version, "strata version recorded in the lockfile")
	return cmd
}

// buildRegistryClient returns an S3Client if STRATA_REGISTRY_URL is set,
// falling back to the embedded Tier 0 catalog as a MemoryStore.
// If STRATA_REGISTRY_URL is set but the S3 client cannot be initialised
// (e.g. bad URL, missing credentials) an error is printed to stderr and the
// embedded catalog is used instead — offline fallback is intentional.
func buildRegistryClient() registry.Client {
	if url := os.Getenv("STRATA_REGISTRY_URL"); url != "" {
		client, err := registry.NewS3Client(url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: S3 registry unavailable (%v); falling back to embedded catalog\n", err) //nolint:errcheck
			return buildCatalog()
		}
		return client
	}
	return buildCatalog()
}

// buildProbeClient returns a probe.Client for use by resolve/freeze.
//
// When STRATA_REGISTRY_URL is set and AWS credentials are available, it wires
// a real SSMResolver with an S3-backed cache so that lockfiles contain real
// AMI IDs. On any initialisation failure it falls back to the static offline
// client. Pre-seed the S3 probe cache with "strata probe <os> <arch>" before
// running strata resolve against a live registry.
func buildProbeClient() *probe.Client {
	if url := os.Getenv("STRATA_REGISTRY_URL"); url != "" {
		reg, err := registry.NewS3Client(url)
		if err == nil {
			r, err := probe.NewSSMResolver(context.Background())
			if err == nil {
				return &probe.Client{
					Resolver: r,
					Runner:   buildKnownFakeRunner(),
					Cache:    probe.NewS3Cache(reg),
				}
			}
		}
	}
	return buildStaticProbeClient()
}

// buildStaticProbeClient returns an offline-safe probe.Client using placeholder
// AMI IDs and KnownBaseCapabilities. Used when no registry or AWS credentials
// are available.
func buildStaticProbeClient() *probe.Client {
	amis := map[string]string{
		"al2023/x86_64":   "ami-al2023-x86_64",
		"al2023/arm64":    "ami-al2023-arm64",
		"rocky9/x86_64":   "ami-rocky9-x86_64",
		"rocky9/arm64":    "ami-rocky9-arm64",
		"rocky10/x86_64":  "ami-rocky10-x86_64",
		"rocky10/arm64":   "ami-rocky10-arm64",
		"ubuntu24/x86_64": "ami-ubuntu24-x86_64",
		"ubuntu24/arm64":  "ami-ubuntu24-arm64",
	}

	caps := make(map[string]*spec.BaseCapabilities)
	for osArch, amiID := range amis {
		parts := strings.SplitN(osArch, "/", 2)
		c, err := probe.KnownBaseCapabilities(parts[0], parts[1], amiID)
		if err != nil {
			continue
		}
		caps[amiID] = c
	}

	return &probe.Client{
		Resolver: &probe.StaticResolver{AMIs: amis},
		Runner:   &probe.FakeRunner{Capabilities: caps},
		Cache:    probe.NewMemoryCache(),
	}
}

// buildKnownFakeRunner returns a FakeRunner pre-loaded with KnownBaseCapabilities
// keyed by the static placeholder AMI IDs.
func buildKnownFakeRunner() *probe.FakeRunner {
	amis := map[string]string{
		"al2023/x86_64":   "ami-al2023-x86_64",
		"al2023/arm64":    "ami-al2023-arm64",
		"rocky9/x86_64":   "ami-rocky9-x86_64",
		"rocky9/arm64":    "ami-rocky9-arm64",
		"rocky10/x86_64":  "ami-rocky10-x86_64",
		"rocky10/arm64":   "ami-rocky10-arm64",
		"ubuntu24/x86_64": "ami-ubuntu24-x86_64",
		"ubuntu24/arm64":  "ami-ubuntu24-arm64",
	}

	caps := make(map[string]*spec.BaseCapabilities)
	for osArch, amiID := range amis {
		parts := strings.SplitN(osArch, "/", 2)
		c, err := probe.KnownBaseCapabilities(parts[0], parts[1], amiID)
		if err != nil {
			continue
		}
		caps[amiID] = c
	}
	return &probe.FakeRunner{Capabilities: caps}
}

// loadProfile reads and validates a profile from path.
func loadProfile(path string) *spec.Profile {
	p, err := spec.ParseProfile(path)
	if err != nil {
		// This is called from RunE so we can't return the error directly here;
		// the caller should check. In practice ParseProfile validates fully so
		// we panic on internal error and return a descriptive error for user errors.
		fmt.Fprintf(os.Stderr, "strata: %v\n", err) //nolint:errcheck
		os.Exit(1)
	}
	return p
}

// writeYAML marshals v to YAML and writes it to path.
func writeYAML(path string, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshaling YAML: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// resolveOutputPath returns the output path for a lockfile.
// If outputFlag is non-empty it is returned directly; otherwise the path
// is derived from inputPath by replacing the extension with suffix.
func resolveOutputPath(inputPath, outputFlag, suffix string) string {
	if outputFlag != "" {
		return outputFlag
	}
	base := filepath.Base(inputPath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	return filepath.Join(filepath.Dir(inputPath), name+suffix)
}

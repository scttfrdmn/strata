package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/strata/internal/probe"
	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

func newProbeCmd() *cobra.Command {
	var registryURL string

	cmd := &cobra.Command{
		Use:   "probe <os> <arch>",
		Short: "Resolve AMI ID and seed the probe cache",
		Long: `Resolve the current AMI ID for a given OS and arch via SSM Parameter
Store, derive its capabilities from KnownBaseCapabilities, and optionally
cache the result in the S3 registry.

  os    — al2023 | rocky9 | rocky10 | ubuntu24
  arch  — x86_64 | arm64`,
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			osName, arch := args[0], args[1]

			if _, err := probe.ResolveSSMParam(osName, arch); err != nil {
				return fmt.Errorf("probe: %w", err)
			}

			ctx := context.Background()
			fmt.Printf("Resolving AMI for %s/%s...\n", osName, arch)

			amiID, caps, err := resolveAndProbe(ctx, osName, arch, registryURL)
			if err != nil {
				return fmt.Errorf("probe: %w", err)
			}

			fmt.Printf("  AMI:    %s\n", amiID)
			fmt.Printf("  ABI:    %s\n", caps.ABI)
			fmt.Printf("  Arch:   %s\n", caps.Arch)
			fmt.Printf("  OS:     %s\n", caps.OS)
			fmt.Printf("  Provides:\n")
			for _, c := range caps.Provides {
				fmt.Printf("    %s@%s\n", c.Name, c.Version)
			}

			if registryURL != "" {
				fmt.Println("Cached to registry.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&registryURL, "registry", os.Getenv("STRATA_REGISTRY_URL"),
		"S3 registry URL (e.g. s3://my-strata-bucket); overrides STRATA_REGISTRY_URL")
	return cmd
}

// resolveAndProbe resolves the real AMI ID via SSM (or static fallback), derives
// capabilities from KnownBaseCapabilities, and stores the result in the registry
// cache if a registry URL is provided. Returns the AMI ID and capabilities.
func resolveAndProbe(ctx context.Context, osName, arch, registryURL string) (string, *spec.BaseCapabilities, error) {
	amiID, err := resolveAMIID(ctx, osName, arch)
	if err != nil {
		return "", nil, err
	}

	if registryURL != "" {
		if cached, ok := getFromRegistryCache(ctx, registryURL, amiID); ok {
			return amiID, cached, nil
		}
	}

	caps, err := probe.KnownBaseCapabilities(osName, arch, amiID)
	if err != nil {
		return "", nil, fmt.Errorf("generating capabilities: %w", err)
	}

	if registryURL != "" {
		storeToRegistryCache(ctx, registryURL, caps)
	}

	return amiID, caps, nil
}

// resolveAMIID returns the AMI ID for the given OS/arch. It attempts SSM
// first and falls back to the static placeholder on any error.
func resolveAMIID(ctx context.Context, osName, arch string) (string, error) {
	r, err := probe.NewSSMResolver(ctx)
	if err != nil {
		return "ami-" + osName + "-" + arch, nil
	}
	amiID, err := r.ResolveAMI(ctx, osName, arch)
	if err != nil {
		return "ami-" + osName + "-" + arch, nil
	}
	return amiID, nil
}

// getFromRegistryCache checks the S3 probe cache for the given AMI ID.
// Returns nil, false on any error or miss.
func getFromRegistryCache(ctx context.Context, registryURL, amiID string) (*spec.BaseCapabilities, bool) {
	reg, err := registry.NewS3Client(registryURL)
	if err != nil {
		return nil, false
	}
	caps, err := reg.GetBaseCapabilities(ctx, amiID)
	if err != nil {
		return nil, false
	}
	return caps, true
}

// storeToRegistryCache writes caps to the S3 probe cache. Errors are printed
// as warnings but do not terminate the command.
func storeToRegistryCache(ctx context.Context, registryURL string, caps *spec.BaseCapabilities) {
	reg, err := registry.NewS3Client(registryURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot connect to registry (%v); cache not updated\n", err) //nolint:errcheck
		return
	}
	if err := reg.StoreBaseCapabilities(ctx, caps); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to cache capabilities (%v)\n", err) //nolint:errcheck
	}
}

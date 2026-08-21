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

// buildFederatedClient builds a registry.Client from the profile's registries
// list, falling back to STRATA_REGISTRY_URL / the embedded catalog when the
// list is empty. The public Strata registry is always appended last so that
// private registries shadow public ones.
func buildFederatedClient(refs []spec.RegistryRef) registry.Client {
	if len(refs) == 0 {
		return buildRegistryClient() // existing single-registry path
	}
	var clients []registry.Client
	for _, ref := range refs {
		c, err := newClientForURL(ref.URL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: registry %q unavailable: %v\n", ref.URL, err) //nolint:errcheck
			continue
		}
		clients = append(clients, c)
	}
	// Public registry always last — private registries take priority.
	clients = append(clients, buildRegistryClient())
	if len(clients) == 1 {
		return clients[0]
	}
	return registry.NewFederatedClient(clients)
}

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
			reg := buildFederatedClient(profile.Registries)
			probeClient := buildProbeClient()

			r, err := resolver.New(resolver.Config{
				Registry:      reg,
				Probe:         probeClient,
				StrataVersion: strataVer,
				Warnings:      os.Stderr,
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

// buildRegistryClient returns a registry client for STRATA_REGISTRY_URL,
// falling back to the embedded Tier 0 catalog as a MemoryStore when the
// variable is unset.
//
// The URL is dispatched by scheme through newClientForURL, so a "file://" URL
// gets a LocalClient and an "s3://" URL gets an S3Client. Routing every URL to
// NewS3Client, as this did before, made the documented offline workflow
// (STRATA_REGISTRY_URL=file:///var/strata-local) fail its scheme check and fall
// silently back to the embedded recipe catalog, which carries no bundles and so
// dies in stage 7 for every profile (#54).
//
// If the client cannot be initialised (e.g. bad URL, missing credentials) an
// error is printed to stderr and the embedded catalog is used instead —
// offline fallback is intentional.
func buildRegistryClient() registry.Client {
	url := os.Getenv("STRATA_REGISTRY_URL")
	if url == "" {
		return buildCatalog()
	}
	client, err := newClientForURL(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: registry %q unavailable (%v); falling back to embedded catalog\n", url, err) //nolint:errcheck
		return buildCatalog()
	}
	return client
}

// buildProbeClient returns a probe.Client for use by resolve/freeze.
//
// When STRATA_REGISTRY_URL names an S3 registry and AWS credentials are
// available, it wires a real SSMResolver with an S3-backed cache so that
// lockfiles contain real AMI IDs. On any initialisation failure it falls back
// to the static offline client. Pre-seed the S3 probe cache with
// "strata probe <os> <arch>" before running strata resolve against a live
// registry.
//
// A file:// registry is the offline path by definition, so it never reaches for
// SSM: doing so would trade "no lockfile" for an IMDS timeout on a machine that
// has no credentials to find.
func buildProbeClient() *probe.Client {
	if url := os.Getenv("STRATA_REGISTRY_URL"); url != "" && !strings.HasPrefix(url, "file://") {
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

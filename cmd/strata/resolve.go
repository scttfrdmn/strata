package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/probe"
	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/internal/resolver"
	"github.com/scttfrdmn/strata/spec"
)

// runResolve implements "strata resolve <profile.yaml>".
//
// It resolves a profile through the full 8-stage pipeline and writes the
// resulting lockfile to disk. If STRATA_REGISTRY_URL is set the S3-backed
// registry is used; otherwise the embedded Tier 0 catalog is used as a
// fallback (resolution will fail at stage 7 until layers are built and signed).
func runResolve(args []string) {
	fs := flag.NewFlagSet("resolve", flag.ExitOnError)
	output := fs.String("o", "", "output lockfile path (default: <profile-basename>.lock.yaml)")
	strataVer := fs.String("strata-version", version, "strata version recorded in the lockfile")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: strata resolve <profile.yaml> [-o output.lock.yaml] [--strata-version v]\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		fatal("resolve: %v", err)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}

	profile := loadProfile(fs.Arg(0))
	reg := buildRegistryClient()
	probeClient := buildProbeClient()

	r, err := resolver.New(resolver.Config{
		Registry:      reg,
		Probe:         probeClient,
		StrataVersion: *strataVer,
	})
	if err != nil {
		fatal("resolve: %v", err)
	}

	lf, err := r.Resolve(context.Background(), profile)
	if err != nil {
		fatal("resolve: %v", err)
	}

	outPath := resolveOutputPath(fs.Arg(0), *output, ".lock.yaml")
	writeYAML(outPath, lf)
	fmt.Printf("resolved: %s\n", outPath)
}

// buildRegistryClient returns an S3Client if STRATA_REGISTRY_URL is set,
// falling back to the embedded Tier 0 catalog as a MemoryStore.
func buildRegistryClient() registry.Client {
	if url := os.Getenv("STRATA_REGISTRY_URL"); url != "" {
		return registry.NewS3Client(url)
	}
	return buildCatalog()
}

// buildProbeClient returns a probe.Client wired with placeholder AMI IDs and
// KnownBaseCapabilities for all supported OS images. Suitable for offline use
// and for wiring resolve/freeze when no live AWS credentials are present.
func buildProbeClient() *probe.Client {
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

// loadProfile reads and validates a profile from path, exiting on error.
func loadProfile(path string) *spec.Profile {
	p, err := spec.ParseProfile(path)
	if err != nil {
		fatal("%v", err)
	}
	return p
}

// writeYAML marshals v to YAML and writes it to path, exiting on error.
func writeYAML(path string, v any) {
	data, err := yaml.Marshal(v)
	if err != nil {
		fatal("marshaling YAML: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fatal("writing %s: %v", path, err)
	}
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

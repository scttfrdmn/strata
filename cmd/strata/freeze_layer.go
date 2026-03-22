package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/strata/internal/build"
	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

func newFreezeLayerCmd() *cobra.Command {
	var (
		upperDir string
		name     string
		ver      string
		abi      string
		arch     string
		reg      string
		key      string
		provides string
		requires string
		dryRun   bool
	)

	cmd := &cobra.Command{
		Use:   "freeze-layer",
		Short: "Convert an interactive EBS upper directory into a signed squashfs layer",
		Long: `Packages the contents of --upper (a persistent EBS upper directory from
the Path B workflow) into a reproducible squashfs image, signs it, and
pushes it to the registry as a first-class signed layer.

The resulting layer is identical in format and trust level to any layer
produced by strata build. Use --dry-run to preview the manifest without
creating a squashfs or requiring a registry.

Examples:

  # Preview what would be pushed (no squashfs created):
  strata freeze-layer --upper /strata/upper --name torch-ml --version 0.1.0 --dry-run

  # Freeze to local registry:
  strata freeze-layer --upper /strata/upper --name torch-ml --version 0.1.0 \
    --abi linux-gnu-2.34 --arch x86_64 \
    --registry file:///var/strata-local

  # Freeze to S3 registry:
  strata freeze-layer --upper /strata/upper --name torch-ml --version 0.1.0 \
    --registry s3://my-strata-bucket \
    --key awskms:///alias/strata-signing-key`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runFreezeLayer(cmd.Context(), upperDir, name, ver, abi, arch, reg, key, provides, requires, dryRun)
		},
	}

	cmd.Flags().StringVar(&upperDir, "upper", "", "path to the upper directory to freeze (required)")
	cmd.Flags().StringVar(&name, "name", "", "layer name, e.g. torch-ml (required)")
	cmd.Flags().StringVar(&ver, "version", "", "layer version, e.g. 0.1.0 (required)")
	cmd.Flags().StringVar(&abi, "abi", "linux-gnu-2.34", "C runtime ABI, e.g. linux-gnu-2.34")
	cmd.Flags().StringVar(&arch, "arch", "x86_64", "target architecture (x86_64, arm64)")
	cmd.Flags().StringVar(&reg, "registry", os.Getenv("STRATA_REGISTRY_URL"),
		"registry URL (s3:// or file://); overrides STRATA_REGISTRY_URL")
	cmd.Flags().StringVar(&key, "key", "awskms:///alias/strata-signing-key",
		"cosign key file or KMS URI")
	cmd.Flags().StringVar(&provides, "provides", "",
		"comma-separated capability=version pairs, e.g. torch=2.2.0,python=3.12.13")
	cmd.Flags().StringVar(&requires, "requires", "",
		"comma-separated requirement strings, e.g. glibc@>=2.34,python@>=3.12")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"print manifest summary without creating squashfs or requiring a registry")

	_ = cmd.MarkFlagRequired("upper")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("version")

	return cmd
}

func runFreezeLayer(ctx context.Context, upperDir, name, ver, abi, arch, reg, key, provides, requires string, dryRun bool) error {
	// Validate upper directory.
	info, err := os.Stat(upperDir)
	if err != nil {
		return fmt.Errorf("freeze-layer: --upper %q: %w", upperDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("freeze-layer: --upper %q is not a directory", upperDir)
	}
	entries, err := os.ReadDir(upperDir)
	if err != nil {
		return fmt.Errorf("freeze-layer: reading --upper %q: %w", upperDir, err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("freeze-layer: --upper %q is empty — nothing to freeze", upperDir)
	}

	if !dryRun && reg == "" {
		return fmt.Errorf("freeze-layer: --registry is required unless --dry-run is set")
	}

	// Parse provides/requires.
	layerProvides, err := parseCapabilities(provides)
	if err != nil {
		return fmt.Errorf("freeze-layer: --provides: %w", err)
	}
	layerRequires, err := parseRequirements(requires)
	if err != nil {
		return fmt.Errorf("freeze-layer: --requires: %w", err)
	}

	// Build manifest skeleton.
	manifest := &spec.LayerManifest{
		ID:             name + "-" + ver + "-" + abi + "-" + arch,
		Name:           name,
		Version:        ver,
		ABI:            abi,
		Arch:           arch,
		BuiltAt:        time.Now().UTC(),
		UserSelectable: true,
		Provides:       layerProvides,
		Requires:       layerRequires,
	}

	// Dry-run: print summary and exit.
	if dryRun {
		fmt.Fprintf(os.Stderr, "dry-run: freeze-layer plan\n")
		fmt.Fprintf(os.Stderr, "  name:     %s\n", name)
		fmt.Fprintf(os.Stderr, "  version:  %s\n", ver)
		fmt.Fprintf(os.Stderr, "  abi:      %s\n", abi)
		fmt.Fprintf(os.Stderr, "  arch:     %s\n", arch)
		fmt.Fprintf(os.Stderr, "  upper:    %s\n", upperDir)
		fmt.Fprintf(os.Stderr, "  id:       %s\n", manifest.ID)
		if len(layerProvides) > 0 {
			fmt.Fprintf(os.Stderr, "  provides: ")
			for i, p := range layerProvides {
				if i > 0 {
					fmt.Fprintf(os.Stderr, ", ")
				}
				fmt.Fprintf(os.Stderr, "%s@%s", p.Name, p.Version)
			}
			fmt.Fprintf(os.Stderr, "\n")
		}
		if len(layerRequires) > 0 {
			fmt.Fprintf(os.Stderr, "  requires: ")
			for i, r := range layerRequires {
				if i > 0 {
					fmt.Fprintf(os.Stderr, ", ")
				}
				fmt.Fprintf(os.Stderr, "%s", r.String())
			}
			fmt.Fprintf(os.Stderr, "\n")
		}
		return nil
	}

	// Generate content manifest.
	contentManifest, err := build.GenerateContentManifest(upperDir, manifest.ID)
	if err != nil {
		return fmt.Errorf("freeze-layer: generating content manifest: %w", err)
	}
	manifest.ContentManifest = contentManifest.Files

	// Create squashfs.
	sqfsFile, err := os.CreateTemp("", "strata-freeze-*.sqfs")
	if err != nil {
		return fmt.Errorf("freeze-layer: creating temp sqfs: %w", err)
	}
	sqfsPath := sqfsFile.Name()
	sqfsFile.Close()          //nolint:errcheck
	defer os.Remove(sqfsPath) //nolint:errcheck

	if err := build.CreateSquashfs(ctx, upperDir, sqfsPath); err != nil {
		return fmt.Errorf("freeze-layer: creating squashfs: %w", err)
	}

	// Compute SHA256 and size.
	sha256hex, err := registry.SHA256HexFile(sqfsPath)
	if err != nil {
		return fmt.Errorf("freeze-layer: hashing squashfs: %w", err)
	}
	stat, err := os.Stat(sqfsPath)
	if err != nil {
		return fmt.Errorf("freeze-layer: stat squashfs: %w", err)
	}
	manifest.SHA256 = sha256hex
	manifest.Size = stat.Size()

	// Sign.
	signer := &trust.CosignSigner{KeyRef: key}
	bundle, err := signer.Sign(ctx, sqfsPath, map[string]string{
		"strata.layer.name":    name,
		"strata.layer.version": ver,
	})
	if err != nil {
		return fmt.Errorf("freeze-layer: signing: %w", err)
	}
	bundleJSON, err := bundle.Marshal()
	if err != nil {
		return fmt.Errorf("freeze-layer: marshaling bundle: %w", err)
	}
	if idx, ok := bundle.RekorLogIndex(); ok {
		manifest.RekorEntry = fmt.Sprintf("%d", idx)
	}
	if len(bundle.VerificationMaterial.TlogEntries) > 0 {
		// Use the public key hint as SignedBy if available.
		if pk := bundle.VerificationMaterial.PublicKey; pk != nil && pk.Hint != "" {
			manifest.SignedBy = pk.Hint
		}
	}
	manifest.CosignVersion = trust.CosignToolVersion(ctx)

	// Push to registry.
	pushClient, err := newClientForURL(reg)
	if err != nil {
		return fmt.Errorf("freeze-layer: initializing registry: %w", err)
	}
	pushReg, ok := pushClient.(build.PushRegistry)
	if !ok {
		return fmt.Errorf("freeze-layer: registry client does not support PushLayer")
	}

	if err := pushReg.PushLayer(ctx, manifest, sqfsPath, bundleJSON); err != nil {
		return fmt.Errorf("freeze-layer: pushing layer: %w", err)
	}

	return printBuildResult(manifest, false)
}

// parseCapabilities parses a comma-separated "name=version" string into Capabilities.
// Empty input returns nil.
func parseCapabilities(s string) ([]spec.Capability, error) {
	if s == "" {
		return nil, nil
	}
	var caps []spec.Capability
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 || kv[0] == "" || kv[1] == "" {
			return nil, fmt.Errorf("expected name=version, got %q", part)
		}
		caps = append(caps, spec.Capability{Name: kv[0], Version: kv[1]})
	}
	return caps, nil
}

// parseRequirements parses a comma-separated requirement string into Requirements.
// Each element is formatted as "name@>=version" or "name@>=min,<max".
// Empty input returns nil.
func parseRequirements(s string) ([]spec.Requirement, error) {
	if s == "" {
		return nil, nil
	}
	var reqs []spec.Requirement
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		atIdx := strings.Index(part, "@")
		if atIdx < 0 {
			// Plain name with no version constraint.
			reqs = append(reqs, spec.Requirement{Name: part})
			continue
		}
		reqName := part[:atIdx]
		constraint := part[atIdx+1:]
		req := spec.Requirement{Name: reqName}
		if strings.HasPrefix(constraint, ">=") {
			req.MinVersion = strings.TrimPrefix(constraint, ">=")
		} else if strings.HasPrefix(constraint, "<") {
			req.MaxVersion = strings.TrimPrefix(constraint, "<")
		} else {
			req.MinVersion = constraint
		}
		reqs = append(reqs, req)
	}
	return reqs, nil
}

// newClientForURL constructs a registry Client from a URL.
// Supports "file://" (LocalClient) and "s3://" (S3Client) schemes.
func newClientForURL(url string) (registry.Client, error) {
	if strings.HasPrefix(url, "file://") {
		return registry.NewLocalClient(url)
	}
	return registry.NewS3Client(url)
}

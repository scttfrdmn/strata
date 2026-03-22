package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/strata/internal/capture"
	"github.com/scttfrdmn/strata/internal/scan"
	"github.com/scttfrdmn/strata/internal/trust"
)

func newCaptureCmd() *cobra.Command {
	var (
		name      string
		ver       string
		prefix    string
		fromLmod  string
		fromConda string
		condaEnv  string
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
		Use:   "capture",
		Short: "Snapshot an installed prefix into a signed squashfs layer",
		Long: `Capture packages an existing software installation into a reproducible
squashfs layer and pushes it to a local or remote registry. The resulting
layer is in the same format as any layer produced by strata build.

Examples:

  # Dry-run (no squashfs created):
  strata capture --name mylib --version 2.4.1 --prefix /opt/mylib/2.4.1 --dry-run

  # Capture to local registry (no signing):
  strata capture --name mylib --version 2.4.1 --prefix /opt/mylib/2.4.1 --no-sign

  # Capture from loaded Lmod module:
  strata capture --from-lmod python/3.11.11 --no-sign

  # Capture with signing and push to S3:
  strata capture --name gcc --version 14.2.0 --prefix /opt/gcc/14.2.0 \
    --key awskms:///alias/strata-signing-key \
    --registry s3://my-strata-registry`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCapture(cmd.Context(), name, ver, prefix, fromLmod, fromConda, condaEnv,
				abiFlag, archFlag, normalize, noSign, key, reg, provides, requires, dryRun)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "layer name (required unless --from-lmod/--from-conda)")
	cmd.Flags().StringVar(&ver, "version", "", "layer version (required unless --from-lmod/--from-conda)")
	cmd.Flags().StringVar(&prefix, "prefix", "", "installed directory to snapshot")
	cmd.Flags().StringVar(&fromLmod, "from-lmod", "", "resolve name/version/prefix from Lmod module (e.g. python/3.11.11)")
	cmd.Flags().StringVar(&fromConda, "from-conda", "", "resolve from conda package in active environment")
	cmd.Flags().StringVar(&condaEnv, "conda-env", "", "conda environment name (with --from-conda)")
	cmd.Flags().StringVar(&abiFlag, "abi", "", "override detected ABI")
	cmd.Flags().StringVar(&archFlag, "arch", "", "override detected arch")
	cmd.Flags().BoolVar(&normalize, "normalize", false, "rewrite absolute paths in the captured layer")
	cmd.Flags().BoolVar(&noSign, "no-sign", false, "skip cosign signing")
	cmd.Flags().StringVar(&key, "key", "awskms:///alias/strata-signing-key", "cosign key or KMS URI")
	cmd.Flags().StringVar(&reg, "registry", "", "destination registry (s3:// or file://); default: local captured dir")
	cmd.Flags().StringVar(&provides, "provides", "", "comma-separated capability=version pairs")
	cmd.Flags().StringVar(&requires, "requires", "", "comma-separated requirement strings")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print manifest without building")

	return cmd
}

func runCapture(ctx context.Context, name, ver, prefix, fromLmod, fromConda, condaEnv,
	abiFlag, archFlag string, normalize, noSign bool, key, reg, provides, requires string, dryRun bool) error {

	// Resolve from Lmod if requested.
	if fromLmod != "" {
		n, v, p, err := resolveLmodPrefix(fromLmod)
		if err != nil {
			return fmt.Errorf("capture: --from-lmod: %w", err)
		}
		if name == "" {
			name = n
		}
		if ver == "" {
			ver = v
		}
		if prefix == "" {
			prefix = p
		}
	}

	// Resolve from conda if requested.
	if fromConda != "" {
		n, v, p, err := resolveCondaPrefix(fromConda, condaEnv)
		if err != nil {
			return fmt.Errorf("capture: --from-conda: %w", err)
		}
		if name == "" {
			name = n
		}
		if ver == "" {
			ver = v
		}
		if prefix == "" {
			prefix = p
		}
	}

	if name == "" {
		return fmt.Errorf("capture: --name is required (or use --from-lmod/--from-conda)")
	}
	if ver == "" {
		return fmt.Errorf("capture: --version is required (or use --from-lmod/--from-conda)")
	}
	if prefix == "" && !dryRun {
		return fmt.Errorf("capture: --prefix is required (or use --from-lmod/--from-conda)")
	}

	if noSign {
		fmt.Fprintln(os.Stderr, "WARNING: layer captured without signing; cannot be used with strata verify")
		fmt.Fprintln(os.Stderr, "         use strata run --no-verify to use unsigned layers")
	}

	// Default registry: file://<parent-of-defaultCacheDir>/captured
	regURL := reg
	if regURL == "" {
		captureDir := filepath.Join(filepath.Dir(defaultCacheDir()), "captured")
		regURL = "file://" + captureDir
	}

	// Build registry client.
	var pushReg capture.PushRegistry
	if !dryRun {
		c, err := newClientForURL(regURL)
		if err != nil {
			return fmt.Errorf("capture: registry: %w", err)
		}
		pr, ok := c.(capture.PushRegistry)
		if !ok {
			return fmt.Errorf("capture: registry does not support PushLayer")
		}
		pushReg = pr
	}

	// Parse provides/requires.
	layerProvides, err := parseCapabilities(provides)
	if err != nil {
		return fmt.Errorf("capture: --provides: %w", err)
	}
	layerRequires, err := parseRequirements(requires)
	if err != nil {
		return fmt.Errorf("capture: --requires: %w", err)
	}

	// Determine capture source.
	var captureSource string
	switch {
	case fromLmod != "":
		captureSource = "lmod"
	case fromConda != "":
		captureSource = "conda"
	default:
		captureSource = "filesystem"
	}

	// Build signer.
	var signer trust.Signer
	if !noSign {
		signer = &trust.CosignSigner{KeyRef: key}
	}

	cfg := capture.Config{
		Name:          name,
		Version:       ver,
		Prefix:        prefix,
		ABI:           abiFlag,
		Arch:          archFlag,
		Normalize:     normalize,
		CaptureSource: captureSource,
		Signer:        signer,
		Registry:      pushReg,
		DryRun:        dryRun,
		Provides:      layerProvides,
		Requires:      layerRequires,
	}

	result, err := capture.Capture(ctx, cfg)
	if err != nil {
		return fmt.Errorf("capture: %w", err)
	}

	if dryRun {
		return nil
	}

	m := result.Manifest
	fmt.Printf("captured: %s@%s (%s/%s)\n", m.Name, m.Version, m.Arch, m.ABI)
	fmt.Printf("  registry: %s\n", regURL)

	return nil
}

// resolveLmodPrefix resolves name, version, and prefix from a module specifier
// like "python/3.11.11" by searching $MODULEPATH.
func resolveLmodPrefix(moduleSpec string) (name, version, prefix string, err error) {
	parts := strings.SplitN(moduleSpec, "/", 2)
	if len(parts) != 2 {
		return "", "", "", fmt.Errorf("expected name/version format, got %q", moduleSpec)
	}
	name = parts[0]
	version = parts[1]

	modulePath := os.Getenv("MODULEPATH")
	if modulePath == "" {
		return name, version, "", fmt.Errorf("MODULEPATH is not set")
	}

	for _, dir := range strings.Split(modulePath, ":") {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}

		// Look for <dir>/<name>/<version>.lua or .tcl
		for _, ext := range []string{".lua", ".tcl"} {
			modPath := filepath.Join(dir, name, version+ext)
			if _, statErr := os.Stat(modPath); statErr == nil {
				data, readErr := os.ReadFile(modPath)
				if readErr != nil {
					continue
				}
				content := string(data)
				var p scan.DetectedPackage
				if ext == ".lua" {
					parseLuaContent(content, &p)
				} else {
					parseTclContent(content, &p)
				}
				if p.Prefix != "" {
					return name, version, p.Prefix, nil
				}
			}
		}
	}

	return name, version, "", fmt.Errorf("module %q not found in MODULEPATH or has no prefix", moduleSpec)
}

// parseLuaContent extracts prefix from Lua modulefile content.
// This duplicates a subset of internal/scan/lmod.go to avoid circular deps.
func parseLuaContent(content string, p *scan.DetectedPackage) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		for _, kw := range []string{"base", "root", "prefix"} {
			pfx := "local " + kw + " ="
			if strings.HasPrefix(line, pfx) {
				val := strings.TrimPrefix(line, pfx)
				val = strings.TrimSpace(val)
				val = strings.Trim(val, `"`)
				if strings.HasPrefix(val, "/") {
					p.Prefix = val
					return
				}
			}
		}
	}
}

// parseTclContent extracts prefix from TCL modulefile content.
func parseTclContent(content string, p *scan.DetectedPackage) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 3 && parts[0] == "set" {
			kw := parts[1]
			if kw == "root" || kw == "base" || kw == "prefix" {
				val := parts[2]
				if strings.HasPrefix(val, "/") {
					p.Prefix = val
					return
				}
			}
		}
	}
}

// resolveCondaPrefix resolves name, version, and prefix from a conda package.
func resolveCondaPrefix(pkgName, envName string) (name, version, prefix string, err error) {
	condaPrefix := os.Getenv("CONDA_PREFIX")
	if envName != "" {
		condaExe := os.Getenv("CONDA_EXE")
		if condaExe == "" {
			return "", "", "", fmt.Errorf("CONDA_EXE not set; cannot resolve --conda-env")
		}
		condaRoot := filepath.Dir(filepath.Dir(condaExe))
		condaPrefix = filepath.Join(condaRoot, "envs", envName)
	}
	if condaPrefix == "" {
		return "", "", "", fmt.Errorf("no active conda environment (CONDA_PREFIX not set)")
	}

	// Read conda-meta to find the package.
	metaDir := filepath.Join(condaPrefix, "conda-meta")
	entries, readErr := os.ReadDir(metaDir)
	if readErr != nil {
		return "", "", "", fmt.Errorf("reading conda-meta: %w", readErr)
	}

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(metaDir, e.Name()))
		if readErr != nil {
			continue
		}
		var entry struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if jsonErr := json.Unmarshal(data, &entry); jsonErr != nil {
			continue
		}
		if strings.EqualFold(entry.Name, pkgName) {
			return entry.Name, entry.Version, condaPrefix, nil
		}
	}

	return "", "", "", fmt.Errorf("package %q not found in conda env %s", pkgName, condaPrefix)
}

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/spec"
)

func newRunCmd() *cobra.Command {
	var lockfilePath, cacheDir string
	var noVerify bool
	var envOverrides []string

	cmd := &cobra.Command{
		Use:   "run --lockfile <lock.yaml> [--no-verify] [--cache-dir DIR] [--env KEY=VAL] -- <command> [args...]",
		Short: "Run a command inside a Strata environment",
		Long: `Mount the layers described by a lockfile and run a command inside the
resulting environment. Works for both privileged (OverlayFS) and
unprivileged (FUSE) contexts.

Layer files are cached in the user cache directory and reused on
subsequent runs. Pass --no-verify to skip signature verification
on air-gapped systems.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if lockfilePath == "" {
				return errors.New("--lockfile is required")
			}
			if len(args) == 0 {
				return errors.New("command to run is required after --")
			}
			if cacheDir == "" {
				cacheDir = defaultCacheDir()
			}
			return runRun(cmd.Context(), lockfilePath, args, noVerify, cacheDir, envOverrides)
		},
	}

	cmd.Flags().StringVar(&lockfilePath, "lockfile", "", "path to the lockfile (required)")
	cmd.Flags().BoolVar(&noVerify, "no-verify", false, "skip signature verification (use on air-gapped systems)")
	cmd.Flags().StringVar(&cacheDir, "cache-dir", "", "layer cache directory (default: ~/.cache/strata/layers)")
	cmd.Flags().StringArrayVar(&envOverrides, "env", nil, "additional environment variables (KEY=VAL)")
	return cmd
}

func runRun(ctx context.Context, lockfilePath string, args []string, noVerify bool, cacheDir string, envOverrides []string) error {
	// 1. Read lockfile.
	data, err := os.ReadFile(lockfilePath)
	if err != nil {
		return fmt.Errorf("run: reading lockfile: %w", err)
	}
	var lf spec.LockFile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return fmt.Errorf("run: parsing lockfile: %w", err)
	}

	// 2. Signature verification — best-effort warning (not fatal) since strata
	//    run is primarily for unprivileged/HPC use where cosign may be absent.
	if !noVerify && lf.RekorEntry == "" {
		fmt.Fprintln(os.Stderr, "run: warning: lockfile has no Rekor entry — not signed")
	}

	// 3. Warn if the lockfile has package entries — packages are installed by
	//    strata-agent at instance boot, not by strata run.
	if len(lf.Packages) > 0 {
		total := 0
		for _, ps := range lf.Packages {
			total += len(ps.Packages)
		}
		noun := "entries"
		if total == 1 {
			noun = "entry"
		}
		fmt.Fprintf(os.Stderr,
			"run: warning: lockfile has %d package %s (pip/conda/cran) — packages are installed by strata-agent at boot, not by strata run; the mounted environment will not include these packages\n",
			total, noun)
	}

	// 4. Ensure cache dir exists.
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("run: creating cache dir: %w", err)
	}

	// 5. Fetch layers to cache.
	layerPaths, err := fetchLayersToCache(ctx, lf, cacheDir)
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}

	// 6. Create temp working directory.
	workDir, err := os.MkdirTemp("", fmt.Sprintf("strata-%d-*", os.Getpid()))
	if err != nil {
		return fmt.Errorf("run: creating work dir: %w", err)
	}
	defer os.RemoveAll(workDir) //nolint:errcheck

	// 7. Mount overlay with user-local paths and auto-detected strategy.
	cfg := overlay.Config{
		LayersDir: filepath.Join(workDir, "layers"),
		RWDir:     filepath.Join(workDir, "rw"),
		MergedDir: filepath.Join(workDir, "env"),
	}
	ov, err := overlay.MountWithConfig(layerPaths, cfg)
	if err != nil {
		return fmt.Errorf("run: mounting overlay: %w", err)
	}

	// Install cleanup on signals so mounts are unmounted on Ctrl-C / kill.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer ov.Cleanup() //nolint:errcheck

	// 8. Build environment for the child process.
	env := buildRunEnv(&lf, ov.MergedPath, envOverrides)

	// 9. Execute the command.
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("run: %w", err)
	}
	return nil
}

// buildRunEnv builds the environment for the child process from os.Environ()
// plus Strata-specific variables derived from the lockfile and overlay path.
func buildRunEnv(lf *spec.LockFile, mergedPath string, overrides []string) []string {
	// Build per-layer PATH and LD_LIBRARY_PATH (same logic as ConfigureEnvironment).
	lastVersionOf := make(map[string]string)
	for _, layer := range lf.Layers {
		if layer.InstallLayout != "flat" {
			lastVersionOf[layer.Name] = layer.Version
		}
	}
	var pathParts, ldParts []string
	for _, layer := range lf.Layers {
		if layer.InstallLayout == "flat" {
			continue
		}
		if lastVersionOf[layer.Name] != layer.Version {
			continue
		}
		base := fmt.Sprintf("%s/%s/%s", mergedPath, layer.Name, layer.Version)
		pathParts = append(pathParts, base+"/bin")
		ldParts = append(ldParts, base+"/lib", base+"/lib64")
	}

	// Start from current environment.
	env := os.Environ()

	// Override PATH and LD_LIBRARY_PATH.
	if len(pathParts) > 0 {
		env = setEnvVar(env, "PATH", strings.Join(pathParts, ":")+":"+os.Getenv("PATH"))
	}
	if len(ldParts) > 0 {
		existing := os.Getenv("LD_LIBRARY_PATH")
		value := strings.Join(ldParts, ":")
		if existing != "" {
			value += ":" + existing
		}
		env = setEnvVar(env, "LD_LIBRARY_PATH", value)
	}

	// Strata metadata variables.
	env = setEnvVar(env, "STRATA_PROFILE", lf.ProfileName)
	env = setEnvVar(env, "STRATA_ENV", mergedPath)
	env = setEnvVar(env, "STRATA_REKOR_ENTRY", lf.RekorEntry)

	// Lockfile Env overrides.
	for k, v := range lf.Env {
		env = setEnvVar(env, k, v)
	}

	// CLI --env overrides (highest priority).
	for _, kv := range overrides {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			env = setEnvVar(env, parts[0], parts[1])
		}
	}

	return env
}

// setEnvVar sets KEY=VAL in env, replacing any existing KEY= entry.
func setEnvVar(env []string, key, val string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + val
			return env
		}
	}
	return append(env, prefix+val)
}

// fetchLayersToCache downloads all layers in the lockfile to cacheDir,
// verifying SHA256 after download. Layers already in cache are reused.
func fetchLayersToCache(ctx context.Context, lf spec.LockFile, cacheDir string) ([]overlay.LayerPath, error) {
	if len(lf.Layers) == 0 {
		return nil, nil
	}

	// Lazily initialise S3 client — only needed if any layer is not cached.
	var s3Client *awss3.Client

	paths := make([]overlay.LayerPath, 0, len(lf.Layers))
	for _, layer := range lf.Layers {
		cachePath := filepath.Join(cacheDir, layer.SHA256+".sqfs")

		// Cache hit.
		if _, err := os.Stat(cachePath); err == nil {
			paths = append(paths, overlay.LayerPath{
				ID:         layer.ID,
				SHA256:     layer.SHA256,
				Path:       cachePath,
				MountOrder: layer.MountOrder,
			})
			continue
		}

		// Determine download source.
		if strings.HasPrefix(layer.Source, "s3://") {
			if s3Client == nil {
				cfg, err := awsconfig.LoadDefaultConfig(ctx)
				if err != nil {
					return nil, fmt.Errorf("loading AWS config: %w", err)
				}
				if cfg.Region == "" {
					cfg.Region = "us-east-1"
				}
				s3Client = awss3.NewFromConfig(cfg)
			}
			if err := downloadS3Layer(ctx, s3Client, layer.Source, cachePath); err != nil {
				return nil, fmt.Errorf("downloading layer %q: %w", layer.ID, err)
			}
		} else if strings.HasPrefix(layer.Source, "file://") {
			src := filepath.Clean(strings.TrimPrefix(layer.Source, "file://"))
			// Reject traversal sequences so a crafted lockfile cannot read
			// arbitrary host files (e.g. file://../../../etc/passwd).
			for _, part := range strings.Split(src, string(filepath.Separator)) {
				if part == ".." {
					return nil, fmt.Errorf("layer %q: file:// source contains path traversal: %q", layer.ID, layer.Source)
				}
			}
			if err := copyFile(src, cachePath); err != nil {
				return nil, fmt.Errorf("copying layer %q: %w", layer.ID, err)
			}
		} else {
			return nil, fmt.Errorf("layer %q: unsupported source scheme in %q", layer.ID, layer.Source)
		}

		// Verify SHA256 after download.
		if layer.SHA256 != "" {
			actual, err := sha256File(cachePath)
			if err != nil {
				return nil, fmt.Errorf("hashing layer %q: %w", layer.ID, err)
			}
			if actual != layer.SHA256 {
				os.Remove(cachePath) //nolint:errcheck
				return nil, fmt.Errorf("layer %q SHA256 mismatch: manifest=%q file=%q", layer.ID, layer.SHA256, actual)
			}
		}

		paths = append(paths, overlay.LayerPath{
			ID:         layer.ID,
			SHA256:     layer.SHA256,
			Path:       cachePath,
			MountOrder: layer.MountOrder,
		})
	}
	return paths, nil
}

// downloadS3Layer downloads an S3 URI to destPath atomically.
func downloadS3Layer(ctx context.Context, client *awss3.Client, uri, destPath string) error {
	bucket, key, ok := parseS3URIRun(uri)
	if !ok {
		return fmt.Errorf("invalid S3 URI %q", uri)
	}

	out, err := client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return fmt.Errorf("S3 GetObject %q: %w", uri, err)
	}
	defer out.Body.Close() //nolint:errcheck

	// Write atomically: temp file then rename.
	dir := filepath.Dir(destPath)
	tmp, err := os.CreateTemp(dir, "*.sqfs.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := io.Copy(tmp, out.Body); err != nil {
		tmp.Close()        //nolint:errcheck
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("writing layer: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("closing temp file: %w", err)
	}
	return os.Rename(tmpPath, destPath)
}

// copyFile copies src to dest atomically.
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck

	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, "*.sqfs.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()        //nolint:errcheck
		os.Remove(tmpPath) //nolint:errcheck
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return err
	}
	return os.Rename(tmpPath, dest)
}

// sha256File computes the hex-encoded SHA256 of the named file.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// parseS3URIRun parses "s3://bucket/key" → (bucket, key, true).
func parseS3URIRun(uri string) (bucket, key string, ok bool) {
	const prefix = "s3://"
	if !strings.HasPrefix(uri, prefix) {
		return "", "", false
	}
	rest := uri[len(prefix):]
	idx := strings.IndexByte(rest, '/')
	if idx < 0 {
		return rest, "", true
	}
	return rest[:idx], rest[idx+1:], true
}

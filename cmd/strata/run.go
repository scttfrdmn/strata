package main

import (
	"context"
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
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

func newRunCmd() *cobra.Command {
	var lockfilePath, cacheDir, keyRef string
	var noVerify bool
	var envOverrides []string

	cmd := &cobra.Command{
		Use:   "run --lockfile <lock.yaml> [--no-verify] [--cache-dir DIR] [--env KEY=VAL] -- <command> [args...]",
		Short: "Run a command inside a Strata environment",
		Long: `Mount the layers described by a lockfile and run a command inside the
resulting environment. Works for both privileged (OverlayFS) and
unprivileged (FUSE) contexts.

Layer files are cached in the user cache directory and reused on
subsequent runs.

Every layer is verified before anything is mounted: its bundle must be
readable, parse, carry a Rekor entry, and attest the digest the lockfile
pins, and its signature must verify with cosign against --key. A layer
that fails any of those is not mounted.

Pass --no-verify to mount unverified layers on air-gapped systems. It
reports, on stderr, that verification was skipped — before this was a
flag that disabled nothing, because nothing was enabled.`,
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
			return runRun(cmd.Context(), lockfilePath, args, noVerify, cacheDir, keyRef, envOverrides)
		},
	}

	cmd.Flags().StringVar(&lockfilePath, "lockfile", "", "path to the lockfile (required)")
	cmd.Flags().BoolVar(&noVerify, "no-verify", false, "mount layers without verifying their signatures (use on air-gapped systems)")
	cmd.Flags().StringVar(&keyRef, "key", "", "cosign public key for layer signature verification (required unless --no-verify)")
	cmd.Flags().StringVar(&cacheDir, "cache-dir", "", "layer cache directory (default: ~/.cache/strata/layers)")
	cmd.Flags().StringArrayVar(&envOverrides, "env", nil, "additional environment variables (KEY=VAL)")
	return cmd
}

func runRun(ctx context.Context, lockfilePath string, args []string, noVerify bool, cacheDir, keyRef string, envOverrides []string) error {
	// 1. Read lockfile.
	data, err := os.ReadFile(lockfilePath)
	if err != nil {
		return fmt.Errorf("run: reading lockfile: %w", err)
	}
	var lf spec.LockFile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return fmt.Errorf("run: parsing lockfile: %w", err)
	}

	// 2. Signature verification happens in step 6, after the layers are on disk:
	//    the bytes about to be mounted are what has to be verified, and a
	//    lockfile field is not evidence about a file. This used to be a warning
	//    here that fired only on an empty RekorEntry, which is what made
	//    --no-verify a flag that disabled nothing (#55).
	if lf.RekorEntry == "" {
		fmt.Fprintln(os.Stderr, "run: warning: lockfile has no Rekor entry — not signed") //nolint:errcheck
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

	// 6. Verify what is about to be mounted, and refuse to mount it otherwise.
	if err := verifyRunLayers(ctx, &lf, layerPaths, noVerify, keyRef); err != nil {
		return err
	}

	// 7. Create temp working directory.
	workDir, err := os.MkdirTemp("", fmt.Sprintf("strata-%d-*", os.Getpid()))
	if err != nil {
		return fmt.Errorf("run: creating work dir: %w", err)
	}
	defer os.RemoveAll(workDir) //nolint:errcheck

	// 8. Mount overlay with user-local paths and auto-detected strategy.
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

	// 9. Build environment for the child process.
	env := buildRunEnv(&lf, ov.MergedPath, envOverrides)

	// 10. Execute the command.
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

// runLayerCheck is one layer that passed every check not needing cosign, held
// with its parsed bundle so the signature step does not re-read or re-parse it.
type runLayerCheck struct {
	layerID string
	sqfs    string
	bundle  *trust.Bundle
}

// verifyRunLayers refuses to let runRun proceed unless every layer about to be
// mounted verifies. It reports all failures rather than the first, because a
// user fixing a lockfile wants the whole list.
//
// noVerify skips the whole step and says so on stderr. That announcement is
// load-bearing: the defect this replaces (#55) was a --no-verify flag that
// disabled nothing, and a silent skip is indistinguishable from a check that
// ran and passed.
func verifyRunLayers(ctx context.Context, lf *spec.LockFile, paths []overlay.LayerPath, noVerify bool, keyRef string) error {
	if noVerify {
		fmt.Fprintln(os.Stderr, //nolint:errcheck
			"run: warning: --no-verify given: layer signatures were NOT verified; mounting unverified content")
		return nil
	}

	failures, checks := collectRunBundleFailures(lf, paths)

	// The signature step runs only when every structural check passed, matching
	// strata verify's shape (verify.go: presence failures gate the Rekor step).
	// Ordering matters for more than tidiness: the structural checks need no
	// cosign, so they stay reachable — and testable — on a machine that has
	// none, while a missing cosign would otherwise mask every bundle defect
	// behind one prerequisite error.
	if len(failures) == 0 {
		v, err := newRunVerifier(keyRef)
		if err != nil {
			failures = append(failures, err.Error())
		} else {
			failures = append(failures, verifyRunSignatures(ctx, checks, v)...)
		}
	}

	if len(failures) == 0 {
		return nil
	}

	fmt.Fprintf(os.Stderr, "run: %d verification failure(s):\n", len(failures)) //nolint:errcheck
	for _, f := range failures {
		fmt.Fprintf(os.Stderr, "  - %s\n", f) //nolint:errcheck
	}
	return errors.New("run: refusing to mount unverified layers (pass --no-verify to mount them anyway)")
}

// collectRunBundleFailures performs every per-layer check that does not need
// cosign, and returns the layers that still need a signature check.
//
// This discharges trust.VerifyLayer's steps in a different order rather than
// calling it, so each of that function's guarantees is accounted for here
// explicitly:
//
//   - step 1, file digest vs manifest: already done, unconditionally, by
//     fetchLayersToCache — which will not return a path it has not hashed
//     against a well-formed digest (#57). Re-hashing here would hash the same
//     squashfs a third time for no new information.
//   - step 2, bundle non-empty / not a bare URI / readable / parses: done here
//     via loadLocalBundle, the same helper strata verify --rekor uses.
//   - step 2, HasRekorEntry: done here.
//   - step 3, cosign signature: done by verifyRunSignatures.
//
// One check here is *not* in trust.VerifyLayer: the bundle must attest the
// digest this lockfile pins. VerifyLayer can omit it because cosign verify-blob
// ties bundle to artifact itself; this path cannot, because a missing cosign
// must not be the difference between "wrong layer's bundle" and "accepted".
// Without it, a valid bundle for a different artifact passes every local check.
func collectRunBundleFailures(lf *spec.LockFile, paths []overlay.LayerPath) ([]string, []runLayerCheck) {
	pathByID := make(map[string]string, len(paths))
	for _, p := range paths {
		pathByID[p.ID] = p.Path
	}

	var (
		failures []string
		checks   []runLayerCheck
	)
	for _, layer := range lf.Layers {
		sqfs, ok := pathByID[layer.ID]
		if !ok {
			failures = append(failures, fmt.Sprintf(
				"layer %s: no fetched file to verify", layer.ID))
			continue
		}

		bundle, err := loadLocalBundle(layer.Bundle)
		if err != nil {
			failures = append(failures, fmt.Sprintf("layer %s: %v", layer.ID, err))
			continue
		}
		if !bundle.HasRekorEntry() {
			failures = append(failures, fmt.Sprintf(
				"layer %s: bundle has no Rekor entry: unsigned layers will not mount", layer.ID))
			continue
		}
		if alg := bundle.MessageSignature.MessageDigest.Algorithm; alg != "" && !isBundleSHA256(alg) {
			failures = append(failures, fmt.Sprintf(
				"layer %s: bundle digest algorithm is %q, not SHA-256", layer.ID, alg))
			continue
		}
		attested := hex.EncodeToString(bundle.MessageSignature.MessageDigest.Digest)
		if !strings.EqualFold(attested, layer.SHA256) {
			failures = append(failures, fmt.Sprintf(
				"layer %s: bundle attests sha256:%s but the lockfile pins sha256:%s",
				layer.ID, attested, layer.SHA256))
			continue
		}

		checks = append(checks, runLayerCheck{layerID: layer.ID, sqfs: sqfs, bundle: bundle})
	}
	return failures, checks
}

// isBundleSHA256 reports whether a Sigstore digest algorithm name means SHA-256.
// Bundles write "SHA2_256"; be lenient about the spellings seen in the wild
// rather than rejecting a correct bundle over punctuation.
func isBundleSHA256(alg string) bool {
	switch strings.ToUpper(strings.NewReplacer("-", "", "_", "").Replace(alg)) {
	case "SHA2256", "SHA256":
		return true
	}
	return false
}

// verifyRunSignatures checks each layer's signature with v.
//
// v is a parameter so a test can drive the pass and fail directions without
// cosign on PATH; runRun supplies a real CosignVerifier. There is deliberately
// no nil-means-skip case — that shape is the defect filed as #56.
func verifyRunSignatures(ctx context.Context, checks []runLayerCheck, v trust.Verifier) []string {
	var failures []string
	for _, c := range checks {
		if err := v.Verify(ctx, c.sqfs, c.bundle); err != nil {
			failures = append(failures, fmt.Sprintf(
				"layer %s: signature verification failed: %v", c.layerID, err))
		}
	}
	return failures
}

// newRunVerifier builds the cosign verifier for strata run.
//
// Both prerequisites are errors rather than a nil verifier: strata run is used
// where cosign is often absent, which is exactly the case that must not
// silently downgrade to mounting unverified content. The message names the flag
// that makes the trade-off explicit instead of making it by default.
func newRunVerifier(keyRef string) (trust.Verifier, error) {
	if keyRef == "" {
		return nil, errors.New("no --key given: layer signatures cannot be verified without a cosign public key (pass --key, or --no-verify to mount unverified layers)")
	}
	if _, err := os.Stat(keyRef); err != nil {
		return nil, fmt.Errorf("--key %q is not readable: layer signatures cannot be verified (fix the path, or pass --no-verify to mount unverified layers): %w", keyRef, err)
	}
	if _, err := exec.LookPath("cosign"); err != nil {
		return nil, fmt.Errorf("cosign not found on PATH: layer signatures cannot be verified (install cosign, or pass --no-verify to mount unverified layers): %w", err)
	}
	return &trust.CosignVerifier{KeyRef: keyRef}, nil
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

// fetchLayersToCache downloads all layers in the lockfile to cacheDir and
// returns their paths. Layers already in cache are reused.
//
// The invariant is single: nothing is returned from here that has not been
// hashed against a well-formed declared digest. That means the digest is
// validated before it is used to build a path, a cache hit is hashed before it
// is reused, and a layer with no digest is rejected rather than passed through
// unverified. Callers (strata run, strata export, strata fold) hand these paths
// straight to overlay assembly, so a path returned from here is content the
// process is about to execute.
func fetchLayersToCache(ctx context.Context, lf spec.LockFile, cacheDir string) ([]overlay.LayerPath, error) {
	if len(lf.Layers) == 0 {
		return nil, nil
	}

	// Lazily initialise S3 client — only needed if any layer is not cached.
	var s3Client *awss3.Client

	paths := make([]overlay.LayerPath, 0, len(lf.Layers))
	for _, layer := range lf.Layers {
		// Validate the digest before anything is built from it. filepath.Join
		// Cleans rather than rejects, so an unvalidated digest of "../x" names a
		// file outside cacheDir, and an empty one names the same ".sqfs" for
		// every hashless layer in every lockfile.
		cachePath, err := spec.LayerCachePath(cacheDir, layer.SHA256)
		if err != nil {
			return nil, fmt.Errorf("layer %q: %w", layer.ID, err)
		}

		// Cache hit — hash it. The filename is not evidence of the contents:
		// the cache directory is shared, long-lived, and writable by whatever
		// else runs as this user, so a file at a well-formed name still has to
		// prove it is the layer the lockfile asked for.
		if _, statErr := os.Stat(cachePath); statErr == nil {
			if err := spec.VerifyFileDigest(cachePath, layer.SHA256); err != nil {
				return nil, fmt.Errorf("cached layer %q: %w (remove the file to re-download it)", layer.ID, err)
			}
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

		// Verify SHA256 after download. Unconditional: the digest was validated
		// above, so there is no hashless case left to skip.
		if err := spec.VerifyFileDigest(cachePath, layer.SHA256); err != nil {
			os.Remove(cachePath) //nolint:errcheck
			return nil, fmt.Errorf("layer %q: %w", layer.ID, err)
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

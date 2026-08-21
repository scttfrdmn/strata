package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// Tests for strata run's pre-mount layer verification (#55).
//
// The defect: --no-verify disabled nothing, because nothing was enabled. So the
// obligation on these tests is not "the happy path still works" — it is that
// material a correct implementation must reject is actually rejected, and
// rejected for the reason under test rather than incidentally. Every negative
// case below therefore asserts exactly one failure and the text of it; a row
// that failed for an earlier check would report the wrong reason and fail here.

// writeBundleFile writes a bundle to a temp file and returns a file:// URI for
// it, which is what a fetched lockfile carries.
func writeBundleFile(t *testing.T, b *trust.Bundle) string {
	t.Helper()
	data, err := b.Marshal()
	if err != nil {
		t.Fatalf("Marshal bundle: %v", err)
	}
	path := filepath.Join(t.TempDir(), "bundle.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	return "file://" + path
}

// bundleAttesting builds a well-formed bundle that attests digest and carries
// sig as its signature. digest and sig are supplied by the caller so no test
// derives its expectation from the code under test.
func bundleAttesting(digest, sig []byte) *trust.Bundle {
	return &trust.Bundle{
		MediaType: trust.BundleMediaType,
		VerificationMaterial: trust.VerificationMaterial{
			PublicKey:   &trust.RawMaterial{Hint: "test-key"},
			TlogEntries: []trust.TlogEntry{{LogIndex: "1234", IntegratedTime: "1700000000"}},
		},
		MessageSignature: trust.MessageSignature{
			MessageDigest: trust.MessageDigest{Algorithm: "SHA2_256", Digest: digest},
			Signature:     sig,
		},
	}
}

// squashfsStub writes a stand-in for a layer file and returns its path and the
// hex SHA256 of its contents, hashed here rather than by anything under test.
func squashfsStub(t *testing.T, contents string) (path, hexDigest string) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "layer.sqfs")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("write layer stub: %v", err)
	}
	sum := sha256.Sum256([]byte(contents))
	return path, hex.EncodeToString(sum[:])
}

func layerWith(id, sha256hex, bundleURI string) spec.ResolvedLayer {
	return spec.ResolvedLayer{
		LayerManifest: spec.LayerManifest{
			ID:         id,
			Name:       id,
			Version:    "1.0.0",
			SHA256:     sha256hex,
			Bundle:     bundleURI,
			RekorEntry: "1234",
		},
		MountOrder: 1,
	}
}

// TestCollectRunBundleFailures_MustReject drives the checks that need no cosign.
//
// Each row is otherwise valid apart from the one defect named, so the asserted
// reason is evidence that this check fired and not a neighbouring one. The final
// row is the accept case: without it, a function that rejected everything would
// pass every other row here.
func TestCollectRunBundleFailures_MustReject(t *testing.T) {
	sqfs, goodDigestHex := squashfsStub(t, "layer contents under test")
	goodDigest, err := hex.DecodeString(goodDigestHex)
	if err != nil {
		t.Fatalf("decode digest: %v", err)
	}

	// A digest of different bytes: a bundle that is internally valid but
	// attests some other artifact. Computed here, from a different input, so
	// the mismatch is a property of the fixture and not of the comparison.
	otherSum := sha256.Sum256([]byte("some entirely different artifact"))
	otherDigest := otherSum[:]

	goodBundleURI := writeBundleFile(t, bundleAttesting(goodDigest, []byte("sig")))

	noRekor := bundleAttesting(goodDigest, []byte("sig"))
	noRekor.VerificationMaterial.TlogEntries = nil
	noRekorURI := writeBundleFile(t, noRekor)

	wrongAlg := bundleAttesting(goodDigest, []byte("sig"))
	wrongAlg.MessageSignature.MessageDigest.Algorithm = "SHA2_512"
	wrongAlgURI := writeBundleFile(t, wrongAlg)

	otherArtifactURI := writeBundleFile(t, bundleAttesting(otherDigest, []byte("sig")))

	notJSON := filepath.Join(t.TempDir(), "bundle.json")
	if err := os.WriteFile(notJSON, []byte("this is not a sigstore bundle"), 0600); err != nil {
		t.Fatalf("write malformed bundle: %v", err)
	}

	tests := []struct {
		name string
		// mutate adjusts the otherwise-valid layer to introduce one defect.
		mutate func(l *spec.ResolvedLayer)
		// omitPath drops the layer from the fetched-paths list.
		omitPath   bool
		wantReason string
		wantChecks int
	}{
		{
			name:       "bundle field empty",
			mutate:     func(l *spec.ResolvedLayer) { l.Bundle = "" },
			wantReason: "empty Bundle field",
		},
		{
			name:       "bundle still an s3 URI",
			mutate:     func(l *spec.ResolvedLayer) { l.Bundle = "s3://strata-registry/bundles/x.json" },
			wantReason: "must be fetched to disk",
		},
		{
			name:       "bundle file missing",
			mutate:     func(l *spec.ResolvedLayer) { l.Bundle = "file:///nonexistent/strata/bundle.json" },
			wantReason: "reading bundle",
		},
		{
			name:       "bundle does not parse",
			mutate:     func(l *spec.ResolvedLayer) { l.Bundle = "file://" + notJSON },
			wantReason: "parsing bundle",
		},
		{
			name:       "bundle has no Rekor entry",
			mutate:     func(l *spec.ResolvedLayer) { l.Bundle = noRekorURI },
			wantReason: "no Rekor entry",
		},
		{
			name:       "bundle digest is not SHA-256",
			mutate:     func(l *spec.ResolvedLayer) { l.Bundle = wrongAlgURI },
			wantReason: "not SHA-256",
		},
		{
			// The check trust.VerifyLayer does not perform: a valid bundle for
			// a different artifact. Without this, substituting any signed
			// layer's bundle passes every local check.
			name:       "bundle attests a different artifact",
			mutate:     func(l *spec.ResolvedLayer) { l.Bundle = otherArtifactURI },
			wantReason: "bundle attests sha256:",
		},
		{
			name:       "no fetched file for the layer",
			omitPath:   true,
			wantReason: "no fetched file to verify",
		},
		{
			name:       "valid layer is accepted",
			wantChecks: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layer := layerWith("python-3.11", goodDigestHex, goodBundleURI)
			if tt.mutate != nil {
				tt.mutate(&layer)
			}
			lf := &spec.LockFile{Layers: []spec.ResolvedLayer{layer}}

			var paths []overlay.LayerPath
			if !tt.omitPath {
				paths = []overlay.LayerPath{{
					ID: layer.ID, SHA256: layer.SHA256, Path: sqfs, MountOrder: 1,
				}}
			}

			failures, checks := collectRunBundleFailures(lf, paths)

			if tt.wantReason == "" {
				if len(failures) != 0 {
					t.Fatalf("want no failures, got %v", failures)
				}
				if len(checks) != tt.wantChecks {
					t.Fatalf("want %d layer(s) passed to the signature step, got %d",
						tt.wantChecks, len(checks))
				}
				return
			}

			if len(failures) != 1 {
				t.Fatalf("want exactly 1 failure (so the reason is unambiguous), got %d: %v",
					len(failures), failures)
			}
			if !strings.Contains(failures[0], tt.wantReason) {
				t.Errorf("failure fired for the wrong reason:\n got: %s\nwant substring: %s",
					failures[0], tt.wantReason)
			}
			if len(checks) != 0 {
				t.Errorf("a rejected layer reached the signature step: %+v", checks)
			}
		})
	}
}

// TestVerifyRunSignatures uses trust.FakeVerifier, whose accept/reject decision
// is a real property of the input (signature must equal the hex SHA256 of the
// artifact) rather than a flag on a stub. A stub told to fail would prove only
// that the error is propagated.
func TestVerifyRunSignatures(t *testing.T) {
	sqfs, digestHex := squashfsStub(t, "signed layer contents")
	digest, err := hex.DecodeString(digestHex)
	if err != nil {
		t.Fatalf("decode digest: %v", err)
	}

	good := bundleAttesting(digest, []byte(digestHex))
	bad := bundleAttesting(digest, []byte("0000000000000000000000000000000000000000000000000000000000000000"))

	t.Run("valid signature passes", func(t *testing.T) {
		checks := []runLayerCheck{{layerID: "python-3.11", sqfs: sqfs, bundle: good}}
		if failures := verifyRunSignatures(context.Background(), checks, &trust.FakeVerifier{}); len(failures) != 0 {
			t.Fatalf("want no failures, got %v", failures)
		}
	})

	t.Run("wrong signature is rejected", func(t *testing.T) {
		checks := []runLayerCheck{{layerID: "python-3.11", sqfs: sqfs, bundle: bad}}
		failures := verifyRunSignatures(context.Background(), checks, &trust.FakeVerifier{})
		if len(failures) != 1 {
			t.Fatalf("want 1 failure, got %d: %v", len(failures), failures)
		}
		if !strings.Contains(failures[0], "signature verification failed") {
			t.Errorf("wrong reason: %s", failures[0])
		}
		if !strings.Contains(failures[0], "python-3.11") {
			t.Errorf("failure does not name the layer: %s", failures[0])
		}
	})
}

// TestNewRunVerifier_NeverReturnsANilVerifier pins the property that makes this
// path different from the agent's (#56): every prerequisite failure is an error,
// never a nil Verifier. trust.VerifyLayer and verifyRunSignatures both call
// v.Verify unconditionally, so a nil verifier is not a skip — it is a panic, and
// the shape that produces one is the fail-open being repaired.
func TestNewRunVerifier_NeverReturnsANilVerifier(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "cosign.pub")
	if err := os.WriteFile(keyPath, []byte("-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----\n"), 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	tests := []struct {
		name       string
		keyRef     string
		wantReason string
	}{
		{name: "no key given", keyRef: "", wantReason: "no --key given"},
		{name: "key not readable", keyRef: filepath.Join(t.TempDir(), "absent.pub"), wantReason: "is not readable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := newRunVerifier(tt.keyRef)
			if err == nil {
				t.Fatalf("want an error, got verifier %+v", v)
			}
			if v != nil {
				t.Errorf("returned a verifier alongside an error: %+v", v)
			}
			if !strings.Contains(err.Error(), tt.wantReason) {
				t.Errorf("wrong reason:\n got: %v\nwant substring: %s", err, tt.wantReason)
			}
			if !strings.Contains(err.Error(), "--no-verify") {
				t.Errorf("error does not name the flag that makes the trade-off explicit: %v", err)
			}
		})
	}

	// With a readable key the result depends on whether cosign is installed,
	// which differs between a developer machine and CI. Both outcomes are
	// asserted, because the property under test is the invariant that holds
	// either way: exactly one of (verifier, error) is non-nil, and it is never
	// (nil, nil).
	t.Run("readable key: never nil verifier with nil error", func(t *testing.T) {
		v, err := newRunVerifier(keyPath)
		switch {
		case err != nil && v != nil:
			t.Errorf("both verifier and error returned: %+v / %v", v, err)
		case err != nil:
			if !strings.Contains(err.Error(), "cosign not found on PATH") {
				t.Errorf("unexpected error for a readable key: %v", err)
			}
		case v == nil:
			t.Error("returned (nil, nil): a nil Verifier is a panic in verifyRunSignatures, not a skip")
		}
	})
}

// captureStderr swaps os.Stderr for a pipe for the duration of fn.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stderr
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				break
			}
		}
		done <- sb.String()
	}()
	fn()
	os.Stderr = saved
	w.Close() //nolint:errcheck
	out := <-done
	r.Close() //nolint:errcheck
	return out
}

// TestVerifyRunLayers_NoVerifyAnnouncesTheSkip is the assertion that keeps
// --no-verify honest in the other direction. Returning nil is not enough: a
// silent skip is indistinguishable from a check that ran and passed, which is
// the condition this issue was filed about.
func TestVerifyRunLayers_NoVerifyAnnouncesTheSkip(t *testing.T) {
	// Deliberately unverifiable material: an empty bundle field. With
	// noVerify the function must not look at it at all.
	lf := &spec.LockFile{Layers: []spec.ResolvedLayer{layerWith("python-3.11", strings.Repeat("a", 64), "")}}

	var err error
	stderr := captureStderr(t, func() {
		err = verifyRunLayers(context.Background(), lf, nil, true, "")
	})
	if err != nil {
		t.Fatalf("--no-verify must not fail: %v", err)
	}
	if !strings.Contains(stderr, "NOT verified") {
		t.Errorf("--no-verify skipped verification silently; stderr was: %q", stderr)
	}
}

// TestVerifyRunLayers_ReportsEveryFailure: a user fixing a lockfile wants the
// whole list, and reporting only the first would make a two-layer problem take
// two runs to see.
func TestVerifyRunLayers_ReportsEveryFailure(t *testing.T) {
	lf := &spec.LockFile{Layers: []spec.ResolvedLayer{
		layerWith("python-3.11", strings.Repeat("a", 64), ""),
		layerWith("openmpi-5.0", strings.Repeat("b", 64), "s3://strata-registry/b.json"),
	}}
	paths := []overlay.LayerPath{
		{ID: "python-3.11", Path: "/nonexistent/a.sqfs", MountOrder: 1},
		{ID: "openmpi-5.0", Path: "/nonexistent/b.sqfs", MountOrder: 2},
	}

	var err error
	stderr := captureStderr(t, func() {
		err = verifyRunLayers(context.Background(), lf, paths, false, "")
	})
	if err == nil {
		t.Fatal("want a refusal, got nil")
	}
	if !strings.Contains(err.Error(), "refusing to mount unverified layers") {
		t.Errorf("wrong error: %v", err)
	}
	for _, want := range []string{"2 verification failure(s)", "python-3.11", "openmpi-5.0"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q; got: %q", want, stderr)
		}
	}
}

// writeRunFixture writes a layer file, a bundle attesting bundleDigestHex, and a
// lockfile pointing at both. Returns the lockfile path and the cache dir.
//
// The layer digest the lockfile pins always matches the file, so
// fetchLayersToCache succeeds and execution reaches the verification step: a
// test that failed at the fetch would prove nothing about this change.
func writeRunFixture(t *testing.T, bundleDigestHex string) (lockPath, cacheDir string) {
	t.Helper()
	dir := t.TempDir()

	contents := "squashfs stand-in for run fixture"
	layerPath := filepath.Join(dir, "python.sqfs")
	if err := os.WriteFile(layerPath, []byte(contents), 0600); err != nil {
		t.Fatalf("write layer: %v", err)
	}
	sum := sha256.Sum256([]byte(contents))
	layerHex := hex.EncodeToString(sum[:])

	bundleDigest, err := hex.DecodeString(bundleDigestHex)
	if err != nil {
		t.Fatalf("decode bundle digest: %v", err)
	}
	bundleData, err := bundleAttesting(bundleDigest, []byte("sig")).Marshal()
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	bundlePath := filepath.Join(dir, "python.bundle.json")
	if err := os.WriteFile(bundlePath, bundleData, 0600); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	lf := spec.LockFile{
		ProfileName: "run-verify-fixture",
		RekorEntry:  "999",
		Layers: []spec.ResolvedLayer{{
			LayerManifest: spec.LayerManifest{
				ID:         "python-3.11",
				Name:       "python",
				Version:    "3.11.9",
				Source:     "file://" + layerPath,
				SHA256:     layerHex,
				Bundle:     "file://" + bundlePath,
				RekorEntry: "1234",
			},
			MountOrder: 1,
		}},
	}
	data, err := yaml.Marshal(lf)
	if err != nil {
		t.Fatalf("marshal lockfile: %v", err)
	}
	lockPath = filepath.Join(dir, "lock.yaml")
	if err := os.WriteFile(lockPath, data, 0600); err != nil {
		t.Fatalf("write lockfile: %v", err)
	}
	return lockPath, filepath.Join(dir, "cache")
}

// TestRunRun_RefusesLayerWhoseBundleAttestsAnotherArtifact drives runRun itself,
// not the helper.
//
// This distinction is the point of the test. Extracting verifyRunLayers and
// testing that in isolation would leave runRun at zero coverage — the command
// could stop calling it and every other test here would still pass.
//
// The discriminating assertion is that the error is the verification refusal and
// NOT the mount error that follows it. Against the unfixed code this test fails:
// verification did not happen, so runRun proceeded to mounting and returned
// "mounting overlay". Asserting only err != nil would have passed there too.
func TestRunRun_RefusesLayerWhoseBundleAttestsAnotherArtifact(t *testing.T) {
	otherSum := sha256.Sum256([]byte("a different artifact entirely"))
	lockPath, cacheDir := writeRunFixture(t, hex.EncodeToString(otherSum[:]))

	var err error
	stderr := captureStderr(t, func() {
		err = runRun(context.Background(), lockPath,
			[]string{"/nonexistent/strata-run-verify-probe"}, false, cacheDir, "", nil)
	})

	if err == nil {
		t.Fatal("runRun mounted a layer whose bundle attests a different artifact")
	}
	if !strings.Contains(err.Error(), "refusing to mount unverified layers") {
		t.Fatalf("runRun failed for the wrong reason — verification did not gate the mount:\n%v", err)
	}
	if strings.Contains(err.Error(), "mounting overlay") {
		t.Errorf("execution reached the mount despite a failed verification: %v", err)
	}
	if !strings.Contains(stderr, "bundle attests sha256:") {
		t.Errorf("stderr does not name the check that fired; got: %q", stderr)
	}
}

// TestRunRun_NoVerifyReachesTheMount is the other half: it proves --no-verify
// now controls something. The pair is what closes #55 — before the fix both
// directions behaved identically, because verification never ran either way.
//
// Mounting a stand-in squashfs cannot succeed in a test environment, so the
// assertion is not on success but on which failure: anything other than the
// verification refusal means execution got past the gate.
func TestRunRun_NoVerifyReachesTheMount(t *testing.T) {
	otherSum := sha256.Sum256([]byte("a different artifact entirely"))
	lockPath, cacheDir := writeRunFixture(t, hex.EncodeToString(otherSum[:]))

	var err error
	stderr := captureStderr(t, func() {
		err = runRun(context.Background(), lockPath,
			[]string{"/nonexistent/strata-run-verify-probe"}, true, cacheDir, "", nil)
	})

	if err != nil && strings.Contains(err.Error(), "refusing to mount unverified layers") {
		t.Fatalf("--no-verify did not skip verification: %v", err)
	}
	if !strings.Contains(stderr, "NOT verified") {
		t.Errorf("--no-verify did not announce the skip; stderr was: %q", stderr)
	}
}

// TestRunRun_MissingKeyIsARefusalNotASkip: on a machine with no cosign key
// configured, the absence of the means to verify must stop the mount. This is
// the failure mode the codebase already has a precedent against — verification
// that cannot be performed is not verification that passed (verify.go).
func TestRunRun_MissingKeyIsARefusalNotASkip(t *testing.T) {
	// Bundle attests the real layer digest, so every structural check passes
	// and the only thing left is the signature — which cannot be checked
	// without a key.
	contents := "squashfs stand-in for run fixture"
	sum := sha256.Sum256([]byte(contents))
	lockPath, cacheDir := writeRunFixture(t, hex.EncodeToString(sum[:]))

	var err error
	stderr := captureStderr(t, func() {
		err = runRun(context.Background(), lockPath,
			[]string{"/nonexistent/strata-run-verify-probe"}, false, cacheDir, "", nil)
	})

	if err == nil || !strings.Contains(err.Error(), "refusing to mount unverified layers") {
		t.Fatalf("a structurally valid but unverifiable layer was mounted: %v", err)
	}
	if !strings.Contains(stderr, "no --key given") {
		t.Errorf("stderr does not say why verification could not be performed; got: %q", stderr)
	}
}

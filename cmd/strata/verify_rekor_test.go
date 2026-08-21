package main

// Call-site tests for `strata verify --rekor` (#59).
//
// The defect being fixed is on the trust side, but it had a call-site half: this
// function used to construct its own *trust.RekorHTTPClient internally, so
// nothing could observe what it passed to VerifyEntry — which is exactly the
// property that let a discarded bundle go unnoticed. The client is now a
// parameter, and these tests assert what arrives through it.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// recordingRekor captures every VerifyEntry call so a test can assert on the
// arguments rather than only on the outcome.
type recordingRekor struct {
	mu    sync.Mutex
	calls []recordedCall
	err   error // returned from VerifyEntry
}

type recordedCall struct {
	logIndex int64
	bundle   *trust.Bundle
}

func (r *recordingRekor) Log(context.Context, *trust.Bundle) (int64, error) {
	return 0, errors.New("not used")
}

func (r *recordingRekor) VerifyEntry(_ context.Context, logIndex int64, bundle *trust.Bundle) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recordedCall{logIndex: logIndex, bundle: bundle})
	return r.err
}

func (r *recordingRekor) recorded() []recordedCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedCall, len(r.calls))
	copy(out, r.calls)
	return out
}

// writeBundle writes a minimal parseable bundle to dir and returns its path.
func writeBundle(t *testing.T, dir, name string, logIndex string, digest, sig []byte) string {
	t.Helper()
	b := &trust.Bundle{MediaType: trust.BundleMediaType}
	b.VerificationMaterial.TlogEntries = []trust.TlogEntry{{LogIndex: logIndex}}
	b.MessageSignature.MessageDigest.Algorithm = "SHA2_256"
	b.MessageSignature.MessageDigest.Digest = digest
	b.MessageSignature.Signature = sig

	data, err := b.Marshal()
	if err != nil {
		t.Fatalf("marshalling bundle: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing bundle: %v", err)
	}
	return path
}

// TestVerifyRekorEntries_PassesTheBundle is the call-site half of #59: the
// bundle named by the layer must reach VerifyEntry. Before the fix this function
// passed no bundle at all, and the check could not fail for the right reason
// because it had nothing to compare.
func TestVerifyRekorEntries_PassesTheBundle(t *testing.T) {
	dir := t.TempDir()
	digest := []byte("32-bytes-of-digest-material-here")
	sig := []byte("signature")
	path := writeBundle(t, dir, "layer.bundle.json", "111", digest, sig)

	lf := &spec.LockFile{
		RekorEntry: "9",
		Layers: []spec.ResolvedLayer{{LayerManifest: spec.LayerManifest{
			ID:         "python-3.11.11",
			Bundle:     path,
			RekorEntry: "111",
		}}},
	}

	client := &recordingRekor{}
	failures := verifyRekorEntries(context.Background(), lf, client)
	if len(failures) != 0 {
		t.Fatalf("expected no failures, got %v", failures)
	}

	calls := client.recorded()
	if len(calls) != 1 {
		t.Fatalf("VerifyEntry called %d times, want 1", len(calls))
	}
	if calls[0].logIndex != 111 {
		t.Errorf("logIndex = %d, want 111", calls[0].logIndex)
	}
	if calls[0].bundle == nil {
		t.Fatal("VerifyEntry received a nil bundle: the call site is still asking the log " +
			"whether something was recorded rather than whether this bundle was")
	}
	if got := string(calls[0].bundle.MessageSignature.MessageDigest.Digest); got != string(digest) {
		t.Errorf("bundle digest = %q, want %q — a bundle arrived, but not this layer's", got, digest)
	}
}

// TestVerifyRekorEntries_FileURI covers the file:// form of the same path.
func TestVerifyRekorEntries_FileURI(t *testing.T) {
	dir := t.TempDir()
	path := writeBundle(t, dir, "b.json", "222", []byte("digest"), []byte("sig"))

	lf := &spec.LockFile{
		RekorEntry: "9",
		Layers: []spec.ResolvedLayer{{LayerManifest: spec.LayerManifest{
			ID: "gcc-13.2.0", Bundle: "file://" + path, RekorEntry: "222",
		}}},
	}

	client := &recordingRekor{}
	if failures := verifyRekorEntries(context.Background(), lf, client); len(failures) != 0 {
		t.Fatalf("expected no failures, got %v", failures)
	}
	if calls := client.recorded(); len(calls) != 1 || calls[0].bundle == nil {
		t.Fatalf("file:// bundle did not reach VerifyEntry: %+v", calls)
	}
}

// TestVerifyRekorEntries_UnfetchableBundleIsAFailure pins the decision that an
// s3:// bundle URI is a reported failure rather than a skipped check. Registry
// manifests carry s3:// URIs, so the tempting alternative — pass nil and let the
// Rekor call fall back to a presence check — would make `--rekor` report success
// for every layer in a realistic lockfile while verifying none of them.
//
// The assertion is not only that a failure is reported: it is that VerifyEntry is
// never reached. A verification that cannot be performed must not be attempted
// against no bundle.
func TestVerifyRekorEntries_UnfetchableBundleIsAFailure(t *testing.T) {
	lf := &spec.LockFile{
		RekorEntry: "9",
		Layers: []spec.ResolvedLayer{{LayerManifest: spec.LayerManifest{
			ID:         "python-3.11.11",
			Bundle:     "s3://strata-test-layers/python-3.11.11/bundle.json",
			RekorEntry: "333",
		}}},
	}

	client := &recordingRekor{}
	failures := verifyRekorEntries(context.Background(), lf, client)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d: %v", len(failures), failures)
	}
	if !strings.Contains(failures[0], "python-3.11.11") {
		t.Errorf("failure should name the layer: %q", failures[0])
	}
	if !strings.Contains(failures[0], "fetched to disk") {
		t.Errorf("failure should say what is missing: %q", failures[0])
	}
	if calls := client.recorded(); len(calls) != 0 {
		t.Errorf("VerifyEntry was called with %d bundle(s) despite there being none to load: %+v",
			len(calls), calls)
	}
}

// TestVerifyRekorEntries_MissingFileIsAFailure: a bundle path that does not exist
// is a failure naming the layer, not a pass.
func TestVerifyRekorEntries_MissingFileIsAFailure(t *testing.T) {
	lf := &spec.LockFile{
		RekorEntry: "9",
		Layers: []spec.ResolvedLayer{{LayerManifest: spec.LayerManifest{
			ID: "absent", Bundle: filepath.Join(t.TempDir(), "nope.json"), RekorEntry: "444",
		}}},
	}
	client := &recordingRekor{}
	failures := verifyRekorEntries(context.Background(), lf, client)
	if len(failures) != 1 || !strings.Contains(failures[0], "absent") {
		t.Fatalf("expected 1 failure naming the layer, got %v", failures)
	}
	if calls := client.recorded(); len(calls) != 0 {
		t.Errorf("VerifyEntry called despite an unreadable bundle: %+v", calls)
	}
}

// TestVerifyRekorEntries_ClientErrorSurfaces: the sentinel the trust package
// returns must reach the user's failure list, not be flattened to "failed".
func TestVerifyRekorEntries_ClientErrorSurfaces(t *testing.T) {
	dir := t.TempDir()
	path := writeBundle(t, dir, "b.json", "555", []byte("digest"), []byte("sig"))

	lf := &spec.LockFile{
		RekorEntry: "9",
		Layers: []spec.ResolvedLayer{{LayerManifest: spec.LayerManifest{
			ID: "mismatched", Bundle: path, RekorEntry: "555",
		}}},
	}
	client := &recordingRekor{err: trust.ErrDigestMismatch}
	failures := verifyRekorEntries(context.Background(), lf, client)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d: %v", len(failures), failures)
	}
	if !strings.Contains(failures[0], trust.ErrDigestMismatch.Error()) {
		t.Errorf("failure should carry the reason the entry was rejected: %q", failures[0])
	}
}

// TestVerifyRekorEntries_BadIndexIsNotVerified: a non-numeric RekorEntry is a
// failure and the client is never called with a nonsense index.
func TestVerifyRekorEntries_BadIndexIsNotVerified(t *testing.T) {
	lf := &spec.LockFile{
		RekorEntry: "9",
		Layers: []spec.ResolvedLayer{{LayerManifest: spec.LayerManifest{
			ID: "pending", Bundle: "irrelevant", RekorEntry: "pending-initial-build",
		}}},
	}
	client := &recordingRekor{}
	failures := verifyRekorEntries(context.Background(), lf, client)
	if len(failures) != 1 || !strings.Contains(failures[0], "not a valid log index") {
		t.Fatalf("expected a log-index failure, got %v", failures)
	}
	if calls := client.recorded(); len(calls) != 0 {
		t.Errorf("VerifyEntry called for an unparseable index: %+v", calls)
	}
}

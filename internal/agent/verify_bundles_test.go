package agent_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/scttfrdmn/strata/internal/agent"
	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// These tests cover the fully-configured shape: both Verifier and BundleFetcher
// non-nil, which is what cmd/strata-agent wires. The inherited tests in
// agent_test.go all leave both nil, so none of them reach verifyBundles past its
// first line and none of them can distinguish a refusal from a skip (#92).
//
// Every negative case here has a control that differs from it in exactly one
// respect — the bundle — so a green negative cannot be explained by the fixture
// failing an earlier check (SHA-256, fetch, mount) instead of verification.

// recordingMounter reports whether the mount step was reached. FakeMounter does
// not record its calls, and "Run returned an error" does not distinguish
// refusing before the mount from mounting and failing afterwards.
type recordingMounter struct {
	mu     sync.Mutex
	called bool
}

func (m *recordingMounter) Mount(_ []overlay.LayerPath) (*overlay.Overlay, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.called = true
	return &overlay.Overlay{MergedPath: "/strata/env"}, nil
}

func (m *recordingMounter) wasCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.called
}

// mapBundleFetcher serves bundle bytes by layer ID and records which layers it
// was asked about. A layer ID present in Bytes with a nil value models the
// shipped s3LayerFetcher, which returns (nil, nil) rather than an error.
type mapBundleFetcher struct {
	Bytes map[string][]byte

	mu     sync.Mutex
	asked  []string
	allNil bool // when true, return (nil, nil) for every layer
}

func (f *mapBundleFetcher) FetchBundleJSON(_ context.Context, layer spec.ResolvedLayer) ([]byte, error) {
	f.mu.Lock()
	f.asked = append(f.asked, layer.ID)
	f.mu.Unlock()
	if f.allNil {
		return nil, nil
	}
	return f.Bytes[layer.ID], nil
}

func (f *mapBundleFetcher) askedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.asked)
}

// countingVerifier delegates to trust.FakeVerifier and counts calls, so a
// passing control can assert that verification actually ran rather than that
// nothing objected.
type countingVerifier struct {
	inner trust.FakeVerifier

	mu    sync.Mutex
	calls int
}

func (v *countingVerifier) Verify(ctx context.Context, artifactPath string, bundle *trust.Bundle) error {
	v.mu.Lock()
	v.calls++
	v.mu.Unlock()
	return v.inner.Verify(ctx, artifactPath, bundle)
}

func (v *countingVerifier) callCount() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.calls
}

// signedBundleJSON returns marshalled bundle bytes that trust.FakeVerifier
// accepts for the file at path.
func signedBundleJSON(t *testing.T, path string) []byte {
	t.Helper()
	signer := &trust.FakeSigner{NextLogIndex: 1}
	bundle, err := signer.Sign(context.Background(), path, nil)
	if err != nil {
		t.Fatalf("FakeSigner.Sign(%q): %v", path, err)
	}
	data, err := bundle.Marshal()
	if err != nil {
		t.Fatalf("Bundle.Marshal: %v", err)
	}
	return data
}

// TestVerifyBundles_ValidBundleMounts is the control for every refusal test
// below. The fixture is identical apart from the bundle: same layer content,
// same matching SHA-256, same wiring. It establishes that these inputs reach
// the mount step when — and only when — verification succeeds, so a refusal
// elsewhere in this file cannot be attributed to the fixture.
func TestVerifyBundles_ValidBundleMounts(t *testing.T) {
	ctx := context.Background()

	layer, path := makeLayer(t, "python-3.11", []byte("squashfs content alpha"), 1)
	layer.Bundle = "s3://strata-registry/bundles/python-3.11.json"

	lf := &spec.LockFile{ProfileName: "ml-env", Layers: []spec.ResolvedLayer{layer}}

	signaler := &agent.FakeReadySignaler{}
	mounter := &recordingMounter{}
	verifier := &countingVerifier{}
	fetcher := &mapBundleFetcher{Bytes: map[string][]byte{
		layer.ID: signedBundleJSON(t, path),
	}}

	a := newAgent(t, agent.Config{
		Source:        &agent.FakeLockfileSource{Lockfile: lf},
		Fetcher:       &agent.FakeLayerFetcher{Paths: map[string]string{layer.ID: path}},
		BundleFetcher: fetcher,
		Verifier:      verifier,
		Signaler:      signaler,
		Mounter:       mounter,
	})

	if _, err := a.Run(ctx); err != nil {
		t.Fatalf("Run with a valid bundle: %v", err)
	}
	if verifier.callCount() != 1 {
		t.Errorf("Verify call count = %d, want 1 — the control must actually verify", verifier.callCount())
	}
	if !mounter.wasCalled() {
		t.Error("Mount was not reached on the passing control")
	}
	if !signaler.ReadyCalled {
		t.Error("SignalReady was not called on the passing control")
	}
	if signaler.FailedCalled {
		t.Errorf("SignalFailed was called on the passing control: %v", signaler.FailedReason)
	}
}

// TestVerifyBundles_AbsentBundleRefused is the #92 counterexample, inverted.
// A layer that names no bundle is refused before the mount, with a verifier
// configured, even though its content hashes correctly.
func TestVerifyBundles_AbsentBundleRefused(t *testing.T) {
	ctx := context.Background()

	// Same content and matching digest as the control; Bundle left empty.
	layer, path := makeLayer(t, "python-3.11", []byte("squashfs content alpha"), 1)
	if layer.Bundle != "" {
		t.Fatalf("fixture: Bundle should be empty, got %q", layer.Bundle)
	}

	lf := &spec.LockFile{ProfileName: "ml-env", Layers: []spec.ResolvedLayer{layer}}

	signaler := &agent.FakeReadySignaler{}
	mounter := &recordingMounter{}
	verifier := &countingVerifier{}
	fetcher := &mapBundleFetcher{}

	a := newAgent(t, agent.Config{
		Source:        &agent.FakeLockfileSource{Lockfile: lf},
		Fetcher:       &agent.FakeLayerFetcher{Paths: map[string]string{layer.ID: path}},
		BundleFetcher: fetcher,
		Verifier:      verifier,
		Signaler:      signaler,
		Mounter:       mounter,
	})

	_, err := a.Run(ctx)
	if err == nil {
		t.Fatal("Run: expected refusal for a layer with no attestation bundle, got nil")
	}
	// Assert which check fired, not merely that one did: the same fixture with a
	// bundle passes (see the control), so a SHA-256 or fetch error here would be
	// a different defect wearing a green test.
	if !strings.Contains(err.Error(), "no attestation bundle") {
		t.Errorf("Run error should name the missing attestation bundle, got: %v", err)
	}
	if !strings.Contains(err.Error(), layer.ID) {
		t.Errorf("Run error should name the offending layer %q, got: %v", layer.ID, err)
	}
	if mounter.wasCalled() {
		t.Error("Mount was reached for a layer with no attestation bundle")
	}
	if signaler.ReadyCalled {
		t.Error("SignalReady was called despite an unverifiable layer")
	}
	if !signaler.FailedCalled {
		t.Error("SignalFailed was not called on refusal")
	}
	if verifier.callCount() != 0 {
		t.Errorf("Verify call count = %d, want 0 — the refusal precedes verification", verifier.callCount())
	}
	if fetcher.askedCount() != 0 {
		t.Errorf("BundleFetcher was asked %d times, want 0 — nothing to fetch for an absent bundle",
			fetcher.askedCount())
	}
}

// TestVerifyBundles_AbsentBundleNamesEveryLayer asserts the whole unverifiable
// set is reported, not just the first one encountered. One boot failure should
// tell the operator every layer they have to fix.
func TestVerifyBundles_AbsentBundleNamesEveryLayer(t *testing.T) {
	ctx := context.Background()

	signed, signedPath := makeLayer(t, "base-os", []byte("base bytes"), 1)
	signed.Bundle = "s3://strata-registry/bundles/base-os.json"
	bare1, bare1Path := makeLayer(t, "python-3.11", []byte("python bytes"), 2)
	bare2, bare2Path := makeLayer(t, "cuda-12.3", []byte("cuda bytes"), 3)

	lf := &spec.LockFile{
		ProfileName: "ml-env",
		Layers:      []spec.ResolvedLayer{signed, bare1, bare2},
	}

	signaler := &agent.FakeReadySignaler{}
	mounter := &recordingMounter{}

	a := newAgent(t, agent.Config{
		Source: &agent.FakeLockfileSource{Lockfile: lf},
		Fetcher: &agent.FakeLayerFetcher{Paths: map[string]string{
			signed.ID: signedPath,
			bare1.ID:  bare1Path,
			bare2.ID:  bare2Path,
		}},
		BundleFetcher: &mapBundleFetcher{Bytes: map[string][]byte{
			signed.ID: signedBundleJSON(t, signedPath),
		}},
		Verifier: &countingVerifier{},
		Signaler: signaler,
		Mounter:  mounter,
	})

	_, err := a.Run(ctx)
	if err == nil {
		t.Fatal("Run: expected refusal, got nil")
	}
	for _, id := range []string{bare1.ID, bare2.ID} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("Run error should name unverifiable layer %q, got: %v", id, err)
		}
	}
	if mounter.wasCalled() {
		t.Error("Mount was reached with unverifiable layers in the set")
	}
}

// TestVerifyBundles_EmptyBundleBytesRefused closes the second door. Inverting
// the filter alone is not enough: the shipped s3LayerFetcher returns
// (nil, nil) for an empty Bundle, so a fetcher answering with no bytes for a
// layer that does name a bundle must also be a refusal, not a skip.
func TestVerifyBundles_EmptyBundleBytesRefused(t *testing.T) {
	ctx := context.Background()

	layer, path := makeLayer(t, "python-3.11", []byte("squashfs content alpha"), 1)
	layer.Bundle = "s3://strata-registry/bundles/python-3.11.json"

	lf := &spec.LockFile{ProfileName: "ml-env", Layers: []spec.ResolvedLayer{layer}}

	signaler := &agent.FakeReadySignaler{}
	mounter := &recordingMounter{}
	verifier := &countingVerifier{}

	a := newAgent(t, agent.Config{
		Source:        &agent.FakeLockfileSource{Lockfile: lf},
		Fetcher:       &agent.FakeLayerFetcher{Paths: map[string]string{layer.ID: path}},
		BundleFetcher: &mapBundleFetcher{allNil: true},
		Verifier:      verifier,
		Signaler:      signaler,
		Mounter:       mounter,
	})

	_, err := a.Run(ctx)
	if err == nil {
		t.Fatal("Run: expected refusal when the bundle fetch yields no bytes, got nil")
	}
	if !strings.Contains(err.Error(), "no bundle bytes") {
		t.Errorf("Run error should name the empty bundle fetch, got: %v", err)
	}
	if !strings.Contains(err.Error(), layer.ID) {
		t.Errorf("Run error should name the offending layer %q, got: %v", layer.ID, err)
	}
	if mounter.wasCalled() {
		t.Error("Mount was reached for a layer whose bundle fetch returned no bytes")
	}
	if signaler.ReadyCalled {
		t.Error("SignalReady was called despite an unverified layer")
	}
	if !signaler.FailedCalled {
		t.Error("SignalFailed was not called on refusal")
	}
	if verifier.callCount() != 0 {
		t.Errorf("Verify call count = %d, want 0 — no material to verify", verifier.callCount())
	}
}

// TestVerifyBundles_NoLayersIsNotAbsentBundle distinguishes the two zero cases.
// An empty lockfile has nothing to verify and boots; a lockfile with layers and
// no bundles does not. Without this, the refusal could be satisfied by an
// implementation that refuses whenever the verify set is empty.
func TestVerifyBundles_NoLayersIsNotAbsentBundle(t *testing.T) {
	ctx := context.Background()

	lf := &spec.LockFile{ProfileName: "empty-env", Layers: nil}

	signaler := &agent.FakeReadySignaler{}
	verifier := &countingVerifier{}

	a := newAgent(t, agent.Config{
		Source:        &agent.FakeLockfileSource{Lockfile: lf},
		Fetcher:       &agent.FakeLayerFetcher{Paths: map[string]string{}},
		BundleFetcher: &mapBundleFetcher{},
		Verifier:      verifier,
		Signaler:      signaler,
		Mounter:       &recordingMounter{},
	})

	if _, err := a.Run(ctx); err != nil {
		t.Fatalf("Run with no layers and a verifier configured: %v", err)
	}
	if !signaler.ReadyCalled {
		t.Error("SignalReady was not called for an empty lockfile")
	}
	if verifier.callCount() != 0 {
		t.Errorf("Verify call count = %d, want 0 for an empty lockfile", verifier.callCount())
	}
}

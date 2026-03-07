package agent_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"github.com/scttfrdmn/strata/internal/agent"
	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/spec"
)

// makeLayer writes content to a temp file and returns a ResolvedLayer whose
// SHA256 matches the content, along with the file path.
func makeLayer(t *testing.T, id string, content []byte, mountOrder int) (spec.ResolvedLayer, string) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "layer-*.sqfs")
	if err != nil {
		t.Fatalf("creating temp layer file: %v", err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatalf("writing temp layer: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing temp layer file: %v", err)
	}

	sum := sha256.Sum256(content)
	layer := spec.ResolvedLayer{
		LayerManifest: spec.LayerManifest{
			ID:     id,
			SHA256: hex.EncodeToString(sum[:]),
		},
		MountOrder: mountOrder,
	}
	return layer, f.Name()
}

// newAgent constructs an agent.Agent with all fakes wired and
// EnvRootDir set to a temp dir so no root access is needed.
func newAgent(t *testing.T, cfg agent.Config) *agent.Agent {
	t.Helper()
	if cfg.EnvRootDir == "" {
		cfg.EnvRootDir = t.TempDir()
	}
	a, err := agent.New(cfg)
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}
	return a
}

func TestRun_HappyPath(t *testing.T) {
	ctx := context.Background()

	layer1, path1 := makeLayer(t, "python-3.11", []byte("squashfs content alpha"), 1)
	layer2, path2 := makeLayer(t, "cuda-12.3", []byte("squashfs content beta"), 2)

	lf := &spec.LockFile{
		ProfileName: "ml-env",
		Layers:      []spec.ResolvedLayer{layer1, layer2},
	}

	signaler := &agent.FakeReadySignaler{}
	mounter := &agent.FakeMounter{Result: &overlay.Overlay{MergedPath: "/strata/env"}}

	a := newAgent(t, agent.Config{
		Source: &agent.FakeLockfileSource{Lockfile: lf},
		Fetcher: &agent.FakeLayerFetcher{Paths: map[string]string{
			layer1.ID: path1,
			layer2.ID: path2,
		}},
		Signaler: signaler,
		Mounter:  mounter,
	})

	if err := a.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !signaler.ReadyCalled {
		t.Error("SignalReady was not called")
	}
	if signaler.FailedCalled {
		t.Errorf("SignalFailed was called unexpectedly: %v", signaler.FailedReason)
	}
}

func TestRun_NoLayers(t *testing.T) {
	ctx := context.Background()

	lf := &spec.LockFile{ProfileName: "empty-env", Layers: nil}
	signaler := &agent.FakeReadySignaler{}
	mounter := &agent.FakeMounter{Result: &overlay.Overlay{}}

	a := newAgent(t, agent.Config{
		Source:   &agent.FakeLockfileSource{Lockfile: lf},
		Fetcher:  &agent.FakeLayerFetcher{Paths: map[string]string{}},
		Signaler: signaler,
		Mounter:  mounter,
	})

	if err := a.Run(ctx); err != nil {
		t.Fatalf("Run with no layers: %v", err)
	}
	if !signaler.ReadyCalled {
		t.Error("SignalReady was not called")
	}
}

func TestRun_AcquireFails(t *testing.T) {
	ctx := context.Background()
	acquireErr := errors.New("metadata service unavailable")

	signaler := &agent.FakeReadySignaler{}
	a := newAgent(t, agent.Config{
		Source:   &agent.FakeLockfileSource{Err: acquireErr},
		Fetcher:  &agent.FakeLayerFetcher{},
		Signaler: signaler,
		Mounter:  &agent.FakeMounter{Result: &overlay.Overlay{}},
	})

	err := a.Run(ctx)
	if err == nil {
		t.Fatal("Run: expected error, got nil")
	}
	if !errors.Is(err, acquireErr) {
		t.Errorf("Run error should wrap acquire error: got %v", err)
	}
	if !signaler.FailedCalled {
		t.Error("SignalFailed was not called")
	}
	if signaler.ReadyCalled {
		t.Error("SignalReady should not be called after acquire failure")
	}
}

func TestRun_FetchFails(t *testing.T) {
	ctx := context.Background()

	layer, _ := makeLayer(t, "broken-layer", []byte("data"), 1)
	lf := &spec.LockFile{
		ProfileName: "test",
		Layers:      []spec.ResolvedLayer{layer},
	}

	fetchErr := errors.New("S3 access denied")
	signaler := &agent.FakeReadySignaler{}

	a := newAgent(t, agent.Config{
		Source:   &agent.FakeLockfileSource{Lockfile: lf},
		Fetcher:  &agent.FakeLayerFetcher{Err: fetchErr},
		Signaler: signaler,
		Mounter:  &agent.FakeMounter{Result: &overlay.Overlay{}},
	})

	err := a.Run(ctx)
	if err == nil {
		t.Fatal("Run: expected error, got nil")
	}
	if !signaler.FailedCalled {
		t.Error("SignalFailed was not called")
	}
}

func TestRun_SHA256Mismatch(t *testing.T) {
	ctx := context.Background()

	// Create a real file but put the wrong SHA256 in the manifest.
	f, err := os.CreateTemp(t.TempDir(), "layer-*.sqfs")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	if _, err := f.Write([]byte("actual content")); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing temp file: %v", err)
	}

	layer := spec.ResolvedLayer{
		LayerManifest: spec.LayerManifest{
			ID:     "wrong-hash-layer",
			SHA256: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		},
		MountOrder: 1,
	}
	lf := &spec.LockFile{
		ProfileName: "test",
		Layers:      []spec.ResolvedLayer{layer},
	}

	signaler := &agent.FakeReadySignaler{}

	a := newAgent(t, agent.Config{
		Source: &agent.FakeLockfileSource{Lockfile: lf},
		Fetcher: &agent.FakeLayerFetcher{Paths: map[string]string{
			layer.ID: f.Name(),
		}},
		Signaler: signaler,
		Mounter:  &agent.FakeMounter{Result: &overlay.Overlay{}},
	})

	err = a.Run(ctx)
	if err == nil {
		t.Fatal("Run: expected SHA256 mismatch error, got nil")
	}
	if !signaler.FailedCalled {
		t.Error("SignalFailed was not called on SHA256 mismatch")
	}
}

func TestRun_MountFails(t *testing.T) {
	ctx := context.Background()

	layer, path := makeLayer(t, "good-layer", []byte("content"), 1)
	lf := &spec.LockFile{
		ProfileName: "test",
		Layers:      []spec.ResolvedLayer{layer},
	}

	signaler := &agent.FakeReadySignaler{}

	a := newAgent(t, agent.Config{
		Source: &agent.FakeLockfileSource{Lockfile: lf},
		Fetcher: &agent.FakeLayerFetcher{Paths: map[string]string{
			layer.ID: path,
		}},
		Signaler: signaler,
		Mounter:  &agent.FakeMounter{Err: overlay.ErrNotSupported},
	})

	err := a.Run(ctx)
	if err == nil {
		t.Fatal("Run: expected mount error, got nil")
	}
	if !errors.Is(err, overlay.ErrNotSupported) {
		t.Errorf("Run error should wrap ErrNotSupported: got %v", err)
	}
	if !signaler.FailedCalled {
		t.Error("SignalFailed was not called on mount failure")
	}
}

func TestNew_RequiredFields(t *testing.T) {
	sig := &agent.FakeReadySignaler{}
	fet := &agent.FakeLayerFetcher{}
	src := &agent.FakeLockfileSource{}

	tests := []struct {
		name string
		cfg  agent.Config
	}{
		{"missing Source", agent.Config{Fetcher: fet, Signaler: sig}},
		{"missing Fetcher", agent.Config{Source: src, Signaler: sig}},
		{"missing Signaler", agent.Config{Source: src, Fetcher: fet}},
	}
	for _, tt := range tests {
		_, err := agent.New(tt.cfg)
		if err == nil {
			t.Errorf("%s: expected error, got nil", tt.name)
		}
	}
}

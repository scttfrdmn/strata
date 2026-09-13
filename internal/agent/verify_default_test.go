package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/agent"
	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/spec"
)

// These pin the #93 contract in both directions: the agent fails closed when it
// has no way to verify authenticity, and boots only when the skip is the
// explicit Config.AllowUnverified opt-out. The rewritten TestRun_HappyPath is the
// third leg — a fully-configured boot verifies and succeeds — so a blanket
// "always refuse" would fail there.

// TestRun_RefusesWithoutVerifier is the core of #93: a boot with layers but no
// Verifier/BundleFetcher must refuse, not silently mount unverified layers. The
// assertion is on the message, so a refusal from some other step (e.g. the mount)
// cannot be mistaken for this one.
func TestRun_RefusesWithoutVerifier(t *testing.T) {
	ctx := context.Background()

	layer, path := makeLayer(t, "python-3.11", []byte("squashfs content"), 1)
	lf := &spec.LockFile{ProfileName: "ml-env", Layers: []spec.ResolvedLayer{layer}}

	signaler := &agent.FakeReadySignaler{}
	a := newAgent(t, agent.Config{
		Source:   &agent.FakeLockfileSource{Lockfile: lf},
		Fetcher:  &agent.FakeLayerFetcher{Paths: map[string]string{layer.ID: path}},
		Signaler: signaler,
		Mounter:  &agent.FakeMounter{Result: &overlay.Overlay{MergedPath: "/strata/env"}},
		// No Verifier, no BundleFetcher, AllowUnverified unset (the default).
	})

	_, err := a.Run(ctx)
	if err == nil {
		t.Fatal("Run booted with no verifier configured; expected a refusal")
	}
	if !strings.Contains(err.Error(), "no way to verify layer authenticity") {
		t.Fatalf("refusal must name the missing verification, got: %v", err)
	}
	if signaler.ReadyCalled {
		t.Error("SignalReady was called on a boot that could not verify its layers")
	}
	if !signaler.FailedCalled {
		t.Error("SignalFailed was not called on the refusal")
	}
}

// TestRun_AllowUnverifiedBoots proves the deliberate opt-out is preserved: with
// Config.AllowUnverified set (what cmd/strata-agent does under
// STRATA_AGENT_ALLOW_UNVERIFIED), the same unverifiable boot succeeds. Without
// this, the refusal above would just be "always refuse" and would break the
// air-gapped opt-out (#56).
func TestRun_AllowUnverifiedBoots(t *testing.T) {
	ctx := context.Background()

	layer, path := makeLayer(t, "python-3.11", []byte("squashfs content"), 1)
	lf := &spec.LockFile{ProfileName: "ml-env", Layers: []spec.ResolvedLayer{layer}}

	signaler := &agent.FakeReadySignaler{}
	a := newAgent(t, agent.Config{
		Source:          &agent.FakeLockfileSource{Lockfile: lf},
		Fetcher:         &agent.FakeLayerFetcher{Paths: map[string]string{layer.ID: path}},
		Signaler:        signaler,
		Mounter:         &agent.FakeMounter{Result: &overlay.Overlay{MergedPath: "/strata/env"}},
		AllowUnverified: true,
	})

	if _, err := a.Run(ctx); err != nil {
		t.Fatalf("Run with AllowUnverified set: %v", err)
	}
	if !signaler.ReadyCalled {
		t.Error("SignalReady was not called on the deliberate opt-out boot")
	}
}

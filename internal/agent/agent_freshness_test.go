// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package agent_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/scttfrdmn/strata/internal/agent"
	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/spec"
)

// TestRun_RefusesStaleLockfileUnderBound is the #224 (T8) refusal at the boot
// boundary: with a MaxAge bound set, a lockfile resolved longer ago than the
// bound stops the boot before any layer is fetched, verified, or mounted, and
// the instance signals failure rather than booting a stale-but-signed
// environment. The must-not fakes prove the refusal happens before consumption.
func TestRun_RefusesStaleLockfileUnderBound(t *testing.T) {
	valid := strings.Repeat("a", 64)
	lf := &spec.LockFile{
		ProfileName: "ml-env",
		ResolvedAt:  time.Now().Add(-400 * 24 * time.Hour), // ~13 months old
		Layers: []spec.ResolvedLayer{
			{LayerManifest: spec.LayerManifest{ID: "python-3.11-x86_64", SHA256: valid}, MountOrder: 1},
		},
	}

	signaler := &agent.FakeReadySignaler{}
	a := newAgent(t, agent.Config{
		Source:        &agent.FakeLockfileSource{Lockfile: lf},
		Fetcher:       mustNotFetch{t},
		BundleFetcher: mustNotBundle{t},
		Verifier:      mustNotVerify{t},
		Signaler:      signaler,
		Mounter:       mustNotMount{t},
		MaxAge:        30 * 24 * time.Hour,
	})

	_, err := a.Run(context.Background())
	if err == nil {
		t.Fatal("Run booted a lockfile older than the configured MaxAge bound")
	}
	if !strings.Contains(err.Error(), "stale") {
		t.Errorf("error does not name staleness: %v", err)
	}
	if !signaler.FailedCalled {
		t.Error("SignalFailed was not called for a stale lockfile")
	}
	if signaler.ReadyCalled {
		t.Error("SignalReady was called for a stale lockfile")
	}
}

// TestRun_BootsStaleLockfileWithNoBoundAndSurfacesAge is the opt-in half: with
// no MaxAge bound (the default), a stale lockfile still boots — a frozen/cited
// environment is meant to be old — but its age is always surfaced on the boot
// log so the staleness is never silent. This is the "surface always, refuse
// opt-in" policy (#224).
func TestRun_BootsStaleLockfileWithNoBoundAndSurfacesAge(t *testing.T) {
	ctx := context.Background()

	layer1, path1 := makeLayer(t, "python-3.11", []byte("squashfs content alpha"), 1)
	layer1.Bundle = "s3://strata-registry/bundles/python-3.11.json"

	lf := &spec.LockFile{
		ProfileName: "ml-env",
		ResolvedAt:  time.Now().Add(-400 * 24 * time.Hour),
		Layers:      []spec.ResolvedLayer{layer1},
	}

	var log bytes.Buffer
	signaler := &agent.FakeReadySignaler{}
	a := newAgent(t, agent.Config{
		Source:        &agent.FakeLockfileSource{Lockfile: lf},
		Fetcher:       &agent.FakeLayerFetcher{Paths: map[string]string{layer1.ID: path1}},
		BundleFetcher: &mapBundleFetcher{Bytes: map[string][]byte{layer1.ID: signedBundleJSON(t, path1)}},
		Verifier:      &countingVerifier{},
		Signaler:      signaler,
		Mounter:       &agent.FakeMounter{Result: &overlay.Overlay{MergedPath: "/strata/env"}},
		Warnings:      &log,
		// MaxAge left zero: no bound.
	})

	if _, err := a.Run(ctx); err != nil {
		t.Fatalf("Run refused a stale lockfile with no bound: %v", err)
	}
	if !signaler.ReadyCalled {
		t.Error("SignalReady was not called — a stale lockfile with no bound should boot")
	}
	if got := log.String(); !strings.Contains(got, "days ago") {
		t.Errorf("boot log did not surface the lockfile's age; got %q", got)
	}
}

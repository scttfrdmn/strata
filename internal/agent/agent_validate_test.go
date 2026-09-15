package agent_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/agent"
	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// The must-not-* fakes fail the test if their method is called: a structurally
// invalid lockfile must be refused before any layer is fetched, verified, or
// mounted. Without the Validate() call in Agent.Run these fire, which is exactly
// the pre-fix behaviour this pins against.

type mustNotFetch struct{ t *testing.T }

func (m mustNotFetch) Fetch(context.Context, spec.ResolvedLayer) (string, error) {
	m.t.Error("Fetch called on an invalid lockfile — Validate() did not gate the boot")
	return "", errors.New("must not be called")
}

type mustNotBundle struct{ t *testing.T }

func (m mustNotBundle) FetchBundleJSON(context.Context, spec.ResolvedLayer) ([]byte, error) {
	m.t.Error("FetchBundleJSON called on an invalid lockfile")
	return nil, errors.New("must not be called")
}

type mustNotVerify struct{ t *testing.T }

func (m mustNotVerify) Verify(context.Context, string, *trust.Bundle) error {
	m.t.Error("Verify called on an invalid lockfile")
	return errors.New("must not be called")
}

type mustNotMount struct{ t *testing.T }

func (m mustNotMount) Mount([]overlay.LayerPath) (*overlay.Overlay, error) {
	m.t.Error("Mount called on an invalid lockfile")
	return nil, errors.New("must not be called")
}

// TestRun_RefusesInvalidLockfile is the boundary #65/#96 missed: the agent — the
// most important execution boundary — must call LockFile.Validate() after
// Acquire, so a structurally invalid lockfile stops the boot before any
// fetch/verify/mount and the instance signals failure rather than mounting an
// environment whose mount precedence and reported EnvironmentID depend on YAML
// order (#95). The three cases are the ones LockFile.Validate rejects.
func TestRun_RefusesInvalidLockfile(t *testing.T) {
	valid := strings.Repeat("a", 64)
	cases := map[string]*spec.LockFile{
		"duplicate mount_order": {
			ProfileName: "evil",
			Layers: []spec.ResolvedLayer{
				{LayerManifest: spec.LayerManifest{ID: "alpha-1.0-x86_64", SHA256: valid}, MountOrder: 1},
				{LayerManifest: spec.LayerManifest{ID: "bravo-1.0-x86_64", SHA256: valid}, MountOrder: 1},
			},
		},
		"malformed base digest": {
			ProfileName: "evil",
			Base:        spec.ResolvedBase{AMISHA256: "bbbbbb"},
			Layers:      []spec.ResolvedLayer{{LayerManifest: spec.LayerManifest{ID: "alpha-1.0-x86_64", SHA256: valid}, MountOrder: 1}},
		},
		"unsafe layer id": {
			ProfileName: "evil",
			Layers:      []spec.ResolvedLayer{{LayerManifest: spec.LayerManifest{ID: "../../etc/cron.d/evil", SHA256: valid}, MountOrder: 1}},
		},
	}
	for name, lf := range cases {
		t.Run(name, func(t *testing.T) {
			signaler := &agent.FakeReadySignaler{}
			a := newAgent(t, agent.Config{
				Source:        &agent.FakeLockfileSource{Lockfile: lf},
				Fetcher:       mustNotFetch{t},
				BundleFetcher: mustNotBundle{t},
				Verifier:      mustNotVerify{t},
				Signaler:      signaler,
				Mounter:       mustNotMount{t},
			})
			_, err := a.Run(context.Background())
			if err == nil {
				t.Fatal("Run accepted an invalid lockfile")
			}
			if !strings.Contains(err.Error(), "invalid lockfile") {
				t.Errorf("error does not name the invalid lockfile: %v", err)
			}
			if !signaler.FailedCalled {
				t.Error("SignalFailed was not called on an invalid lockfile")
			}
			if signaler.ReadyCalled {
				t.Error("SignalReady was called for an invalid lockfile")
			}
		})
	}
}

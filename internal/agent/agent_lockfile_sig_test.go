package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/agent"
	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// signedNoLayerLockfile is a valid, signed lockfile with no layers — enough to
// exercise the agent's lockfile-signature check without also standing up layer
// bundles. FakeSigner produces a bundle a FakeVerifier accepts.
func signedNoLayerLockfile(t *testing.T) *spec.LockFile {
	t.Helper()
	lf := &spec.LockFile{ProfileName: "signed-env"}
	if err := trust.SignLockFile(context.Background(), lf, &trust.FakeSigner{}); err != nil {
		t.Fatalf("signing fixture: %v", err)
	}
	return lf
}

// TestRun_VerifiesLockfileSignature: a lockfile with a valid set signature boots.
func TestRun_VerifiesLockfileSignature(t *testing.T) {
	signaler := &agent.FakeReadySignaler{}
	a := newAgent(t, agent.Config{
		Source:        &agent.FakeLockfileSource{Lockfile: signedNoLayerLockfile(t)},
		Fetcher:       &agent.FakeLayerFetcher{Paths: map[string]string{}},
		BundleFetcher: &mapBundleFetcher{Bytes: map[string][]byte{}},
		Verifier:      &trust.FakeVerifier{},
		Signaler:      signaler,
		Mounter:       &agent.FakeMounter{Result: &overlay.Overlay{}},
	})
	if _, err := a.Run(context.Background()); err != nil {
		t.Fatalf("boot with a valid lockfile signature: %v", err)
	}
	if !signaler.ReadyCalled {
		t.Error("SignalReady was not called")
	}
}

// TestRun_RejectsTamperedLockfileSignature: a lockfile altered after signing is
// refused before any layer is fetched or mounted — the mix-and-match / rollback
// case (#101) caught at the boot boundary.
func TestRun_RejectsTamperedLockfileSignature(t *testing.T) {
	lf := signedNoLayerLockfile(t)
	lf.ProfileName = "attacker-env" // tamper after signing

	signaler := &agent.FakeReadySignaler{}
	a := newAgent(t, agent.Config{
		Source:        &agent.FakeLockfileSource{Lockfile: lf},
		Fetcher:       mustNotFetch{t},
		BundleFetcher: mustNotBundle{t},
		Verifier:      &trust.FakeVerifier{},
		Signaler:      signaler,
		Mounter:       mustNotMount{t},
	})
	_, err := a.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "lockfile signature") {
		t.Fatalf("expected a lockfile signature failure, got %v", err)
	}
	if !signaler.FailedCalled {
		t.Error("SignalFailed was not called")
	}
	if signaler.ReadyCalled {
		t.Error("SignalReady was called for a tampered lockfile")
	}
}

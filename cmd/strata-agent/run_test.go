package main

import (
	"context"
	"errors"
	"testing"

	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// recordingSignaler is a bootSignaler that records SignalFailed calls, so a test
// can assert the agent was signalled before the boot died.
type recordingSignaler struct {
	failed   []error
	failErr  error // returned by SignalFailed, to simulate the signal itself failing
	readyErr error
}

func (s *recordingSignaler) SignalReady(_ context.Context, _ *spec.LockFile) error { return s.readyErr }
func (s *recordingSignaler) SignalFailed(_ context.Context, reason error) error {
	s.failed = append(s.failed, reason)
	return s.failErr
}
func (s *recordingSignaler) getInstanceID(_ context.Context) (string, error) { return "i-test", nil }

// TestRun_VerifierRefusalIsFatalAndSignals is the property #56/#93 rest on and
// that lived only in main until #94: when the verifier cannot be resolved and the
// operator has not opted out, the boot stops (run returns the error) and the
// agent is signalled first. Before the extraction nothing asserted this — a
// refactor that turned the fatal exit into a return-and-boot would have gone
// green.
func TestRun_VerifierRefusalIsFatalAndSignals(t *testing.T) {
	sig := &recordingSignaler{}
	refusal := errors.New("cosign not found: layer signatures cannot be verified")

	err := run(context.Background(), agentDeps{
		signaler:        sig,
		resolveVerifier: func(context.Context) (trust.Verifier, error) { return nil, refusal },
		// source/fetcher/installer are unused: the refusal returns before the
		// agent is built.
	})

	if err == nil {
		t.Fatal("run returned nil for an unresolvable verifier; the boot must stop (#56/#94)")
	}
	if !errors.Is(err, refusal) {
		t.Errorf("run returned %v, want the verifier refusal", err)
	}
	if len(sig.failed) != 1 {
		t.Errorf("SignalFailed called %d times, want exactly 1 (signal before dying)", len(sig.failed))
	}
}

// TestRun_RefusalStaysFatalWhenSignalFails pins the other half of that comment:
// the signalling attempt is best-effort and its own failure must not make the
// refusal non-fatal.
func TestRun_RefusalStaysFatalWhenSignalFails(t *testing.T) {
	sig := &recordingSignaler{failErr: errors.New("IMDS unreachable")}
	refusal := errors.New("could not fetch the cosign public key")

	err := run(context.Background(), agentDeps{
		signaler:        sig,
		resolveVerifier: func(context.Context) (trust.Verifier, error) { return nil, refusal },
	})

	if err == nil {
		t.Fatal("a failed SignalFailed made the verifier refusal non-fatal (#94)")
	}
	if !errors.Is(err, refusal) {
		t.Errorf("run returned %v, want the verifier refusal even though signalling failed", err)
	}
}

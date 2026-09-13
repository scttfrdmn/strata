package resolver

import (
	"context"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

// TestVerifyBundle_OfflinePresenceOnly pins the behaviour pkg/strata.Resolve's
// doc now describes and #61 corrected: with no Rekor verifier configured (the
// offline / pkg/strata config), stage 7 is a presence check — it verifies no
// payload, so a non-empty placeholder Bundle/RekorEntry passes. The doc used to
// claim payloads were "still verified"; this test makes the true behaviour
// executable so the doc cannot drift back.
func TestVerifyBundle_OfflinePresenceOnly(t *testing.T) {
	r := &Resolver{} // cfg.Rekor == nil: the offline configuration

	layer := func(bundle, rekor string) resolvedLayer {
		return resolvedLayer{manifest: &spec.LayerManifest{
			ID:         "python-3.13.2-linux-gnu-2.34-x86_64",
			Bundle:     bundle,
			RekorEntry: rekor,
		}}
	}

	// The placeholder the shipped formation catalog carries passes offline — no
	// signature is verified. This is the case the corrected doc predicts.
	if err := r.verifyBundle(context.Background(), layer("pending-initial-build", "pending-initial-build")); err != nil {
		t.Errorf("offline verifyBundle must pass a non-empty placeholder bundle (presence check), got: %v", err)
	}

	// Presence is still required: an empty Bundle or RekorEntry fails, so the
	// pass above is a genuine presence check and not a function that accepts
	// everything.
	if err := r.verifyBundle(context.Background(), layer("", "123")); err == nil {
		t.Error("verifyBundle must reject an empty Bundle even offline")
	}
	if err := r.verifyBundle(context.Background(), layer("s3://b/x.json", "")); err == nil {
		t.Error("verifyBundle must reject an empty RekorEntry even offline")
	}
}

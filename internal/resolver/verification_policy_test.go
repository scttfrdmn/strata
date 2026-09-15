package resolver_test

import (
	"context"
	"testing"

	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/internal/resolver"
	"github.com/scttfrdmn/strata/spec"
)

// TestResolve_RecordsVerificationPolicy is #100 / T6: a resolved lockfile
// carries a field naming what the resolver checked about layer attestations, so
// a consumer reading only the artifact can tell what was verified without
// knowing how the resolve was invoked.
func TestResolve_RecordsVerificationPolicy(t *testing.T) {
	// Signed layers, ordinary resolve: attestation present.
	t.Run("attestation-references-present", func(t *testing.T) {
		store := registry.NewMemoryStore()
		store.AddLayer(signedLayer("tool", "1.0.0", "linux-gnu-2.34",
			[]spec.Capability{{Name: "tool", Version: "1.0.0"}}, nil))

		lf, err := newResolver(t, store, nil).Resolve(context.Background(),
			testProfile(softwareRef("tool", "1.0.0")))
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if lf.VerificationPolicy != spec.VerifyAttestationReferencesPresent {
			t.Errorf("VerificationPolicy = %q, want %q", lf.VerificationPolicy, spec.VerifyAttestationReferencesPresent)
		}
	})

	// An unsigned layer accepted on the offline-catalog path: unsigned-offline.
	// This is the case the label exists to make observable — a lockfile that was
	// not a trust decision must say so in the artifact.
	t.Run("unsigned-offline", func(t *testing.T) {
		store := registry.NewMemoryStore()
		unsigned := signedLayer("tool", "1.0.0", "linux-gnu-2.34",
			[]spec.Capability{{Name: "tool", Version: "1.0.0"}}, nil)
		unsigned.Bundle = ""
		unsigned.RekorEntry = ""
		store.AddLayer(unsigned)

		r, err := resolver.New(resolver.Config{
			Registry:             store,
			Probe:                testProbe(),
			StrataVersion:        "0.0.0-test",
			AllowUnsignedOffline: true,
		})
		if err != nil {
			t.Fatalf("resolver.New: %v", err)
		}
		lf, err := r.Resolve(context.Background(), testProfile(softwareRef("tool", "1.0.0")))
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if lf.VerificationPolicy != spec.VerifyUnsignedOffline {
			t.Errorf("VerificationPolicy = %q, want %q", lf.VerificationPolicy, spec.VerifyUnsignedOffline)
		}
	})
}

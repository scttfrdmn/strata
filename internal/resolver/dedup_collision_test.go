// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package resolver_test

import (
	"context"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

// TestResolve_RejectsIDCollisionWithDifferentContent is an A1/R3 case introduced
// by the #208 dedup. dedupLayers collapses resolved layers by manifest ID, but
// the ID is not authenticated — layer verification checks the squashfs digest
// and its cosign signature over the bytes (#146), not the ID a registry stamps
// on the manifest. So a tampering registry (A1) can give two separately
// requested, separately signed layers the same ID; collapsing them would delete
// one requested, authenticated layer from the set the signer then signs, and the
// lockfile would claim to satisfy a request whose bytes never enter it (R3: a
// dropped request returned as a "complete" set). The resolver must refuse.
func TestResolve_RejectsIDCollisionWithDifferentContent(t *testing.T) {
	store := registry.NewMemoryStore()
	alpha := signedLayer("alpha", "1.0.0", "linux-gnu-2.34",
		[]spec.Capability{{Name: "alpha", Version: "1.0.0"}}, nil)
	beta := signedLayer("beta", "1.0.0", "linux-gnu-2.34",
		[]spec.Capability{{Name: "beta", Version: "1.0.0"}}, nil)
	// Both manifests stamped with one ID while their content (and signatures) differ.
	alpha.ID, alpha.SHA256 = "collision", strings.Repeat("a", 64)
	beta.ID, beta.SHA256 = "collision", strings.Repeat("b", 64)
	store.AddLayer(alpha)
	store.AddLayer(beta)

	_, err := newResolver(t, store, nil).Resolve(context.Background(),
		testProfile(softwareRef("alpha", "1.0.0"), softwareRef("beta", "1.0.0")))
	assertResolutionError(t, err, "LAYER_ID_COLLISION")
}

// TestResolve_DedupsGenuineDuplicate is the non-vacuity control: two requests
// that resolve to the *same* content (same ID and digest) still collapse to one
// without error — the #208 behaviour the collision guard must not break. Two
// formations sharing a layer is the realistic path (TestResolve_OverlappingFormations);
// this is the minimal direct case.
func TestResolve_DedupsGenuineDuplicate(t *testing.T) {
	store := registry.NewMemoryStore()
	store.AddLayer(signedLayer("tool", "1.0.0", "linux-gnu-2.34",
		[]spec.Capability{{Name: "tool", Version: "1.0.0"}}, nil))
	// Requesting the same layer twice resolves to one manifest each time (same ID,
	// same digest), so the deduper collapses them with no error.
	lf, err := newResolver(t, store, nil).Resolve(context.Background(),
		testProfile(softwareRef("tool", "1.0.0"), softwareRef("tool", "1.0.0")))
	if err != nil {
		t.Fatalf("Resolve of a genuine duplicate: %v", err)
	}
	if len(lf.Layers) != 1 {
		t.Fatalf("genuine duplicate not collapsed: %d layers", len(lf.Layers))
	}
}

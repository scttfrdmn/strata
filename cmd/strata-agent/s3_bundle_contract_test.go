package main

import (
	"context"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

// TestFetchBundleJSON_EmptyBundleReturnsNoBytes pins the shipped fetcher's
// answer for a layer that names no bundle: no bytes and no error.
//
// This is the second door in #92. Inverting only the agent's layer filter would
// still admit a bundle-less layer if the agent treated an empty fetch result as
// "nothing to verify", because this is the implementation the agent is wired to
// in production (main.go). The agent's refusal on len(data) == 0 is what makes
// this return safe, and this test is the record of why that refusal exists —
// so a later simplification of either side has to contend with the other.
//
// A nil S3 client is deliberate: reaching the client at all would falsify the
// claim that this path short-circuits before any network use.
func TestFetchBundleJSON_EmptyBundleReturnsNoBytes(t *testing.T) {
	f := newS3LayerFetcherWithAPI(nil, t.TempDir())

	data, err := f.FetchBundleJSON(context.Background(), spec.ResolvedLayer{
		LayerManifest: spec.LayerManifest{ID: "python-3.11"},
	})
	if err != nil {
		t.Fatalf("FetchBundleJSON with empty Bundle: unexpected error %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("FetchBundleJSON with empty Bundle returned %d bytes, want 0", len(data))
	}
}

// TestFetchBundleJSON_NamedBundleIsNotSilentlyEmpty is the control for the test
// above: with a bundle named, the method does not return the same (nil, nil)
// answer. Without this, the assertion "empty Bundle yields no bytes" would also
// be satisfied by a method that always yields no bytes — which would defeat the
// agent's refusal by making every layer look like a fetch failure.
func TestFetchBundleJSON_NamedBundleIsNotSilentlyEmpty(t *testing.T) {
	f := newS3LayerFetcherWithAPI(nil, t.TempDir())

	data, err := f.FetchBundleJSON(context.Background(), spec.ResolvedLayer{
		LayerManifest: spec.LayerManifest{
			ID:     "python-3.11",
			Bundle: "s3://strata-registry/bundles/python-3.11.json",
		},
	})
	if err == nil {
		t.Fatal("FetchBundleJSON with a named bundle and no S3 client: expected an error, got nil")
	}
	if len(data) != 0 {
		t.Errorf("FetchBundleJSON returned %d bytes alongside an error, want 0", len(data))
	}
}

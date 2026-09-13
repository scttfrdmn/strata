package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

// TestCollectBundleFailures covers the check strata verify gained in #60: without
// --rekor it now parses each layer's bundle, so a Bundle field whose file
// contents are prose — which the presence check accepted and printed "verified"
// over — is rejected. This is short of signature verification (no layer content
// to hash, no trust root; --rekor checks the live log), but no longer a lie.
//
// The valid bundle is the non-vacuity anchor: it proves the checker passes a
// well-formed input, so each rejection is attributable to the one defect it
// introduces. Each rejection asserts the specific reason string, so a checker
// reverted to presence-only (which would return no failure for prose, an s3 URI,
// or an unsigned bundle) fails here rather than passing quietly.
func TestCollectBundleFailures(t *testing.T) {
	validURI := writeBundleFile(t, bundleAttesting([]byte("digest"), []byte("sig")))
	proseURI := writeProseFile(t, "THIS IS NOT A VALID SIGSTORE BUNDLE")

	noRekor := bundleAttesting([]byte("digest"), []byte("sig"))
	noRekor.VerificationMaterial.TlogEntries = nil
	noRekorURI := writeBundleFile(t, noRekor)

	tests := []struct {
		name      string
		bundleURI string
		wantSub   string // "" means: expect no failure from this checker
	}{
		{"valid signed bundle passes", validURI, ""},
		{"prose bundle is rejected", proseURI, "parsing bundle"},
		{"s3 URI is reported as needing a fetch", "s3://bucket/layer.bundle.json", "must be fetched"},
		{"well-formed but unsigned bundle is rejected", noRekorURI, "no Rekor entry"},
		{"empty bundle is left to the presence check", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lf := &spec.LockFile{Layers: []spec.ResolvedLayer{
				layerWith("python-3.13.2", strings.Repeat("a", 64), tc.bundleURI),
			}}
			failures := collectBundleFailures(lf)
			if tc.wantSub == "" {
				if len(failures) != 0 {
					t.Fatalf("expected no bundle failures, got %v", failures)
				}
				return
			}
			if len(failures) == 0 {
				t.Fatalf("expected a failure containing %q, got none", tc.wantSub)
			}
			if !strings.Contains(failures[0], tc.wantSub) {
				t.Fatalf("failure %q does not name the reason %q", failures[0], tc.wantSub)
			}
		})
	}
}

// writeProseFile writes arbitrary (non-bundle) content to a temp file and returns
// a file:// URI for it — the shape a fetched lockfile carries.
func writeProseFile(t *testing.T, contents string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "bundle.json")
	if err := os.WriteFile(p, []byte(contents), 0600); err != nil {
		t.Fatalf("write prose file: %v", err)
	}
	return "file://" + p
}

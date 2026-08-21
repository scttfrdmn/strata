package spec_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

// valid is a real hex SHA256 (of the empty string) — 64 lowercase hex chars.
const valid = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func TestValidateLayerDigest(t *testing.T) {
	cases := []struct {
		name   string
		digest string
		ok     bool
	}{
		{"valid 64 hex", valid, true},
		{"empty", "", false},
		{"63 hex", valid[:63], false},
		{"65 hex", valid + "a", false},
		{"uppercase hex", strings.ToUpper(valid), false},
		{"traversal", "../escaped", false},
		{"traversal padded to 64", strings.Repeat("a", 54) + "/../../etc/x", false},
		{"path separator", valid[:32] + "/" + valid[33:], false},
		{"absolute", "/tmp/strata-probe/escaped", false},
		{"sha256-prefixed", "sha256:" + valid[7:], false},
		{"non-hex letter", strings.Repeat("g", 64), false},
		{"trailing space", valid[:63] + " ", false},
		{"extension included", valid + ".sqfs", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := spec.ValidateLayerDigest(tc.digest)
			if tc.ok && err != nil {
				t.Fatalf("ValidateLayerDigest(%q) = %v, want nil", tc.digest, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("ValidateLayerDigest(%q) = nil, want an error", tc.digest)
			}
		})
	}
}

// TestLayerCachePathRejectsEscape is the property that matters: a path is only
// returned when the filename is the digest and nothing else. filepath.Join
// resolves ".." instead of rejecting it, so a rejected digest is the only thing
// standing between a lockfile field and a write outside the cache directory.
func TestLayerCachePathRejectsEscape(t *testing.T) {
	cacheDir := t.TempDir()

	got, err := spec.LayerCachePath(cacheDir, valid)
	if err != nil {
		t.Fatalf("LayerCachePath with a valid digest: %v", err)
	}
	if want := filepath.Join(cacheDir, valid+".sqfs"); got != want {
		t.Errorf("LayerCachePath = %q, want %q", got, want)
	}

	for _, bad := range []string{"", "../escaped", "a/b", strings.ToUpper(valid)} {
		got, err := spec.LayerCachePath(cacheDir, bad)
		if err == nil {
			t.Errorf("LayerCachePath(%q) returned %q, want an error", bad, got)
			continue
		}
		if got != "" {
			t.Errorf("LayerCachePath(%q) returned path %q alongside an error", bad, got)
		}
	}
}

// TestLayerCachePathEmptyDigestDoesNotCollide covers the collision half of the
// empty-digest defect: before validation, every hashless layer in every lockfile
// mapped to the single filename ".sqfs", so the first one downloaded satisfied
// all the others through the cache-hit path.
func TestLayerCachePathEmptyDigestDoesNotCollide(t *testing.T) {
	if _, err := spec.LayerCachePath(t.TempDir(), ""); err == nil {
		t.Fatal("an empty digest produced a cache path — every hashless layer shares it")
	}
}

func TestVerifyFileDigest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "layer.sqfs")
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := spec.VerifyFileDigest(path, valid); err != nil {
		t.Fatalf("VerifyFileDigest on matching content: %v", err)
	}

	if err := os.WriteFile(path, []byte("planted"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := spec.VerifyFileDigest(path, valid)
	var mismatch *spec.DigestMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("VerifyFileDigest on planted content = %v, want *DigestMismatchError", err)
	}
	if mismatch.Want != valid || mismatch.Got == valid || mismatch.Path != path {
		t.Errorf("mismatch error does not describe the failure: %+v", mismatch)
	}

	// A malformed digest must fail before the file is hashed, so that a
	// mismatch error is never how a bad digest surfaces.
	if err := spec.VerifyFileDigest(path, "../escaped"); err == nil {
		t.Error("VerifyFileDigest accepted a malformed digest")
	} else if errors.As(err, &mismatch) {
		t.Errorf("malformed digest surfaced as a content mismatch: %v", err)
	}
}

func TestFileDigestIsLowercaseHex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := spec.FileDigest(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != valid {
		t.Fatalf("FileDigest(empty file) = %q, want %q", got, valid)
	}
	if err := spec.ValidateLayerDigest(got); err != nil {
		t.Fatalf("FileDigest produced a digest its own validator rejects: %v", err)
	}
}

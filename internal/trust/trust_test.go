package trust_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// fileHex returns the hex SHA256 of a file's contents — for test setup only.
func fileHex(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fileHex: read %q: %v", path, err)
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// writeFile writes content to path — for test setup only.
func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("writeFile %q: %v", path, err)
	}
}

// TestBundleParseMarshal tests Bundle JSON round-trip.
func TestBundleParseMarshal(t *testing.T) {
	b := &trust.Bundle{
		MediaType: trust.BundleMediaType,
		VerificationMaterial: trust.VerificationMaterial{
			TlogEntries: []trust.TlogEntry{
				{
					LogIndex:       "42",
					LogID:          "abc123",
					IntegratedTime: "1234567890",
				},
			},
		},
		MessageSignature: trust.MessageSignature{
			MessageDigest: trust.MessageDigest{
				Algorithm: "SHA2_256",
				Digest:    []byte("fakedigest"),
			},
			Signature: []byte("fakesig"),
		},
	}

	data, err := b.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	b2, err := trust.ParseBundle(data)
	if err != nil {
		t.Fatalf("ParseBundle() error: %v", err)
	}
	if b.MediaType != b2.MediaType {
		t.Errorf("MediaType mismatch: %q != %q", b.MediaType, b2.MediaType)
	}
	idx, ok := b2.RekorLogIndex()
	if !ok || idx != 42 {
		t.Errorf("RekorLogIndex() = (%d, %v), want (42, true)", idx, ok)
	}
}

// TestParseBundleRejectsWrongMediaType tests that bundles with wrong media type are rejected.
func TestParseBundleRejectsWrongMediaType(t *testing.T) {
	b := map[string]string{"mediaType": "application/json"}
	data, _ := json.Marshal(b)
	if _, err := trust.ParseBundle(data); err == nil {
		t.Error("ParseBundle with wrong mediaType should return error")
	}
}

// TestBundleRekorLogIndex tests RekorLogIndex parsing.
func TestBundleRekorLogIndex(t *testing.T) {
	noEntry := &trust.Bundle{MediaType: trust.BundleMediaType}
	if _, ok := noEntry.RekorLogIndex(); ok {
		t.Error("empty TlogEntries should return (0, false)")
	}

	badIndex := &trust.Bundle{
		MediaType: trust.BundleMediaType,
		VerificationMaterial: trust.VerificationMaterial{
			TlogEntries: []trust.TlogEntry{{LogIndex: "not-a-number"}},
		},
	}
	if _, ok := badIndex.RekorLogIndex(); ok {
		t.Error("non-numeric LogIndex should return (0, false)")
	}
}

// TestBundleHasRekorEntry tests HasRekorEntry.
func TestBundleHasRekorEntry(t *testing.T) {
	noEntry := &trust.Bundle{MediaType: trust.BundleMediaType}
	if noEntry.HasRekorEntry() {
		t.Error("bundle with no TlogEntries should have HasRekorEntry() == false")
	}

	withEntry := &trust.Bundle{
		MediaType: trust.BundleMediaType,
		VerificationMaterial: trust.VerificationMaterial{
			TlogEntries: []trust.TlogEntry{{LogIndex: "1"}},
		},
	}
	if !withEntry.HasRekorEntry() {
		t.Error("bundle with TlogEntries should have HasRekorEntry() == true")
	}
}

// TestFakeSignerAndVerifier tests the happy path with FakeSigner + FakeVerifier.
func TestFakeSignerAndVerifier(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	artifactPath := dir + "/artifact.sqfs"
	writeFile(t, artifactPath, []byte("layer content abc123"))

	signer := &trust.FakeSigner{}
	bundle, err := signer.Sign(ctx, artifactPath, map[string]string{"strata.layer.name": "python"})
	if err != nil {
		t.Fatalf("FakeSigner.Sign() error: %v", err)
	}
	if !bundle.HasRekorEntry() {
		t.Error("FakeSigner bundle should have a Rekor entry")
	}
	idx, ok := bundle.RekorLogIndex()
	if !ok || idx != 0 {
		t.Errorf("first sign should have log index 0, got (%d, %v)", idx, ok)
	}

	// Second sign should increment log index.
	bundle2, err := signer.Sign(ctx, artifactPath, nil)
	if err != nil {
		t.Fatalf("FakeSigner.Sign() second call error: %v", err)
	}
	idx2, _ := bundle2.RekorLogIndex()
	if idx2 != 1 {
		t.Errorf("second sign should have log index 1, got %d", idx2)
	}

	// Verify with FakeVerifier should succeed.
	verifier := &trust.FakeVerifier{}
	if err := verifier.Verify(ctx, artifactPath, bundle); err != nil {
		t.Errorf("FakeVerifier.Verify() unexpected error: %v", err)
	}
}

// TestFakeVerifierRejectsWrongSignature tests that FakeVerifier rejects tampered artifacts.
func TestFakeVerifierRejectsWrongSignature(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	pathA := dir + "/artifact-a.sqfs"
	pathB := dir + "/artifact-b.sqfs"
	writeFile(t, pathA, []byte("content A"))
	writeFile(t, pathB, []byte("content B"))

	signer := &trust.FakeSigner{}
	bundle, err := signer.Sign(ctx, pathA, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verifying A's bundle against B should fail.
	verifier := &trust.FakeVerifier{}
	if err := verifier.Verify(ctx, pathB, bundle); err == nil {
		t.Error("FakeVerifier should reject bundle signed for a different artifact")
	}
}

// TestFakeVerifierRejectsBundleWithoutRekor tests that bundles without Rekor entries are rejected.
func TestFakeVerifierRejectsBundleWithoutRekor(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	artifactPath := dir + "/artifact.sqfs"
	writeFile(t, artifactPath, []byte("content"))

	// Bundle with no Rekor entry.
	bundle := &trust.Bundle{MediaType: trust.BundleMediaType}
	verifier := &trust.FakeVerifier{}
	if err := verifier.Verify(ctx, artifactPath, bundle); err == nil {
		t.Error("FakeVerifier should reject bundle with no Rekor entry")
	}
}

// TestFakeRekorClient tests FakeRekorClient.
func TestFakeRekorClient(t *testing.T) {
	ctx := context.Background()
	client := &trust.FakeRekorClient{}
	bundle := &trust.Bundle{MediaType: trust.BundleMediaType}

	idx, err := client.Log(ctx, bundle)
	if err != nil {
		t.Fatalf("FakeRekorClient.Log() error: %v", err)
	}
	if idx != 0 {
		t.Errorf("first Log() should return index 0, got %d", idx)
	}

	idx2, _ := client.Log(ctx, bundle)
	if idx2 != 1 {
		t.Errorf("second Log() should return index 1, got %d", idx2)
	}

	if len(client.LoggedBundles) != 2 {
		t.Errorf("FakeRekorClient should have 2 logged bundles, got %d", len(client.LoggedBundles))
	}

	if err := client.VerifyEntry(ctx, 0, bundle); err != nil {
		t.Errorf("FakeRekorClient.VerifyEntry() unexpected error: %v", err)
	}
}

// TestVerifyLayerHappyPath tests the VerifyLayer helper end-to-end.
func TestVerifyLayerHappyPath(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sqfsPath := dir + "/python-3.11.9-rhel-x86_64.sqfs"
	sqfsContent := []byte("fake squashfs content for python 3.11.9")
	writeFile(t, sqfsPath, sqfsContent)

	// Sign the squashfs.
	signer := &trust.FakeSigner{}
	bundle, err := signer.Sign(ctx, sqfsPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	bundleData, err := bundle.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	bundlePath := dir + "/python-3.11.9.bundle.json"
	writeFile(t, bundlePath, bundleData)

	manifest := &spec.LayerManifest{
		ID:     "python-3.11.9-rhel-x86_64",
		Name:   "python",
		SHA256: fileHex(t, sqfsPath),
		Bundle: bundlePath,
	}

	verifier := &trust.FakeVerifier{}
	if err := trust.VerifyLayer(ctx, manifest, sqfsPath, verifier); err != nil {
		t.Errorf("VerifyLayer() unexpected error: %v", err)
	}
}

// TestVerifyLayerFailsOnSHA256Mismatch tests content integrity enforcement.
func TestVerifyLayerFailsOnSHA256Mismatch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sqfsPath := dir + "/layer.sqfs"
	writeFile(t, sqfsPath, []byte("original content"))

	manifest := &spec.LayerManifest{
		ID:     "test-layer",
		SHA256: "0000000000000000000000000000000000000000000000000000000000000000",
		Bundle: dir + "/fake.bundle.json",
	}

	if err := trust.VerifyLayer(ctx, manifest, sqfsPath, &trust.FakeVerifier{}); err == nil {
		t.Error("VerifyLayer should fail when SHA256 does not match")
	}
}

// TestVerifyLayerFailsOnMissingBundle tests that unsigned layers are rejected.
func TestVerifyLayerFailsOnMissingBundle(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sqfsPath := dir + "/layer.sqfs"
	writeFile(t, sqfsPath, []byte("layer content"))

	manifest := &spec.LayerManifest{
		ID:     "test-layer",
		SHA256: fileHex(t, sqfsPath),
		Bundle: "", // unsigned
	}

	if err := trust.VerifyLayer(ctx, manifest, sqfsPath, &trust.FakeVerifier{}); err == nil {
		t.Error("VerifyLayer should reject layer with empty Bundle field")
	}
}

// TestVerifyLayers tests parallel layer verification with FakeVerifier.
func TestVerifyLayers(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	signer := &trust.FakeSigner{}
	lockfile := &spec.LockFile{}

	// Create and sign three fake squashfs files.
	// Layer IDs must match the basename of the .sqfs file in squashfsDir.
	ids := []string{"python-3.11.9-rhel-x86_64", "cuda-12.3.2-rhel-x86_64", "openmpi-4.1.6-rhel-x86_64"}
	for i, id := range ids {
		sqfsPath := dir + "/" + id + ".sqfs"
		writeFile(t, sqfsPath, []byte("fake squashfs for "+id))

		bundle, err := signer.Sign(ctx, sqfsPath, nil)
		if err != nil {
			t.Fatalf("sign %s: %v", id, err)
		}
		bundleData, err := bundle.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		bundlePath := dir + "/" + id + ".bundle.json"
		writeFile(t, bundlePath, bundleData)

		lockfile.Layers = append(lockfile.Layers, spec.ResolvedLayer{
			LayerManifest: spec.LayerManifest{
				ID:     id,
				SHA256: fileHex(t, sqfsPath),
				Bundle: bundlePath,
			},
			MountOrder: i + 1,
		})
	}

	verifier := &trust.FakeVerifier{}
	if err := trust.VerifyLayers(ctx, lockfile, dir, verifier); err != nil {
		t.Errorf("VerifyLayers() unexpected error: %v", err)
	}
}

// TestVerifyLayersFailsOnOneBadLayer tests that one bad layer fails the whole batch.
func TestVerifyLayersFailsOnOneBadLayer(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	signer := &trust.FakeSigner{}

	// Good layer — filename must be layerID + ".sqfs".
	goodID := "python-3.11.9-rhel-x86_64"
	goodPath := dir + "/" + goodID + ".sqfs"
	writeFile(t, goodPath, []byte("good content"))
	bundle, _ := signer.Sign(ctx, goodPath, nil)
	bundleData, _ := bundle.Marshal()
	bundlePath := dir + "/" + goodID + ".bundle.json"
	writeFile(t, bundlePath, bundleData)

	lockfile := &spec.LockFile{
		Layers: []spec.ResolvedLayer{
			{
				LayerManifest: spec.LayerManifest{
					ID:     goodID,
					SHA256: fileHex(t, goodPath),
					Bundle: bundlePath,
				},
				MountOrder: 1,
			},
			{
				// Layer with no SHA256 — will fail verification.
				LayerManifest: spec.LayerManifest{
					ID:     "bad-layer-rhel-x86_64",
					SHA256: "",
					Bundle: dir + "/bad.bundle.json",
				},
				MountOrder: 2,
			},
		},
	}

	verifier := &trust.FakeVerifier{}
	if err := trust.VerifyLayers(ctx, lockfile, dir, verifier); err == nil {
		t.Error("VerifyLayers should fail when one layer is invalid")
	}
}

// TestVerifyLayerFailsOnMissingSHA256 tests that manifests without SHA256 are rejected.
func TestVerifyLayerFailsOnMissingSHA256(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sqfsPath := dir + "/layer.sqfs"
	writeFile(t, sqfsPath, []byte("content"))

	manifest := &spec.LayerManifest{
		ID:     "test-layer",
		SHA256: "", // missing
		Bundle: dir + "/fake.bundle.json",
	}

	if err := trust.VerifyLayer(ctx, manifest, sqfsPath, &trust.FakeVerifier{}); err == nil {
		t.Error("VerifyLayer should reject manifest with empty SHA256")
	}
}

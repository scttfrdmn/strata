package registry

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/spec"
)

// sqfsPath creates a temporary file with fake sqfs content and returns its path.
func tempSqfsFile(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test-*.sqfs")
	if err != nil {
		t.Fatalf("creating temp sqfs: %v", err)
	}
	if _, err := f.Write([]byte("fake-sqfs-content")); err != nil {
		t.Fatalf("writing temp sqfs: %v", err)
	}
	f.Close() //nolint:errcheck
	return f.Name()
}

// mockS3 is a hand-written S3 mock that stores objects as raw bytes.
// ListObjectsV2 simulates common-prefix behaviour with the "/" delimiter.
// GetObject returns *types.NoSuchKey for missing keys.
// PutObject stores data into the map.
type mockS3 struct {
	objects map[string][]byte // S3 key → content
}

func newMockS3() *mockS3 {
	return &mockS3{objects: make(map[string][]byte)}
}

func (m *mockS3) put(key string, v any) {
	data, err := yaml.Marshal(v)
	if err != nil {
		panic(err)
	}
	m.objects[key] = data
}

func (m *mockS3) ListObjectsV2(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	prefix := ""
	if in.Prefix != nil {
		prefix = *in.Prefix
	}
	delimiter := ""
	if in.Delimiter != nil {
		delimiter = *in.Delimiter
	}

	seen := make(map[string]bool)
	out := &s3.ListObjectsV2Output{}

	for key := range m.objects {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		rest := key[len(prefix):]
		if delimiter != "" {
			idx := strings.Index(rest, delimiter)
			if idx >= 0 {
				cp := prefix + rest[:idx+len(delimiter)]
				if !seen[cp] {
					seen[cp] = true
					cp := cp
					out.CommonPrefixes = append(out.CommonPrefixes, types.CommonPrefix{Prefix: &cp})
				}
				continue
			}
		}
		key := key
		out.Contents = append(out.Contents, types.Object{Key: &key})
	}
	return out, nil
}

func (m *mockS3) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	key := ""
	if in.Key != nil {
		key = *in.Key
	}
	data, ok := m.objects[key]
	if !ok {
		return nil, &types.NoSuchKey{}
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(data))}, nil
}

func (m *mockS3) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	key := ""
	if in.Key != nil {
		key = *in.Key
	}
	data, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	m.objects[key] = data
	return &s3.PutObjectOutput{}, nil
}

// layerManifest is a helper to build a *spec.LayerManifest for tests.
func layerManifest(name, version, arch, abi string) *spec.LayerManifest {
	return &spec.LayerManifest{
		Name:    name,
		Version: version,
		Arch:    arch,
		ABI:     abi,
	}
}

// putIndex is a test helper that stores a LayerIndex at index/layers.yaml.
func putIndex(mock *mockS3, manifests ...*spec.LayerManifest) {
	mock.put("index/layers.yaml", LayerIndex{Layers: manifests})
}

// ---- ResolveLayer -----------------------------------------------------------

func TestResolveLayer_ReturnsNewest(t *testing.T) {
	mock := newMockS3()
	putIndex(mock,
		layerManifest("python", "3.11.7", "x86_64", "linux-gnu-2.34"),
		layerManifest("python", "3.11.9", "x86_64", "linux-gnu-2.34"),
		layerManifest("python", "3.11.8", "x86_64", "linux-gnu-2.34"),
	)
	c := newS3ClientWithAPI("bucket", mock)

	m, err := c.ResolveLayer(context.Background(), "python", "", "x86_64", "linux-gnu-2.34")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Version != "3.11.9" {
		t.Errorf("expected 3.11.9, got %s", m.Version)
	}
}

func TestResolveLayer_VersionPrefixFilter(t *testing.T) {
	mock := newMockS3()
	putIndex(mock,
		layerManifest("python", "3.11.9", "x86_64", "linux-gnu-2.34"),
		layerManifest("python", "3.12.1", "x86_64", "linux-gnu-2.34"),
	)
	c := newS3ClientWithAPI("bucket", mock)

	m, err := c.ResolveLayer(context.Background(), "python", "3.11", "x86_64", "linux-gnu-2.34")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Version != "3.11.9" {
		t.Errorf("expected 3.11.9, got %s", m.Version)
	}
}

func TestResolveLayer_NotFound(t *testing.T) {
	mock := newMockS3()
	putIndex(mock) // empty index
	c := newS3ClientWithAPI("bucket", mock)
	_, err := c.ResolveLayer(context.Background(), "python", "", "x86_64", "linux-gnu-2.34")
	if !IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestResolveLayer_VersionPrefixNoMatch(t *testing.T) {
	mock := newMockS3()
	putIndex(mock, layerManifest("python", "3.12.1", "x86_64", "linux-gnu-2.34"))
	c := newS3ClientWithAPI("bucket", mock)

	_, err := c.ResolveLayer(context.Background(), "python", "3.11", "x86_64", "linux-gnu-2.34")
	if !IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ---- ResolveFormation -------------------------------------------------------

func TestResolveFormation_HappyPath(t *testing.T) {
	mock := newMockS3()
	mock.put("formations/cuda-python-ml/2024.03/manifest.yaml", &spec.Formation{
		Name:    "cuda-python-ml",
		Version: "2024.03",
	})
	c := newS3ClientWithAPI("bucket", mock)

	f, err := c.ResolveFormation(context.Background(), "cuda-python-ml@2024.03", "x86_64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Name != "cuda-python-ml" {
		t.Errorf("expected cuda-python-ml, got %s", f.Name)
	}
}

func TestResolveFormation_NotFound(t *testing.T) {
	c := newS3ClientWithAPI("bucket", newMockS3())
	_, err := c.ResolveFormation(context.Background(), "missing@1.0", "x86_64")
	if !IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestResolveFormation_InvalidRef(t *testing.T) {
	c := newS3ClientWithAPI("bucket", newMockS3())
	_, err := c.ResolveFormation(context.Background(), "no-at-sign", "x86_64")
	if err == nil {
		t.Error("expected error for invalid ref")
	}
}

// ---- BaseCapabilities -------------------------------------------------------

func TestGetBaseCapabilities_NotFoundBeforeStore(t *testing.T) {
	c := newS3ClientWithAPI("bucket", newMockS3())
	_, err := c.GetBaseCapabilities(context.Background(), "ami-abc123")
	if !IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStoreAndGetBaseCapabilities_RoundTrip(t *testing.T) {
	c := newS3ClientWithAPI("bucket", newMockS3())
	caps := &spec.BaseCapabilities{
		AMIID: "ami-abc123",
		Arch:  "x86_64",
		ABI:   "linux-gnu-2.34",
	}

	if err := c.StoreBaseCapabilities(context.Background(), caps); err != nil {
		t.Fatalf("store: %v", err)
	}

	got, err := c.GetBaseCapabilities(context.Background(), "ami-abc123")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.AMIID != caps.AMIID {
		t.Errorf("AMIID: expected %s, got %s", caps.AMIID, got.AMIID)
	}
	if got.Arch != caps.Arch {
		t.Errorf("Arch: expected %s, got %s", caps.Arch, got.Arch)
	}
}

// ---- ListLayers -------------------------------------------------------------

func TestListLayers_FilterByName(t *testing.T) {
	mock := newMockS3()
	idx := LayerIndex{Layers: []*spec.LayerManifest{
		layerManifest("python", "3.12.1", "x86_64", "linux-gnu-2.34"),
		layerManifest("python", "3.11.9", "x86_64", "linux-gnu-2.34"),
		layerManifest("gcc", "13.2.0", "x86_64", "linux-gnu-2.34"),
	}}
	mock.put("index/layers.yaml", idx)
	c := newS3ClientWithAPI("bucket", mock)

	layers, err := c.ListLayers(context.Background(), "python", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(layers))
	}
	// newest first
	if layers[0].Version != "3.12.1" {
		t.Errorf("expected 3.12.1 first, got %s", layers[0].Version)
	}
}

func TestListLayers_FilterByArch(t *testing.T) {
	mock := newMockS3()
	idx := LayerIndex{Layers: []*spec.LayerManifest{
		layerManifest("python", "3.12.1", "x86_64", "linux-gnu-2.34"),
		layerManifest("python", "3.12.1", "arm64", "linux-gnu-2.34"),
	}}
	mock.put("index/layers.yaml", idx)
	c := newS3ClientWithAPI("bucket", mock)

	layers, err := c.ListLayers(context.Background(), "", "arm64", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layers) != 1 || layers[0].Arch != "arm64" {
		t.Errorf("expected 1 arm64 layer, got %v", layers)
	}
}

func TestListLayers_FilterByFamily(t *testing.T) {
	mock := newMockS3()
	idx := LayerIndex{Layers: []*spec.LayerManifest{
		layerManifest("python", "3.12.1", "x86_64", "linux-gnu-2.34"),
		layerManifest("python", "3.12.1", "x86_64", "linux-gnu-2.35"),
	}}
	mock.put("index/layers.yaml", idx)
	c := newS3ClientWithAPI("bucket", mock)

	layers, err := c.ListLayers(context.Background(), "", "", "linux-gnu-2.35")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layers) != 1 || layers[0].ABI != "linux-gnu-2.35" {
		t.Errorf("expected 1 linux-gnu-2.35 layer, got %v", layers)
	}
}

func TestListLayers_EmptyIndex(t *testing.T) {
	c := newS3ClientWithAPI("bucket", newMockS3())
	_, err := c.ListLayers(context.Background(), "", "", "")
	if !IsNotFound(err) {
		t.Errorf("expected ErrNotFound for missing index, got %v", err)
	}
}

func TestListLayers_NoMatchReturnsEmpty(t *testing.T) {
	mock := newMockS3()
	idx := LayerIndex{Layers: []*spec.LayerManifest{
		layerManifest("gcc", "13.2.0", "x86_64", "linux-gnu-2.34"),
	}}
	mock.put("index/layers.yaml", idx)
	c := newS3ClientWithAPI("bucket", mock)

	layers, err := c.ListLayers(context.Background(), "python", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layers) != 0 {
		t.Errorf("expected 0 layers, got %d", len(layers))
	}
}

// ---- NewS3Client URL parsing -------------------------------------------------

func TestParseBucketURL(t *testing.T) {
	tests := []struct {
		url    string
		want   string
		wantOK bool
	}{
		{"s3://my-bucket", "my-bucket", true},
		{"s3://my-bucket/", "my-bucket", true},
		{"s3://my-bucket/prefix/path", "my-bucket", true},
		{"https://bucket.s3.amazonaws.com", "", false},
		{"s3://", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := parseBucketURL(tt.url)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("parseBucketURL(%q) = (%q, %v), want (%q, %v)",
				tt.url, got, ok, tt.want, tt.wantOK)
		}
	}
}

// ---- PushLayer --------------------------------------------------------------

func TestPushLayer_UploadsThreeObjects(t *testing.T) {
	mock := newMockS3()
	c := newS3ClientWithAPI("bucket", mock)

	manifest := &spec.LayerManifest{
		ID: "python-3.11.9-rhel-x86_64", Name: "python", Version: "3.11.9",
		Arch: "x86_64", ABI: "linux-gnu-2.34",
	}
	sqfsPath := tempSqfsFile(t)

	if err := c.PushLayer(context.Background(), manifest, sqfsPath, []byte(`{"bundle":true}`)); err != nil {
		t.Fatalf("PushLayer: %v", err)
	}

	prefix := "layers/linux-gnu-2.34/x86_64/python/3.11.9/"
	for _, key := range []string{prefix + "layer.sqfs", prefix + "manifest.yaml", prefix + "bundle.json"} {
		if _, ok := mock.objects[key]; !ok {
			t.Errorf("expected key %q to be present in mock S3", key)
		}
	}
}

func TestPushLayer_UpdatesIndex(t *testing.T) {
	mock := newMockS3()
	c := newS3ClientWithAPI("bucket", mock)

	manifest := &spec.LayerManifest{
		ID: "gcc-13.2.0-rhel-x86_64", Name: "gcc", Version: "13.2.0",
		Arch: "x86_64", ABI: "linux-gnu-2.34",
	}
	sqfsPath := tempSqfsFile(t)

	if err := c.PushLayer(context.Background(), manifest, sqfsPath, []byte("{}")); err != nil {
		t.Fatalf("PushLayer: %v", err)
	}

	if _, ok := mock.objects["index/layers.yaml"]; !ok {
		t.Fatal("expected index/layers.yaml to be present")
	}

	layers, err := c.ListLayers(context.Background(), "gcc", "", "")
	if err != nil {
		t.Fatalf("ListLayers: %v", err)
	}
	if len(layers) != 1 || layers[0].Name != "gcc" {
		t.Errorf("expected 1 gcc layer, got %v", layers)
	}
}

func TestPushLayer_UpsertReplacesExisting(t *testing.T) {
	mock := newMockS3()
	c := newS3ClientWithAPI("bucket", mock)

	manifest := &spec.LayerManifest{
		ID: "gcc-13.2.0-rhel-x86_64", Name: "gcc", Version: "13.2.0",
		Arch: "x86_64", ABI: "linux-gnu-2.34", SHA256: "aaa",
	}
	sqfsPath := tempSqfsFile(t)

	if err := c.PushLayer(context.Background(), manifest, sqfsPath, []byte("{}")); err != nil {
		t.Fatalf("first PushLayer: %v", err)
	}

	// Push the same ID with an updated SHA256.
	manifest.SHA256 = "bbb"
	if err := c.PushLayer(context.Background(), manifest, sqfsPath, []byte("{}")); err != nil {
		t.Fatalf("second PushLayer: %v", err)
	}

	layers, err := c.ListLayers(context.Background(), "gcc", "", "")
	if err != nil {
		t.Fatalf("ListLayers: %v", err)
	}
	if len(layers) != 1 {
		t.Fatalf("expected 1 layer after upsert, got %d", len(layers))
	}
	if layers[0].SHA256 != "bbb" {
		t.Errorf("expected updated SHA256 bbb, got %q", layers[0].SHA256)
	}
}

// ---- RebuildIndex -----------------------------------------------------------

func TestRebuildIndex_ScansAllManifests(t *testing.T) {
	mock := newMockS3()
	c := newS3ClientWithAPI("bucket", mock)

	manifests := []*spec.LayerManifest{
		{ID: "gcc-13.2.0-rhel-x86_64", Name: "gcc", Version: "13.2.0", Arch: "x86_64", ABI: "linux-gnu-2.34"},
		{ID: "python-3.11.9-rhel-x86_64", Name: "python", Version: "3.11.9", Arch: "x86_64", ABI: "linux-gnu-2.34"},
		{ID: "python-3.11.9-rhel-arm64", Name: "python", Version: "3.11.9", Arch: "arm64", ABI: "linux-gnu-2.34"},
	}
	for _, m := range manifests {
		key := "layers/" + m.ABI + "/" + m.Arch + "/" + m.Name + "/" + m.Version + "/manifest.yaml"
		mock.put(key, m)
	}

	if err := c.RebuildIndex(context.Background()); err != nil {
		t.Fatalf("RebuildIndex: %v", err)
	}

	all, err := c.ListLayers(context.Background(), "", "", "")
	if err != nil {
		t.Fatalf("ListLayers: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 layers in rebuilt index, got %d", len(all))
	}
}

func TestRebuildIndex_EmptyRegistry(t *testing.T) {
	mock := newMockS3()
	c := newS3ClientWithAPI("bucket", mock)

	if err := c.RebuildIndex(context.Background()); err != nil {
		t.Fatalf("RebuildIndex on empty registry: %v", err)
	}

	if _, ok := mock.objects["index/layers.yaml"]; !ok {
		t.Fatal("expected index/layers.yaml to be written even for empty registry")
	}
}

// ---- Integration tests (skipped unless STRATA_TEST_BUCKET is set) -----------

func TestS3ClientIntegration(t *testing.T) {
	bucket := os.Getenv("STRATA_TEST_BUCKET")
	if bucket == "" {
		t.Skip("STRATA_TEST_BUCKET not set")
	}
	c, err := NewS3Client(bucket)
	if err != nil {
		t.Fatalf("NewS3Client: %v", err)
	}

	// Round-trip a BaseCapabilities record.
	caps := &spec.BaseCapabilities{
		AMIID: "ami-integration-test",
		Arch:  "x86_64",
		ABI:   "linux-gnu-2.34",
	}
	ctx := context.Background()
	if err := c.StoreBaseCapabilities(ctx, caps); err != nil {
		t.Fatalf("StoreBaseCapabilities: %v", err)
	}
	got, err := c.GetBaseCapabilities(ctx, caps.AMIID)
	if err != nil {
		t.Fatalf("GetBaseCapabilities: %v", err)
	}
	if got.AMIID != caps.AMIID {
		t.Errorf("AMIID mismatch: want %s, got %s", caps.AMIID, got.AMIID)
	}
}

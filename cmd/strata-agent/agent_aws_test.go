package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/spec"
)

// ---- IMDS mock --------------------------------------------------------------

type mockIMDS struct {
	userData    []byte
	userDataErr error
	metadata    map[string]string
	metadataErr map[string]error
}

func (m *mockIMDS) GetUserData(_ context.Context, _ *imds.GetUserDataInput,
	_ ...func(*imds.Options)) (*imds.GetUserDataOutput, error) {
	if m.userDataErr != nil {
		return nil, m.userDataErr
	}
	return &imds.GetUserDataOutput{
		Content: io.NopCloser(bytes.NewReader(m.userData)),
	}, nil
}

func (m *mockIMDS) GetMetadata(_ context.Context, in *imds.GetMetadataInput,
	_ ...func(*imds.Options)) (*imds.GetMetadataOutput, error) {
	if err, ok := m.metadataErr[in.Path]; ok {
		return nil, err
	}
	val, ok := m.metadata[in.Path]
	if !ok {
		return nil, errors.New("not found: " + in.Path)
	}
	return &imds.GetMetadataOutput{
		Content: io.NopCloser(bytes.NewReader([]byte(val))),
	}, nil
}

// ---- S3 GetObject mock -------------------------------------------------------

type mockS3Get struct {
	objects map[string][]byte
}

func (m *mockS3Get) GetObject(_ context.Context, in *s3.GetObjectInput,
	_ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	key := aws.ToString(in.Key)
	data, ok := m.objects[key]
	if !ok {
		return nil, &s3types.NoSuchKey{}
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(data))}, nil
}

// ---- EC2 CreateTags mock -----------------------------------------------------

type mockEC2Tag struct {
	tags map[string]string
}

func (m *mockEC2Tag) CreateTags(_ context.Context, in *ec2.CreateTagsInput,
	_ ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error) {
	for _, t := range in.Tags {
		m.tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return &ec2.CreateTagsOutput{}, nil
}

// ---- metadataLockfileSource tests -------------------------------------------

func TestAcquire_UserData(t *testing.T) {
	lf := &spec.LockFile{ProfileName: "my-profile"}
	data, err := yaml.Marshal(lf)
	if err != nil {
		t.Fatal(err)
	}
	src := newMetadataLockfileSourceWithAPIs(
		&mockIMDS{userData: data},
		nil,
	)
	got, err := src.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if got.ProfileName != "my-profile" {
		t.Errorf("ProfileName: got %q, want %q", got.ProfileName, "my-profile")
	}
}

func TestAcquire_Tag_S3(t *testing.T) {
	lf := &spec.LockFile{ProfileName: "s3-profile"}
	data, err := yaml.Marshal(lf)
	if err != nil {
		t.Fatal(err)
	}
	mock3 := &mockS3Get{objects: map[string][]byte{
		"envs/prod.lock.yaml": data,
	}}
	src := newMetadataLockfileSourceWithAPIs(
		&mockIMDS{
			userDataErr: errors.New("no user data"),
			metadata: map[string]string{
				"tags/instance/strata:lockfile-s3-uri": "s3://my-bucket/envs/prod.lock.yaml",
			},
		},
		mock3,
	)
	got, err := src.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if got.ProfileName != "s3-profile" {
		t.Errorf("ProfileName: got %q, want %q", got.ProfileName, "s3-profile")
	}
}

func TestAcquire_NoSource(t *testing.T) {
	src := newMetadataLockfileSourceWithAPIs(
		&mockIMDS{
			userDataErr: errors.New("no user data"),
			metadataErr: map[string]error{
				"tags/instance/strata:lockfile-s3-uri": errors.New("no tag"),
			},
		},
		nil,
	)
	_, err := src.Acquire(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---- s3LayerFetcher tests ----------------------------------------------------

func TestFetch_CacheHit(t *testing.T) {
	dir := t.TempDir()
	sha := "abc123"
	cached := filepath.Join(dir, sha+".sqfs")
	if err := os.WriteFile(cached, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	// S3 mock with no objects — a real S3 call would fail.
	f := newS3LayerFetcherWithAPI(&mockS3Get{objects: map[string][]byte{}}, dir)
	got, err := f.Fetch(context.Background(), spec.ResolvedLayer{
		LayerManifest: spec.LayerManifest{SHA256: sha, Source: "s3://bucket/layer.sqfs"},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got != cached {
		t.Errorf("got %q, want %q", got, cached)
	}
}

func TestFetch_CacheMiss(t *testing.T) {
	dir := t.TempDir()
	content := []byte("squashfs-layer-bytes")
	mock3 := &mockS3Get{objects: map[string][]byte{"layers/python.sqfs": content}}
	sha := "deadbeef"

	f := newS3LayerFetcherWithAPI(mock3, dir)
	got, err := f.Fetch(context.Background(), spec.ResolvedLayer{
		LayerManifest: spec.LayerManifest{
			SHA256: sha,
			Source: "s3://mybucket/layers/python.sqfs",
		},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	want := filepath.Join(dir, sha+".sqfs")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(data, content) {
		t.Errorf("file content mismatch")
	}
}

func TestFetch_S3Error(t *testing.T) {
	dir := t.TempDir()
	mock3 := &mockS3Get{objects: map[string][]byte{}} // empty — will 404

	f := newS3LayerFetcherWithAPI(mock3, dir)
	_, err := f.Fetch(context.Background(), spec.ResolvedLayer{
		LayerManifest: spec.LayerManifest{
			SHA256: "missing",
			Source: "s3://bucket/missing.sqfs",
		},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---- ec2ReadySignaler tests --------------------------------------------------

func testLockfile() *spec.LockFile {
	return &spec.LockFile{
		ProfileName:   "test-profile",
		ProfileSHA256: "abc",
		Base: spec.ResolvedBase{
			AMIID:     "ami-test",
			AMISHA256: "sha256base",
		},
	}
}

func TestSignalReady_Tags(t *testing.T) {
	mockTags := &mockEC2Tag{tags: make(map[string]string)}
	sig := newEC2ReadySignalerWithAPIs(
		&mockIMDS{metadata: map[string]string{"meta-data/instance-id": "i-abc123"}},
		mockTags,
	)

	if err := sig.SignalReady(context.Background(), testLockfile()); err != nil {
		t.Fatalf("SignalReady: %v", err)
	}
	if mockTags.tags["strata:status"] != "ready" {
		t.Errorf("strata:status: got %q, want %q", mockTags.tags["strata:status"], "ready")
	}
	if _, ok := mockTags.tags["strata:environment-id"]; !ok {
		t.Error("strata:environment-id tag not set")
	}
}

func TestSignalFailed_Tags(t *testing.T) {
	mockTags := &mockEC2Tag{tags: make(map[string]string)}
	sig := newEC2ReadySignalerWithAPIs(
		&mockIMDS{metadata: map[string]string{"meta-data/instance-id": "i-abc123"}},
		mockTags,
	)

	reason := errors.New("boot failure: missing layer")
	if err := sig.SignalFailed(context.Background(), reason); err != nil {
		t.Fatalf("SignalFailed: %v", err)
	}
	if mockTags.tags["strata:status"] != "failed" {
		t.Errorf("strata:status: got %q, want %q", mockTags.tags["strata:status"], "failed")
	}
	if mockTags.tags["strata:failure-reason"] != reason.Error() {
		t.Errorf("strata:failure-reason: got %q, want %q",
			mockTags.tags["strata:failure-reason"], reason.Error())
	}
}

func TestSignalFailed_TruncatesLongReason(t *testing.T) {
	mockTags := &mockEC2Tag{tags: make(map[string]string)}
	sig := newEC2ReadySignalerWithAPIs(
		&mockIMDS{metadata: map[string]string{"meta-data/instance-id": "i-abc123"}},
		mockTags,
	)

	long := make([]byte, 300)
	for i := range long {
		long[i] = 'x'
	}
	_ = sig.SignalFailed(context.Background(), errors.New(string(long)))

	got := mockTags.tags["strata:failure-reason"]
	if len(got) != 256 {
		t.Errorf("expected tag truncated to 256 chars, got %d", len(got))
	}
}

// ---- parseS3URI tests -------------------------------------------------------

func TestParseS3URI(t *testing.T) {
	tests := []struct {
		uri        string
		wantBucket string
		wantKey    string
		wantOK     bool
	}{
		{"s3://bucket/key/path", "bucket", "key/path", true},
		{"s3://bucket/key", "bucket", "key", true},
		{"s3://bucket/", "", "", false}, // empty key
		{"s3://bucket", "", "", false},  // no slash
		{"s3://", "", "", false},        // no bucket
		{"https://bucket.s3.amazonaws.com/key", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		b, k, ok := parseS3URI(tt.uri)
		if ok != tt.wantOK || b != tt.wantBucket || k != tt.wantKey {
			t.Errorf("parseS3URI(%q) = (%q,%q,%v), want (%q,%q,%v)",
				tt.uri, b, k, ok, tt.wantBucket, tt.wantKey, tt.wantOK)
		}
	}
}

// ---- suppress unused import warnings ----------------------------------------
// (ec2types is used in mockEC2Tag; this ensures the compiler is satisfied.)
var _ ec2types.Tag

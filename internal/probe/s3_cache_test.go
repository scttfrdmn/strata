package probe

import (
	"context"
	"errors"
	"testing"

	"github.com/scttfrdmn/strata/spec"
)

// mockCapStore is a hand-written mock of CapabilityStore used by S3Cache tests.
type mockCapStore struct {
	store map[string]*spec.BaseCapabilities
	// getErr is returned by GetBaseCapabilities when non-nil.
	getErr error
}

func newMockCapStore() *mockCapStore {
	return &mockCapStore{store: make(map[string]*spec.BaseCapabilities)}
}

func (m *mockCapStore) GetBaseCapabilities(_ context.Context, amiID string) (*spec.BaseCapabilities, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	caps, ok := m.store[amiID]
	if !ok {
		return nil, errors.New("not found")
	}
	return caps, nil
}

func (m *mockCapStore) StoreBaseCapabilities(_ context.Context, caps *spec.BaseCapabilities) error {
	m.store[caps.AMIID] = caps
	return nil
}

func TestS3Cache_GetMiss(t *testing.T) {
	t.Parallel()
	cache := NewS3Cache(newMockCapStore())

	caps, ok := cache.Get(context.Background(), "ami-missing")
	if ok {
		t.Error("expected cache miss, got hit")
	}
	if caps != nil {
		t.Error("expected nil caps on miss")
	}
}

func TestS3Cache_SetAndGet(t *testing.T) {
	t.Parallel()
	store := newMockCapStore()
	cache := NewS3Cache(store)

	want, err := KnownBaseCapabilities("al2023", "x86_64", "ami-test123")
	if err != nil {
		t.Fatalf("KnownBaseCapabilities: %v", err)
	}

	if err := cache.Set(context.Background(), "ami-test123", want); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, ok := cache.Get(context.Background(), "ami-test123")
	if !ok {
		t.Fatal("expected cache hit after Set")
	}
	if got.AMIID != want.AMIID {
		t.Errorf("got AMIID %q, want %q", got.AMIID, want.AMIID)
	}
	if got.OS != want.OS {
		t.Errorf("got OS %q, want %q", got.OS, want.OS)
	}
}

func TestS3Cache_GetError(t *testing.T) {
	t.Parallel()
	store := newMockCapStore()
	store.getErr = errors.New("s3: request timeout")
	cache := NewS3Cache(store)

	caps, ok := cache.Get(context.Background(), "ami-anything")
	if ok {
		t.Error("expected cache miss on error, got hit")
	}
	if caps != nil {
		t.Error("expected nil caps on error")
	}
}

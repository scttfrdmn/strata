package testregistry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/scttfrdmn/strata/internal/probe"
	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

// TB is the subset of *testing.T this package needs. Declaring it here rather
// than importing "testing" keeps the testing package out of the import graph of
// mkregistry and of anything else that materializes a fixture outside a test.
type TB interface {
	Helper()
	TempDir() string
	Fatalf(format string, args ...any)
}

// New materializes the fixture registry into a fresh temporary directory and
// returns its absolute root together with a client for it. The directory is
// removed by the test framework when the test ends.
func New(t TB) (root string, client *registry.LocalClient) {
	t.Helper()
	root, err := Materialize(context.Background(), filepath.Join(t.TempDir(), "registry"))
	if err != nil {
		t.Fatalf("materializing fixture registry: %v", err)
	}
	client, err = registry.NewLocalClient(URI(root))
	if err != nil {
		t.Fatalf("opening fixture registry at %s: %v", root, err)
	}
	return root, client
}

// URI returns the STRATA_REGISTRY_URL value for a materialized registry root.
func URI(root string) string {
	return uriFor(root)
}

// Base is the base OS the fixture layers are built for. Profiles that resolve
// against the fixture must declare it, because layer lookup filters on the ABI
// this OS provides (linux-gnu-2.34).
const Base = "al2023"

// BaseArch is the architecture the fixture layers are built for.
const BaseArch = "x86_64"

// Probe returns an offline probe.Client for the fixture's base OS: a static AMI
// table and KnownBaseCapabilities, with an in-memory cache. It never touches
// SSM or IMDS, so it behaves the same on a developer machine with credentials
// and in CI without them.
func Probe() (*probe.Client, error) {
	amiID := "ami-" + Base + "-" + BaseArch
	caps, err := probe.KnownBaseCapabilities(Base, BaseArch, amiID)
	if err != nil {
		return nil, fmt.Errorf("testregistry: base capabilities for %s/%s: %w", Base, BaseArch, err)
	}
	return &probe.Client{
		Resolver: &probe.StaticResolver{AMIs: map[string]string{Base + "/" + BaseArch: amiID}},
		Runner:   &probe.FakeRunner{Capabilities: map[string]*spec.BaseCapabilities{amiID: caps}},
		Cache:    probe.NewMemoryCache(),
	}, nil
}

// WriteProfile writes a fixture profile into dir and returns its path.
// name is one of the Profile* constants.
func WriteProfile(t TB, dir, name string) string {
	t.Helper()
	data, err := ProfileBytes(name)
	if err != nil {
		t.Fatalf("%v", err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("writing profile %s: %v", p, err)
	}
	return p
}

package packages_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scttfrdmn/strata/internal/packages"
	"github.com/scttfrdmn/strata/spec"
)

// newTestResolver returns a Resolver that routes all HTTP calls through srv.
func newTestResolver(srv *httptest.Server) *packages.Resolver {
	return &packages.Resolver{
		Client: srv.Client(),
		// The test server's URL — we rewrite all requests to point to it.
		BaseURL: srv.URL,
	}
}

func TestResolvePip_Latest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pypi/numpy/json" {
			http.NotFound(w, r)
			return
		}
		resp := map[string]interface{}{
			"info": map[string]string{"version": "1.26.4"},
			"urls": []map[string]interface{}{
				{
					"packagetype": "sdist",
					"digests":     map[string]string{"sha256": "abc123def456"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer srv.Close()

	r := newTestResolver(srv)
	sets, err := r.ResolveAll(context.Background(), []spec.PackageSpec{
		{Manager: spec.PackageManagerPip, Packages: []spec.PackageEntry{{Name: "numpy"}}},
	})
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	if len(sets) != 1 || len(sets[0].Packages) != 1 {
		t.Fatalf("unexpected result: %+v", sets)
	}
	got := sets[0].Packages[0]
	if got.Name != "numpy" {
		t.Errorf("name: got %q, want %q", got.Name, "numpy")
	}
	if got.Version != "1.26.4" {
		t.Errorf("version: got %q, want %q", got.Version, "1.26.4")
	}
	if got.SHA256 != "abc123def456" {
		t.Errorf("sha256: got %q, want %q", got.SHA256, "abc123def456")
	}
}

func TestResolvePip_SpecifiedVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pypi/numpy/1.25.0/json" {
			http.NotFound(w, r)
			return
		}
		resp := map[string]interface{}{
			"info": map[string]string{"version": "1.25.0"},
			"urls": []interface{}{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer srv.Close()

	r := newTestResolver(srv)
	sets, err := r.ResolveAll(context.Background(), []spec.PackageSpec{
		{Manager: spec.PackageManagerPip, Packages: []spec.PackageEntry{{Name: "numpy", Version: "1.25.0"}}},
	})
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	if sets[0].Packages[0].Version != "1.25.0" {
		t.Errorf("version: got %q, want 1.25.0", sets[0].Packages[0].Version)
	}
}

func TestResolvePip_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	r := newTestResolver(srv)
	_, err := r.ResolveAll(context.Background(), []spec.PackageSpec{
		{Manager: spec.PackageManagerPip, Packages: []spec.PackageEntry{{Name: "no-such-package-xyz"}}},
	})
	if err == nil {
		t.Error("expected error for missing package, got nil")
	}
}

func TestResolveCRAN_Latest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ggplot2" {
			http.NotFound(w, r)
			return
		}
		resp := map[string]interface{}{
			"Version":  "3.5.1",
			"versions": map[string]interface{}{"3.5.1": nil, "3.4.0": nil},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer srv.Close()

	r := newTestResolver(srv)
	sets, err := r.ResolveAll(context.Background(), []spec.PackageSpec{
		{Manager: spec.PackageManagerCRAN, Packages: []spec.PackageEntry{{Name: "ggplot2"}}},
	})
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	if sets[0].Packages[0].Version != "3.5.1" {
		t.Errorf("version: got %q, want 3.5.1", sets[0].Packages[0].Version)
	}
}

func TestResolveCRAN_SpecifiedVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ggplot2" {
			http.NotFound(w, r)
			return
		}
		resp := map[string]interface{}{
			"Version":  "3.5.1",
			"versions": map[string]interface{}{"3.5.1": nil, "3.4.0": nil},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer srv.Close()

	r := newTestResolver(srv)
	sets, err := r.ResolveAll(context.Background(), []spec.PackageSpec{
		{Manager: spec.PackageManagerCRAN, Packages: []spec.PackageEntry{{Name: "ggplot2", Version: "3.4.0"}}},
	})
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	if sets[0].Packages[0].Version != "3.4.0" {
		t.Errorf("version: got %q, want 3.4.0", sets[0].Packages[0].Version)
	}
}

func TestResolveCRAN_VersionNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ggplot2" {
			http.NotFound(w, r)
			return
		}
		resp := map[string]interface{}{
			"Version":  "3.5.1",
			"versions": map[string]interface{}{"3.5.1": nil},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer srv.Close()

	r := newTestResolver(srv)
	_, err := r.ResolveAll(context.Background(), []spec.PackageSpec{
		{Manager: spec.PackageManagerCRAN, Packages: []spec.PackageEntry{{Name: "ggplot2", Version: "2.0.0"}}},
	})
	if err == nil {
		t.Error("expected error for missing version, got nil")
	}
}

func TestResolveConda_PinsVersions(t *testing.T) {
	sets, err := packages.ResolveAll(context.Background(), []spec.PackageSpec{
		{
			Manager: spec.PackageManagerConda,
			Packages: []spec.PackageEntry{
				{Name: "scipy", Version: "1.12.0"},
				{Name: "matplotlib"}, // no version → pinned as "latest"
			},
		},
	})
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	if len(sets) != 1 || len(sets[0].Packages) != 2 {
		t.Fatalf("unexpected result: %+v", sets)
	}
	if sets[0].Packages[0].Version != "1.12.0" {
		t.Errorf("scipy version: got %q, want 1.12.0", sets[0].Packages[0].Version)
	}
	if sets[0].Packages[1].Version != "latest" {
		t.Errorf("matplotlib version: got %q, want latest", sets[0].Packages[1].Version)
	}
}

func TestResolveAll_UnsupportedManager(t *testing.T) {
	_, err := packages.ResolveAll(context.Background(), []spec.PackageSpec{
		{Manager: "homebrew", Packages: []spec.PackageEntry{{Name: "foo"}}},
	})
	if err == nil {
		t.Error("expected error for unsupported manager, got nil")
	}
}

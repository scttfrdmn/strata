// Package packages resolves package manager declarations to exact pinned versions.
//
// It queries public registries (PyPI for pip, crandb for CRAN) to find the
// latest or specified version of each package, returning a ResolvedPackageSet
// suitable for inclusion in a lockfile. Conda packages are pinned as-declared
// because no stable public REST API exists for conda-forge.
package packages

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/scttfrdmn/strata/spec"
)

// defaultHTTPClient is a shared client used when Resolver.Client is nil.
// Timeout prevents hung requests to PyPI / CRAN registries.
var defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}

// Resolver resolves package declarations to pinned versions using public registries.
// The zero value is ready to use (HTTP calls use http.DefaultClient).
type Resolver struct {
	// Client is the HTTP client used for registry API calls.
	// nil uses http.DefaultClient.
	Client *http.Client

	// BaseURL overrides the registry base URL for testing.
	// When set, PyPI calls use "<BaseURL>/pypi/..." and CRAN calls use "<BaseURL>/...".
	// Empty string (default) uses the real public registry URLs.
	BaseURL string
}

func (r *Resolver) httpClient() *http.Client {
	if r.Client != nil {
		return r.Client
	}
	return defaultHTTPClient
}

func (r *Resolver) pypiURL(name, version string) string {
	base := "https://pypi.org"
	if r.BaseURL != "" {
		base = r.BaseURL
	}
	if version != "" {
		return fmt.Sprintf("%s/pypi/%s/%s/json", base, name, version)
	}
	return fmt.Sprintf("%s/pypi/%s/json", base, name)
}

func (r *Resolver) cranURL(name string) string {
	base := "https://crandb.r-pkg.org"
	if r.BaseURL != "" {
		base = r.BaseURL
	}
	return fmt.Sprintf("%s/%s", base, name)
}

// ResolveAll resolves each PackageSpec in the profile to pinned versions.
// pip and cran query public APIs; conda pins versions as-declared.
func ResolveAll(ctx context.Context, specs []spec.PackageSpec) ([]spec.ResolvedPackageSet, error) {
	return (&Resolver{}).ResolveAll(ctx, specs)
}

// ResolveAll resolves each PackageSpec to pinned versions using the receiver's HTTP client.
func (r *Resolver) ResolveAll(ctx context.Context, specs []spec.PackageSpec) ([]spec.ResolvedPackageSet, error) {
	sets := make([]spec.ResolvedPackageSet, 0, len(specs))
	for _, ps := range specs {
		var (
			entries []spec.ResolvedPackageEntry
			err     error
		)
		switch ps.Manager {
		case spec.PackageManagerPip:
			entries, err = r.resolvePip(ctx, ps.Packages)
		case spec.PackageManagerCRAN:
			entries, err = r.resolveCRAN(ctx, ps.Packages)
		case spec.PackageManagerConda:
			entries, err = resolveConda(ps.Packages)
		default:
			return nil, fmt.Errorf("packages: unsupported manager %q", ps.Manager)
		}
		if err != nil {
			return nil, fmt.Errorf("packages: resolving %s packages: %w", ps.Manager, err)
		}
		sets = append(sets, spec.ResolvedPackageSet{
			Manager:  ps.Manager,
			Env:      ps.Env,
			Packages: entries,
		})
	}
	return sets, nil
}

// pypiInfoResponse is the subset of the PyPI JSON API response we need.
type pypiInfoResponse struct {
	Info struct {
		Version string `json:"version"`
	} `json:"info"`
	URLs []struct {
		PackageType string            `json:"packagetype"`
		Digests     map[string]string `json:"digests"`
	} `json:"urls"`
}

// resolvePip queries https://pypi.org/pypi/<name>/json for each entry.
// Returns pinned version + SHA256 of the source distribution when available.
func (r *Resolver) resolvePip(ctx context.Context, entries []spec.PackageEntry) ([]spec.ResolvedPackageEntry, error) {
	resolved := make([]spec.ResolvedPackageEntry, 0, len(entries))
	for _, entry := range entries {
		url := r.pypiURL(entry.Name, entry.Version)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("pip: building request for %q: %w", entry.Name, err)
		}

		resp, err := r.httpClient().Do(req)
		if err != nil {
			return nil, fmt.Errorf("pip: fetching info for %q: %w", entry.Name, err)
		}
		const maxRegistryBytes = 10 << 20 // 10 MiB; PyPI JSON responses are typically < 100 KiB
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxRegistryBytes))
		resp.Body.Close() //nolint:errcheck
		if readErr != nil {
			return nil, fmt.Errorf("pip: reading response for %q: %w", entry.Name, readErr)
		}

		switch resp.StatusCode {
		case http.StatusNotFound:
			if entry.Version != "" {
				return nil, fmt.Errorf("pip: package %q version %q not found on PyPI", entry.Name, entry.Version)
			}
			return nil, fmt.Errorf("pip: package %q not found on PyPI", entry.Name)
		case http.StatusOK:
			// ok
		default:
			return nil, fmt.Errorf("pip: unexpected status %d for %q", resp.StatusCode, entry.Name)
		}

		var info pypiInfoResponse
		if err := json.Unmarshal(body, &info); err != nil {
			return nil, fmt.Errorf("pip: parsing response for %q: %w", entry.Name, err)
		}

		// Find the SHA256 of the sdist (first match).
		var sha256 string
		for _, u := range info.URLs {
			if u.PackageType == "sdist" {
				sha256 = u.Digests["sha256"]
				break
			}
		}

		resolved = append(resolved, spec.ResolvedPackageEntry{
			Name:    entry.Name,
			Version: info.Info.Version,
			SHA256:  sha256,
		})
	}
	return resolved, nil
}

// cranInfoResponse is the subset of the CRAN DB JSON API response we need.
type cranInfoResponse struct {
	Version  string                 `json:"Version"`
	Versions map[string]interface{} `json:"versions"`
}

// resolveCRAN queries https://crandb.r-pkg.org/<name> for each entry.
// Returns the latest version (or validates a specified version exists).
func (r *Resolver) resolveCRAN(ctx context.Context, entries []spec.PackageEntry) ([]spec.ResolvedPackageEntry, error) {
	resolved := make([]spec.ResolvedPackageEntry, 0, len(entries))
	for _, entry := range entries {
		url := r.cranURL(entry.Name)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("cran: building request for %q: %w", entry.Name, err)
		}

		resp, err := r.httpClient().Do(req)
		if err != nil {
			return nil, fmt.Errorf("cran: fetching info for %q: %w", entry.Name, err)
		}
		const maxRegistryBytes = 10 << 20 // 10 MiB; CRAN JSON responses are typically < 50 KiB
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxRegistryBytes))
		resp.Body.Close() //nolint:errcheck
		if readErr != nil {
			return nil, fmt.Errorf("cran: reading response for %q: %w", entry.Name, readErr)
		}

		switch resp.StatusCode {
		case http.StatusNotFound:
			return nil, fmt.Errorf("cran: package %q not found on CRAN", entry.Name)
		case http.StatusOK:
			// ok
		default:
			return nil, fmt.Errorf("cran: unexpected status %d for %q", resp.StatusCode, entry.Name)
		}

		var info cranInfoResponse
		if err := json.Unmarshal(body, &info); err != nil {
			return nil, fmt.Errorf("cran: parsing response for %q: %w", entry.Name, err)
		}

		version := info.Version
		if entry.Version != "" {
			// Validate that the requested version is listed in the versions map.
			if _, ok := info.Versions[entry.Version]; !ok {
				return nil, fmt.Errorf("cran: package %q version %q not found on CRAN", entry.Name, entry.Version)
			}
			version = entry.Version
		}

		resolved = append(resolved, spec.ResolvedPackageEntry{
			Name:    entry.Name,
			Version: version,
		})
	}
	return resolved, nil
}

// resolveConda pins versions as-declared. No public REST API exists for
// conda-forge that returns stable content addresses, so conda packages are
// version-pinned only. The agent installs them via `conda install` at boot.
func resolveConda(entries []spec.PackageEntry) ([]spec.ResolvedPackageEntry, error) {
	resolved := make([]spec.ResolvedPackageEntry, 0, len(entries))
	for _, entry := range entries {
		version := entry.Version
		if version == "" {
			// No version pinned — the agent installs the latest at boot time.
			// Use "latest" as a sentinel so the lockfile records the intent.
			version = "latest"
		}
		resolved = append(resolved, spec.ResolvedPackageEntry{
			Name:    entry.Name,
			Version: version,
		})
	}
	return resolved, nil
}

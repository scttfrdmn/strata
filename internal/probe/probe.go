// Package probe resolves OS aliases to AMI IDs and produces BaseCapabilities
// for the resolver's Stage 1 (base resolution).
//
// The key design principle: layers declare requirements against capabilities,
// not OS names. "glibc@>=2.34" not "al2023". A single layer artifact runs on
// AL2023, Rocky 9, Rocky 10, and RHEL 9 because they share the "rhel" family
// and all ship glibc >= 2.34.
//
// Probes are cached by AMI ID. The same AMI probed twice returns the cached
// result. Probing a new AMI requires an EC2 launch (integration-only).
package probe

import (
	"context"
	"fmt"
	"time"

	"github.com/scttfrdmn/strata/spec"
)

// OSAlias maps a user-facing OS name to the SSM parameter path that holds
// the current AMI ID for that OS on each architecture.
//
// This is the canonical alias table. Adding a new supported OS means adding
// an entry here and verifying that its probed capabilities include the expected
// family and glibc version.
var osAliasSSM = map[string]map[string]string{
	"al2023": {
		"x86_64": "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64",
		"arm64":  "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64",
	},
	"rocky9": {
		"x86_64": "/aws/service/marketplace/prod-ami/rocky-linux-9-x86_64-latest",
		"arm64":  "/aws/service/marketplace/prod-ami/rocky-linux-9-arm64-latest",
	},
	"rocky10": {
		"x86_64": "/aws/service/marketplace/prod-ami/rocky-linux-10-x86_64-latest",
		"arm64":  "/aws/service/marketplace/prod-ami/rocky-linux-10-arm64-latest",
	},
	"ubuntu24": {
		"x86_64": "/aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvm/ebs-gp3/ami-id",
		"arm64":  "/aws/service/canonical/ubuntu/server/24.04/stable/current/arm64/hvm/ebs-gp3/ami-id",
	},
}

// OSFamily maps a Strata OS name to its capability family.
// The family is used by the resolver to filter the layer catalog:
// all layers tagged family=rhel are candidates for any rhel-family OS.
var OSFamily = map[string]string{
	"al2023":   "rhel",
	"rocky9":   "rhel",
	"rocky10":  "rhel",
	"ubuntu24": "debian",
}

// ResolveSSMParam is the SSM parameter path for a given OS and arch.
// Returns an error if the OS or arch is not recognized.
func ResolveSSMParam(os, arch string) (string, error) {
	archMap, ok := osAliasSSM[os]
	if !ok {
		return "", fmt.Errorf("unknown OS %q — supported: al2023, rocky9, rocky10, ubuntu24", os)
	}
	param, ok := archMap[arch]
	if !ok {
		return "", fmt.Errorf("unknown arch %q for OS %q — supported: x86_64, arm64", arch, os)
	}
	return param, nil
}

// Resolver resolves an OS alias and arch to an AMI ID.
// The real implementation queries AWS SSM; use StaticResolver for testing.
type Resolver interface {
	// ResolveAMI resolves os + arch to an AMI ID.
	ResolveAMI(ctx context.Context, os, arch string) (amiID string, err error)
}

// Runner executes a probe on a base AMI and returns its capabilities.
// The real implementation launches a t3.micro on EC2, runs the probe script,
// and terminates the instance. Use FakeRunner for testing.
type Runner interface {
	// ProbeAMI launches a probe instance for amiID and returns the
	// discovered capabilities. The probe must complete within 90 seconds.
	ProbeAMI(ctx context.Context, amiID, arch string) (*spec.BaseCapabilities, error)
}

// Cache stores and retrieves BaseCapabilities keyed by AMI ID.
// The real implementation is S3-backed. Use MemoryCache for testing.
type Cache interface {
	// Get returns the cached capabilities for amiID, if present.
	Get(ctx context.Context, amiID string) (*spec.BaseCapabilities, bool)

	// Set stores capabilities for amiID.
	Set(ctx context.Context, amiID string, caps *spec.BaseCapabilities) error
}

// Client is the high-level probe client used by the resolver.
// It combines Resolver, Runner, and Cache into a single interface.
type Client struct {
	Resolver Resolver
	Runner   Runner
	Cache    Cache
}

// GetCapabilities returns the BaseCapabilities for the given OS alias and arch.
// It first resolves the OS alias to an AMI ID, then checks the cache, and
// only runs a probe if there is no cache hit.
func (c *Client) GetCapabilities(ctx context.Context, os, arch string) (*spec.BaseCapabilities, error) {
	amiID, err := c.Resolver.ResolveAMI(ctx, os, arch)
	if err != nil {
		return nil, fmt.Errorf("resolving AMI for %s/%s: %w", os, arch, err)
	}

	if caps, ok := c.Cache.Get(ctx, amiID); ok {
		return caps, nil
	}

	caps, err := c.Runner.ProbeAMI(ctx, amiID, arch)
	if err != nil {
		return nil, fmt.Errorf("probing AMI %s: %w", amiID, err)
	}

	if err := c.Cache.Set(ctx, amiID, caps); err != nil {
		// Cache write failure is non-fatal — we have the capabilities.
		// Log it but proceed.
		_ = err // TODO: structured logging
	}

	return caps, nil
}

// AMIResult is the full result of resolving an OS alias, including both
// the AMI ID and the probed capabilities.
type AMIResult struct {
	// AMIID is the resolved EC2 AMI ID.
	AMIID string
	// Capabilities are the probed capabilities of the AMI.
	Capabilities *spec.BaseCapabilities
}

// Resolve is a convenience wrapper that resolves and probes in one call,
// returning the combined AMIResult for lockfile assembly.
func (c *Client) Resolve(ctx context.Context, os, arch string) (*AMIResult, error) {
	amiID, err := c.Resolver.ResolveAMI(ctx, os, arch)
	if err != nil {
		return nil, fmt.Errorf("resolving AMI for %s/%s: %w", os, arch, err)
	}

	caps, err := c.GetCapabilities(ctx, os, arch)
	if err != nil {
		return nil, err
	}

	return &AMIResult{AMIID: amiID, Capabilities: caps}, nil
}

// StaticResolver implements Resolver with a fixed map of OS→AMI.
// Used for testing and for offline scenarios where SSM is unavailable.
type StaticResolver struct {
	// AMIs maps "os/arch" → AMI ID.
	AMIs map[string]string
}

// ResolveAMI returns the AMI ID from the static map.
func (r *StaticResolver) ResolveAMI(_ context.Context, os, arch string) (string, error) {
	key := os + "/" + arch
	id, ok := r.AMIs[key]
	if !ok {
		return "", fmt.Errorf("static resolver: no AMI configured for %q", key)
	}
	return id, nil
}

// FakeRunner implements Runner with pre-configured capability results.
// It does not launch any EC2 instances.
type FakeRunner struct {
	// Capabilities maps AMI ID → BaseCapabilities.
	Capabilities map[string]*spec.BaseCapabilities
}

// ProbeAMI returns the pre-configured capabilities for amiID.
func (r *FakeRunner) ProbeAMI(_ context.Context, amiID, _ string) (*spec.BaseCapabilities, error) {
	caps, ok := r.Capabilities[amiID]
	if !ok {
		return nil, fmt.Errorf("FakeRunner: no capabilities configured for AMI %q", amiID)
	}
	return caps, nil
}

// MemoryCache implements Cache with an in-memory map. Safe for concurrent use
// only when accessed from a single goroutine. For testing.
type MemoryCache struct {
	entries map[string]*spec.BaseCapabilities
}

// NewMemoryCache returns an empty MemoryCache.
func NewMemoryCache() *MemoryCache {
	return &MemoryCache{entries: make(map[string]*spec.BaseCapabilities)}
}

// Get returns the cached capabilities for amiID.
func (c *MemoryCache) Get(_ context.Context, amiID string) (*spec.BaseCapabilities, bool) {
	caps, ok := c.entries[amiID]
	return caps, ok
}

// Set stores capabilities for amiID.
func (c *MemoryCache) Set(_ context.Context, amiID string, caps *spec.BaseCapabilities) error {
	c.entries[amiID] = caps
	return nil
}

// KnownBaseCapabilities returns realistic BaseCapabilities for well-known
// Strata base OS images. Used to bootstrap the registry's probe cache and
// for testing without running actual probes.
func KnownBaseCapabilities(os, arch, amiID string) (*spec.BaseCapabilities, error) {
	family, ok := OSFamily[os]
	if !ok {
		return nil, fmt.Errorf("unknown OS %q", os)
	}

	var provides []spec.Capability
	var systemCompiler string
	switch os {
	case "al2023":
		provides = []spec.Capability{
			{Name: "glibc", Version: "2.34"},
			{Name: "kernel", Version: "6.1"},
			{Name: "systemd", Version: "252"},
			{Name: "rpm", Version: "4.16"},
			{Name: "family", Version: family},
		}
		// AL2023 ships gcc 11 as the system compiler, locked for the lifetime of the distro.
		systemCompiler = "gcc-11.4.1-2.amzn2023.0.1." + arch
	case "rocky9":
		provides = []spec.Capability{
			{Name: "glibc", Version: "2.34"},
			{Name: "kernel", Version: "5.14"},
			{Name: "systemd", Version: "252"},
			{Name: "rpm", Version: "4.16"},
			{Name: "family", Version: family},
		}
		systemCompiler = "gcc-11.4.1-3.el9." + arch
	case "rocky10":
		provides = []spec.Capability{
			{Name: "glibc", Version: "2.39"},
			{Name: "kernel", Version: "6.8"},
			{Name: "systemd", Version: "255"},
			{Name: "rpm", Version: "4.19"},
			{Name: "family", Version: family},
		}
		systemCompiler = "gcc-14.2.1-6.el10." + arch
	case "ubuntu24":
		provides = []spec.Capability{
			{Name: "glibc", Version: "2.39"},
			{Name: "kernel", Version: "6.8"},
			{Name: "systemd", Version: "255"},
			{Name: "dpkg", Version: "1.22"},
			{Name: "family", Version: family},
		}
		// Ubuntu 24.04 ships gcc-13 as the default system compiler.
		systemCompiler = "gcc-13-13.2.0-23ubuntu4-" + arch
	default:
		return nil, fmt.Errorf("no known capabilities for OS %q", os)
	}

	return &spec.BaseCapabilities{
		AMIID:          amiID,
		OS:             os,
		Arch:           arch,
		Family:         family,
		ProbedAt:       time.Now(),
		SystemCompiler: systemCompiler,
		Provides:       provides,
	}, nil
}

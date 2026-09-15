// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

// Package strata provides a Go library API for Strata environment resolution.
//
// It is the integration surface for external tools (e.g. spore.host/spawn) that
// need to resolve a software selection into a deployable lockfile without
// importing internal packages directly.
//
// Typical usage:
//
//	c, err := strata.NewClient(ctx, strata.Options{RegistryURL: "s3://strata-registry"})
//	lockfile, err := c.Resolve(ctx, profile, strata.ResolveOptions{})
//	uri, err := c.UploadLockfile(ctx, lockfile)
//	// set EC2 tag strata:lockfile-s3-uri = uri, then launch instance
package strata

import (
	"context"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/probe"
	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/internal/resolver"
	"github.com/scttfrdmn/strata/spec"
)

const maxUserDataBytes = 16 * 1024 // EC2 user-data hard limit (16 KB)

// Options configures a Client.
type Options struct {
	// RegistryURL is the S3 URL of the Strata registry bucket.
	// Example: "s3://strata-registry"
	RegistryURL string

	// StrataVersion is embedded in generated lockfiles.
	// Optional; defaults to empty string.
	StrataVersion string

	// Warnings, if non-nil, receives the resolver's advisory warnings — most
	// importantly that a resolved formation carries a placeholder attestation
	// ("pending-initial-build") and so has no Rekor entry, which is the only
	// signal that the environment is not cryptographically verifiable (the
	// placeholder is never written into the lockfile, #46/#138). It is left nil
	// by default deliberately: a library that writes to the process's stderr
	// unbidden is a surprise, so the caller opts in by supplying a writer
	// (e.g. os.Stderr).
	Warnings io.Writer
}

// ResolveOptions controls how a Profile is resolved to a LockFile.
type ResolveOptions struct {
	// AMI is the EC2 AMI ID to embed in the lockfile's BaseCapabilities.
	// Optional; if empty, "ami-unknown" is used. Does not affect which
	// layers are selected — only the recorded metadata.
	AMI string
}

// Client provides Strata catalog and resolution operations backed by an S3 registry.
type Client struct {
	s3c      *registry.S3Client // nil when constructed via newClientFromRegistry
	reg      registry.Client    // always set; equals s3c when using S3
	version  string
	warnings io.Writer // nil = advisory warnings are discarded (Options.Warnings)
}

// NewClient creates a Client backed by the given S3 registry.
func NewClient(_ context.Context, opts Options) (*Client, error) {
	s3c, err := registry.NewS3Client(opts.RegistryURL)
	if err != nil {
		return nil, fmt.Errorf("strata: %w", err)
	}
	return &Client{s3c: s3c, reg: s3c, version: opts.StrataVersion, warnings: opts.Warnings}, nil
}

// clientOption configures a Client built through the internal registry-injection
// seam (newClientFromRegistry). It exists so that seam can accept a warnings
// writer without changing its two-argument signature, which inherited tests
// call directly.
type clientOption func(*Client)

// withWarnings sets the writer that receives the resolver's advisory warnings.
func withWarnings(w io.Writer) clientOption {
	return func(c *Client) { c.warnings = w }
}

// newClientFromRegistry creates a Client using an existing registry.Client
// implementation. UploadLockfile is unavailable when using this constructor (it
// requires a real S3Client).
//
// It is unexported because registry.Client lives under internal/, so an external
// module could never name an argument for it — an exported constructor here was
// public API that no public caller could invoke (#76). It remains a test seam,
// exposed to this package's external tests through export_test.go. External
// consumers construct a Client with NewClient.
func newClientFromRegistry(reg registry.Client, strataVersion string, opts ...clientOption) *Client {
	c := &Client{reg: reg, version: strataVersion}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Resolve transforms a Profile into a fully resolved LockFile.
//
// Base capabilities are synthesised from profile.Base.OS without a live AWS
// probe, so resolution succeeds offline.
//
// This offline resolve does NOT verify any signature. The resolver is built
// with no Rekor verifier, and stage 7 in that configuration only checks that
// each layer names a non-empty Bundle and RekorEntry — it parses no bundle,
// checks no signature, and inspects no certificate identity. A placeholder such
// as "pending-initial-build" passes. So a lockfile produced here is not a trust
// decision: verify it out of band (e.g. "strata verify --rekor", which fetches
// and checks the bundle) before relying on it. Making the offline path verify
// bundle payloads is tracked in #60/#62.
//
// profile.Base.OS must be a recognised alias ("al2023", "rocky9", "ubuntu24").
func (c *Client) Resolve(ctx context.Context, profile *spec.Profile, opts ResolveOptions) (*spec.LockFile, error) {
	os := profile.Base.OS
	arch := profile.Base.NormalizedArch()

	amiID := opts.AMI
	if amiID == "" {
		amiID = "ami-unknown"
	}

	caps, err := probe.KnownBaseCapabilities(os, arch, amiID)
	if err != nil {
		return nil, fmt.Errorf("strata: base capabilities for %q: %w", os, err)
	}

	cache := probe.NewMemoryCache()
	if err := cache.Set(ctx, amiID, caps); err != nil {
		return nil, fmt.Errorf("strata: seeding probe cache: %w", err)
	}

	probeClient := &probe.Client{
		Resolver: &probe.StaticResolver{AMIs: map[string]string{os + "/" + arch: amiID}},
		Runner:   &probe.FakeRunner{Capabilities: map[string]*spec.BaseCapabilities{amiID: caps}},
		Cache:    cache,
	}

	r, err := resolver.New(resolver.Config{
		Registry:      c.reg,
		Probe:         probeClient,
		StrataVersion: c.version,
		Warnings:      c.warnings,
	})
	if err != nil {
		return nil, fmt.Errorf("strata: creating resolver: %w", err)
	}
	return r.Resolve(ctx, profile)
}

// UploadLockfile stores the lockfile in S3 under locks/<environmentID>.yaml
// and returns its S3 URI (e.g. "s3://strata-registry/locks/abc123.yaml").
//
// Set the returned URI as EC2 instance tag strata:lockfile-s3-uri; strata-agent
// fetches and applies the lockfile at boot.
//
// UploadLockfile requires a Client created with NewClient (it needs a real
// S3Client).
func (c *Client) UploadLockfile(ctx context.Context, lockfile *spec.LockFile) (string, error) {
	// Validate before anything else: a lockfile uploaded here is fetched and
	// mounted by the agent, so the library route must not be able to publish a
	// structurally invalid one (malformed digest, unsafe layer id, duplicate
	// mount_order). The CLI boundaries validate; this closes the library path.
	// It runs before the S3-client check so the input is judged on its own terms.
	if err := lockfile.Validate(); err != nil {
		return "", fmt.Errorf("strata: %w", err)
	}
	if c.s3c == nil {
		return "", fmt.Errorf("strata: UploadLockfile requires an S3-backed Client (use NewClient)")
	}
	uri, err := c.s3c.PutLockfile(ctx, lockfile)
	if err != nil {
		return "", fmt.Errorf("strata: %w", err)
	}
	return uri, nil
}

// LockfileUserData YAML-encodes the lockfile for direct use as EC2 user-data.
//
// strata-agent treats user-data containing a valid YAML LockFile as the
// highest-priority lockfile source — no S3 tag needed. This is simpler than
// UploadLockfile for small environments but returns an error if the encoded
// YAML exceeds the 16 KB EC2 user-data hard limit. Use UploadLockfile for
// large lockfiles or when keeping user-data free for other content.
func LockfileUserData(lockfile *spec.LockFile) (string, error) {
	// The agent treats user-data as its highest-priority lockfile source, so a
	// malformed lockfile placed here reaches the boot path. Validate before
	// serialising — the same structural check the CLI and UploadLockfile apply.
	if err := lockfile.Validate(); err != nil {
		return "", fmt.Errorf("strata: %w", err)
	}
	data, err := yaml.Marshal(lockfile)
	if err != nil {
		return "", fmt.Errorf("strata: marshalling lockfile: %w", err)
	}
	if len(data) > maxUserDataBytes {
		return "", fmt.Errorf("strata: lockfile YAML (%d bytes) exceeds 16 KB EC2 user-data limit; use UploadLockfile instead", len(data))
	}
	return string(data), nil
}

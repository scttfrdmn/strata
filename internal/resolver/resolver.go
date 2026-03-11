// Package resolver implements the 8-stage Strata resolution pipeline.
//
// The resolver transforms a *spec.Profile into a *spec.LockFile. It wires
// registry.Client, probe.Client, and trust.RekorClient together into a
// deterministic, fail-fast pipeline.
//
// No partial lockfiles: every stage is a clean pass or a hard stop with an
// actionable error. Callers either receive a fully populated LockFile or a
// *ResolutionError with stage, code, and a message pointing to the fix.
package resolver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/probe"
	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// Config holds the external dependencies for a Resolver.
type Config struct {
	// Registry is the layer catalog client. Required.
	Registry registry.Client

	// Probe resolves OS aliases to AMI IDs and BaseCapabilities. Required.
	Probe *probe.Client

	// Rekor is the transparency log client for Sigstore verification.
	// Optional: when nil, bundle and Rekor entry presence are still required
	// but no live API verification is performed.
	Rekor trust.RekorClient

	// StrataVersion is written into the resolved LockFile.
	StrataVersion string
}

// Resolver transforms a *spec.Profile into a *spec.LockFile via an
// 8-stage deterministic pipeline.
type Resolver struct {
	cfg Config
}

// New creates a Resolver and validates that required config fields are set.
func New(cfg Config) (*Resolver, error) {
	if cfg.Registry == nil {
		return nil, fmt.Errorf("resolver: Registry is required")
	}
	if cfg.Probe == nil {
		return nil, fmt.Errorf("resolver: Probe is required")
	}
	return &Resolver{cfg: cfg}, nil
}

// resolvedLayer is the internal accumulator for a single resolved layer.
// It is unexported and used only within the resolver pipeline.
type resolvedLayer struct {
	manifest      *spec.LayerManifest
	satisfiedBy   string // SoftwareRef.String() from the profile
	fromFormation string // "name@version" if expanded from a formation; empty for standalone refs
}

// Resolve transforms profile into a fully resolved LockFile.
// If any stage fails the entire resolution fails — no partial lockfiles are returned.
func (r *Resolver) Resolve(ctx context.Context, profile *spec.Profile) (*spec.LockFile, error) {
	if err := profile.Validate(); err != nil {
		return nil, fmt.Errorf("invalid profile: %w", err)
	}

	arch := profile.Base.NormalizedArch()

	// Compute profile SHA256 for lockfile identity.
	profileBytes, err := yaml.Marshal(profile)
	if err != nil {
		return nil, fmt.Errorf("resolver: marshalling profile: %w", err)
	}
	sum := sha256.Sum256(profileBytes)
	profileSHA256 := hex.EncodeToString(sum[:])

	// Stage 1: resolve base OS → AMI + capabilities.
	base, err := r.stage1Base(ctx, profile)
	if err != nil {
		return nil, err
	}

	// Stage 2: expand formation refs → resolved layers; collect remaining regular refs.
	formationLayers, remaining, err := r.stage2ExpandFormations(ctx, profile.Software, arch)
	if err != nil {
		return nil, err
	}

	// Stage 3: resolve regular software refs → resolved layers.
	regularLayers, err := r.stage3ResolveSoftware(ctx, remaining, arch, base.Capabilities.ABI)
	if err != nil {
		return nil, err
	}

	allLayers := append(formationLayers, regularLayers...)

	// Stage 4: validate dependency graph — all requirements must be satisfied.
	if err := r.stage4ValidateGraph(base.Capabilities, allLayers); err != nil {
		return nil, err
	}

	// Stage 5: conflict detection — capability and file level.
	if err := r.stage5DetectConflicts(allLayers); err != nil {
		return nil, err
	}

	// Stage 6: topological sort → MountOrder assignment.
	ordered, err := r.stage6TopoSort(allLayers)
	if err != nil {
		return nil, err
	}

	// Stage 7: Sigstore bundle presence + Rekor entry verification.
	if err := r.stage7VerifyBundles(ctx, ordered); err != nil {
		return nil, err
	}

	// Stage 8: assemble the final LockFile.
	return r.stage8Assemble(profile, profileSHA256, base, ordered), nil
}

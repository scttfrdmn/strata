// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

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
	"io"

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

	// Warnings is an optional writer for non-fatal diagnostic messages.
	// If nil, warnings are silently discarded.
	Warnings io.Writer

	// AllowUnsignedOffline permits stage 7 to accept a layer with no Sigstore
	// bundle or Rekor entry, warning instead of refusing. It exists for one
	// caller: a resolve against the embedded, unsigned offline catalog (no
	// registry configured), where refusing every layer means the shipped
	// formations cannot be resolved even for local, non-trust use (#108). It is
	// set only on that path; a resolve against a real registry leaves it false and
	// stage 7 refuses an unsigned layer as before. An offline resolve is not a
	// trust decision and the lockfile it produces is not signed.
	AllowUnsignedOffline bool
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

// warn writes a warning message to cfg.Warnings if it is set.
func (r *Resolver) warn(format string, args ...any) {
	if r.cfg.Warnings == nil {
		return
	}
	fmt.Fprintf(r.cfg.Warnings, "warning: "+format+"\n", args...) //nolint:errcheck
}

// resolvedLayer is the internal accumulator for a single resolved layer.
// It is unexported and used only within the resolver pipeline.
type resolvedLayer struct {
	manifest *spec.LayerManifest
	// satisfiedBy and fromFormation are sets: a layer requested through more than
	// one formation is deduped to a single instance (dedupLayers, #208), and its
	// provenance is the sorted union of every requester and formation, so the
	// surviving layer records all of them independently of software: order.
	satisfiedBy   []string // SoftwareRef.String()s that requested this layer
	fromFormation []string // formation "name@version"s it was expanded from; empty for standalone
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

	// Collapse layers that resolve to the same content (e.g. a layer requested
	// through two formations) into one, merging their provenance. Without this the
	// same squashfs is mounted twice and its satisfied_by/from_formation follow
	// software: order (#208). Done before stages 4–8 so conflict detection, the
	// mount order, and the identity all see the deduped set.
	allLayers = dedupLayers(allLayers)

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

// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package resolver

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scttfrdmn/strata/internal/build"
	"github.com/scttfrdmn/strata/internal/probe"
	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/spec"
)

// stage1Base resolves the profile's base OS alias to an AMI ID and its
// probed capabilities.
func (r *Resolver) stage1Base(ctx context.Context, profile *spec.Profile) (*probe.AMIResult, error) {
	arch := profile.Base.NormalizedArch()
	result, err := r.cfg.Probe.Resolve(ctx, profile.Base.OS, arch)
	if err != nil {
		return nil, &ResolutionError{
			Stage:   "stage1",
			Code:    "BASE_RESOLUTION_FAILED",
			Message: fmt.Sprintf("failed to resolve base OS %q/%s: %v", profile.Base.OS, arch, err),
		}
	}
	return result, nil
}

// stage2ExpandFormations separates formation refs from regular software refs.
// For each formation it fetches the Formation manifest, checks that it is
// signed, and resolves each of its layer refs. It returns the resolved
// formation layers plus the remaining non-formation refs.
func (r *Resolver) stage2ExpandFormations(
	ctx context.Context,
	refs []spec.SoftwareRef,
	arch string,
) ([]resolvedLayer, []spec.SoftwareRef, error) {
	var (
		layers    []resolvedLayer
		remaining []spec.SoftwareRef
	)

	for _, ref := range refs {
		if !ref.IsFormation() {
			remaining = append(remaining, ref)
			continue
		}

		formation, err := r.cfg.Registry.ResolveFormation(ctx, ref.Formation, arch)
		if err != nil {
			if registry.IsNotFound(err) {
				return nil, nil, errFormationNotFound(ref.Formation)
			}
			return nil, nil, &ResolutionError{
				Stage:   "stage2",
				Code:    "FORMATION_FETCH_FAILED",
				Message: fmt.Sprintf("fetching formation %q: %v", ref.Formation, err),
			}
		}

		// An unsigned formation is refused, unless this is the offline-catalog
		// path, where it is accepted (the layer-level check and stage 7's warning
		// cover it) so the shipped formations resolve for local use (#108).
		if !r.cfg.AllowUnsignedOffline {
			if formation.Bundle == "" {
				return nil, nil, errBundleMissing("formation:" + ref.Formation)
			}
			if formation.RekorEntry == "" {
				return nil, nil, errRekorEntryMissing("formation:" + ref.Formation)
			}
		}

		const pendingPlaceholder = "pending-initial-build"
		if formation.RekorEntry == pendingPlaceholder || formation.Bundle == pendingPlaceholder {
			r.warn("formation %q has no Rekor attestation (rekor_entry: %q) — environment reproducibility cannot be cryptographically verified",
				ref.Formation, formation.RekorEntry)
		}

		for _, layerRef := range formation.Layers {
			if layerRef.IsFormation() {
				return nil, nil, &ResolutionError{
					Stage: "stage2",
					Code:  "NESTED_FORMATION",
					Message: fmt.Sprintf(
						"formation %q contains nested formation ref %q — not supported",
						ref.Formation, layerRef.Formation),
				}
			}

			manifest, err := r.cfg.Registry.ResolveLayer(ctx, layerRef.Name, layerRef.Version, arch, "")
			if err != nil {
				if registry.IsNotFound(err) {
					return nil, nil, errLayerNotFound(layerRef.Name, layerRef.Version, nil)
				}
				return nil, nil, &ResolutionError{
					Stage: "stage2",
					Code:  "FORMATION_LAYER_FETCH_FAILED",
					Message: fmt.Sprintf(
						"resolving layer %s in formation %q: %v",
						layerRef.String(), ref.Formation, err),
				}
			}

			layers = append(layers, resolvedLayer{
				manifest:      manifest,
				satisfiedBy:   []string{ref.String()},
				fromFormation: []string{ref.Formation},
			})
		}
	}

	return layers, remaining, nil
}

// dedupLayers collapses layers that resolve to the same content — the same
// manifest ID — into a single instance, merging their provenance. Two formations
// that both include a layer would otherwise each contribute an instance, so the
// same squashfs is mounted twice and its satisfied_by/from_formation follow the
// order the formations appear in `software:` (#208). A layer from one registry
// resolves to one manifest for a given (name, version, arch, abi), so equal IDs
// are equal content; the surviving instance's satisfiedBy/fromFormation become
// the sorted union of every requester, independent of input order.
func dedupLayers(layers []resolvedLayer) []resolvedLayer {
	byID := make(map[string]int, len(layers))
	out := make([]resolvedLayer, 0, len(layers))
	for _, rl := range layers {
		if idx, seen := byID[rl.manifest.ID]; seen {
			out[idx].satisfiedBy = mergeSorted(out[idx].satisfiedBy, rl.satisfiedBy)
			out[idx].fromFormation = mergeSorted(out[idx].fromFormation, rl.fromFormation)
			continue
		}
		byID[rl.manifest.ID] = len(out)
		out = append(out, rl)
	}
	return out
}

// mergeSorted returns the sorted, de-duplicated union of two string slices with
// empty strings dropped — the deterministic merge of two layers' provenance.
func mergeSorted(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	for _, s := range a {
		if s != "" {
			seen[s] = struct{}{}
		}
	}
	for _, s := range b {
		if s != "" {
			seen[s] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// sharesFormation reports whether two layers came from at least one common
// formation — the "pre-validated as a unit" exemption from conflict checks. A
// deduped layer can belong to several formations, so this is set intersection
// rather than the string equality it replaced.
func sharesFormation(a, b resolvedLayer) bool {
	for _, fa := range a.fromFormation {
		for _, fb := range b.fromFormation {
			if fa != "" && fa == fb {
				return true
			}
		}
	}
	return false
}

// stage3ResolveSoftware resolves each regular SoftwareRef to a LayerManifest.
// On failure it fetches available versions to produce an actionable error.
func (r *Resolver) stage3ResolveSoftware(
	ctx context.Context,
	refs []spec.SoftwareRef,
	arch, abi string,
) ([]resolvedLayer, error) {
	layers := make([]resolvedLayer, 0, len(refs))

	for _, ref := range refs {
		manifest, err := r.cfg.Registry.ResolveLayer(ctx, ref.Name, ref.Version, arch, abi)
		if err != nil {
			if registry.IsNotFound(err) {
				available, listErr := r.cfg.Registry.ListLayers(ctx, ref.Name, arch, abi)
				var versions []string
				if listErr == nil {
					for _, m := range available {
						versions = append(versions, m.Version)
					}
				}
				return nil, errLayerNotFound(ref.Name, ref.Version, versions)
			}
			return nil, &ResolutionError{
				Stage:   "stage3",
				Code:    "LAYER_FETCH_FAILED",
				Message: fmt.Sprintf("fetching layer %s: %v", ref.String(), err),
			}
		}

		layers = append(layers, resolvedLayer{
			manifest:    manifest,
			satisfiedBy: []string{ref.String()},
		})
	}

	return layers, nil
}

// stage4ValidateGraph verifies that every layer's requirements are satisfied
// by the base capabilities or by another resolved layer.
func (r *Resolver) stage4ValidateGraph(base *spec.BaseCapabilities, layers []resolvedLayer) error {
	// Build a merged view of all capabilities provided by all layers so we
	// can check cross-layer satisfaction with the existing SatisfiesRequirement logic.
	allProvides := make([]spec.Capability, 0, len(layers)*4)
	for _, rl := range layers {
		allProvides = append(allProvides, rl.manifest.Provides...)
	}
	layersCaps := spec.BaseCapabilities{Provides: allProvides}

	for _, rl := range layers {
		for _, req := range rl.manifest.Requires {
			if base.SatisfiesRequirement(req) {
				continue
			}
			if layersCaps.SatisfiesRequirement(req) {
				continue
			}
			return errUnsatisfiedRequirement(rl.manifest.Name, req)
		}
	}

	return nil
}

// canCoexist reports whether two layers can both be present in the same
// OverlayFS without conflicting at the filesystem level. Layers using the
// versioned install layout install to non-overlapping paths
// (<name>/<version>/), so different versions of the same software can
// coexist physically. Lmod's conflict() directive in the generated
// modulefile prevents simultaneous activation in user sessions.
//
// Returns false if either layer uses the flat layout, which installs directly
// to / and would produce real filesystem conflicts.
func canCoexist(a, b *spec.LayerManifest) bool {
	aVersioned := a.InstallLayout == "" || a.InstallLayout == "versioned"
	bVersioned := b.InstallLayout == "" || b.InstallLayout == "versioned"
	return aVersioned && bVersioned
}

// stage5DetectConflicts checks for capability-level and file-level conflicts
// between layers. Layers within the same formation are exempt from
// intra-formation conflict checks (they were pre-validated as a unit).
// Layers that canCoexist (both versioned layout) are also exempt from
// capability-level conflicts — Lmod handles mutual exclusion at activation.
func (r *Resolver) stage5DetectConflicts(layers []resolvedLayer) error {
	// Capability-level: two layers both provide the same capability name.
	// capProviders maps capability name → list of layer indices providing it.
	capProviders := make(map[string][]int)

	for i, rl := range layers {
		for _, cap := range rl.manifest.Provides {
			if prevs, exists := capProviders[cap.Name]; exists {
				for _, prev := range prevs {
					prevLayer := layers[prev]
					// Layers sharing a formation are pre-validated as a unit — exempt.
					if sharesFormation(rl, prevLayer) {
						continue
					}
					// Versioned layout: paths don't overlap; Lmod prevents
					// simultaneous activation. Allow coexistence.
					if canCoexist(prevLayer.manifest, rl.manifest) {
						continue
					}
					return errCapabilityConflict(prevLayer.manifest.ID, rl.manifest.ID, cap)
				}
			}
			capProviders[cap.Name] = append(capProviders[cap.Name], i)
		}
	}

	// File-level: two layers with overlapping ContentManifest paths and
	// different SHA256 values.
	for i := range layers {
		if len(layers[i].manifest.ContentManifest) == 0 {
			continue
		}
		mI := &build.ContentManifest{
			LayerID: layers[i].manifest.ID,
			Files:   layers[i].manifest.ContentManifest,
		}
		for j := i + 1; j < len(layers); j++ {
			if len(layers[j].manifest.ContentManifest) == 0 {
				continue
			}
			// Exempt pairs sharing a formation.
			if sharesFormation(layers[i], layers[j]) {
				continue
			}
			mJ := &build.ContentManifest{
				LayerID: layers[j].manifest.ID,
				Files:   layers[j].manifest.ContentManifest,
			}
			if conflicts := mI.ConflictsWith(mJ); len(conflicts) > 0 {
				return errConflict(layers[i].manifest.ID, layers[j].manifest.ID, conflicts[0])
			}
		}
	}

	return nil
}

// selectProvider returns the index of the layer, among cands, whose capability
// best satisfies req. The bool reports whether any candidate satisfies it at all
// — false means no dependency edge is drawn, which is correct when the
// requirement is met by the base image instead.
//
// Selection is by highest satisfying version, ties broken by lowest layer ID.
// Both parts are deliberate, and the alternative is worth naming because it
// looks simpler: picking the FIRST satisfying candidate would also close #67 as
// stated, since the stated property is only that the consumer sorts after a
// provider that satisfies it. It was rejected because cands is in layer slice
// order, which comes from the order the profile author listed the layers. That
// would leave the dependency edge — and therefore mount order, and therefore the
// lockfile — a function of an author-visible ordering rather than of content,
// which is the same order-dependence class as the defect being fixed. Highest
// version is a function of the capabilities alone, and the ID tie-break makes it
// total, so two profiles naming the same layers in different orders resolve
// identically.
//
// Highest is also the defensible semantic on its own: given two providers that
// both satisfy a constraint, the newer is the one a user asking for ">= X"
// expects to be wired to.
func selectProvider(layers []resolvedLayer, cands []int, req spec.Requirement) (int, bool) {
	best, bestVersion := -1, ""
	for _, idx := range cands {
		for _, cap := range layers[idx].manifest.Provides {
			if !cap.Satisfies(req) {
				continue
			}
			if best >= 0 {
				cmp := spec.CompareVersions(cap.Version, bestVersion)
				if cmp < 0 {
					continue
				}
				if cmp == 0 && layers[idx].manifest.ID >= layers[best].manifest.ID {
					continue
				}
			}
			best, bestVersion = idx, cap.Version
		}
	}
	if best < 0 {
		return 0, false
	}
	return best, true
}

// lessByLayerContent is a total order on layers computed from their content —
// name, then version, then layer ID (which carries the content digest) — and
// never from their position in the resolver's input. It is the tie-break for
// mutually-unordered layers in the topological sort, so the mount order the
// identity depends on follows what the layers are rather than the order they
// were written down (R2, #95).
func lessByLayerContent(a, b resolvedLayer) bool {
	if a.manifest.Name != b.manifest.Name {
		return a.manifest.Name < b.manifest.Name
	}
	if cmp := spec.CompareVersions(a.manifest.Version, b.manifest.Version); cmp != 0 {
		return cmp < 0
	}
	return a.manifest.ID < b.manifest.ID
}

// stage6TopoSort performs a topological sort of layers based on their
// capability dependency edges using Kahn's algorithm. The returned slice
// is in dependency order (dependencies before dependents). MountOrder
// is assigned by stage8 based on position in this slice.
func (r *Resolver) stage6TopoSort(layers []resolvedLayer) ([]resolvedLayer, error) {
	n := len(layers)
	if n == 0 {
		return layers, nil
	}

	// Map capability name → every layer index providing it, for resolving which
	// layer satisfies a requirement (used to build dependency edges).
	//
	// Every provider is kept. The previous form was a map[string]int assigned
	// inside this loop, so the LAST provider of a name won the edge with no
	// version check at all — #67's stage-6 half. Stage 4 would validate the
	// requirement against a satisfying provider and stage 6 would wire the
	// dependency to a different, non-satisfying one, which puts the consumer
	// ahead of the layer it needs in mount order.
	capProviders := make(map[string][]int, n*4)
	for i, rl := range layers {
		for _, cap := range rl.manifest.Provides {
			capProviders[cap.Name] = append(capProviders[cap.Name], i)
		}
	}

	// inDegree[i] = number of layers that layer i depends on.
	// dependedBy[j] = list of layer indices that have layer j as a dependency.
	inDegree := make([]int, n)
	dependedBy := make([][]int, n)

	for i, rl := range layers {
		seen := make(map[int]bool)
		for _, req := range rl.manifest.Requires {
			j, ok := selectProvider(layers, capProviders[req.Name], req)
			if !ok || j == i || seen[j] {
				continue // satisfied by base, self-dep, or already counted
			}
			seen[j] = true
			dependedBy[j] = append(dependedBy[j], i)
			inDegree[i]++
		}
	}

	// Kahn's algorithm: start with all zero-in-degree nodes.
	queue := make([]int, 0, n)
	for i := range n {
		if inDegree[i] == 0 {
			queue = append(queue, i)
		}
	}

	ordered := make([]resolvedLayer, 0, n)
	for len(queue) > 0 {
		// Tie-break mutually-unordered layers by content, never by input index.
		// Ordering the ready set by (name, version, ID) makes the topological
		// order — and therefore the MountOrder stage 8 assigns from it — a
		// function of the layers' content, so permuting a profile's `software:`
		// list cannot change it (R2). The previous `sort.Ints(queue)` ordered by
		// layer index, which is exactly `software:` declaration order (#95).
		sort.Slice(queue, func(a, b int) bool {
			return lessByLayerContent(layers[queue[a]], layers[queue[b]])
		})
		idx := queue[0]
		queue = queue[1:]
		ordered = append(ordered, layers[idx])

		for _, dep := range dependedBy[idx] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(ordered) != n {
		return nil, &ResolutionError{
			Stage:   "stage6",
			Code:    "DEPENDENCY_CYCLE",
			Message: "circular dependency detected in layer requirements",
		}
	}

	return ordered, nil
}

// stage7VerifyBundles verifies Sigstore bundle presence and (when a Rekor
// client is configured) log entry validity for all layers in parallel.
// Any failure causes the entire resolution to fail.
func (r *Resolver) stage7VerifyBundles(ctx context.Context, layers []resolvedLayer) error {
	if len(layers) == 0 {
		return nil
	}

	if r.cfg.AllowUnsignedOffline && anyUnsigned(layers) {
		r.warn("resolving against an unsigned offline catalog — Sigstore bundle and " +
			"Rekor verification are skipped; this resolve is not a trust decision and the " +
			"lockfile it produces is not signed (set a registry to require signatures)")
	}

	errs := make(chan error, len(layers))

	var wg sync.WaitGroup
	for _, rl := range layers {
		rl := rl
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- r.verifyBundle(ctx, rl)
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// verificationPolicy names what stage 7 checked about layer attestations, for
// the record LockFile.VerificationPolicy carries (#100). Resolution never
// verifies against the transparency log — it holds bundle URIs, not bytes (#85)
// — so the strongest honest claim is that every layer's attestation was present
// and well-formed, unless the offline-catalog path accepted an unsigned layer.
func (r *Resolver) verificationPolicy(layers []resolvedLayer) string {
	if r.cfg.AllowUnsignedOffline && anyUnsigned(layers) {
		return spec.VerifyUnsignedOffline
	}
	return spec.VerifyAttestationReferencesPresent
}

// anyUnsigned reports whether any layer lacks a bundle or Rekor entry, so the
// offline-catalog warning fires only when there is actually something unsigned.
func anyUnsigned(layers []resolvedLayer) bool {
	for _, rl := range layers {
		if rl.manifest.Bundle == "" || rl.manifest.RekorEntry == "" {
			return true
		}
	}
	return false
}

// verifyBundle checks a single layer's bundle and Rekor entry fields, and
// optionally verifies the log entry against the Rekor API.
func (r *Resolver) verifyBundle(ctx context.Context, rl resolvedLayer) error {
	if rl.manifest.Bundle == "" || rl.manifest.RekorEntry == "" {
		// An unsigned layer: refuse it, unless this is the offline-catalog path
		// (warned once in stage7VerifyBundles), where it is accepted so the
		// shipped formations can resolve for local use (#108). A signed layer is
		// still fully verified below.
		if r.cfg.AllowUnsignedOffline {
			return nil
		}
		if rl.manifest.Bundle == "" {
			return errBundleMissing(rl.manifest.ID)
		}
		return errRekorEntryMissing(rl.manifest.ID)
	}

	if r.cfg.Rekor != nil {
		logIndex, err := strconv.ParseInt(rl.manifest.RekorEntry, 10, 64)
		if err != nil {
			return &ResolutionError{
				Stage: "stage7",
				Code:  "INVALID_REKOR_ENTRY",
				Message: fmt.Sprintf(
					"layer %q has non-numeric Rekor entry %q: %v",
					rl.manifest.ID, rl.manifest.RekorEntry, err),
			}
		}
		// Stage 7 holds a bundle *URI* (manifest.Bundle), never bundle bytes:
		// resolution runs before anything is downloaded, and fetching is the
		// caller's job everywhere else in this codebase (trust.VerifyLayer
		// refuses URIs outright). So no bundle can be passed here, and since
		// #59 a RekorClient that is given none returns trust.ErrNoBundle —
		// configuring a real client makes resolution fail closed rather than
		// pass on the strength of "something was logged at that index".
		//
		// The #85 decision, made: resolve-time stage 7 stays a presence-and-
		// structure check and does NOT fetch. Cryptographic Rekor verification
		// happens at the boundaries where the bundle bytes exist — `strata verify
		// --rekor` and `strata run`'s pre-mount check, both of which load the
		// bundle (loadLocalBundle) before calling VerifyEntry. This branch is kept
		// as a fail-closed guard: a resolver given a real Rekor client (no shipped
		// path sets one) rejects every layer rather than passing, because all it
		// can offer VerifyEntry is a nil bundle. TestStage7_RekorVerification
		// records that nil argument, so if a fetch is ever added here it breaks a
		// test rather than silently changing what "verified" means (#86).
		if err := r.cfg.Rekor.VerifyEntry(ctx, logIndex, nil); err != nil {
			return &ResolutionError{
				Stage:   "stage7",
				Code:    "REKOR_VERIFICATION_FAILED",
				Message: fmt.Sprintf("Rekor verification failed for layer %q: %v", rl.manifest.ID, err),
			}
		}
	}

	return nil
}

// stage8Assemble constructs the final LockFile from all resolved components.
// MountOrder is assigned 1..N based on position in the ordered slice.
func (r *Resolver) stage8Assemble(
	profile *spec.Profile,
	profileSHA256 string,
	base *probe.AMIResult,
	layers []resolvedLayer,
) *spec.LockFile {
	resolvedLayers := make([]spec.ResolvedLayer, len(layers))
	for i, rl := range layers {
		resolvedLayers[i] = spec.ResolvedLayer{
			LayerManifest: *rl.manifest,
			MountOrder:    i + 1,
			SatisfiedBy:   strings.Join(rl.satisfiedBy, ", "),
			FromFormation: strings.Join(rl.fromFormation, ", "),
		}
	}

	for _, req := range profile.RequiresHost {
		r.warn("requires_host %q: %q — host capability not verified (probe integration pending)",
			req.Key, req.Value)
	}

	return &spec.LockFile{
		ProfileName:        profile.Name,
		ProfileSHA256:      profileSHA256,
		ResolvedAt:         time.Now(),
		StrataVersion:      r.cfg.StrataVersion,
		VerificationPolicy: r.verificationPolicy(layers),
		Base: spec.ResolvedBase{
			DeclaredOS: profile.Base.OS,
			AMIID:      base.AMIID,
			// The base's content digest is the hash of its capability record
			// (#64). Without this the literal left AMISHA256 empty, so IsFrozen()
			// could never be true and strata freeze structurally could not succeed.
			AMISHA256:    base.Capabilities.ContentDigest(),
			Capabilities: *base.Capabilities,
		},
		Layers:       resolvedLayers,
		Env:          profile.Env,
		OnReady:      profile.OnReady,
		Defaults:     profile.Defaults,
		RequiresHost: profile.RequiresHost,
	}
}

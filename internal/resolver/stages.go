package resolver

import (
	"context"
	"fmt"
	"sort"
	"strconv"
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

		if formation.Bundle == "" {
			return nil, nil, errBundleMissing("formation:" + ref.Formation)
		}
		if formation.RekorEntry == "" {
			return nil, nil, errRekorEntryMissing("formation:" + ref.Formation)
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
				satisfiedBy:   ref.String(),
				fromFormation: ref.Formation,
			})
		}
	}

	return layers, remaining, nil
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
			satisfiedBy: ref.String(),
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
					// Layers within the same formation are pre-validated — exempt.
					if rl.fromFormation != "" && rl.fromFormation == prevLayer.fromFormation {
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
			// Exempt same-formation pairs.
			if layers[i].fromFormation != "" && layers[i].fromFormation == layers[j].fromFormation {
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

// stage6TopoSort performs a topological sort of layers based on their
// capability dependency edges using Kahn's algorithm. The returned slice
// is in dependency order (dependencies before dependents). MountOrder
// is assigned by stage8 based on position in this slice.
func (r *Resolver) stage6TopoSort(layers []resolvedLayer) ([]resolvedLayer, error) {
	n := len(layers)
	if n == 0 {
		return layers, nil
	}

	// Map capability name → layer index for resolving which layer satisfies
	// a requirement (used to build dependency edges).
	capProviderIdx := make(map[string]int, n*4)
	for i, rl := range layers {
		for _, cap := range rl.manifest.Provides {
			capProviderIdx[cap.Name] = i
		}
	}

	// inDegree[i] = number of layers that layer i depends on.
	// dependedBy[j] = list of layer indices that have layer j as a dependency.
	inDegree := make([]int, n)
	dependedBy := make([][]int, n)

	for i, rl := range layers {
		seen := make(map[int]bool)
		for _, req := range rl.manifest.Requires {
			j, ok := capProviderIdx[req.Name]
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
		sort.Ints(queue) // deterministic tie-breaking
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

// verifyBundle checks a single layer's bundle and Rekor entry fields, and
// optionally verifies the log entry against the Rekor API.
func (r *Resolver) verifyBundle(ctx context.Context, rl resolvedLayer) error {
	if rl.manifest.Bundle == "" {
		return errBundleMissing(rl.manifest.ID)
	}
	if rl.manifest.RekorEntry == "" {
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
			SatisfiedBy:   rl.satisfiedBy,
			FromFormation: rl.fromFormation,
		}
	}

	return &spec.LockFile{
		ProfileName:   profile.Name,
		ProfileSHA256: profileSHA256,
		ResolvedAt:    time.Now(),
		StrataVersion: r.cfg.StrataVersion,
		Base: spec.ResolvedBase{
			DeclaredOS:   profile.Base.OS,
			AMIID:        base.AMIID,
			Capabilities: *base.Capabilities,
		},
		Layers:   resolvedLayers,
		Env:      profile.Env,
		OnReady:  profile.OnReady,
		Defaults: profile.Defaults,
	}
}

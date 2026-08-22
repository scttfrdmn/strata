package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// envHashInput is the canonical content struct hashed to produce EnvironmentID.
//
// Membership is not a field-by-field judgement. A LockFile field appears here
// unless it takes one of four routes out, each of which is a checkable
// obligation rather than an assertion:
//
//   - inert — no consumer outside spec reads it (Base.DeclaredOS,
//     ProfileSHA256, MutableLayer, RequiresHost, SatisfiedBy, FromFormation).
//   - digest-mediated — its influence is bounded by a check against a field
//     that is hashed (Layers[].Source is fetched and then verified against
//     Layers[].SHA256: cmd/strata/run.go:436, :481; internal/agent/agent.go:244).
//   - abort-only — it can cause assembly to fail but never to differ
//     (Layers[].Bundle, Layers[].RekorEntry: internal/agent/agent.go:367
//     verifies the fetched file and refuses on mismatch).
//   - declared provenance — exported under a name that EnvironmentID's claim
//     excludes by name, with no in-tree reader (LockFile.RekorEntry, exported
//     as STRATA_REKOR_ENTRY; the absence of a reader is enforced by
//     TestDeclaredProvenance_NoInTreeReader, not asserted here).
//
// Everything else is content and is hashed. Adding a LockFile field without
// recording which route it takes is the defect this list exists to prevent.
// The reasoning is docs/environment-identity.md.
type envHashInput struct {
	BaseAMISHA256 string                `json:"base_ami_sha256"`
	ProfileName   string                `json:"profile_name"`
	Layers        []layerHashInput      `json:"layers,omitempty"`
	Env           map[string]string     `json:"env,omitempty"`
	Defaults      []moduleHashInput     `json:"defaults,omitempty"`
	OnReady       []string              `json:"on_ready,omitempty"`
	Packages      []packageSetHashInput `json:"packages,omitempty"`
}

// layerHashInput is one layer's contribution to the identity.
//
// MountOrder is deliberately absent. The layers slice is sorted by it, so the
// ordering it expresses is captured while its literal value is not: two
// lockfiles whose mount orders are 1,2 and 10,20 mount the same layers in the
// same sequence and assemble the same environment, so they must not differ in
// identity. Hashing the number would manufacture a distinction.
type layerHashInput struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	InstallLayout string `json:"install_layout"`
	SHA256        string `json:"sha256"`
}

// moduleHashInput is one entry of LockFile.Defaults. Only Name and Version are
// hashed because they are the only fields any consumer reads: overlay writes
// "module load <name>/<version>" lines from them
// (internal/overlay/overlay.go:171-176). Order is preserved — module load
// sequencing is meaningful.
type moduleHashInput struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// packageSetHashInput and packageEntryHashInput mirror ResolvedPackageSet and
// ResolvedPackageEntry with one difference: the inner Packages slice carries
// omitempty, so a set with no entries hashes identically whether the slice is
// nil or empty. A set with no entries installs nothing either way (#117).
//
// Order is preserved, in both dimensions, because it is consumed in order:
// internal/agent/package_installer.go:37 iterates the sets and :99, :113 run
// one pip/conda command per entry, so a later entry can change what an earlier
// one installed. Sorting either dimension would give two different
// environments one identity.
type packageSetHashInput struct {
	Manager  PackageManager          `json:"manager"`
	Env      string                  `json:"env,omitempty"`
	Packages []packageEntryHashInput `json:"packages,omitempty"`
}

type packageEntryHashInput struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	SHA256  string `json:"sha256,omitempty"`
}

// computeEnvironmentID returns a hex SHA256 of the lockfile's canonical
// content, or an empty string if the lockfile is not frozen (missing SHA256s).
//
// Layers are sorted by MountOrder with a stable sort, which is the same order
// the mounter uses (internal/overlay/mount_linux.go:134) — including for tied
// MountOrders, where both fall back to the order the layers appear in the
// lockfile. That fallback is why the sort must be stable: an unstable sort
// leaves the identity of a lockfile with tied mount orders dependent on an
// implementation detail of sort.Slice. Ties are underdetermined in a further
// sense that this function cannot fix — see #95.
func computeEnvironmentID(l *LockFile) string {
	if !l.IsFrozen() {
		return ""
	}

	// Sort a copy of layers by MountOrder so the identity follows the mount
	// sequence rather than the order the layers were written down.
	layers := make([]ResolvedLayer, len(l.Layers))
	copy(layers, l.Layers)
	sort.SliceStable(layers, func(i, j int) bool {
		return layers[i].MountOrder < layers[j].MountOrder
	})

	hashLayers := make([]layerHashInput, len(layers))
	for i, layer := range layers {
		hashLayers[i] = layerHashInput{
			ID:            layer.ID,
			Name:          layer.Name,
			Version:       layer.Version,
			InstallLayout: canonicalInstallLayout(layer.InstallLayout),
			SHA256:        layer.SHA256,
		}
	}

	defaults := make([]moduleHashInput, len(l.Defaults))
	for i, ref := range l.Defaults {
		defaults[i] = moduleHashInput{Name: ref.Name, Version: ref.Version}
	}

	packages := make([]packageSetHashInput, len(l.Packages))
	for i, ps := range l.Packages {
		entries := make([]packageEntryHashInput, len(ps.Packages))
		for j, e := range ps.Packages {
			// A conversion rather than a field-by-field literal, deliberately: it
			// is legal only while the two structs have identical fields, so a new
			// field on ResolvedPackageEntry breaks the build here instead of being
			// silently dropped from the identity. If such a field is genuinely
			// outside the identity, this becomes a literal again and the field gets
			// a row in packageEntryRoutes with its route.
			entries[j] = packageEntryHashInput(e)
		}
		packages[i] = packageSetHashInput{Manager: ps.Manager, Env: ps.Env, Packages: entries}
	}

	input := envHashInput{
		BaseAMISHA256: l.Base.AMISHA256,
		ProfileName:   l.ProfileName,
		Layers:        hashLayers,
		Env:           l.Env,
		Defaults:      defaults,
		OnReady:       l.OnReady,
		Packages:      packages,
	}

	// json.Marshal on a struct with string/map/slice fields is deterministic
	// when map keys are sorted, which encoding/json does by default.
	data, err := json.Marshal(input)
	if err != nil {
		// envHashInput contains only basic types; Marshal cannot fail here.
		panic("spec: computeEnvironmentID: unexpected marshal error: " + err.Error())
	}

	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// canonicalInstallLayout maps the empty layout to its documented meaning.
// spec/layer.go declares "versioned" as the default and the empty string means
// exactly that, so two lockfiles differing only in whether the key was written
// out assemble the same environment and must share an identity.
//
// The mapping stops there deliberately. Every consumer compares this field for
// equality against "flat" (internal/overlay/overlay.go:109,116;
// cmd/strata/run.go:336,342; internal/fold/eject.go:220,225;
// internal/export/oci.go:368,375), so today any non-"flat" spelling behaves
// like "versioned" and could be folded in too. That equivalence rests on an
// absence — no consumer testing for "versioned" positively — which nothing
// checks, whereas the empty case rests on the spec.
func canonicalInstallLayout(layout string) string {
	if layout == "" {
		return "versioned"
	}
	return layout
}

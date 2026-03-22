package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// envHashInput is the canonical content struct hashed to produce EnvironmentID.
// Only fields that define environment content are included. Attestation fields
// (RekorEntry, Bundle), timing fields (ResolvedAt), and identity fields
// (ProfileName, StrataVersion) are excluded — they do not affect what runs.
// MutableLayer is also excluded — it is metadata about the build process,
// not the environment content itself (the upper is not content-addressed).
type envHashInput struct {
	BaseAMISHA256 string               `json:"base_ami_sha256"`
	LayerSHA256s  []string             `json:"layer_sha256s"`
	Env           map[string]string    `json:"env,omitempty"`
	OnReady       []string             `json:"on_ready,omitempty"`
	Packages      []ResolvedPackageSet `json:"packages,omitempty"`
}

// computeEnvironmentID returns a hex SHA256 of the lockfile's canonical
// content, or an empty string if the lockfile is not frozen (missing SHA256s).
//
// Layers are sorted by MountOrder before hashing to ensure determinism
// regardless of the order they appear in the lockfile YAML.
func computeEnvironmentID(l *LockFile) string {
	if !l.IsFrozen() {
		return ""
	}

	// Sort a copy of layers by MountOrder so hashing is order-independent.
	layers := make([]ResolvedLayer, len(l.Layers))
	copy(layers, l.Layers)
	sort.Slice(layers, func(i, j int) bool {
		return layers[i].MountOrder < layers[j].MountOrder
	})

	sha256s := make([]string, len(layers))
	for i, layer := range layers {
		sha256s[i] = layer.SHA256
	}

	input := envHashInput{
		BaseAMISHA256: l.Base.AMISHA256,
		LayerSHA256s:  sha256s,
		Env:           l.Env,
		OnReady:       l.OnReady,
		Packages:      l.Packages,
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

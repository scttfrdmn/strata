package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// baseCapsHashInput is the canonical content struct hashed to produce a base's
// AMISHA256. It mirrors BaseCapabilities' reproducible fields and deliberately
// omits ProbedAt: that field is a wall-clock timestamp, so two probes of the
// same AMI minutes apart would otherwise yield different base identities.
// Provides is sorted before hashing so the digest does not depend on the order
// the probe happened to emit capabilities in.
type baseCapsHashInput struct {
	AMIID          string       `json:"ami_id"`
	OS             string       `json:"os"`
	Arch           string       `json:"arch"`
	ABI            string       `json:"abi"`
	SystemCompiler string       `json:"system_compiler"`
	Provides       []Capability `json:"provides"`
}

// ContentDigest returns a hex SHA256 identifying the base by its declared
// capabilities. Stage 8 records it as ResolvedBase.AMISHA256 — the value
// IsFrozen requires and that participates in EnvironmentID.
//
// It is a digest of the capability *record* — AMI ID, OS, arch, ABI, system
// compiler, and the provided-capability set — not of the AMI's raw bytes: no
// shipped code boots or reads the base image, and docs/build-provenance-chain.md
// already treats a hash of this record as the base identity. ProbedAt is excluded
// so the digest is reproducible across probes; every other field participates, so
// a change to any of them is a different base.
func (b BaseCapabilities) ContentDigest() string {
	provides := make([]Capability, len(b.Provides))
	copy(provides, b.Provides)
	sort.Slice(provides, func(i, j int) bool {
		if provides[i].Name != provides[j].Name {
			return provides[i].Name < provides[j].Name
		}
		return provides[i].Version < provides[j].Version
	})

	data, err := json.Marshal(baseCapsHashInput{
		AMIID:          b.AMIID,
		OS:             b.OS,
		Arch:           b.Arch,
		ABI:            b.ABI,
		SystemCompiler: b.SystemCompiler,
		Provides:       provides,
	})
	if err != nil {
		// baseCapsHashInput holds only strings and a slice of them; Marshal
		// cannot fail. A panic here would be a programming error, not input.
		panic("spec: BaseCapabilities.ContentDigest: unexpected marshal error: " + err.Error())
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

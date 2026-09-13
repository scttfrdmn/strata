package spec_test

import (
	"testing"
	"time"

	"github.com/scttfrdmn/strata/spec"
)

func baseCapsFixture() spec.BaseCapabilities {
	return spec.BaseCapabilities{
		AMIID:          "ami-0abc",
		OS:             "al2023",
		Arch:           "x86_64",
		ABI:            "linux-gnu-2.34",
		SystemCompiler: "gcc-11.4.1-2.amzn2023.0.1.x86_64",
		ProbedAt:       time.Unix(1_700_000_000, 0),
		Provides: []spec.Capability{
			{Name: "glibc", Version: "2.34"},
			{Name: "kernel", Version: "6.1"},
		},
	}
}

// TestBaseCapabilitiesContentDigest pins the base content digest (#64): the value
// stage 8 records as AMISHA256, which IsFrozen() requires and which participates
// in EnvironmentID.
func TestBaseCapabilitiesContentDigest(t *testing.T) {
	d := baseCapsFixture().ContentDigest()

	if d == "" {
		t.Fatal("ContentDigest is empty — IsFrozen() could never be satisfied")
	}
	if again := baseCapsFixture().ContentDigest(); again != d {
		t.Errorf("ContentDigest not deterministic: %q vs %q", d, again)
	}

	// ProbedAt is the one deliberate omission: two probes of the same AMI minutes
	// apart must be the same base.
	later := baseCapsFixture()
	later.ProbedAt = time.Unix(1_700_009_999, 0)
	if later.ContentDigest() != d {
		t.Error("ContentDigest changed with ProbedAt — the timestamp must not participate")
	}

	// Provides order must not matter — the digest sorts before hashing.
	reordered := baseCapsFixture()
	reordered.Provides = []spec.Capability{
		{Name: "kernel", Version: "6.1"},
		{Name: "glibc", Version: "2.34"},
	}
	if reordered.ContentDigest() != d {
		t.Error("ContentDigest changed when Provides was reordered — it must sort")
	}

	// Non-vacuity anchor: every other field participates. An exclusion that
	// dropped real fields would pass the checks above; this catches it.
	mutations := map[string]func(*spec.BaseCapabilities){
		"AMIID":          func(b *spec.BaseCapabilities) { b.AMIID = "ami-9zzz" },
		"OS":             func(b *spec.BaseCapabilities) { b.OS = "rocky9" },
		"Arch":           func(b *spec.BaseCapabilities) { b.Arch = "aarch64" },
		"ABI":            func(b *spec.BaseCapabilities) { b.ABI = "linux-gnu-2.35" },
		"SystemCompiler": func(b *spec.BaseCapabilities) { b.SystemCompiler = "gcc-13.2.0" },
		"Provides": func(b *spec.BaseCapabilities) {
			b.Provides = append(b.Provides, spec.Capability{Name: "cuda", Version: "12.3"})
		},
	}
	for name, mutate := range mutations {
		m := baseCapsFixture()
		mutate(&m)
		if m.ContentDigest() == d {
			t.Errorf("ContentDigest unchanged after mutating %s — the field does not participate", name)
		}
	}
}

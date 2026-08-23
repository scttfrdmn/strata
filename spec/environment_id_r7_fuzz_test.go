package spec

import (
	"fmt"
	"testing"
	"time"
)

// This file is the generator for PROPERTIES.md proposition R7:
//
//	No spurious distinctions. Two lockfiles that assemble the same environment
//	have the same EnvironmentID.
//
// R7 quantifies over pairs of lockfiles, so a generator needs a supply of pairs
// that provably assemble the same environment. It cannot ask the implementation
// which fields matter — deferring the domain to the code under test is what made
// the retired R6 unfalsifiable. So "assembles the same environment" is defined
// here, by an enumerated set of transformations, each justified from the
// specification rather than from envHashInput's membership.
//
// # Operational floor
//
// go test ./spec/ runs the seed corpus only. That takes milliseconds and
// searches nothing: it is a regression check on inputs already known, and must
// not be read as evidence that R7 was explored. The search happens in the
// scheduled job (.github/workflows/fuzz.yml) which passes an explicit -fuzztime.
// Crashers are committed under spec/testdata/fuzz/ so the ordinary job replays
// them as seeds forever after.
//
// # Stated domain, and why each exclusion is named rather than avoided
//
// Three transformations change the identity while the reason they are said to
// preserve the assembled environment stands on record. They are excluded from the
// live set here because a target that fails on every input searches nothing:
//
//   - reordering package sets or their entries — #95
//   - mutating on_ready, which is hashed and executed nowhere — #69
//   - mutating a package entry's sha256, which the installer ignores — #98
//
// A fourth, a nil versus empty inner Packages slice (#117), was fixed by #147 and
// is no longer a counterexample. It is not in the live set either: adding it widens
// the property this target asserts and needs a seed that reaches an entry-free
// package set, so it travels on its own change (#150). Until then it is neither
// excluded nor live, and saying so is what this paragraph is for.
//
// #95's two transformations have controls in environment_id_r7_exclusions_test.go
// asserting the distinction is still there. #69's and #98's do not, and the absence
// is named rather than left to be found: #148 measured that a control reading
// EnvironmentID() cannot observe either fix — #69's lands in an executor, #98's in
// an installer's argv — so those controls were deleted and the observable each one
// needs is recorded on its issue.
//
// Whether #95's pair are R7 counterexamples at all is an open question recorded on
// the issue: TestEnvironmentID_PackageOrderIsContent holds that package order *is*
// content, and a transformation whose environment-preserving premise fails is not
// R7's business. Either way they stay out of the live set; what would change is
// which list they belong in, and what PROPERTIES.md's R7 row may claim.
//
// Excluded for the opposite reason — these change the environment, so R7's
// premise does not hold and they are not R7's business:
// Env, Defaults (#118), ProfileName and RekorEntry (#120), the layer manifest's
// Name, Version and InstallLayout (#122), MutableLayer, and the layer set itself.
//
// ProfileName and RekorEntry were in the live set when this file was written, on
// justifications copied from spec/lockfile_hash.go:11-13 — "they do not affect
// what runs". That comment is false: internal/overlay writes both into
// /etc/profile.d/strata.sh and /etc/strata/environment, and cmd/strata/run.go
// puts both in the child process environment. A transformation whose premise
// fails does not merely fail to test R7 — asserting that the identity is
// unchanged across it asserts that an X2 violation is *correct*. Taking a
// justification from the implementation is the R6 mistake, and it was made here
// one level down: not by asking envHashInput what mattered, but by believing a
// comment next to it.
//
// # The domain rests on a scope clause PROPERTIES.md does not yet state — #121
//
// internal/overlay/overlay.go:207-211 marshals the entire lockfile to
// /etc/strata/active.lock.yaml inside the assembled root. Read literally, every
// field alters the assembled environment, no two distinct lockfiles assemble the
// same one, R7's premise is unsatisfiable and this whole file searches an empty
// domain. The transformations below are justified against the *intended* scope —
// mounted content plus exported process environment, excluding the provenance
// record — which is #121's subject and is currently an assumption of this file
// rather than a statement anywhere. Until #121 lands, read every result here as
// conditional on it.

// r7Stream turns fuzz bytes into generator decisions. Every method is total: a
// stream that has run out keeps yielding zero, so no input is rejected and no
// iteration is skipped. Discarding inputs that miss a precondition would let the
// target report success on work it never did.
type r7Stream struct {
	b []byte
	i int
}

func (s *r7Stream) next() byte {
	if s.i >= len(s.b) {
		return 0
	}
	c := s.b[s.i]
	s.i++
	return c
}

// intn returns a value in [0,n). n must be positive.
func (s *r7Stream) intn(n int) int {
	if n <= 0 {
		panic("r7Stream.intn: n must be positive")
	}
	return int(s.next()) % n
}

func (s *r7Stream) boolean() bool { return s.next()%2 == 1 }

// str returns a short string drawn from the stream. It can return "", which is
// deliberate: empty strings are where nil-versus-empty defects live.
func (s *r7Stream) str() string {
	n := s.intn(4)
	out := make([]byte, n)
	for i := range out {
		// Printable ASCII, so a crasher committed to testdata stays readable.
		out[i] = byte(' ' + s.intn(95))
	}
	return string(out)
}

// digest returns a non-empty hex-ish digest. Non-empty matters: IsFrozen()
// returns false when any layer digest is blank, and computeEnvironmentID then
// returns "" for both sides of a pair, which would make the comparison hold
// without exercising anything.
func (s *r7Stream) digest() string {
	return fmt.Sprintf("%02x%02x%02x%02x", s.next(), s.next(), s.next(), s.next())
}

// buildLockFile draws a frozen lockfile from the stream.
//
// MountOrder is assigned distinctly by index rather than drawn, so that
// permuting the layer slice is always an environment-preserving transformation.
// Under tied MountOrder values, which layer shadows which is undefined — so a
// permutation may assemble a *different* environment, R7's premise fails, and
// the pair says nothing about R7. That case is a determinism defect belonging to
// R2 (#95), and canonicalising here keeps the two apart instead of letting ties
// silently exit the iteration.
func buildLockFile(s *r7Stream) *LockFile {
	l := &LockFile{
		ProfileName:   s.str(),
		ProfileSHA256: s.digest(),
		ResolvedAt:    time.Unix(int64(s.next())*3600, 0).UTC(),
		StrataVersion: s.str(),
		Base: ResolvedBase{
			DeclaredOS: s.str(),
			AMIID:      s.str(),
			AMISHA256:  s.digest(),
		},
	}

	// Every count is drawn once, into a variable, rather than in the loop
	// condition. `i < s.intn(5)` re-rolls the bound on every iteration, which
	// makes the number of layers a function of several bytes instead of one and
	// makes "which input reaches two layers" close to unanswerable — the property
	// still holds either way, but a domain nobody can reason about is a domain
	// nobody can check the coverage of.
	nLayers := s.intn(5)
	for i := 0; i < nLayers; i++ {
		l.Layers = append(l.Layers, ResolvedLayer{
			LayerManifest: LayerManifest{
				Name:    s.str(),
				Version: s.str(),
				SHA256:  s.digest(),
			},
			MountOrder: i + 1,
		})
	}

	if n := s.intn(4); n > 0 {
		l.Env = make(map[string]string, n)
		for i := 0; i < n; i++ {
			l.Env[s.str()] = s.str()
		}
	}

	nOnReady := s.intn(3)
	for i := 0; i < nOnReady; i++ {
		l.OnReady = append(l.OnReady, s.str())
	}

	nSets := s.intn(3)
	for i := 0; i < nSets; i++ {
		set := ResolvedPackageSet{Manager: PackageManager(s.str()), Env: s.str()}
		// Non-nil so the nil-versus-empty distinction is reached only by a
		// transformation that means to reach it, not incidentally here. #147 fixed
		// that distinction (#117); this stays non-nil because the transformation
		// varying it is not in the live set yet — see the stated domain above.
		set.Packages = []ResolvedPackageEntry{}
		nEntries := s.intn(4)
		for j := 0; j < nEntries; j++ {
			set.Packages = append(set.Packages, ResolvedPackageEntry{
				Name:    s.str(),
				Version: s.str(),
				SHA256:  s.digest(),
			})
		}
		l.Packages = append(l.Packages, set)
	}

	return l
}

// clone deep-copies every field the transformations below reach into, so a
// transformation cannot mutate the original and compare a lockfile against
// itself. A shallow copy would make this target pass by construction.
func clone(l *LockFile) *LockFile {
	out := *l

	if l.Layers != nil {
		out.Layers = make([]ResolvedLayer, len(l.Layers))
		copy(out.Layers, l.Layers)
	}
	if l.Env != nil {
		out.Env = make(map[string]string, len(l.Env))
		for k, v := range l.Env {
			out.Env[k] = v
		}
	}
	if l.OnReady != nil {
		out.OnReady = make([]string, len(l.OnReady))
		copy(out.OnReady, l.OnReady)
	}
	if l.RequiresHost != nil {
		out.RequiresHost = make([]HostRequirement, len(l.RequiresHost))
		copy(out.RequiresHost, l.RequiresHost)
	}
	if l.Packages != nil {
		out.Packages = make([]ResolvedPackageSet, len(l.Packages))
		for i, set := range l.Packages {
			cp := set
			if set.Packages != nil {
				cp.Packages = make([]ResolvedPackageEntry, len(set.Packages))
				copy(cp.Packages, set.Packages)
			}
			out.Packages[i] = cp
		}
	}
	return &out
}

// r7Transform is an environment-preserving rewrite: applied to a frozen
// lockfile it must produce one that assembles the identical environment.
type r7Transform struct {
	// name appears in the failure message and in the exclusion controls.
	name string
	// why is the specification-level reason the environment is unchanged. It is
	// recorded because the justification, not the field list, is what makes this
	// a test of R7 rather than a restatement of envHashInput.
	why string
	// apply rewrites in place. It returns false when nothing actually changed —
	// either because the input offered nothing to change (no layers to permute)
	// or because the rewrite computed the identity. The distinction matters: a
	// no-op compared against itself passes, so counting it as an exercise of R7
	// overstates the coverage by exactly the amount that is hardest to notice.
	apply func(l *LockFile, s *r7Stream) bool
}

// liveTransforms returns the transformations R7 is asserted over. A function
// rather than a package-level var: this package declares no globals.
func liveTransforms() []r7Transform {
	return []r7Transform{
		{
			name: "permute-layers",
			why: "MountOrder defines the mount stack; slice position is the order the " +
				"layers happened to appear in YAML. All MountOrder values here are distinct.",
			apply: func(l *LockFile, s *r7Stream) bool {
				n := len(l.Layers)
				if n < 2 {
					return false
				}
				// Both branches are provably non-identity for n >= 2: a reversal
				// moves the first element, and a transposition of two distinct
				// indices moves both. An earlier version rotated by k and then
				// reversed, which for n=2, k=1 composes to the identity — it
				// returned true having permuted nothing, and the pair it produced
				// was a lockfile compared against a copy of itself.
				if s.boolean() {
					for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
						l.Layers[i], l.Layers[j] = l.Layers[j], l.Layers[i]
					}
				} else {
					j := 1 + s.intn(n-1)
					l.Layers[0], l.Layers[j] = l.Layers[j], l.Layers[0]
				}
				return true
			},
		},
		{
			name: "vary-bundle",
			why: "the attestation bundle is read by no assembler — grep for `.Bundle` " +
				"finds no consumer that writes it into the assembled root or the child " +
				"environment. RekorEntry was in this transformation and is now excluded: " +
				"it reaches both (#120).",
			apply: func(l *LockFile, s *r7Stream) bool {
				// Append rather than redraw. s.str() returns "" whenever the
				// stream has run out, and buildLockFile leaves Bundle empty, so
				// redrawing was a no-op on every seed — which the reachability
				// test below caught. Appending is provably non-identity: the
				// result is one byte longer than the input, always.
				before := l.Bundle
				l.Bundle += string(rune('a' + s.intn(26)))
				return l.Bundle != before
			},
		},
		{
			name: "vary-provenance-and-timing",
			why: "profile digest, resolver version and resolution time are read by no " +
				"assembler. ProfileName was in this transformation and is now excluded: " +
				"it becomes $STRATA_PROFILE in every login shell (#120).",
			apply: func(l *LockFile, s *r7Stream) bool {
				beforeSHA, beforeVer, beforeAt := l.ProfileSHA256, l.StrataVersion, l.ResolvedAt
				l.ProfileSHA256 = s.digest()
				l.StrataVersion = s.str()
				l.ResolvedAt = l.ResolvedAt.Add(time.Duration(s.next()) * time.Hour)
				return l.ProfileSHA256 != beforeSHA ||
					l.StrataVersion != beforeVer ||
					!l.ResolvedAt.Equal(beforeAt)
			},
		},
		{
			name: "vary-requires-host",
			why: "requires_host is advisory by specification — spec/lockfile.go:57-60, " +
				"\"Advisory only in v0.21.0\" — so nothing acts on it.",
			apply: func(l *LockFile, s *r7Stream) bool {
				l.RequiresHost = append(l.RequiresHost, HostRequirement{Key: s.str(), Value: s.str()})
				return true
			},
		},
		{
			name: "rebuild-env-map",
			why: "a map has no order; rebuilding it with the same pairs is the identity " +
				"on the environment. Guards the JSON encoder's key sorting.",
			apply: func(l *LockFile, s *r7Stream) bool {
				// Two keys, not one: with a single entry there is no order for the
				// rebuild to vary, so the pair could not distinguish an encoder
				// that sorts keys from one that does not.
				if len(l.Env) < 2 {
					return false
				}
				keys := make([]string, 0, len(l.Env))
				for k := range l.Env {
					keys = append(keys, k)
				}
				rebuilt := make(map[string]string, len(l.Env))
				for i := len(keys) - 1; i >= 0; i-- {
					rebuilt[keys[i]] = l.Env[keys[i]]
				}
				l.Env = rebuilt
				return true
			},
		},
	}
}

// r7Seeds is the seed corpus, declared once so that
// TestR7SeedsReachEveryTransform reasons about the same inputs the fuzz target
// replays. Two lists would let the test certify seeds the fuzzer does not use.
//
// These are seeds, not a search — see the operational floor above.
func r7Seeds() [][]byte {
	return [][]byte{
		{},
		{3, 1, 'a', 2, 'b', 'c', 9, 9, 9, 9, 4, 4, 4, 4},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{255, 254, 253, 252, 251, 250, 249, 248, 247, 246, 245, 244},
		{2, 7, 'x', 'y', 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 5, 5},

		// The two seeds below were added because TestR7SeedsReachEveryTransform
		// failed without them: none of the five above produces two layers or a
		// two-entry Env, so permute-layers and rebuild-env-map declined every
		// seed and the ordinary `go test ./spec/` run asserted R7 over three of
		// the five transformations while reporting PASS.
		//
		// They are traced against the stream rather than found by luck. Byte 4
		// makes str() return "" (4%4 == 0, so it draws no characters), which is
		// what keeps the layout readable:
		//   4                 ProfileName ""
		//   0,0,0,1           ProfileSHA256
		//   0                 ResolvedAt
		//   4 4 4             StrataVersion, DeclaredOS, AMIID
		//   0,0,0,2           AMISHA256
		//   7                 nLayers = 7%5 = 2   <- reaches permute-layers
		//   4 4 0,0,0,3       layer 0
		//   4 4 0,0,0,4       layer 1
		//   2                 Env entries = 2     <- reaches rebuild-env-map
		//   1,65 4            "a" -> ""
		//   1,66 4            "b" -> ""
		//   3 3               nOnReady = 0, nSets = 0
		{4, 0, 0, 0, 1, 0, 4, 4, 4, 0, 0, 0, 2, 7, 4, 4, 0, 0, 0, 3,
			4, 4, 0, 0, 0, 4, 2, 1, 65, 4, 1, 66, 4, 3, 3},

		// As above with three layers (8%5 == 3) and a trailing odd byte, so
		// permute-layers takes its reversal branch rather than its transposition
		// branch. Without this, one of the two branches is never executed.
		{4, 0, 0, 0, 1, 0, 4, 4, 4, 0, 0, 0, 2, 8, 4, 4, 0, 0, 0, 3,
			4, 4, 0, 0, 0, 4, 4, 4, 0, 0, 0, 5, 2, 1, 65, 4, 1, 66, 4, 3, 3, 1},
	}
}

// r7Check is R7's assertion for one input, shared by the fuzz target and the
// reachability test below.
//
// applied, when non-nil, records how many times each transformation actually
// rewrote something. Two transformations decline inputs that offer nothing to
// change — permute-layers needs two layers, rebuild-env-map needs a non-empty
// map — so a transformation that went permanently dead would make this function
// pass without evaluating it, and would read exactly like one that held.
func r7Check(t *testing.T, data []byte, applied map[string]int) {
	s := &r7Stream{b: data}
	original := buildLockFile(s)

	// A pair whose members are not frozen both hash to "", which would make
	// every comparison below hold without testing anything. The builder
	// always produces non-empty digests, so this is a guard against the
	// builder regressing, not a filter on inputs.
	if !original.IsFrozen() {
		t.Fatalf("builder produced an unfrozen lockfile: %+v", original)
	}
	before := original.EnvironmentID()
	if before == "" {
		t.Fatal("frozen lockfile hashed to the empty string")
	}

	for _, tr := range liveTransforms() {
		transformed := clone(original)
		if !tr.apply(transformed, s) {
			continue
		}
		if applied != nil {
			applied[tr.name]++
		}

		if got := transformed.EnvironmentID(); got != before {
			t.Errorf("R7 refuted by %s: EnvironmentID changed under a transformation "+
				"that preserves the assembled environment.\n"+
				"  why the environment is unchanged: %s\n"+
				"  before: %s\n"+
				"  after:  %s",
				tr.name, tr.why, before, got)
		}

		// The original must not have been mutated, or the comparison above
		// was against a moving target.
		if original.EnvironmentID() != before {
			t.Fatalf("%s mutated the original lockfile", tr.name)
		}
	}
}

// FuzzR7NoSpuriousDistinctions asserts R7 over the stated domain: for every
// transformation that provably preserves the assembled environment, the
// EnvironmentID is unchanged.
func FuzzR7NoSpuriousDistinctions(f *testing.F) {
	for _, seed := range r7Seeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		r7Check(t, data, nil)
	})
}

// TestR7SeedsReachEveryTransform converts the seed corpus's job from a comment
// into a check: every transformation in the live set must actually fire on at
// least one seed.
//
// Without this, deleting a transformation's effect — or narrowing its guard until
// it always declines — leaves both the ordinary run and the search reporting
// success over a smaller property than R7. The seed corpus runs on every
// `go test ./spec/`, so this holds the floor even when no search has run.
//
// The limit is worth stating: this proves the five transformations are reachable,
// not that the *search* exercised each one on many inputs. Go's fuzz workers are
// separate processes with no shared counters, so per-transform application counts
// under -fuzz are not measured here.
func TestR7SeedsReachEveryTransform(t *testing.T) {
	applied := make(map[string]int)
	for i, seed := range r7Seeds() {
		t.Run(fmt.Sprintf("seed%d", i), func(t *testing.T) {
			r7Check(t, seed, applied)
		})
	}

	for _, tr := range liveTransforms() {
		if applied[tr.name] == 0 {
			t.Errorf("transformation %q never fired on any seed, so R7 was asserted "+
				"without it. Either its guard now declines every seed, or it lost its "+
				"effect. Add a seed that reaches it, or remove it and say so in the "+
				"stated domain.", tr.name)
		}
	}
	t.Logf("applications per transformation across %d seeds: %v", len(r7Seeds()), applied)
}

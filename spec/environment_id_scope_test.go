package spec

import (
	"reflect"
	"testing"
	"time"
)

// This file is the machine-checked form of the membership rule stated in
// spec/lockfile_hash.go: a LockFile field is hashed into EnvironmentID unless it
// takes one of four named routes out.
//
// It exists because the rule's previous form — a prose comment plus a field list
// — drifted twice, and because the drift was invisible in both directions. Two
// hashed fields had no consumer at all; ten fields the assembler reads were not
// hashed. Neither is detectable by reading either list alone: the comment was
// self-consistent, and every test in the tree passed. What makes a route
// checkable is stating the field set and the expected behaviour together, so
// that a new field has to be classified before the package compiles green.
//
// # What the three tests do
//
//  1. TestEnvironmentIDRoutes_CoverEveryField compares the route tables below
//     against the struct definitions by reflection, in both directions. A field
//     added to any table's struct fails until it is given a route; a route naming
//     a field that no longer exists fails too, which is the direction a stale
//     list rots in.
//  2. TestEnvironmentIDRoutes_MatchBehaviour mutates each field on a frozen
//     fixture and asserts the identity moved if and only if the table says the
//     field is content. This is what makes the table a claim about the code
//     rather than a description of itself.
//  3. TestEnvironmentIDRoutes_ContentStructsHaveTables checks that the set of
//     tables is closed under "reached through a content field", because (1) can
//     only compare a struct that has a table. Two content fields had element
//     types with no table at all until it was written.
//
// # The LayerManifest class route, and the hole in it
//
// LayerManifest has thirty-odd fields and enumerating a bespoke argument for
// each would be thirty unverified claims. They share one: the manifest describes
// a squashfs artifact whose bytes are pinned by SHA256, which is hashed. Each
// field therefore either describes those bytes — and so cannot vary
// independently of the digest without the manifest being false — or is consumed
// at resolve time, before the lockfile exists, and so cannot change what an
// existing lockfile assembles.
//
// The exceptions are the fields the assembler reads out of the lockfile's own
// copy of the manifest while assembling, which are content and are hashed:
// ID, Name, Version, InstallLayout, SHA256.
//
// The class argument rests on the manifest's copy inside the lockfile being
// *correct*, and nothing enforces that: the lockfile's metadata sits outside the
// signed envelope, so an attacker with registry write can edit Name, Version or
// InstallLayout without breaking a signature. Hashing them makes the edit
// visible in the identity; it does not make it refused. That gap is #146 and is
// not closed here.

// scopeFixture is a frozen lockfile with every field the route tables reach set
// to a distinguishable non-zero value. Two calls return independent values —
// no shared maps or slice backing arrays — so a test can mutate one and compare
// against the other without a clone step that could alias.
func scopeFixture() *LockFile {
	return &LockFile{
		ProfileName:   "genomics",
		ProfileSHA256: "1111111111",
		ResolvedAt:    time.Unix(1700000000, 0).UTC(),
		StrataVersion: "v0.22.0",
		RekorEntry:    "rekor-lockfile-1",
		Bundle:        "bundle-lockfile-1",
		Base: ResolvedBase{
			DeclaredOS: "al2023",
			AMIID:      "ami-0abc",
			AMISHA256:  "2222222222",
			Capabilities: BaseCapabilities{
				Arch: "x86_64",
			},
		},
		Layers: []ResolvedLayer{
			{
				LayerManifest: LayerManifest{
					ID:              "python-3.11.9-x86_64",
					Name:            "python",
					Version:         "3.11.9",
					Source:          "s3://strata-layers/python.sqfs",
					SHA256:          "3333333333",
					Size:            4096,
					ContentManifest: map[string]string{"/bin/python3": "4444444444"},
					RekorEntry:      "rekor-layer-python",
					Bundle:          "bundle-layer-python",
					SignedBy:        "strata-builder",
					CosignVersion:   "v3.0.5",
					Provides:        []Capability{{Name: "python", Version: "3.11"}},
					Requires:        []Requirement{{Name: "glibc", MinVersion: "2.34"}},
					Arch:            "x86_64",
					ABI:             "linux-gnu-2.34",
					BuiltAt:         time.Unix(1690000000, 0).UTC(),
					UserSelectable:  true,
					InstallLayout:   "versioned",
					HasModulefile:   true,
					RecipeSHA256:    "5555555555",
					BuildEnvLockID:  "6666666666",
					BootstrapBuild:  false,
					CaptureSource:   "",
					OriginalPrefix:  "",
					Normalized:      false,
				},
				MountOrder:  1,
				SatisfiedBy: "python@3.11",
			},
			{
				LayerManifest: LayerManifest{
					ID:            "samtools-1.21-x86_64",
					Name:          "samtools",
					Version:       "1.21",
					Source:        "s3://strata-layers/samtools.sqfs",
					SHA256:        "7777777777",
					InstallLayout: "versioned",
				},
				MountOrder:  2,
				SatisfiedBy: "samtools@1.21",
			},
		},
		Env:      map[string]string{"OMP_NUM_THREADS": "8", "MPLBACKEND": "Agg"},
		OnReady:  []string{"echo one", "echo two"},
		Defaults: []SoftwareRef{{Name: "python", Version: "3.11.9"}, {Name: "samtools"}},
		Packages: []ResolvedPackageSet{
			{Manager: "pip", Packages: []ResolvedPackageEntry{
				{Name: "numpy", Version: "2.1.0", SHA256: "8888888888"},
				{Name: "scipy", Version: "1.14.0", SHA256: "9999999999"},
			}},
			{Manager: "conda", Env: "bio", Packages: []ResolvedPackageEntry{
				{Name: "bwa", Version: "0.7.18"},
			}},
		},
		RequiresHost: []HostRequirement{{Key: "nvidia_driver", Value: ">=550"}},
	}
}

// fieldRoute is one field's classification. hashed=true means the field is
// content: the identity must move when it moves. hashed=false names the route it
// takes out, and the identity must not move.
type fieldRoute struct {
	// name is the Go field name, checked against the struct by reflection.
	name string
	// hashed is the expected behaviour, asserted by mutation.
	hashed bool
	// route is content, one of the four routes out named in
	// spec/lockfile_hash.go (inert, digest-mediated, abort-only,
	// declared-provenance), or one of three labels that are not additional
	// routes: order-expressing is rule 2 — the ordering is hashed and only the
	// literal value is dropped; manifest-metadata is the digest-mediated-or-inert
	// argument applied to LayerManifest as a class; nested defers to another
	// table. docs/environment-identity.md states the same reconciliation, because
	// a route set stated in three places is a route set that drifts in three
	// places.
	route string
	// why is the evidence. For an unhashed field it is what makes the route
	// hold; a route without one is an assertion, which is what this file
	// replaced.
	why string
	// mutate overrides the generic reflection mutator. Needed where a generic
	// mutation would change the answer for the wrong reason — appending a
	// zero-valued layer, for instance, makes the lockfile unfrozen, and an
	// unfrozen lockfile has no identity at all.
	mutate func(l *LockFile)
	// nested marks a struct field enumerated by its own table.
	nested bool
}

// lockFileRoutes classifies every field of LockFile.
func lockFileRoutes() []fieldRoute {
	return []fieldRoute{
		{name: "ProfileName", hashed: true, route: "content",
			why: "exported as $STRATA_PROFILE into /etc/profile.d/strata.sh, " +
				"/etc/strata/environment and the child process environment (#120)"},
		{name: "ProfileSHA256", hashed: false, route: "inert",
			why: "no consumer outside spec — grep '\\.ProfileSHA256' finds only " +
				"ProvenanceRecord and the resolver that sets it"},
		{name: "ResolvedAt", hashed: false, route: "inert",
			why: "a timestamp reported in provenance; no assembler reads it"},
		{name: "StrataVersion", hashed: false, route: "inert",
			why: "records which resolver produced the lockfile; nothing at assembly " +
				"time branches on it"},
		{name: "RekorEntry", hashed: false, route: "declared-provenance",
			why: "exported as STRATA_REKOR_ENTRY, which no in-tree code reads back — " +
				"enforced by TestDeclaredProvenance_NoInTreeReader, not asserted here"},
		{name: "Bundle", hashed: false, route: "inert",
			why: "the lockfile's own cosign bundle path; no assembler reads it"},
		{name: "Base", hashed: false, route: "nested", nested: true,
			why: "enumerated by resolvedBaseRoutes"},
		{name: "Layers", hashed: true, route: "content",
			why: "the layer set and its mount sequence are the environment",
			mutate: func(l *LockFile) {
				// A frozen layer, so the mutation tests membership rather than
				// IsFrozen: appending a layer with an empty SHA256 makes the
				// whole lockfile unfrozen and the identity empty, which would
				// register as "changed" without saying anything about Layers.
				l.Layers = append(l.Layers, ResolvedLayer{
					LayerManifest: LayerManifest{
						ID: "r-4.4.3-x86_64", Name: "R", Version: "4.4.3",
						SHA256: "aaaaaaaaaa", InstallLayout: "versioned",
					},
					MountOrder: 3,
				})
			}},
		{name: "Env", hashed: true, route: "content",
			why: "written into /etc/profile.d/strata.sh and /etc/strata/environment " +
				"(internal/overlay/overlay.go:145-150, :200-202)"},
		{name: "OnReady", hashed: true, route: "content",
			why: "declared as commands to run after assembly. Hashed although no " +
				"in-tree executor exists (#69): the field is an instruction, and rule 3 " +
				"binds the identity to instructions rather than to their outcomes"},
		{name: "Defaults", hashed: true, route: "content",
			why: "internal/overlay/overlay.go:171-176 writes 'module load <name>/<version>' " +
				"lines into /etc/profile.d/strata-defaults.sh (#118)"},
		{name: "Packages", hashed: true, route: "content",
			why: "internal/agent/package_installer.go:37 installs each set, :99 and :113 " +
				"run one pip/conda command per entry"},
		{name: "MutableLayer", hashed: false, route: "inert",
			why: "latent: grep 'MutableLayer' outside spec and tests finds no consumer, " +
				"so the persistent-upper workflow it declares is not implemented. A " +
				"lockfile carrying one cannot be published (HasMutableLayer gates it), " +
				"and if an agent ever mounts the upper this route fails and the field " +
				"becomes content"},
		{name: "RequiresHost", hashed: false, route: "inert",
			why: "\"Advisory only in v0.21.0\" by specification — spec/lockfile.go:57-60"},
	}
}

// resolvedBaseRoutes classifies every field of ResolvedBase.
func resolvedBaseRoutes() []fieldRoute {
	return []fieldRoute{
		{name: "DeclaredOS", hashed: false, route: "inert",
			why: "the profile's OS string, used at resolve time to pick the AMI; " +
				"no assembler reads it",
			mutate: func(l *LockFile) { l.Base.DeclaredOS += "-probe" }},
		{name: "AMIID", hashed: false, route: "digest-mediated — unenforced",
			why: "the base image is content, and AMISHA256 is the digest of it. AMIID is " +
				"the locator. Hashing the locator instead would give the same filesystem " +
				"copied to a second region two identities. The mediation this route names " +
				"is not implemented: nothing compares the booted AMI against AMISHA256, " +
				"and AMISHA256 is never populated by shipped code (#64). This is the one " +
				"row in these tables whose route is a promise rather than a check, and it " +
				"is stated here so it is visible rather than absent",
			mutate: func(l *LockFile) { l.Base.AMIID += "-probe" }},
		{name: "AMISHA256", hashed: true, route: "content",
			why:    "the digest of the base filesystem every layer is stacked on",
			mutate: func(l *LockFile) { l.Base.AMISHA256 += "aa" }},
		{name: "Capabilities", hashed: false, route: "inert",
			why: "the probe record the resolver validated layer requirements against; " +
				"consumed at resolve time, before the lockfile exists",
			mutate: func(l *LockFile) { l.Base.Capabilities.Arch += "-probe" }},
	}
}

// resolvedLayerRoutes classifies ResolvedLayer's own fields — those not part of
// the embedded LayerManifest.
func resolvedLayerRoutes() []fieldRoute {
	return []fieldRoute{
		{name: "LayerManifest", hashed: false, route: "nested", nested: true,
			why: "enumerated by layerManifestRoutes"},
		{name: "MountOrder", hashed: false, route: "order-expressing",
			why: "the ordering it expresses is hashed — computeEnvironmentID sorts the " +
				"layers by it before hashing — while the literal value is not. Mount " +
				"orders 1,2 and 10,20 mount the same layers in the same sequence, so " +
				"they must not differ in identity. Asserted directly by " +
				"TestEnvironmentID_MountOrderValuesAreNotHashed",
			mutate: func(l *LockFile) {
				// Scale rather than permute: 1,2 becomes 10,20, which preserves
				// the sequence. Adding a constant would too, but scaling also
				// rules out an implementation that hashed differences.
				for i := range l.Layers {
					l.Layers[i].MountOrder *= 10
				}
			}},
		{name: "SatisfiedBy", hashed: false, route: "inert",
			why:    "records which profile ref this layer satisfied; read by no assembler",
			mutate: func(l *LockFile) { l.Layers[0].SatisfiedBy += "-probe" }},
		{name: "FromFormation", hashed: false, route: "inert",
			why:    "records the formation a layer was expanded from; read by no assembler",
			mutate: func(l *LockFile) { l.Layers[0].FromFormation += "-probe" }},
	}
}

// layerManifestRoutes classifies every field of LayerManifest. All but five take
// the manifest-metadata class route argued for in this file's header comment;
// the five the assembler reads out of the lockfile's copy are content.
func layerManifestRoutes() []fieldRoute {
	const class = "manifest-metadata"
	const classWhy = "describes the squashfs artifact pinned by SHA256, or is consumed " +
		"at resolve time before the lockfile exists — see this file's header for the " +
		"class argument and the hole in it (#146)"

	content := func(name, why string) fieldRoute {
		return fieldRoute{name: name, hashed: true, route: "content", why: why,
			mutate: layerFieldMutator(name)}
	}
	metadata := func(name string) fieldRoute {
		return fieldRoute{name: name, hashed: false, route: class, why: classWhy,
			mutate: layerFieldMutator(name)}
	}

	return []fieldRoute{
		content("ID", "internal/overlay/mount_linux.go:147 makes the layer's mount point "+
			"/strata/layers/<id>"),
		content("Name", "internal/overlay/overlay.go:112 builds $STRATA_ENV/<name>/<version>/bin "+
			"into PATH (#122)"),
		content("Version", "same PATH construction as Name (#122)"),
		content("SHA256", "the digest of the mounted filesystem — the layer's content"),
		content("InstallLayout", "decides whether the layer contributes a versioned PATH "+
			"entry at all: every consumer compares it against \"flat\" "+
			"(internal/overlay/overlay.go:109,116; cmd/strata/run.go:336,342; "+
			"internal/fold/eject.go:220,225; internal/export/oci.go:371,378)"),

		{name: "Source", hashed: false, route: "digest-mediated",
			why: "the fetch locator. What it returns is verified against SHA256 before " +
				"use — cmd/strata/run.go:436, :481; internal/agent/agent.go:244 — so a " +
				"different Source either yields the same bytes or aborts",
			mutate: layerFieldMutator("Source")},
		{name: "RekorEntry", hashed: false, route: "abort-only",
			why: "internal/agent/agent.go:367 verifies the fetched layer against it and " +
				"refuses on mismatch; it can stop assembly, never change it",
			mutate: layerFieldMutator("RekorEntry")},
		{name: "Bundle", hashed: false, route: "abort-only",
			why:    "same verification path as RekorEntry",
			mutate: layerFieldMutator("Bundle")},

		metadata("Size"),
		metadata("ContentManifest"),
		metadata("SignedBy"),
		metadata("CosignVersion"),
		metadata("Provides"),
		metadata("Requires"),
		metadata("Arch"),
		metadata("ABI"),
		metadata("BuiltAt"),
		metadata("UserSelectable"),
		metadata("HasModulefile"),
		metadata("RecipeSHA256"),
		metadata("BuildEnvLockID"),
		metadata("BuiltWith"),
		metadata("BootstrapBuild"),
		metadata("BootstrapCompiler"),
		metadata("CaptureSource"),
		metadata("FoldedFrom"),
		metadata("OriginalPrefix"),
		metadata("Normalized"),
	}
}

// packageSetRoutes classifies every field of ResolvedPackageSet. The struct has
// a table because LockFile.Packages is content: a content field's own fields are
// unclassified until they are enumerated, which is the hole
// TestEnvironmentIDRoutes_ContentStructsHaveTables closes.
func packageSetRoutes() []fieldRoute {
	return []fieldRoute{
		{name: "Manager", hashed: true, route: "content",
			why: "internal/agent/package_installer.go:39-45 switches on it to pick pip, " +
				"conda or CRAN — the same package names installed by a different manager " +
				"are different software",
			mutate: packageSetFieldMutator("Manager")},
		{name: "Env", hashed: true, route: "content",
			why: "internal/agent/package_installer.go:109-111 passes it as conda's -n, so " +
				"it decides which environment the packages land in",
			mutate: packageSetFieldMutator("Env")},
		{name: "Packages", hashed: true, route: "content",
			why:    "the entries installed; enumerated by packageEntryRoutes",
			mutate: packageSetFieldMutator("Packages")},
	}
}

// packageEntryRoutes classifies every field of ResolvedPackageEntry.
func packageEntryRoutes() []fieldRoute {
	return []fieldRoute{
		{name: "Name", hashed: true, route: "content",
			why:    "internal/agent/package_installer.go:100 builds the pip argument from it",
			mutate: packageEntryFieldMutator("Name")},
		{name: "Version", hashed: true, route: "content",
			why:    "pinned into the same argument (package_installer.go:100, :114-117)",
			mutate: packageEntryFieldMutator("Version")},
		{name: "SHA256", hashed: true, route: "content",
			why: "recorded but not checked at install time (#98), so it is hashed as an " +
				"instruction under rule 3 rather than as a guarantee — the same shape as " +
				"OnReady. It is not inert: it is written into the lockfile as part of the " +
				"request, and if the installer ever enforces it the field becomes a " +
				"guarantee without this row needing to change",
			mutate: packageEntryFieldMutator("SHA256")},
	}
}

// softwareRefRoutes classifies every field of SoftwareRef as it appears in
// LockFile.Defaults. The same struct also appears in a Profile, where Formation
// is read; a route here is a claim about the lockfile's copy only.
func softwareRefRoutes() []fieldRoute {
	return []fieldRoute{
		{name: "Name", hashed: true, route: "content",
			why: "internal/overlay/overlay.go:171-176 writes 'module load <name>/<version>' " +
				"from it (#118)",
			mutate: softwareRefFieldMutator("Name")},
		{name: "Version", hashed: true, route: "content",
			why:    "the other half of the same module load line",
			mutate: softwareRefFieldMutator("Version")},
		{name: "Formation", hashed: false, route: "inert",
			why: "every reader of this field takes it from a Profile at resolve time, " +
				"before a lockfile exists — internal/resolver/stages.go:52 and " +
				"spec/profile.go:156,163,267. Nothing reads it from LockFile.Defaults: " +
				"overlay.go:171-176 is the only lockfile consumer and reads Name and " +
				"Version. A lockfile default that is a formation ref therefore has an " +
				"empty Name, and overlay writes a bare 'module load ' line — a defect in " +
				"that writer, not evidence that this field is content",
			mutate: softwareRefFieldMutator("Formation")},
	}
}

// layerFieldMutator returns a mutator that applies the generic reflection
// mutation to one field of the first layer's manifest.
func layerFieldMutator(field string) func(l *LockFile) {
	return func(l *LockFile) {
		mutateValue(reflect.ValueOf(&l.Layers[0].LayerManifest).Elem().FieldByName(field))
	}
}

// packageSetFieldMutator mutates one field of the first package set.
func packageSetFieldMutator(field string) func(l *LockFile) {
	return func(l *LockFile) {
		mutateValue(reflect.ValueOf(&l.Packages[0]).Elem().FieldByName(field))
	}
}

// packageEntryFieldMutator mutates one field of the first entry of the first
// package set.
func packageEntryFieldMutator(field string) func(l *LockFile) {
	return func(l *LockFile) {
		mutateValue(reflect.ValueOf(&l.Packages[0].Packages[0]).Elem().FieldByName(field))
	}
}

// softwareRefFieldMutator mutates one field of the first lockfile default.
func softwareRefFieldMutator(field string) func(l *LockFile) {
	return func(l *LockFile) {
		mutateValue(reflect.ValueOf(&l.Defaults[0]).Elem().FieldByName(field))
	}
}

// mutateValue replaces v with a value that differs from it, for the kinds the
// spec structs use. It returns false when it has no rule for the kind — the
// caller fails rather than skipping, because a field nothing can mutate is a
// field nothing checks.
func mutateValue(v reflect.Value) bool {
	if !v.IsValid() || !v.CanSet() {
		return false
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(v.String() + "-probe")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(v.Int() + 1)
	case reflect.Bool:
		v.SetBool(!v.Bool())
	case reflect.Slice:
		v.Set(reflect.Append(v, reflect.Zero(v.Type().Elem())))
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return false
		}
		if v.IsNil() {
			v.Set(reflect.MakeMap(v.Type()))
		}
		v.SetMapIndex(reflect.ValueOf("strata-probe"), reflect.Zero(v.Type().Elem()))
	case reflect.Pointer:
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		} else {
			v.Set(reflect.Zero(v.Type()))
		}
	case reflect.Struct:
		if t, ok := v.Interface().(time.Time); ok {
			v.Set(reflect.ValueOf(t.Add(time.Hour)))
			return true
		}
		return false
	default:
		return false
	}
	return true
}

// routeTable pairs a route list with the struct it classifies.
func routeTables() []struct {
	label  string
	typ    reflect.Type
	routes []fieldRoute
} {
	return []struct {
		label  string
		typ    reflect.Type
		routes []fieldRoute
	}{
		{"LockFile", reflect.TypeOf(LockFile{}), lockFileRoutes()},
		{"ResolvedBase", reflect.TypeOf(ResolvedBase{}), resolvedBaseRoutes()},
		{"ResolvedLayer", reflect.TypeOf(ResolvedLayer{}), resolvedLayerRoutes()},
		{"LayerManifest", reflect.TypeOf(LayerManifest{}), layerManifestRoutes()},
		{"ResolvedPackageSet", reflect.TypeOf(ResolvedPackageSet{}), packageSetRoutes()},
		{"ResolvedPackageEntry", reflect.TypeOf(ResolvedPackageEntry{}), packageEntryRoutes()},
		{"SoftwareRef", reflect.TypeOf(SoftwareRef{}), softwareRefRoutes()},
	}
}

// structTypeOf unwraps slices, arrays, pointers and map values down to a struct
// type, and reports whether it found one. time.Time is excluded: it is a struct
// whose fields are unexported implementation detail, not a spec type whose
// membership anyone can classify.
func structTypeOf(t reflect.Type) (reflect.Type, bool) {
	for {
		switch t.Kind() {
		case reflect.Slice, reflect.Array, reflect.Pointer, reflect.Map:
			t = t.Elem()
		case reflect.Struct:
			if t == reflect.TypeOf(time.Time{}) {
				return nil, false
			}
			return t, true
		default:
			return nil, false
		}
	}
}

// TestEnvironmentIDRoutes_ContentStructsHaveTables checks the tables reach as far
// as the identity does.
//
// TestEnvironmentIDRoutes_CoverEveryField compares each table against its own
// struct, so it says nothing about a struct that has no table at all. That is not
// a hypothetical: LockFile.Packages and LockFile.Defaults were classified content
// and their element types went unenumerated, so a field added to
// ResolvedPackageEntry or SoftwareRef would have been dropped from the identity
// with every test in this file still green.
//
// The rule is: a struct type needs a table when it is reached through a field the
// tables call content, or through a field explicitly marked nested. A field
// classified as taking a route out needs no table, because the route covers the
// whole subtree beneath it — that is what makes "inert" a claim worth checking
// rather than a claim about one field.
func TestEnvironmentIDRoutes_ContentStructsHaveTables(t *testing.T) {
	tables := routeTables()

	covered := make(map[reflect.Type]string, len(tables))
	for _, table := range tables {
		covered[table.typ] = table.label
	}

	required := 0
	for _, table := range tables {
		for _, r := range table.routes {
			field, ok := table.typ.FieldByName(r.name)
			if !ok {
				// CoverEveryField reports this; nothing to unwrap here.
				continue
			}
			if !r.hashed && !r.nested {
				continue
			}
			typ, isStruct := structTypeOf(field.Type)
			if !isStruct {
				continue
			}
			required++
			if _, ok := covered[typ]; !ok {
				t.Errorf("%s.%s is %s and reaches %s, which has no route table.\n"+
					"  Add one and register it in routeTables. Until then every field of %s "+
					"is unclassified, and a new one joins or leaves the identity with no test "+
					"noticing.",
					table.label, r.name,
					map[bool]string{true: "content", false: "marked nested"}[r.hashed],
					typ.Name(), typ.Name())
			}
		}
	}

	// Non-vacuity: if no row reached a struct at all, the loop above proved
	// nothing and would keep passing after the unwrap broke.
	if required == 0 {
		t.Fatal("no content or nested row reached a struct type; structTypeOf is not " +
			"unwrapping, so this test cannot fail")
	}
	t.Logf("%d content/nested field(s) reach a struct type; %d table(s) registered",
		required, len(tables))
}

func TestEnvironmentIDRoutes_CoverEveryField(t *testing.T) {
	for _, table := range routeTables() {
		t.Run(table.label, func(t *testing.T) {
			if table.typ.NumField() == 0 {
				t.Fatalf("%s has no fields; reflection found nothing to compare against",
					table.label)
			}

			declared := make(map[string]bool, table.typ.NumField())
			for i := 0; i < table.typ.NumField(); i++ {
				declared[table.typ.Field(i).Name] = true
			}

			classified := make(map[string]bool, len(table.routes))
			for _, r := range table.routes {
				if classified[r.name] {
					t.Errorf("%s.%s is classified twice", table.label, r.name)
				}
				classified[r.name] = true
				if r.route == "" || r.why == "" {
					t.Errorf("%s.%s has an empty route or reason; a route without "+
						"evidence is the assertion this table replaced", table.label, r.name)
				}
				if !declared[r.name] {
					t.Errorf("%s.%s is classified here but no longer exists on the struct.\n"+
						"  A renamed or deleted field leaves a route that reads as coverage "+
						"and checks nothing.", table.label, r.name)
				}
			}

			for name := range declared {
				if !classified[name] {
					t.Errorf("%s.%s has no route.\n"+
						"  Every field is hashed into EnvironmentID unless it takes one of the "+
						"named routes out (spec/lockfile_hash.go). Add it to the table with the "+
						"evidence for its route — an unclassified field is how the previous "+
						"field list drifted.", table.label, name)
				}
			}
		})
	}
}

func TestEnvironmentIDRoutes_MatchBehaviour(t *testing.T) {
	baseline := scopeFixture().EnvironmentID()
	if baseline == "" {
		t.Fatal("the fixture is not frozen, so every comparison below would be \"\" == \"\"")
	}

	for _, table := range routeTables() {
		for _, r := range table.routes {
			if r.nested {
				continue
			}
			t.Run(table.label+"/"+r.name, func(t *testing.T) {
				before := scopeFixture()
				mutated := scopeFixture()

				if r.mutate != nil {
					r.mutate(mutated)
				} else if !mutateValue(reflect.ValueOf(mutated).Elem().FieldByName(r.name)) {
					t.Fatalf("no generic mutator for %s.%s (kind %s); give the route an "+
						"explicit mutate func — a field that cannot be varied is a field "+
						"this test does not check",
						table.label, r.name, reflect.ValueOf(mutated).Elem().FieldByName(r.name).Kind())
				}

				// Non-vacuity: an identical pair compares equal, which reads as
				// "the field is not hashed" for every unhashed row in the table.
				if reflect.DeepEqual(before, mutated) {
					t.Fatalf("the mutation for %s.%s left the lockfile unchanged, so this "+
						"row asserts nothing", table.label, r.name)
				}

				got := mutated.EnvironmentID()
				if got == "" {
					t.Fatalf("the mutation for %s.%s made the lockfile unfrozen; the empty "+
						"identity is not a value and this row would pass for the wrong reason",
						table.label, r.name)
				}

				switch changed := got != baseline; {
				case r.hashed && !changed:
					t.Errorf("%s.%s is classified content and the identity did not move.\n"+
						"  why: %s\n"+
						"  Either the field is missing from envHashInput, or the reason above "+
						"is wrong. Both are defects; the first is the one that has happened "+
						"before.", table.label, r.name, r.why)
				case !r.hashed && changed:
					t.Errorf("%s.%s takes the %q route and the identity moved anyway.\n"+
						"  route holds because: %s\n"+
						"  A field outside the identity that changes it is a spurious "+
						"distinction: two lockfiles that assemble the same environment now "+
						"have different identities (R7).", table.label, r.name, r.route, r.why)
				}
			})
		}
	}
}

// TestEnvironmentID_MountOrderValuesAreNotHashed states the order-expressing
// route directly rather than through the table, because the property is about
// two mount orders being equivalent rather than about one field's presence.
func TestEnvironmentID_MountOrderValuesAreNotHashed(t *testing.T) {
	dense := scopeFixture()
	sparse := scopeFixture()
	for i := range sparse.Layers {
		sparse.Layers[i].MountOrder = (i + 1) * 100
	}

	if got, want := sparse.EnvironmentID(), dense.EnvironmentID(); got != want {
		t.Errorf("mount orders 1,2 and 100,200 gave different identities: %s != %s\n"+
			"  Both mount the same layers in the same sequence, so they assemble the same "+
			"environment. Hashing the literal MountOrder manufactures a distinction (R7).",
			got, want)
	}
}

// TestEnvironmentID_LayerSequenceIsContent is the other half: the sequence the
// sort produces is content even though the numbers are not.
func TestEnvironmentID_LayerSequenceIsContent(t *testing.T) {
	forward := scopeFixture()
	reversed := scopeFixture()
	reversed.Layers[0].MountOrder, reversed.Layers[1].MountOrder =
		reversed.Layers[1].MountOrder, reversed.Layers[0].MountOrder

	if forward.EnvironmentID() == reversed.EnvironmentID() {
		t.Error("swapping two layers' mount orders left the identity unchanged.\n" +
			"  The stack order decides which layer's files win in the merged view and " +
			"which version lands first on PATH, so the sequence is content.")
	}
}

// TestEnvironmentID_TiedMountOrderKeepsLockfileOrder pins the reason
// computeEnvironmentID and internal/overlay/mount_linux.go both use a *stable*
// sort. Under a tied MountOrder the mounter's tie-break is the order the layers
// appear in the lockfile, so that order is content; an unstable sort would leave
// the identity resting on sort.Slice's internals, where the two could disagree.
//
// Which layer *should* win under a tie is underdetermined by the specification —
// that is #95, and it is a validation question rather than something this
// function can fix. This test asserts only that the identity follows the same
// tie-break the mounter uses.
func TestEnvironmentID_TiedMountOrderKeepsLockfileOrder(t *testing.T) {
	tied := func() *LockFile {
		l := scopeFixture()
		for i := range l.Layers {
			l.Layers[i].MountOrder = 1
		}
		return l
	}

	asWritten := tied()
	swapped := tied()
	swapped.Layers[0], swapped.Layers[1] = swapped.Layers[1], swapped.Layers[0]

	if asWritten.EnvironmentID() == swapped.EnvironmentID() {
		t.Error("with tied mount orders, swapping the two layers in the lockfile left " +
			"the identity unchanged.\n" +
			"  internal/overlay/mount_linux.go:140 breaks the tie by slice order, so the " +
			"two lockfiles mount a different stack and must not share an identity.")
	}

	// Repeat to catch the failure mode a single comparison cannot see: an
	// unstable sort can return either order for the same input, so one equal
	// comparison is consistent with a stable sort and one unequal comparison is
	// consistent with an unstable one that happened to reorder.
	first := asWritten.EnvironmentID()
	for i := 0; i < 32; i++ {
		if got := tied().EnvironmentID(); got != first {
			t.Fatalf("the identity of a lockfile with tied mount orders is not stable: "+
				"iteration %d gave %s, first call gave %s.\n"+
				"  computeEnvironmentID must use a stable sort.", i, got, first)
		}
	}
}

// TestEnvironmentID_EmptyInstallLayoutEqualsVersioned pins the canonicalisation.
// spec/layer.go declares "versioned" as the default and the empty string means
// exactly that, so two lockfiles differing only in whether the key was written
// out assemble the same environment.
func TestEnvironmentID_EmptyInstallLayoutEqualsVersioned(t *testing.T) {
	omitted := scopeFixture()
	written := scopeFixture()
	for i := range omitted.Layers {
		omitted.Layers[i].InstallLayout = ""
		written.Layers[i].InstallLayout = "versioned"
	}

	if got, want := omitted.EnvironmentID(), written.EnvironmentID(); got != want {
		t.Errorf("an omitted install_layout and an explicit \"versioned\" gave different "+
			"identities: %s != %s\n"+
			"  Every consumer compares this field against \"flat\", so the two assemble "+
			"the same environment (R7).", got, want)
	}
}

// TestEnvironmentID_NilAndEmptyInnerPackagesAgree is #117, fixed by the
// omitempty on packageSetHashInput.Packages. A package set with no entries
// installs nothing whether the slice is nil or empty.
//
// It replaces the control that was supposed to detect this fix and could not:
// TestR7Exclusion_NilVersusEmptyInnerPackages mutated the whole Packages field
// rather than varying nil against empty, so it asserted the identity changes
// when two entries are replaced by none — true before and after the fix, and
// silent about either. That control was deleted by #148, which measured the
// failure rather than reasoning about it: with #147's fix applied it still passed.
func TestEnvironmentID_NilAndEmptyInnerPackagesAgree(t *testing.T) {
	withNil := scopeFixture()
	withEmpty := scopeFixture()

	// An entry-free set, which is the only case the distinction can arise in:
	// dropping entries from a set that has them changes what is installed.
	withNil.Packages = append(withNil.Packages,
		ResolvedPackageSet{Manager: "cran", Packages: nil})
	withEmpty.Packages = append(withEmpty.Packages,
		ResolvedPackageSet{Manager: "cran", Packages: []ResolvedPackageEntry{}})

	if got, want := withNil.EnvironmentID(), withEmpty.EnvironmentID(); got != want {
		t.Errorf("a package set with nil entries and one with empty entries gave "+
			"different identities: %s != %s\n"+
			"  Both install nothing (#117).", got, want)
	}

	// And the distinction that must survive: a set with entries is not the same
	// as a set without them. Without this, the assertion above is satisfied by a
	// hash that ignores package entries entirely.
	populated := scopeFixture()
	populated.Packages = append(populated.Packages,
		ResolvedPackageSet{Manager: "cran", Packages: []ResolvedPackageEntry{
			{Name: "ggplot2", Version: "3.5.1"},
		}})
	if populated.EnvironmentID() == withEmpty.EnvironmentID() {
		t.Error("a package set with an entry and one with no entries share an identity; " +
			"the entries are not reaching the hash at all")
	}
}

// TestEnvironmentID_PackageOrderIsContent records why neither dimension of
// Packages is sorted before hashing. internal/agent/package_installer.go:37
// iterates the sets and :99, :113 run one pip/conda command per entry, so a
// later entry can change what an earlier one installed — pip resolves each
// command against the environment the previous one left behind.
//
// #95 prescribes sorting both dimensions to make the identity
// order-insensitive. That prescription is refuted by the consumer above: it
// would give two different install sequences one identity. The refutation is
// recorded on #95; this test pins the behaviour the refutation implies so that
// the fix cannot be applied without the assertion failing.
func TestEnvironmentID_PackageOrderIsContent(t *testing.T) {
	asWritten := scopeFixture()

	setsSwapped := scopeFixture()
	setsSwapped.Packages[0], setsSwapped.Packages[1] =
		setsSwapped.Packages[1], setsSwapped.Packages[0]
	if asWritten.EnvironmentID() == setsSwapped.EnvironmentID() {
		t.Error("swapping two package sets left the identity unchanged; the sets are " +
			"installed in order, so the two lockfiles can produce different environments")
	}

	entriesSwapped := scopeFixture()
	e := entriesSwapped.Packages[0].Packages
	e[0], e[1] = e[1], e[0]
	if asWritten.EnvironmentID() == entriesSwapped.EnvironmentID() {
		t.Error("swapping two entries within a package set left the identity unchanged; " +
			"one pip command runs per entry, in order")
	}
}

// TestEnvironmentID_UnfrozenHasNoIdentity is rule 4 of the decision: the
// undefined case is not a value. It is stated here because every other test in
// this file compares two identities, and a comparison of two empty strings
// succeeds without exercising anything.
func TestEnvironmentID_UnfrozenHasNoIdentity(t *testing.T) {
	unfrozen := scopeFixture()
	unfrozen.Layers[0].SHA256 = ""

	if unfrozen.IsFrozen() {
		t.Fatal("clearing a layer digest left the lockfile frozen; the fixture or " +
			"IsFrozen has changed and this test no longer reaches its subject")
	}
	if got := unfrozen.EnvironmentID(); got != "" {
		t.Errorf("an unfrozen lockfile returned an identity %q; only frozen lockfiles "+
			"have one", got)
	}

	noBase := scopeFixture()
	noBase.Base.AMISHA256 = ""
	if got := noBase.EnvironmentID(); got != "" {
		t.Errorf("a lockfile with no base digest returned an identity %q", got)
	}
}

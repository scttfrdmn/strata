# Environment identity

`LockFile.EnvironmentID()` returns the SHA256 of a canonical encoding of a
lockfile's content. It is used as a registry storage key, an EC2 instance tag, an
OCI image label and DOI metadata, so what it does and does not commit to decides
whether those uses are sound.

This document is the reasoning behind `envHashInput` in `spec/lockfile_hash.go`.
The field list itself is not repeated here — a second copy is a second thing to
keep current, and the copy that used to live in `spec/lockfile.go` drifted twice.
What is here is the rule for deciding membership, the residual the identity does
*not* cover, and the gaps that are known and open.

## Four rules

**1. Content, or a named route out.** A lockfile field is hashed unless it takes
one of four routes, each a checkable obligation rather than a judgement:

| Route | Means | Fails when |
|---|---|---|
| `inert` | No consumer outside `spec` reads it | A consumer appears |
| `digest-mediated` | Its influence is bounded by a check against a hashed field | The check is removed, or was never implemented |
| `abort-only` | It can stop assembly but never change it | A code path degrades instead of refusing |
| `declared-provenance` | Exported under a name the identity's claim excludes, with no in-tree reader | Something reads the name back |

Two labels in the route tables are not additional routes and should not be read
as extending this list. `order-expressing` (`ResolvedLayer.MountOrder`) is rule 2
below: the ordering the field defines *is* hashed and only its literal value is
dropped. `manifest-metadata` is the `digest-mediated`-or-`inert` argument applied
to `LayerManifest` as a class — every field of it either describes the squashfs
artifact pinned by `SHA256`, which is hashed, or is consumed at resolve time
before the lockfile exists. The five fields the assembler reads out of the
lockfile's own copy of the manifest — `ID`, `Name`, `Version`, `InstallLayout`,
`SHA256` — are excepted from that class and hashed.

A field with *no* route is the defect this list exists to catch. Three tests
enforce it, all in `spec/environment_id_scope_test.go`:
`TestEnvironmentIDRoutes_CoverEveryField` compares the route tables against the
struct definitions by reflection in both directions;
`TestEnvironmentIDRoutes_MatchBehaviour` mutates each field and asserts the
identity moves if and only if the table says the field is content; and
`TestEnvironmentIDRoutes_ContentStructsHaveTables` requires a table for every
struct reached through a field the tables call content, since the first test can
only compare a struct that already has one.

A field classified as taking a route out needs no table for its own type: the
route covers the whole subtree beneath it, which is what makes `inert` a claim
worth checking rather than a claim about one field.

The `declared-provenance` route has its own enforcement, because it is the one
route that rests entirely on an absence: `TestDeclaredProvenance_NoInTreeReader`
in `spec/declared_provenance_test.go` enumerates every in-tree mention of
`STRATA_REKOR_ENTRY` and `strata.lockfile.rekor_entry` and fails on any mention
that is not a known write site.

**2. Hash the meaning, not the encoding.** Two lockfiles that assemble the same
environment must not differ in identity because of how they were written down.
Two consequences are implemented:

| Encoding difference | Treated as | Where |
|---|---|---|
| `install_layout:` omitted vs. `versioned` | Same identity | `canonicalInstallLayout`, `spec/lockfile_hash.go` |
| A package set's entries `nil` vs. `[]` | Same identity | `omitempty` on `packageSetHashInput.Packages` |
| Mount orders `1,2` vs. `10,20` | Same identity | Layers are sorted by `MountOrder`; the value is not hashed |
| Order of two package sets | **Different** identity | One install command runs per entry, in order |

The last row is not an exception to rule 2 — it is rule 2 applied correctly.
`internal/agent/package_installer.go:37` iterates the sets and `:99` and `:113`
run one `pip`/`conda` command per entry, so a later entry can change what an
earlier one installed. Sorting either dimension would give two different install
sequences one identity.

**3. Equal identities mean identical instructions, not identical state.** This is
the residual, and it is the reason the stronger claim — same ID, same environment
— is recorded REFUTED in `PROPERTIES.md` as X2.

| Instruction | Deterministic? | Consequence of equal IDs |
|---|---|---|
| Base AMI digest, layer digests | Yes | Identical bytes |
| Layer set and mount sequence | Yes | Identical stack |
| Exported environment (`env:`, profile name) | Yes | Identical variables |
| Packages with a recorded digest | Yes | Identical bytes |
| Packages without a digest | No | Same request, whatever upstream now resolves it to |
| `on_ready:` commands | No | Same commands, no claim about their effect |
| A mutable EBS upper | No | Same declaration, unknown contents |

For the bottom three, the identity commits to the instruction and not to its
outcome. A reader who needs the stronger claim needs a lockfile that avoids them,
which is what `strata freeze` is for.

**4. The undefined case is not a value.** An unfrozen lockfile has no identity,
and `EnvironmentID()` returns `""` to say so. `""` is not an identifier: it must
not be published, tagged, or used as a storage key. This is currently violated —
see below.

## Known gaps, all open

These are stated rather than fixed, because each is tracked separately and
because a document that quietly omits them reads as a claim they do not exist.

| Gap | Effect | Issue |
|---|---|---|
| No shipped code assigns `Base.AMISHA256` | `IsFrozen()` is false for every lockfile the resolver produces, so `EnvironmentID()` returns `""` for all of them | #64 |
| The lockfile's copy of a layer's `name`, `version` and `install_layout` sits outside the signed envelope | An attacker with registry write can change them; hashing them makes the change *visible* in the identity, not *refused* | #146 |
| `""` reaches a storage key and an instance tag | Two of the three publishing paths have no frozen gate. `pkg/strata.UploadLockfile` (`pkg/strata/strata.go:125`) is the only caller of `PutLockfile`, so an unfrozen lockfile is stored at `locks/.yaml` — one key shared by every such lockfile. `cmd/strata-agent/ec2_signaler.go:84` tags every instance with `strata:environment-id` unconditionally. The DOI path *is* gated: `strata publish` refuses an unfrozen lockfile before reaching Zenodo (`cmd/strata/publish.go:30`, `:73`) | #124 |
| Which layer wins under a tied `MountOrder` is undefined | The identity follows the mounter's tie-break — lockfile order, via a stable sort — but the specification does not say what a tie *means*, so a resolver could emit ties that assemble unpredictably | #95 |
| `on_ready:` has no in-tree executor | Hashed as an instruction under rule 3; if one is added, the outcome becomes part of the environment and rule 3's residual shrinks | #69 |
| Package digests are recorded but not enforced at install time | Two lockfiles differing only in a recorded digest install identical bytes, so the digest is hashed as an instruction rather than a guarantee | #98 |

## Changing what is hashed

Widening the hash changes every identity it touches, so the cost depends
entirely on how many identities are stored. Today that number is zero: because
no shipped code populates `Base.AMISHA256` (#64), every lockfile the resolver can
produce returns `""`, and there is nothing whose identity a widening could
invalidate. That is a property of the present, not of the design — it stops being
true the moment #64 is fixed, and the cost of widening rises from nothing to a
migration.

Any change to membership must:

1. add or move the field's row in the route tables in
   `spec/environment_id_scope_test.go`, with the evidence for its route;
2. state the reason in `spec/lockfile_hash.go` next to the field, not here;
3. update `PROPERTIES.md` where a proposition's evidence cites the old
   membership.

The tests enforce (1) mechanically. Nothing enforces (2) or (3), which is why
they are written down.

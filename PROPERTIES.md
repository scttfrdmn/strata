# Strata — Security and Correctness Properties

**Status: first population, 2026-08-21; amended the same day after review.**
Every proposition below carries a verdict from an adversarial review of the
proposition *text*, an authored `Basis`, and a **generated** `Status` — see §3
and §6, and do not edit a Status cell by hand. The bibliography in §5 has been
verified against primary sources. Of the 34 propositions, 27 are `REFUTED`, 4
are `ENFORCED E1`, 1 is `ASSERTED (E0)`, 1 is `UNPOPULATED` and 1 is
`WITHDRAWN`; that distribution is derived, not counted by eye —
`go run ./cmd/propgen` prints the totals it was taken from. The principal
finding is §0.2.

Provenance for every measurement in this document unless stated otherwise:

| | |
|---|---|
| commit | `339329fb08f2876c5d08405d25f40540f1609268` |
| tree | `457cd8e5350336aa2fe197eed388162f267b10a2` |
| working tree | clean at that commit; this branch adds this file, `internal/propdoc/` and `cmd/propgen/`, and changes nothing else |
| re-derive | `git rev-parse HEAD; git rev-parse HEAD^{tree}; git status --porcelain` |

Note on self-reference: this document is inside the tree it measures, so a
repository-wide `grep` run after it lands will match its own prose. Every grep
cited below therefore carries `--exclude=PROPERTIES.md` where the absence of a
string is the evidence. One claim in this document was already falsified by
writing it down; see §7, item 22.

Statuses are as of the dated entries in §7. Anything that must reflect the
present carries the command that re-derives it.

---

## 0. Purpose and scope

Strata asserts a chain:

> profile → deterministic resolution → immutable layer set → exact lockfile →
> independently verifiable provenance → runnable environment → citable artifact

Each arrow carries a claim. This document states those claims as numbered,
falsifiable propositions, fixes an adversary model against which the security
propositions are evaluated, and defines what counts as evidence for each.

The document exists because inspection has no completion criterion. Five
distinct paths by which a layer could be mounted without verification were found
by reading code. Each was real. None of them told us how many remain — and a
sixth was then found by *measuring* rather than reading, after the issue
enumerating the first five had settled on two (§7, and the `BundleFetcher` row in
§4). A stated property with a model behind it can be *checked*; a property that
lives only in prose can only be spot-checked forever.

**Out of scope for this document:** performance, usability, API design, and any
claim about the scientific validity of what runs inside an environment.

### 0.2 The principal finding

Strata's differentiating claim is *independently verifiable provenance*. The
expected shape of a review like this one is a list of attacks. That is not what
the register in §4 contains.

**Seventeen of thirty-three refutations need no adversary at all.** They are
`H1` — permissive default (§1.4) — with no capability from §1.1 exercised, no
position to occupy, and nothing to compromise:

```
awk '/^## 4\./{f=1;next} /^### 4\.1/{f=0}
     f && /^\| / && $0 !~ /^\| Proposition/ && $0 !~ /^\|---/ {
       n++; split($0,c,"|"); if (c[4] ~ /H1/ && c[4] !~ /A[1-7]/) h++ }
     END{printf "%d of %d\n", h, n}' PROPERTIES.md
```

Of the fourteen rows refuting a group T (trust) proposition, six are in that
class. So the honest account of this system is not that it is under attack and
losing. It is that **the defaults do not verify, and the propositions were
written to be conditional on configuration nobody sets.** T1 — *no unverified
mount* — is satisfiable by an agent whose default policy is `allow-unverified`,
because T1 quantifies over "the trust policy in force" and lets the
implementation choose it (§3.1, rule 9). Five of the six fail-open paths were
reachable by an operator who did nothing wrong and read the documentation
correctly — every one except the absent-bundle omission, which the register
attributes to A1 + A5.

Two consequences for how the rest of this document should be read:

1. A proposition in group T that does not fix the default configuration is not
   measuring anything an operator will encounter. §1.4 states the reading; rule 9
   in §2.1 states the prohibition.
2. An adversary model is necessary and not sufficient. Just over half of what
   this document found lives in a class the model cannot express, which is why H1
   is named as a hazard class beside the model rather than smuggled in as a
   capability.

### 0.1 What this document is not

It is not a status page and it is not a second tracker. Where a proposition is
refuted, the defect's home is a GitHub issue and the register in §4 records the
issue number, not the defect's current state. Where §4 and the tracker disagree,
the tracker is right and §4 is stale — the register is the *history* of what has
been refuted, which is information the tracker does not keep once an issue
closes.

---

## 1. Adversary model

The security propositions (groups I, T, P) are evaluated against an adversary
with the capabilities below. A proposition is *refuted* if any adversary in this
model can violate it.

### 1.1 Capabilities

| ID | Capability | Rationale for inclusion |
|----|-----------|------------------------|
| **A1** | **Registry write.** Replace or add layer bytes, layer manifests, formation manifests, attestation bundles, and any key material served from the registry. | Strata's own documentation offers independent verifiability as a defence against exactly this. If the registry is trusted, the signatures are decorative. |
| **A2** | **Local cache write.** Create, replace, or truncate files under the layer cache as an unprivileged local user, or as a previously-compromised process running as the same user. | Multi-user HPC systems are a stated target. A shared or predictable cache path is reachable by other users. `cmd/strata/cache.go:22` falls back to `filepath.Join(os.TempDir(), "strata", "layers")` when neither `XDG_CACHE_HOME` nor `HOME` is set, which is that premise exactly. |
| **A3** | **Network interposition on unauthenticated fetches.** Serve arbitrary bytes for any URL not protected by a digest recorded in a signed artifact. TLS is assumed sound; this capability covers content served over TLS from a host the attacker controls or has compromised. | Build recipes fetch source over the network. So does the verification binary's own installer (#63) and every `packages:` install (§3, I6). |
| **A4** | **Transparency-log borrowing.** Reference any entry that genuinely exists in a public transparency log, including entries belonging to third-party artifacts. | Not hypothetical: a fixture in this repository once carried three invented Rekor indices that were live entries belonging to other people's artifacts, together with the genuine public log's key identity. |
| **A5** | **Metadata omission.** Present a lockfile or manifest with optional-looking fields absent — an empty digest, an absent bundle, a missing Rekor entry. | Absent is not the same as invalid, and a check written for invalid input frequently admits absent input. |
| **A6** | **Stale-state substitution.** Serve an old but genuinely signed artifact in place of a current one (rollback), or withhold updates indefinitely (freeze). | Standard in the update-security literature; see §5. |
| **A7** | **Upstream mutation of an unpinned dependency.** Change what a name resolves to at a third-party package index between resolution and assembly, without touching Strata's registry. | Distinct from A3: no interposition, no compromise. `conda install <name>` where the recorded version is `latest` (`internal/agent/package_installer.go:115`) and `pip install name==version` with no hash (`:101`) both admit it. Added 2026-08-21; see §7. |

### 1.2 Explicitly out of model

- Compromise of the signing identity's private key or of the transparency log
  operator. (Key compromise *survivability* is a separate property class; see
  the TUF line in §5.)
- Compromise of the host kernel, or of the verification binary as executed.
  Note that **obtaining** the verification binary is in scope under A3.
- **Compromise of the mounting process** — meaning an attacker who has already
  taken control of `strata run` or `strata-agent` at runtime. *Defects* in the
  mounting process are emphatically in scope; they are the subject of group T.
  (Narrowed 2026-08-21; see §7.)
- Physical attacks, side channels, and traffic analysis.
- A malicious *author* of the profile. Strata resolves what the user asked for;
  asking for malicious software is not a Strata failure.

### 1.3 A note on composition

Several capabilities compose into attacks that neither yields alone. **A1 + A5**
is the interesting pair for this codebase: an attacker who can write the registry
and strip an optional field defeats checks that were written assuming the field
is present and malformed. Propositions must be evaluated against composed
adversaries, not one capability at a time.

That nomination is no longer a prediction. `internal/agent/agent.go:285-293`
drops any layer whose `Bundle` field is empty from the set to be verified, and an
empty set returns success — so A1 (write the manifest) plus A5 (omit the bundle
field) produces a clean boot with a fully configured verifier that is never
consulted. The probe is on #92.

Line numbers here and in T1/T5 are as of the commit in the provenance header.
#104 inverts both sites and deletes `:285-293`, so this citation names the tree
this document measures rather than the tree that will exist after that PR merges;
§7 enumerates what to refresh, and in which order.

### 1.4 What the model cannot express — and the class that fills the gap

**Five of the six fail-open paths found in this codebase need no adversary at
all.** `internal/agent/agent.go:270` skips verification when the caller supplied
no verifier; the historical `strata run --no-verify` disabled a check that was
never performed (#55). No capability in §1.1 describes "the deployment's default
is weaker than the operator believes", because that is not something an attacker
*does*.

The sixth path sharpens the class rather than sitting outside it. `BundleFetcher`'s
godoc **instructed** implementations to return `(nil, nil)` for a layer naming no
bundle, and `verifyBundles` read no bytes as nothing to verify. The fail-open was
therefore *specified*: every conforming implementation inherits it, the shipped
`s3LayerFetcher` does, and no amount of testing the implementations against their
documented contract could have found it — the contract was the defect. H1 is
usually a default nobody set deliberately; this is a default written down on
purpose, which is worse and belongs in the same class.

Rather than invent a capability for it, the model names it as a separate hazard
class:

> **H1 — permissive default.** A configuration that a reasonable operator would
> read as verifying does not verify, and nothing at run time says so.

H1 is not an adversary capability and a proposition is not "refuted under H1".
H1 failures are refutations *simpliciter*: they hold for the empty adversary.
Every proposition in group T must be read as quantifying over the default
configuration, not over some configuration.

---

## 2. Proof standard

Each proposition carries an **evidence tier**. Tiers are not interchangeable and
a lower tier is not partial credit for a higher one.

| Tier | Name | What it establishes |
|------|------|---------------------|
| **E0** | **Asserted** | The property is stated in documentation or established by the structure of the code, and no executed test checks it. This is an honest status, not a failure — but it must be labelled. |
| **E1** | **Witnessed** | One or more example tests exercise the property on chosen inputs, and the site the property is about is executed by them. Establishes that the property holds *for those inputs*. |
| **E2** | **Quantified** | A property-based or metamorphic test exercises the property over generated inputs, with the generator's domain stated. Establishes the property over that domain. |
| **E3** | **Exhausted** | An abstract model is checked exhaustively over a bounded state space, *plus* an explicit faithfulness argument relating the model to the implementation. |

**E2 is currently unreachable in this repository.** There are no fuzz targets
and no property-testing library:

```
grep -rn '^func Fuzz' --include='*_test.go' .                    # no output
grep -rn 'testing/quick\|gopter\|pgregory\|rapid' --include='*.go' .  # no output
```

No proposition may claim E2 until one exists. Recording that here prevents the
tier being mistaken for available.

### 2.1 Rules

1. **E3 without a faithfulness argument is E0.** A model check proves a property
   of the model. The claim that the model represents the code is a separate
   claim and must be made separately, with its own evidence.
2. **A generator's domain is part of the claim.** "Holds for all profiles" is not
   established by a generator that only emits single-layer profiles. State the
   domain; a property established over a stated domain is a real result.
3. **An input that cannot fail establishes nothing.** A test whose expected input
   is derived from the artifact it is compared against is a tautology regardless
   of tier. Every negative test must include a control demonstrating that the
   test can fail.
4. **Evidence must be executed, not merely compiled.** A test that exists but is
   unreached by any CI job is E0 until reachability is shown.
5. **A refutation outranks any tier.** One counterexample refutes; no quantity of
   passing evidence restores the proposition until the counterexample is
   discharged and the discharge is itself evidenced.
6. **A test that does not execute the property's site is not E1 for that
   property.** Rule 4 asks whether the *test* runs. Rule 6 asks whether the
   *line* runs. Both have been violated here. Inverting
   `internal/agent/agent.go:285-293` leaves all seven tests in the package green
   with zero coverage hits on the inverted statements (measurement on #93): the
   suite is green and the change is unexecuted. Reachability is shown with a
   coverage delta on the lines the property is about, not with a passing suite.
   (Added 2026-08-21; see §7.)
7. **A documented claim that is false is not E0 — it is a refutation, and the
   documentation is where the counterexample lives.** E0 as originally worded
   admitted a doc comment stating an invariant the code does not have, and rated
   it the same as an honest silence. `internal/trust/verify.go:90-91` asserts
   that `filepath.Join` prevents a `..` escape; it does not. A false invariant is
   worse than an absent one, because it gives a future auditor a reason to stop
   looking. (Added 2026-08-21; see §7.)
8. **Every status is dated.** A tier is a claim about a commit. §7 carries the
   dates; a row without one is `UNPOPULATED`.
9. **A proposition may not defer its content to a definition the implementation
   under test controls.** Where a policy term is unavoidable, the proposition
   fixes the policy externally: it names the policy, or it ranges over all
   policies including the weakest. A proposition whose subject is "whatever the
   specification says" is not a constraint on the specification.

   *Worked example — T1.* As drafted: *"no execution path … that does not pass a
   successful signature verification **under the trust policy in force**."* The
   implementation chooses the policy in force. So an agent that defaults to
   `allow-unverified`, routing every layer through a verifier that returns
   success for a layer naming no bundle, satisfies T1 exactly — while being the
   fail-open this document was written about. T1 could have been claimed
   satisfied throughout the entire period in which six fail-open paths existed,
   and no measurement would have contradicted the claim.

   Repaired, T1 quantifies over the *default* configuration and admits a weaker
   policy only where the operator selected it — which is T5's second sentence,
   and the reason T1 is not worth having except read with T5. The general repair
   is the same in every case: move the definition out of the implementation's
   reach. Rule 9 is the check §3.1 arrived at inductively, promoted to a rule so
   that a new proposition is tested against it before it is written down rather
   than after it has been satisfied.

---

## 3. Propositions

Three columns, two authored and one generated:

- **Verdict** — *authored.* The result of attacking the proposition's *text*:
  `SOUND`, `TOO WEAK` (satisfiable by an implementation that still has the defect
  the proposition was written to exclude — the broken implementation is named),
  `TOO STRONG`, or `ILL-FORMED` (with the falsifiable replacement).
- **Basis** — *authored.* The highest evidence tier claimed for the proposition
  and the citation carrying it, `none` if nothing is cited, or `withdrawn`
  naming what superseded it. A tier with no citation is not a tier.
- **Status** — **generated** from the Basis cell and the §4 register by
  `internal/propdoc.DeriveStatus`. Do not edit it by hand; run
  `go run ./cmd/propgen -write`.

The status function, in the order it applies:

1. A `withdrawn` proposition is not measured — `WITHDRAWN (superseded by …)`.
2. Any **live** register row (`Discharged` not `Yes`; `Partially` is live) makes
   the proposition `REFUTED`, whatever else is cited for it — proof-standard
   rule 5. Where a proposition has more than one row the count is reported as
   `REFUTED (n of m live)`, so discharging one of several counterexamples is
   visible here rather than only in prose.
3. Otherwise the Basis decides: `ENFORCED E1`/`E2`/`E3`, `ASSERTED (E0)`, or
   `UNPOPULATED` where nothing is cited.

A `SOUND` verdict and a `REFUTED` status are the healthy combination: the
proposition is worth having and the system does not yet satisfy it.

Where a proposition's verdict is `TOO STRONG`, Status measures the *repaired*
form named in its evidence cell; that the as-written form is unsatisfiable is the
verdict, not a measurement of the implementation.

### Group R — Resolution

| # | Proposition | Verdict | Basis | Status | Evidence |
|---|-------------|---------|-------|--------|----------|
| **R1** | *Determinism.* Resolution is a function of (profile, registry state, resolver version). Two resolutions with identical inputs produce byte-identical lockfiles. | **TOO STRONG** | E1 — `internal/resolver/resolver_test.go:574` | ENFORCED E1 | A lockfile records `ResolvedAt` from the wall clock, so two resolutions are never byte-identical; satisfying R1 literally means abandoning a field Strata deliberately records. Repaired form — *identical on the canonical content projection* — is witnessed by `internal/resolver/resolver_test.go:574 TestEnvironmentID_Stability`, which resolves the same profile twice and compares `EnvironmentID` and `ProfileSHA256`. Domain: one single-layer profile, one in-memory store. |
| **R2** | *Order-independence.* Permuting the entries of `software:` in a profile does not change the resulting lockfile, except in fields whose specification says they record declaration order. | **TOO WEAK** | none | REFUTED | Broken-but-satisfying implementation: add the sentence *"`mount_order` records declaration order"* to the spec. R2's exception clause then exempts the field that decides OverlayFS shadowing, so YAML key order silently determines which file wins — and R2 still holds. The exception must be closed by naming a canonical tie-break, not by whatever the spec happens to say. Counterexample, executed: two lockfiles differing only in the slice order of two layers that share `MountOrder` yield different `EnvironmentID`s (`ef104908…` vs `4535c5c4…`), with the distinct-`MountOrder` control returning equal (`ef104908…` twice); permuting `Packages` likewise changes the ID (`789830da…` vs `fd576991…`). The doc comment at `spec/lockfile_hash.go:27-28` claims determinism "regardless of the order they appear in the lockfile YAML" — true only when `MountOrder` is distinct. Upstream, the resolver's tie-break *is* declaration order: `internal/resolver/stages.go:301` `sort.Ints(queue) // deterministic tie-breaking` orders by input index. Filed as #95. |
| **R3** | *Totality.* Resolution yields either a complete lockfile or an error. No execution produces a partially-populated lockfile. | **TOO WEAK** | none | REFUTED (2 of 2 live) | Broken-but-satisfying implementation: drop every request that cannot be resolved and return a fully-populated lockfile for the remainder. Every layer it names is complete, so R3 holds, and the environment is silently not the one that was asked for. "Complete" must be quantified over the *request*: every element of `software:` is either represented in the lockfile or named in an error. Realised instance: #79 — a null entry in a software list is silently dropped and the profile resolves without it. Second instance, untracked: `Profile.Instance` and `Profile.Storage` are parsed and then referenced nowhere (see X1). |
| **R4** | *Provider soundness.* If the dependency graph contains an edge from a consumer to a provider for capability *c*, that provider satisfies the consumer's declared version constraint on *c*. | SOUND | none | REFUTED | #67. `internal/resolver/stages.go:266-270` builds `capProviderIdx[cap.Name] = i` in a nested loop with no guard, so the highest-indexed provider of a capability wins the edge irrespective of version. |
| **R5** | *Provider completeness.* If some layer in the resolved set satisfies a consumer's constraint on *c*, resolution does not fail for want of a provider of *c*. | SOUND | none | REFUTED | #67, same issue, different half: `spec/layer.go:192-205 SatisfiesRequirement` is version-aware but first-match, so a satisfiable profile is rejected when a non-satisfying provider of the same capability name is encountered first. |
| **R6** | *Environment identity is functional.* `EnvironmentID` is a function of exactly the fields the specification enumerates: changing any enumerated field changes the ID, and changing any non-enumerated field does not. | **TOO WEAK** | E1 — `spec/spec_test.go:542`, `spec/packages_test.go:208` | ENFORCED E1 | Broken-but-satisfying implementation: hash `base_ami_sha256` alone and enumerate `base_ami_sha256` alone. R6 holds exactly, and the identity distinguishes nothing. R6 defers its whole content to a list the implementation controls — the same defect as R2's exception clause and P1's `IsFrozen`. The repair is to derive the enumeration from behaviour, which is what X2 attempts; R6 and X2 are one proposition split in two, and R6 alone is not worth having. Under the current enumeration (`spec/lockfile_hash.go:16-22`) both directions are witnessed: `spec/spec_test.go:542 TestEnvironmentID` (layer digest changes the ID; `RekorEntry` does not) and `spec/packages_test.go:208 TestEnvironmentIDIncludesPackages`. |
| **R7** | *(new)* *No spurious distinctions.* Two lockfiles that assemble the same environment have the same `EnvironmentID`. | SOUND | none | REFUTED | Added 2026-08-21 (§7). R6 and X2 together constrain only one direction — that behaviour reaches the identity. Nothing forbids the identity distinguishing environments that are identical. `OnReady` is hashed (`spec/lockfile_hash.go:20,50`) and executed by nothing (#69), so two lockfiles that differ only in a never-run command list get different IDs and the same environment. |

R4 and R5 are separate and a system can fail either independently. R4 failing
means a consumer is wired to a provider that does not satisfy it; R5 failing
means a satisfiable profile is rejected. #67 does both, which is why the register
in §4 is many-to-many.

### Group I — Integrity

| # | Proposition | Verdict | Basis | Status | Evidence |
|---|-------------|---------|-------|--------|----------|
| **I1** | *Mount integrity.* Every byte made visible in an assembled environment belongs to a layer whose content hashes to the digest recorded for it in the lockfile. | **TOO STRONG** | withdrawn — I1′, I6 | WITHDRAWN (superseded by I1′, I6) | Satisfying I1 as a universal claim over every visible byte requires abandoning two things Strata deliberately does: `packages:` installs (`internal/agent/agent.go:172`) put bytes from PyPI, conda and CRAN into the merged overlay, and Path B mounts a writable EBS upper (`MutableLayerSpec`). Neither set of bytes belongs to any layer. I1 is therefore restricted to layer-derived bytes, and I6 below covers the remainder. |
| **I1′** | *(restriction of I1)* Every byte made visible **from a layer** hashes to that layer's recorded digest. | SOUND | E1 — `cmd/strata/layer_cache_integrity_test.go:74,123,185,249`, `internal/agent/agent_test.go:181` | ENFORCED E1 | `strata run` route: `cmd/strata/run.go:426` builds the cache path only through `spec.LayerCachePath`, and both return paths hash the file against the declared digest first (`:436`, `:481`). Witnessed by `cmd/strata/layer_cache_integrity_test.go:74 TestLayerCacheAcceptsHonestLayers` (the control, showing the harness can pass) with `:185 TestLayerCacheRejectsPlantedCacheHit`, `:123 TestLayerCacheRejectsTraversalDigest`, `:249 TestLayerCacheRejectsEmptyDigest`. Agent route: `internal/agent/agent.go:231-241` hashes every path `Fetch` returns and compares unconditionally, with no `!= ""` guard; witnessed by `internal/agent/agent_test.go:181 TestRun_SHA256Mismatch`. Domain: these two routes. |
| **I2** | *Cache soundness.* Content obtained from a local cache is used only after its bytes have been hashed and compared against the declared digest, on every use. Under **A2** this must hold for cache hits, not only for fresh downloads. | SOUND | E1 — `cmd/strata/layer_cache_integrity_test.go:74,185` | ENFORCED E1 | Three cache-hit sites, all routed through validation: `cmd/strata/run.go:426` + `spec.VerifyFileDigest`, `internal/registry/s3client.go:366-371`, `internal/registry/localclient.go:268-273`. Witnessed by `TestLayerCacheRejectsPlantedCacheHit` (plants different content under a correct digest and requires refusal) against `TestLayerCacheAcceptsHonestLayers` as the control. `#83` records the standing cost objection — re-hashing every hit is O(environment size) per invocation — and is a design question, not a refutation. |
| **I3** | *Digest well-formedness.* Any value used as a digest — in a comparison, a filesystem path, or a cache key — is syntactically a SHA-256 digest before it is so used. Absent and malformed are both rejected (**A5**). | SOUND | E1 — `spec/digest_test.go:16`, `spec/digest_test.go:53` | REFUTED (2 of 3 live) | The rule exists and is enforced where it is called: `spec/digest.go:30-47 ValidateLayerDigest` requires exactly 64 lowercase hex and rejects empty explicitly, witnessed at `spec/digest_test.go:16 TestValidateLayerDigest` and `:53 TestLayerCachePathRejectsEscape`. Two sites do not call it. (a) `cmd/strata-agent/s3_fetcher.go:73` builds `filepath.Join(f.cacheDir, layer.SHA256+".sqfs")` from an unvalidated digest and `os.Rename`s into it — #81. (b) `LockFile.IsFrozen()` and `EnvironmentID()` treat any non-empty string as a digest: the repository's own `spec/spec_test.go:544` passes `SHA256: "bbbbbb"` and `internal/resolver/resolver_test.go:597` passes `AMISHA256: "sha256-ami-test123456789"`, and both tests pass. So "frozen" is satisfied by a lockfile with no digests in it. (b) (b) untracked → filed as #96. |
| **I4** | *Path confinement.* No field of a lockfile or manifest can cause a filesystem operation outside the directory designated for it, for any field value. | SOUND | E1 — `spec/digest_test.go:53` | REFUTED (3 of 4 live) | The mechanism is understood and stated correctly in one place — `spec/digest.go:52-55`: *"`filepath.Join` calls `Clean`, which resolves `..` rather than rejecting it, so joining an unvalidated digest can name a file outside `cacheDir`. Every site that builds a layer cache path must go through here."* Executed confirmation: `filepath.Join("/var/cache/strata/layers", "../../../../etc/cron.d/evil.sqfs")` → `/etc/cron.d/evil.sqfs`. Four sites build a path from an unvalidated lockfile field: `internal/trust/verify.go:92` (`layer.ID`, and its own comment at `:90-91` asserts the opposite — #58); `internal/overlay/mount_linux.go:141` (`layer.ID` into `os.MkdirAll` then `MountSquashfs` — a **write** primitive, untracked); `internal/export/oci.go:60` (`"layer-"+lp.ID`, untracked); `cmd/strata-agent/s3_fetcher.go:73` (`layer.SHA256`, #81). Untracked pair → filed as #97. |
| **I5** | *Distinctness.* Two layers with distinct content never share a cache location. | **TOO WEAK** | E1 — `spec/digest_test.go:80` | REFUTED (1 of 2 live) | Broken-but-satisfying implementation: key the cache on a fresh UUID per fetch. Distinct content never collides, so I5 holds — and nothing is ever a cache hit, because the useful property is the converse one I5 does not state: *the cache location is a function of the content*. Restated, that is what makes a digest a cache key rather than a label. Counterexample to I5 even as written: `cmd/strata-agent/s3_fetcher.go:73` with `layer.SHA256 == ""` yields the filename `.sqfs` for every hashless layer, so distinct content collides (#81). The confined route rejects it — `spec/digest_test.go:80 TestLayerCachePathEmptyDigestDoesNotCollide`. |
| **I6** | *(new)* *Non-layer byte accountability.* Every byte made visible in an assembled environment that does not come from a layer is either (a) produced inside the environment after assembly, or (b) named in the lockfile with a digest that is checked at the moment it is installed. | SOUND | none | REFUTED (2 of 2 live) | Added 2026-08-21 (§7), as the remainder I1 had to give up. `spec.ResolvedPackageEntry` carries a `SHA256` field (`spec/packages.go`), and the installer never passes it: `internal/agent/package_installer.go:101` runs `pip install --quiet <name>==<version>` with no `--require-hashes` (`grep -rn 'require-hashes' . --exclude=PROPERTIES.md` → no output; see §7 item 22), `:115` runs `conda install` and treats a recorded version of `latest` as "whatever is current". The recorded digest is read only by `strata verify --packages` out of band (`internal/packages/resolve.go:272-287`). Refuted for the empty adversary; A3 and A7 make it an attack rather than a hazard. Filed as #98. |

### Group T — Authenticity and trust

This group is the core of Strata's differentiating claim.

| # | Proposition | Verdict | Basis | Status | Evidence |
|---|-------------|---------|-------|--------|----------|
| **T1** | *No unverified mount.* There exists no execution path from *layer declared in a lockfile* to *layer mounted in an assembled environment* that does not pass a successful signature verification under the trust policy in force. | **TOO WEAK** | E1 — `cmd/strata/run_verify_test.go:370,519` | REFUTED (2 of 4 live) | *The most important finding in this review.* Broken-but-satisfying implementation: ship an agent whose default policy is `allow-unverified`, and route every layer through a verifier that returns success when a layer names no bundle. Every mount then "passes a successful verification under the trust policy in force" and T1 holds — which is precisely the shape of the five fail-opens this document was written about. T1 defers its content to "the policy in force" and never says what the policy is when the operator selects nothing. It is only worth having when read together with T5's second sentence, and it should be restated to quantify over the *default* configuration (see H1, §1.4). Status under that reading: refuted at `internal/agent/agent.go:270` (nil verifier ⇒ skip) and `:285-293` (layer with `Bundle: ""` dropped from the set; empty set ⇒ success) — #92, #93. Probe 3 on #92 boots READY with a verifier that refuses every artifact, `VerifierCalled=false`. Closed on the `strata run` route by #55: `cmd/strata/run_verify_test.go:519 TestRunRun_MissingKeyIsARefusalNotASkip`, `:370 TestVerifyRunLayers_ReportsEveryFailure`. |
| **T2** | *Verification soundness.* Verification succeeds for a layer only if a signature exists, by an identity admitted by the trust policy, over the digest of the exact bytes that will be mounted. | SOUND | E1 — `cmd/strata/run_verify_test.go:466` | REFUTED | The "exact bytes" clause is the load-bearing part and it is satisfied deliberately: `cmd/strata/run.go` adds a check absent from `trust.VerifyLayer` — that the bundle attests *this lockfile's* digest — with the reason stated in its doc comment (*"a missing cosign must not be the difference between 'wrong layer's bundle' and 'accepted'"*). Witnessed by `cmd/strata/run_verify_test.go:466 TestRunRun_RefusesLayerWhoseBundleAttestsAnotherArtifact`. For the *lockfile*, no signature verification exists at all (#60), so T2 does not hold of the artifact that names the layer set. Weakness worth recording: "an identity admitted by the trust policy" is satisfied here by possession of one `--key`; there is no identity policy to admit or refuse anything. |
| **T3** | *Transparency binding.* A transparency-log entry accepted as evidence for artifact *A* has a body that corresponds to *A*'s attestation. Under **A4**, the existence of the referenced entry is not itself evidence about *A*. | SOUND | E1 — `cmd/strata/verify_rekor_test.go:80,147,217` | REFUTED (2 of 3 live) | Discharged for the verify command by #59, which replaced a log-index existence check that discarded the bundle: `cmd/strata/verify_rekor_test.go:80 TestVerifyRekorEntries_PassesTheBundle`, with `:217 TestVerifyRekorEntries_BadIndexIsNotVerified` and `:147 TestVerifyRekorEntries_UnfetchableBundleIsAFailure` as the failing controls. Refuted on the resolver path: stage 7 holds a bundle *URI*, not bundle bytes, so it cannot compare anything (#85). Two open objections to the *evidence*, not to the proposition: #88 — the `hashedrekord` body shape is inferred from our own `Log`, so writer and verifier can be wrong together, which is rule 3's tautology at the level of a design; #86 — `TestStage7_RekorVerification` asserts a negative case its body never exercises, which is rule 6. |
| **T4** | *Trust-anchor independence.* The root of trust used to verify artifacts is not obtained from the authority that serves those artifacts. Under **A1**, an attacker who can replace an artifact cannot also replace the material that decides who may sign it. | SOUND | none | REFUTED (2 of 2 live) | #62 — the agent fetches its cosign public key from the same S3 bucket that serves the layers, so A1 alone replaces both. #63 — `ec2runner` downloads the cosign binary itself with no checksum or signature check, which is the §1.2 clause "obtaining the verification binary is in scope under A3" paying out. |
| **T5** | *Fail-closed.* Any inability to complete verification — absent tool, absent key, absent bundle, absent log entry, network failure, unparseable material — results in refusal. Degradation to a weaker check occurs only when a weaker policy has been explicitly selected by the operator. | SOUND | E1 — `cmd/strata/run_verify_test.go:264`, `cmd/strata-agent/cosign_verifier_test.go:127,187,249` | REFUTED (2 of 4 live) | The strongest proposition in this document: it names absence alongside invalidity, so A5 cannot slip past it, and it fixes the default in its second sentence, which is what T1 fails to do. Refuted at `internal/agent/agent.go:270` and `:285-293` (#92, #93) — an absent bundle is a skip, not a refusal. Enforced at E1 on the routes already fixed: `cmd/strata/run_verify_test.go:264 TestNewRunVerifier_NeverReturnsANilVerifier` (#55) and `cmd/strata-agent/cosign_verifier_test.go:127 TestResolveVerifier_NilVerifierOnlyWithAnExplicitOptOut`, `:187 TestAllowUnverified_DefaultsToClosed`, `:249 TestProductionPrereqs_ClosedByDefault` (#56). |
| **T6** | *Policy explicitness.* The trust policy under which a result was produced is recorded alongside that result, so that a verification outcome is interpretable without knowledge of the invoking environment. | **ILL-FORMED** | none | REFUTED | "Recorded alongside that result" names neither an artifact nor a lifetime. A line on stderr satisfies it on one reading and no durable record satisfies it on another, so as written it cannot be failed. Falsifiable form: *the lockfile, or an attestation deposited with it, carries a field naming the trust policy in force, and a consumer reading only the artifact can determine what was checked.* Under that form: refuted by absence. `grep -rn 'TrustPolicy\|VerifiedAt' --include='*.go' . \| grep -v _test.go` returns 0 lines, and no lockfile field records what was checked. Filed as #100. |
| **T7** | *Command-name honesty.* A command's name and documented behaviour do not claim a stronger check than it performs. A flag that purports to disable a check disables a check that was otherwise performed. | **TOO WEAK** | E1 — `cmd/strata/run_verify_test.go:350,497` | REFUTED (1 of 4 live) | Broken-but-satisfying implementation: keep the fail-open and fix the *documentation*. T7 constrains names and docs, so the cheapest way to satisfy it is to document the weakness while leaving the command called `verify` and the behaviour unchanged — the operator's attention is displaced exactly as before. Repair: pair T7 with a requirement that a weaker check announces itself **at use time**, not only in prose. `strata run --no-verify` already meets the repaired form (`cmd/strata/run.go:184` prints the warning; `cmd/strata/run_verify_test.go:350 TestVerifyRunLayers_NoVerifyAnnouncesTheSkip` with `:497 TestRunRun_NoVerifyReachesTheMount` as its pair), and so does `STRATA_AGENT_ALLOW_UNVERIFIED` (#56). `strata verify` does not: without `--rekor` it performs presence checks only (`cmd/strata/verify.go:82-98 collectPresenceFailures`) under a name that claims verification — #60. And `verifyBundles`'s doc comment stated the skip as intended design (`internal/agent/agent.go:266-268`) — T7's broken-but-satisfying implementation occurring in the wild rather than as a hypothetical: the weakness was documented accurately and the behaviour left alone, so T7 as written was *satisfied* by the artifact that recorded the fail-open. #104 rewrites the comment and inverts the behaviour together, which is the only combination that discharges anything. |
| **T8** | *(new)* *Freshness.* A verifier can determine that the artifact set it has been given is the current one, and refuses a set that is older than a stated bound. | SOUND | none | REFUTED | Added 2026-08-21 (§7). §5 asked for group T to be checked against TUF's taxonomy; **A6 was in the model and no proposition used it.** Doing the check: nothing records a freshness threshold, an expiry, or a timestamp/snapshot role, so rollback and freeze both succeed against a genuinely signed old artifact, and `strata update` moves forward only when a human runs it. Filed as #101. |
| **T9** | *(new)* *Set integrity.* The layer set is verified as a set, not layer by layer: a verifier refuses a combination of individually valid layers that was never attested together. | SOUND | none | REFUTED (2 of 2 live) | Added 2026-08-21 (§7). This is TUF's mix-and-match attack. The lockfile is the artifact that names the set, and no lockfile-level signature verification exists (#60), so under A1 an attacker composes any set of genuinely signed layers. |

T7 is unusual as a security property and is included deliberately: a verification
step that does nothing is more dangerous than an absent one, because it displaces
the operator's attention. The five fail-open paths found in this codebase were
all instances of a check appearing to exist.

**Coverage against TUF's taxonomy (§5), now performed rather than deferred:**
rollback → T8, new, refuted. Freeze → T8, refuted. Mix-and-match → T9, new,
refuted. Extraneous dependencies → R4, refuted (#67). Endless data → not covered
and not filed: no proposition here bounds the size of a fetched artifact, and
nothing in the codebase does either. Recorded as a gap rather than a proposition
because it is a denial-of-service property and §0 puts availability out of
scope — a scope boundary worth re-examining before this document is cited.

### Group P — Publication and citation

| # | Proposition | Verdict | Basis | Status | Evidence |
|---|-------------|---------|-------|--------|----------|
| **P1** | *Publication precondition.* A persistent identifier is minted only for a lockfile that is fully frozen, contains no mutable layer, and has been verified under a stated trust policy. | **TOO WEAK** | none | REFUTED (3 of 3 live) | Broken-but-satisfying implementation: define `IsFrozen()` as `return true`. P1 defers "fully frozen" to a predicate the implementation owns — the third instance of this pattern, with R2's exception clause and R6's enumeration. And the deferral is not hypothetical: `IsFrozen()` (`spec/lockfile.go:111-118`) inspects layer digests and the base AMI digest and **ignores `Packages` entirely**, so a lockfile that will `conda install` a `latest` version at boot is "fully frozen". Status: `cmd/strata/publish.go:73` checks `IsFrozen()` and nothing else — not `IsSigned()`, not `!HasMutableLayer()`, no trust policy (#66). |
| **P2** | *Independent verifiability.* Given only the published record, a third party with no access to the original registry, no shared secrets, and no contact with the publisher can establish the authenticity of the environment it describes. | SOUND | none | REFUTED | The claim the whole project rests on, and the deposit does not support it. `internal/zenodo/zenodo.go:56-68 Deposit` makes exactly one `uploadFile` call (`:139`), and it deposits **the lockfile YAML and nothing else**. The attestation bundles are named by URI pointing back at the registry (`internal/registry/localclient.go:341` emits `file://…/bundle.json`; the S3 client emits `s3://` URIs), the cosign public key lives in the layer bucket (#62), and no layer bytes are deposited. A third party gets a list of digests and no material to check them against. Filed as #99. |
| **P3** | *Referent stability.* The environment identity attested by a published record cannot subsequently denote different bytes. | SOUND | none | REFUTED | `internal/agent/package_installer.go:115` treats a recorded conda version of `latest` (or empty) as "resolve at boot", and `:101` installs pip packages from PyPI with no hash, so one `EnvironmentID` denotes different bytes on different days. Refuted for the empty adversary; A7 makes it steerable. Same root as I6; filed together as #98. |
| **P4** | *Freeze attainability.* A lockfile produced by ordinary resolution can satisfy the system's own definition of frozen, without manual editing. | SOUND | none | REFUTED (1 of 2 live) | #64 — `strata freeze` structurally cannot succeed because nothing populates `ami_sha256`, and `IsFrozen()` requires `Base.AMISHA256 != ""`. The draft's own note anticipated the consequence and it holds: P1 is presently vacuous, because the precondition P1 guards cannot be reached by ordinary output. |

P4 is a liveness property rather than a safety one, and is included because a
definition of *frozen* that ordinary output cannot satisfy makes P1 vacuous.

### Group B — Build and reproducibility

The literature distinguishes two guarantees that are frequently conflated. Both
are stated here because Strata's documentation makes claims in the vicinity of
each.

| # | Proposition | Verdict | Basis | Status | Evidence |
|---|-------------|---------|-------|--------|----------|
| **B1** | *Artifact identity.* The digest recorded for a built artifact is the digest of the bytes distributed under that identity. | SOUND | E0 — `internal/build/pipeline.go:168,196,212` | ASSERTED (E0) | Holds by construction on the one build path: `internal/build/pipeline.go:168` hashes `sqfsPath`, `:196` signs the same `sqfsPath`, `:212` pushes the same `sqfsPath` — one file object throughout, no re-generation between hash and push. E0 rather than E1 because the only test that could witness it end to end is `internal/build/pipeline_integration_test.go`, which carries a `//go:build` tag and is therefore not run by CI's `go test -race ./...` (`.github/workflows/ci.yml:25`, no `-run` filter, `ubuntu-latest`). Rule 4. Footnote: `pipeline.go:55` writes the literal `"dry-run"` into `manifest.SHA256` on the dry-run path, which is an I3 instance confined to a path that pushes nothing. |
| **B2** | *Input completeness.* Every input consumed by a build is named in the recipe together with a digest, and the build fails if a fetched input does not match. Under **A3**, an unpinned fetch is attacker-controlled. | SOUND | none | REFUTED | #68, confirmed by command: `internal/build/recipe.go`'s `RecipeMeta` has **no source or digest field** at all (`grep -n 'yaml:' internal/build/recipe.go`), and recipes fetch by piping straight into tar — e.g. `cmd/strata/recipes/core/R/4.5.2/build.sh:7`, `curl -fsSL "${URL}" \| tar -xz`. There is nothing to compare against and no place to record it. |
| **B3** | *Build reproducibility.* Re-executing a recipe against its recorded build environment yields a bit-identical artifact. | SOUND | none | REFUTED | Refuted by B2 before any measurement: an unpinned `curl` means the same recipe consumes different bytes over time, so bit-identity cannot hold across the interval in which upstream changes. The claim is nonetheless asserted in the code — `internal/build/recipe.go:277-278`, *"Same recipe + same build environment = same SHA256"* — and the mksquashfs flags that would support it are real (`-mkfs-time 0`, `-all-time 0`). Had B2 held, the status would still be E0: the only test that compares two builds is `internal/build/squashfs_integration_test.go`, build-tagged and therefore unexecuted (rule 4). |
| **B4** | *Environment sufficiency.* The recorded build environment contains enough information to determine the output, given B2. | **ILL-FORMED** | none | UNPOPULATED | No procedure decides "enough information", so as written B4 cannot be failed. Falsifiable form: *two builds of the same recipe on two hosts that differ only in properties the recorded build environment does not name produce bit-identical artifacts.* `UNPOPULATED` because that experiment has not been run and cannot be informative while B2 is refuted — an unpinned source would explain any difference it found. Ordering: B2, then B4. |

B1 is *identity* — these bytes are these bytes. B3 is *reproducibility* — anyone
can obtain these bytes again. Content addressing delivers B1 and says nothing
about B3. A research-citation claim needs B3.

### Group X — Execution

| # | Proposition | Verdict | Basis | Status | Evidence |
|---|-------------|---------|-------|--------|----------|
| **X1** | *Specification completeness.* Every behaviour the profile schema permits an author to specify is either executed by the runtime or rejected at parse time. A field that is accepted, recorded, and never acted upon is a defect. | SOUND | none | REFUTED (3 of 3 live) | Four instances. (a) `OnReady` — parsed at `spec/profile.go:49`, copied into the lockfile at `internal/resolver/stages.go:434`, hashed at `spec/lockfile_hash.go:50`, executed nowhere (#69). (b) `Profile.Instance` and (c) `Profile.Storage` — `grep -rn 'InstanceConfig\|StorageMount' --include='*.go' . \| grep -v _test.go` returns only the declarations at `spec/profile.go:39,42,310,328` and no use; both are parsed, dropped at resolution, and never reach the lockfile. (d) `ResolvedPackageEntry.SHA256` — recorded and hashed into the identity, and never used by the component that installs the package (see I6). `RequiresHost` is a fifth candidate and is exempted only because the schema says so in as many words — *"Advisory only in v0.21.0"* (`spec/lockfile.go:57-60`), which is X1's rule applied honestly rather than a violation of it. (b)–(d) (b) and (c) filed as #102, (d) as #98. |
| **X2** | *Identity captures behaviour.* If a specified behaviour can alter the state of the assembled environment, that behaviour's specification participates in the environment identity — or the identity does not determine the environment. | SOUND | none | REFUTED | Refuted in writing, by design: `spec/lockfile_hash.go:14-15` excludes `MutableLayer` from the hash — *"it is metadata about the build process, not the environment content itself (the upper is not content-addressed)"* — and a writable EBS upper mounted over the stack is exactly a behaviour that alters the assembled environment's state. Second instance: `Packages` *do* participate in the identity and still fail to determine the environment, because the bytes they name are fetched from a moving upstream at boot (I6, P3). The draft's sharp consequence stands and is now concrete. |
| **X3** | *(new)* *Documented-form acceptability.* Every profile form the documentation presents as valid is accepted by the parser, and every artifact the documentation presents as runnable resolves against the shipped catalog. | SOUND | E1 — `spec/softwareref_yaml_test.go:22-24`, `spec/docsnippets_test.go` | REFUTED (1 of 2 live) | Added 2026-08-21 (§7) because two tracker issues refuted nothing in §3 and both were the *converse* of X1. X1 lets a system satisfy it by rejecting everything at parse time; nothing said the documented forms must be accepted. #53 — the inline software-ref form `- python@3.13`, presented in the documentation, did not parse because `SoftwareRef` had no `UnmarshalYAML`; discharged at E1 by `spec/profile.go:237` with `spec/softwareref_yaml_test.go:22-24` and `spec/docsnippets_test.go`, the latter testing the documentation sites themselves. #70 — open, and confirmed live: `examples/alphafold3.yaml:10`, `examples/pytorch-jupyter.yaml:10` and `examples/r-quarto-workstation.yaml:10` name `@2024.03` formations while `cmd/strata/formations/` ships only `@2026.03`, and `go test ./examples/` passes. Four tests parse those files (`examples/examples_test.go:21,48`, `examples/catalog_test.go:19,101`) and none crosses a profile's reference against the catalog — rule 6 in the documentation dimension. |

X2 has a sharp consequence worth stating: a hook permitted to access the network
or mutate the overlay cannot be made deterministic by hashing its command text.
Either such hooks are constrained, or X2 is withdrawn and the identity claim is
narrowed accordingly. `OnReady` is currently hashed and not executed, which
postpones the choice rather than making it.

### 3.1 The pattern across the verdicts

Seven propositions carry a `TOO WEAK` verdict, and **six of the seven fail for
one structural reason**: each defers its content to a definition the
implementation under test controls, so each can be satisfied by weakening that
definition rather than by fixing anything.

| # | The definition it hands over |
|---|------------------------------|
| R2 | the spec's own exception clause — *"except in fields whose specification says they record declaration order"* |
| R3 | what "complete" means |
| R6 | the field enumeration the ID is a function of |
| P1 | `IsFrozen()` |
| T7 | the documentation, which can be corrected instead of the behaviour |
| T1 | "the trust policy in force", with no default fixed |

T1 is the most consequential of the six and is the worked example under rule 9 in
§2.1, which is this pattern promoted from an observation to a prohibition. That a
single sentence in T1 could have absorbed six fail-open paths without ever
reading false is the strongest argument in this document for attacking
proposition text before measuring implementations.

**The seventh, I5, is a different defect and is recorded separately so that
rule 9 is not mistaken for covering it.** I5 does not defer a definition; it
quantifies over the wrong variable. *"Two layers with distinct content never
share a cache location"* is satisfied by keying the cache on a fresh UUID, which
never collides and never hits. The property worth having is the converse — the
location is a *function of* the content — and I5 never states it. The check for
this shape is to ask which direction of the implication the proposition needs,
not whose definition it borrows.

The check to apply when adding a proposition: **name the implementation that
satisfies it and still has the defect.** If one exists, the proposition is
describing the code rather than constraining it. Rule 9 is the commonest reason
such an implementation exists; I5 shows it is not the only one.

---

## 4. Refutation register

Every refutation is recorded here with the proposition it breaks, the adversary
capability it uses, and the artifact tracking its discharge. A refutation is
removed from this register only when its discharge is evidenced at E1 or better.
`H1` in the capability column means no adversary is required (§1.4).

**This table is the source of truth for the Status column in §3.** The
`Discharged` cell must begin with `Yes`, `No` or `Partially` — anything else is a
parse error, not a row that is quietly skipped — and `Partially` counts as live: a
counterexample half-answered still refutes. Adding a row, or moving one to `Yes`,
moves the status of every proposition it names; nothing else does.

| Proposition | Counterexample | Capability | Tracking | Discharged |
|-------------|----------------|------------|----------|-----------|
| T1, T5, T7 | `strata run` performed no signature verification; `--no-verify` disabled nothing | H1 | #55 | Yes — E1, `cmd/strata/run_verify_test.go:264,350,519` |
| T1, T5 | EC2 agent fails open when cosign or the key is unavailable; no test covered it | H1 | #56 | Yes — E1, `cmd/strata-agent/cosign_verifier_test.go:127,187,249` |
| I1′, I2, I3, I4, I5 | `strata run` layer cache: unvalidated path component, unhashed cache hits, empty SHA256 accepted | A2 + A5 | #57 | Yes — E1, `cmd/strata/layer_cache_integrity_test.go:74,123,185,249` |
| T3 | `RekorHTTPClient.VerifyEntry` checked log-index existence and discarded the bundle | A4 | #59 | Yes — E1, `cmd/strata/verify_rekor_test.go:80,147,217` |
| P4 | No profile could resolve offline with the shipped catalog (stage 7 `BUNDLE_MISSING`) | H1 | #54 | Yes — closed completed |
| I6 | pip SHA256 pins were validated against nothing | A3 | #51 | Partially — `strata verify --packages` validates against PyPI out of band; the *install* still ignores the pin (see the I6 row below) |
| T7 | `strata run` did not warn that `packages:` entries are unattested | H1 | #48 | Yes — closed completed |
| T7 | The resolver expanded unattested formations without warning | H1 | #49 | Yes — closed completed |
| X3 | The inline software-ref form `- python@3.13` was documented and did not parse | H1 | #53 | Yes — E1, `spec/softwareref_yaml_test.go:22-24`, `spec/docsnippets_test.go` |
| X3 | All three shipped examples name formation versions the catalog does not contain, and `go test ./examples/` passes | H1 | #70 | No |
| R4, R5 | Stage 4 validates against one provider, stage 6 wires the edge to another; stage 4 is itself first-match | H1 | #67 | No |
| T1, T5 | `verifyBundles` skips when the verifier is nil **and** when a layer names no bundle | A1 + A5; H1 for the nil half | #92 (decision), #93 (fix) | No |
| T1, T5 | `BundleFetcher`'s godoc **specified** the fail-open: implementations were told to return `(nil, nil)` for a layer naming no bundle, and `verifyBundles` treated no bytes as nothing to verify. The shipped `s3LayerFetcher` conforms. A defect in an interface's *specification* is inherited by every conforming implementation, so no amount of testing the implementations finds it | H1 | #92 | No — inverted at both sites on `fix/agent-absent-bundle` (#104) with E1 evidence at `internal/agent/verify_bundles_test.go:291` and `cmd/strata-agent/s3_bundle_contract_test.go:22`, which is not this document's merge target; see §7 |
| T2, T7, T9 | `strata verify` and `IsSigned()` are presence checks; no lockfile verification exists | A1 | #60 | No |
| T4 | Agent fetches its cosign public key from the bucket that serves the layers | A1 | #62 | No |
| T4 | `ec2runner` downloads the cosign binary with no checksum or signature | A3 | #63 | No |
| P4, P1 | `strata freeze` cannot succeed — nothing populates `ami_sha256` | H1 | #64 | No |
| P1 | `strata publish` accepts unsigned lockfiles and dirty mutable layers | H1 | #66 | No |
| B2, B3 | Recipes fetch sources with no digest pinning; the recipe schema has no field for one | A3 | #68 | No |
| X1 | `OnReady` is specified, hashed into the identity, and never executed | H1 | #69 | No |
| I3, I4, I5 | `strata-agent` fetcher builds a cache path from an unvalidated digest; `""` collides on `.sqfs` | A1 + A5 | #81 | No |
| I4 | `trust.VerifyLayers` builds squashfs paths from an unvalidated `layer.ID`; the comment claims `Join` prevents escape | A1 | #58 | No |
| T3 | Stage 7 holds a bundle URI, not bundle bytes, so it cannot Rekor-verify | A4 | #85 | No |
| T3 | `hashedrekord` body shape inferred from our own `Log` — writer and verifier can be wrong together | A4 | #88 | No |
| R3 | A null entry in a software list is silently dropped and the profile resolves without it | H1 | #79 | No |
| R2, R7 | `EnvironmentID` depends on YAML slice order when `MountOrder` ties, and on `Packages` order always | H1 | #95 | No |
| I3, P1 | `IsFrozen()`/`EnvironmentID()` accept any non-empty string as a digest | A5 | #96 | No |
| I4 | `overlay/mount_linux.go:141` and `export/oci.go:60` build paths from an unvalidated `layer.ID` | A1 | #97 | No |
| I6, P3, X1, X2 | `packages:` installs ignore the recorded SHA256; conda `latest` resolves at boot — so `Packages` participate in the identity and still fail to determine the environment | A3, A7, H1 | #98 | No |
| P2 | The Zenodo deposit contains the lockfile only — no bundles, no key, no layers | H1 | #99 | No |
| T6 | Nothing records the trust policy a result was produced under | H1 | #100 | No |
| T8, T9 | No freshness bound and no set-level attestation: rollback, freeze and mix-and-match all succeed | A6, A1 | #101 | No |
| X1, R3 | `Profile.Instance` and `Profile.Storage` are parsed and referenced nowhere else | H1 | #102 | No |

### 4.1 Tracker issues that refute no proposition

Required by the review standard: an issue that refutes nothing is either not
about a claimed property, or evidence that a proposition is missing. Both are
findings, so both are stated.

**Not about a claimed property — construction and hygiene.** There are 57 closed
non-PR issues. Of those, 47 are feature or construction work — 15 `feat:`-prefixed
plus 32 untagged issues from the original build-out ("Design and implement …",
the tier recipe catalogs, `strata build`, `strata fold`, LMod integration, and so
on). They build the subject of the propositions rather than assert anything about
it. The remaining 10 are `fix:`-prefixed and every one appears in the register
above, except #77 (`file://` registries rejected by four commands) which is a
feature-completeness defect.

```
gh api 'repos/scttfrdmn/strata/issues?state=closed&per_page=100' --paginate \
  --jq '.[] | select(.pull_request == null) | .title' | sed 's/:.*//' \
  | sort | uniq -c | sort -rn
```

Open issues in the same category: #15 (a future tool), #46 (catalog refresh),
#71, #72 (documentation truth-passes), #73 (the standing contract issue), #74,
#75, #76, #89 (tooling, packaging and tracker hygiene). #83 is a *design
question* about I2's cost, not a refutation of it. Re-derive with the same
command against `state=open`.

**Refutes no proposition, so a proposition was missing.** #53 and #70 fell
through every group in §3. Both are the same shape — a form the documentation
presents as valid that the system does not accept — and that shape is the
converse of X1, which no proposition stated. **X3 was added because these two
issues had nowhere to go.** This is the mechanism in §1c working as intended, and
it is the strongest argument in this document for keeping the register complete
rather than only recording the issues that already have a home.

**Refutes no proposition, and that is a finding about the propositions.** Three
open issues are about *the evidence*, which no proposition in §3 constrains
because §2 constrains it instead:

- #84 — `internal/registry FetchLayerSqfs` has no test that can observe the #57
  tightening: `localclient` derives the digest from the file (rule 3), and
  `s3client` is uncovered (rule 6). It refutes the E1 claim on I2's second and
  third sites, not I2.
- #86 — `TestStage7_RekorVerification` asserts a negative case its body never
  exercises. Rule 6.
- #94 — `cmd/strata-agent main()` is unreachable from any test, so the fatality
  of the #56 refusal is held by reading rather than by execution. Rule 4.

These belong in the register as *tier corrections*, and §2 has no mechanism for
recording that a tier was once claimed too high. That mechanism is missing;
recorded here rather than invented.

- #65 — no `LockFile.Validate()` exists and `strata run` bypasses the spec
  parser. This refutes no single proposition and is the *common cause* of I3,
  I4 and R3: there is no ingestion point at which any of them could be enforced
  once. A proposition covering it would be about system structure rather than
  behaviour, which this document has no group for. Left as a gap.
- #61 — `pkg/strata Resolve()` documents bundle-payload verification the
  resolver does not perform. This is T7's shape applied to a library API rather
  than a command name; T7 says "a command's name", so #61 falls outside it by a
  word. T7 should say "a command or exported API". Amendment deferred rather
  than made silently, because widening T7 while it is already `TOO WEAK` would
  compound one defect with another.

---

## 5. Relation to prior work

Strata's propositions are not novel individually; the contribution claimed is
their composition into a single artifact that is both runnable and citable. This
section positions each group against existing work so that a reader can see what
is inherited and what is asserted anew.

**Every citation below was verified against a primary source on 2026-08-21.**
Author lists, venues, years and pages were checked; two were wrong and are
corrected in place, with the corrections listed in §7. Nothing was softened —
one reference was reframed because it is not a paper.

- **Update security and attack taxonomy.** The Update Framework (TUF) — Justin
  Samuel, Nick Mathewson, Justin Cappos, Roger Dingledine, *Survivable Key
  Compromise in Software Update Systems*, CCS '10, pp. 61–72,
  [doi:10.1145/1866307.1866315](https://doi.org/10.1145/1866307.1866315) —
  enumerates rollback, freeze, mix-and-match, endless-data, and
  extraneous-dependency attacks. The coverage check §5 asked for has now been
  performed; see the note following group T. It produced two new propositions
  (T8, T9), both refuted on arrival, and one gap left unfiled (endless data).
- **Supply-chain layout integrity.** in-toto — Santiago Torres-Arias, Hammad
  **Afzali**, Trishank Karthik Kuppusamy, Reza Curtmola, Justin Cappos,
  *in-toto: Providing farm-to-table guarantees for bits and bytes*, 28th USENIX
  Security Symposium, 2019, pp. 1393–1410 — addresses the property that a
  delivered artifact is the output of an intended chain of steps. Group B is
  weaker than in-toto's link metadata in a specific way worth stating: Strata
  records *what* was built (`RecipeSHA256`, `BuiltWith`) but attests no chain of
  custody between steps, so B1 is an identity claim about one artifact rather
  than a claim about the process that produced it.
- **Transparency.** Certificate Transparency — Ben Laurie, Adam Langley, Emilia
  Kasper, RFC 6962, June 2013 (Experimental), **obsoleted by RFC 9162,
  *Certificate Transparency Version 2.0*, 2021** — establishes the
  inclusion-proof model that T3 depends on. Sigstore — Zachary Newman, John
  Speed Meyers, Santiago Torres-Arias, *Sigstore: Software Signing for
  Everybody*, CCS '22,
  [doi:10.1145/3548606.3560596](https://doi.org/10.1145/3548606.3560596) — is
  the concrete instantiation used here. Exactly three authors; the draft's
  "et al." implied more and is removed.
- **Reproducible builds.** Chris Lamb and Stefano Zacchiroli, *Reproducible
  Builds: Increasing the Integrity of Software Supply Chains*, IEEE Software
  39(2), March–April 2022, pp. 62–70,
  [doi:10.1109/MS.2021.3073045](https://doi.org/10.1109/MS.2021.3073045) — the
  canonical statement of the B1/B3 distinction.
- **Functional deployment.** Eelco Dolstra, *The Purely Functional Software
  Deployment Model*, PhD thesis, Utrecht University, 2006, and the Nix and Guix
  systems descending from it. R1, R6 and B3 are close relatives of the
  purely-functional derivation model. The difference is precisely where this
  review found its refutations: a Nix derivation's inputs are *all* content-
  addressed, whereas Strata admits conventional package-manager installs whose
  bytes are named but not hashed at install time (I6). R6 is a derivation hash
  computed over a field list rather than over a closure, which is what makes the
  enumeration attack in R6's verdict possible at all.
- **Assurance levels.** SLSA (Supply-chain Levels for Software Artifacts), an
  OpenSSF project — <https://slsa.dev>. Not a paper: a living specification,
  v1.0 released April 2023. Its version is deliberately not pinned here; per §6
  a fact that must reflect the present carries the command that re-derives it,
  and for a web specification that is the URL. Mapping Strata's propositions
  onto its build-track levels would let a reader place the system without
  reading this document, and is not attempted here.

---

## 6. Maintenance

This document is a **plan of record with dated amendments**, not a live status
page. It states propositions and their evidence as of dated entries. Anything
that must reflect the present carries the command that re-derives it.

**The Status column is generated. Do not edit it.** Status is a function of the
§4 register and the authored Basis cell, and the function lives in
`internal/propdoc.DeriveStatus` rather than in a maintainer's head:

```
go run ./cmd/propgen           # report any Status cell that disagrees; exit 1
go run ./cmd/propgen -write    # regenerate the column
```

`internal/propdoc.TestPropertiesStatusColumnIsDerived` runs the same check in CI
(`.github/workflows/ci.yml:25` runs `./...` with no `-run` filter), so a drifting
document fails the build whether or not anyone runs the command. To move a
status, move a register row: add one, or change its `Discharged` cell to `Yes`
with the evidence. **Editing a Status cell directly is now a build failure rather
than a divergence nobody notices**, which is the point — the first population of
this document maintained status beside the register, and the two disagreed within
one PR.

A change to a proposition's text is an amendment with a date and a reason, not a
silent edit: a proposition that quietly narrows until it is satisfied is not
evidence of progress. The verdict column exists to make that visible — a
proposition whose verdict moves from `SOUND` to nothing while its status moves to
`ENFORCED` has been narrowed, not satisfied.

**Obligation on any change that moves a proposition.** A pull request that
changes behaviour states which propositions it moves and from what to what, in
the form *"T5 moves from REFUTED to ENFORCED at E1; T1 remains REFUTED pending
the agent-side counterexample."* A change that moves no proposition says so.

---

## 7. Amendment log

### 2026-08-21 — first population, and the review that preceded it

The draft was attacked before it was populated. Where the review disagreed with
the draft, the change is recorded here rather than made silently.

**Propositions restated or added:**

1. **I1 split.** The draft's I1 quantified over *every* byte in an assembled
   environment. Satisfying it would require abandoning `packages:` and Path B,
   both deliberate. Restricted to I1′ (layer-derived bytes) and the remainder
   given its own proposition, **I6**, which is refuted.
2. **R7 added.** R6 and X2 constrain only the direction behaviour → identity.
   Nothing forbade the identity making distinctions no behaviour justifies, and
   `OnReady` is that case today.
3. **T8 and T9 added.** §5 asked for group T to be checked against TUF's
   taxonomy and the check had not been done. A6 was in the adversary model with
   no proposition using it. Both new propositions are refuted on arrival, which
   is the point of doing the check.
4. **X3 added — documented-form acceptability.** A spurious-distinction property
   was first drafted in group X and moved to R7 instead, since it is about
   identity rather than execution. X3's slot then went to a genuinely new
   proposition, arrived at from the opposite direction: #53 and #70 refuted
   nothing in §3, and both were X1's converse. X1 is satisfiable by rejecting
   every documented form at parse time. X3 closes that.

**Verdicts recorded against draft text, not silently repaired:** R1 and I1 `TOO
STRONG`; R2, R3, R6, I5, T1, T7, P1 `TOO WEAK`; T6, B4 `ILL-FORMED`. The
remaining 23 are `SOUND`. Re-derive the totals with

```
go run ./cmd/propgen
```

which reports the proposition and register counts and fails on any status that
the register does not imply. The propositions are left as written with the
verdict beside them, because rewriting a proposition to match what the code does
is the failure §6 exists to prevent. §3.1 records the structural pattern six of
the seven `TOO WEAK` verdicts share, and why I5 is not one of them.

**Adversary model:**

5. **A7 added** — upstream mutation of an unpinned dependency, distinct from A3
   because it needs no interposition. The conda `latest` path admits it.
6. **A2's rationale strengthened** with the `os.TempDir()` cache fallback, which
   instantiates the shared-path premise the capability asserted abstractly.
7. **§1.2 narrowed.** "Compromise of … the mounting process" excluded, on a
   hostile reading, the entire subject of group T. It now excludes compromise of
   the mounting process at runtime and states explicitly that *defects* in it
   are in scope.
8. **§1.4 added — hazard class H1.** Two of the five fail-opens need no
   adversary. The model had no way to express "the default is weaker than the
   operator believes", and inventing a capability for it would have
   misdescribed it as an attack. H1 is named as a non-adversarial class, and
   group T is read as quantifying over the default configuration.
9. **§1.3's A1+A5 nomination** is recorded as verified rather than predicted
   (#92, probe 3).

**Evidence standard:**

10. **Rule 6 added** — a test that does not execute the property's site is not
    E1 for that property. Rule 4 asks whether the test runs; rule 6 asks whether
    the line runs. The motivating measurement is on #93: seven tests green, zero
    coverage hits on the changed statements.
11. **Rule 7 added** — a false documented claim is a refutation, not E0. E0 as
    drafted rated `internal/trust/verify.go:90-91` the same as an honest
    silence.
12. **Rule 8 added** — every status is dated.
13. **E0's definition widened** to cover a property established by the structure
    of the code with no executed test, which is B1's exact situation.
14. **E2 recorded as currently unreachable**, with the command, so the tier is
    not mistaken for available.
15. **A gap left open**: §2 has no mechanism for recording that a tier was once
    claimed too high. #84, #86 and #94 are corrections of that kind and §4.1
    holds them provisionally.

**Bibliography (§5) — verified against primary sources; two errors found:**

16. in-toto: **"Afzal" → "Afzali"** (Hammad Afzali). Verified against the USENIX
    proceedings page and the author's own publication list. Full title, venue
    and pages added.
17. Sigstore: **"et al." removed** — the paper has exactly three authors
    (Newman, Meyers, Torres-Arias). Verified against the ACM DL record.
18. RFC 6962 recorded as **obsoleted by RFC 9162** (CT v2.0). The draft cited
    the superseded document with no note.
19. TUF, Lamb & Zacchiroli, and Dolstra verified correct as recalled; pages,
    volume, DOIs and the full Lamb & Zacchiroli subtitle added.
20. SLSA reframed: it is a living specification, not a paper. Cited by URL with
    the v1.0 date, and its current version deliberately not pinned.
21. Nothing was removed. The standard was "a citation that cannot be verified is
    removed, not softened"; all six were verifiable.

**Self-reference, and one claim this document falsified by existing:**

22. The draft of I6's evidence cell stated `grep -rn 'require-hashes' .` → no
    output. By the time the claim was on disk the command returned one hit: the
    sentence itself. The citation now carries `--exclude=PROPERTIES.md`, and the
    same exclusion is applied to every grep in this document whose evidence is an
    *absence*. Recorded rather than quietly fixed, because it is a live instance
    of the failure mode this document is supposed to guard: a measurement
    invalidated by the act of writing it down, where the instrument and the
    subject are the same tree. Any future population of §3 must re-run its
    absence-greps with the exclusion, or it will read its own prose as evidence
    that the code contains what the prose says is missing.

**Corrections made between drafting and commit.** Nine `path:line` citations were
generated from earlier reading and then checked against the file before this
document was committed; six were wrong and are corrected in place:
`agent.go:171-173` → `:172`; `agent.go:225-241` → `:231-241`;
`stages.go:293` → `:301`; `lockfile.go:110-118` → `:111-118`;
`package_installer.go:113-117`/`:98-106` → `:115`/`:101`;
`zenodo.go:138-145` → `:56-68` with `uploadFile` at `:139`. Two grep commands
quoted in evidence cells were replaced with the commands actually executed, which
are narrower than the ones written from memory. The corrections changed no
verdict and no status. Recorded because a `path:line` that drifts is
indistinguishable from a fabricated one to a reader checking it later.

### 2026-08-21 — status made derivable, and the review of the first population

The first population was reviewed before it merged. Three of its results were
judged to outrank the fixes they came from, and one of its mechanisms was judged
structurally wrong. Both are recorded here.

**The document defect, and the ruling on it:**

23. **The Status column is now generated, not maintained.** As first written,
    status was authored beside the register, so the two were two sources of truth
    for one fact. The defect was demonstrated within a single PR: #104 discharged
    one of T5's two counterexamples at E1, and because T5 correctly stayed
    `REFUTED` under proof-standard rule 5, the Status column showed **no change at
    all** — the entire movement lived in prose inside an evidence cell where
    nothing could check it. Status is now a function of the register and an
    authored Basis cell, the function is written down in
    `internal/propdoc.DeriveStatus`, and `cmd/propgen` regenerates the column.
    §6 carries the commands.

    Two properties follow that hand-maintenance could not give. A proposition can
    no longer be `REFUTED` in prose without a register row — the derivation would
    render it `UNPOPULATED` and CI would fail. And discharging one of several
    counterexamples is now visible in the Status column as `REFUTED (n of m
    live)`, which is what the review asked for.

24. **A `Basis` column was split out of `Status`.** The authored input is now the
    highest evidence tier claimed plus its citation; a tier with no citation is
    rejected by the parser rather than read as enforcement. Statuses that
    formerly carried two values — `ENFORCED E1 (layers) / REFUTED (lockfiles)` —
    resolve to `REFUTED` with the E1 citation still visible in Basis, because
    rule 5 says a refutation outranks a tier and a column that reports both
    reports neither.

25. **Five statuses changed with no change to the implementation**, which §6
    requires be stated rather than absorbed: R1 `REFUTED (as written) / ENFORCED
    E1` → `ENFORCED E1` (status measures the repaired form; that the as-written
    form is unsatisfiable is the `TOO STRONG` verdict, not a measurement); I1 →
    `WITHDRAWN (superseded by I1′, I6)`; T2, T3 and T7 → `REFUTED` with their E1
    citations moved to Basis. The distribution over 34 propositions is now
    27 `REFUTED`, 4 `ENFORCED E1`, 1 `ASSERTED (E0)`, 1 `UNPOPULATED`, 1
    `WITHDRAWN`. No proposition's text moved.

26. **The tool found a hand-derivation error on its first run.** I4's status was
    written `REFUTED (2 of 3 live)`; the register holds **four** rows naming I4
    (#57, #58, #81, #97) and `propgen` reported `REFUTED (3 of 4 live)`. The #81
    row was missed by eye. This is the same class of error the column now
    prevents, found by the thing that prevents it.

27. **X2 had a `REFUTED` status and no register row.** Its counterexample — the
    `Packages` half, where fields participate in the identity and still fail to
    determine the environment — is tracked by #98, and X2 was added to that row.
    Found while enumerating the register by hand; the derivation requires it
    independently.

**Proof standard:**

28. **Rule 9 added — a proposition may not defer its content to a definition the
    implementation under test controls.** This was the review's second result:
    the `TOO WEAK` verdicts are not seven findings but one rule, with T1 as the
    clean demonstration. Promoted from §3.1's observation to a rule so that a new
    proposition is tested against it *before* it is written down. T1 is the
    worked example.

29. **§3.1 corrected.** It said four propositions shared the deferral pattern and
    T1 was the fifth. Recounted against the verdicts: **six** of the seven
    `TOO WEAK` propositions defer a definition (R2, R3, R6, P1, T7, T1) and the
    seventh, **I5, does not** — it quantifies over the wrong variable, and the
    property worth having is the converse it never states. Recorded because the
    review's summary of this document asserted all seven shared one shape, and
    they do not; rule 9 would otherwise be read as covering a case it does not
    reach.

**The account of the findings:**

30. **§0.2 added — H1 is the principal finding.** The register is not a list of
    attacks. Seventeen of thirty-three refutations exercise no adversary
    capability at all, and six of the fourteen rows refuting a group T
    proposition are in that class. The honest account is that the defaults do not
    verify and the propositions were written to be conditional on configuration
    nobody sets — a claim about the system's posture rather than a bug list.
    **Correction to a count stated in review:** that discussion said "20 of 32"
    register rows were H1. It is not reproducible under either reading — at the
    first population the register held 32 rows, of which 18 mention H1 and 16
    exercise no adversary capability. The command is now in §0.2 and the number
    is derived rather than recalled.

31. **The `BundleFetcher` godoc has its own register row, capability H1**, and
    §1.4 now describes **six** fail-open paths rather than five. This was the
    review's third result: the godoc *instructed* implementations to return
    `(nil, nil)` for a layer naming no bundle, so the fail-open was specified
    rather than committed. Every conforming implementation inherits it, the
    shipped `s3LayerFetcher` does, and testing implementations against their
    documented contract cannot find a defect that *is* the contract. It was found
    by measuring the change, not by reading the code, and after #92 had already
    enumerated the fail-opens and settled on two.

**Merge order (contract item 0e), enumerated rather than absorbed:**

32. This document and #104 touch no common line, so nothing will report a
    conflict, and merge order alone decides whether the following citations name
    the tree they were measured against. Every measurement here is pinned to the
    commit in the provenance header, so nothing below is *false* — it is
    stale-on-merge, which is a different defect with a different fix.

    If #103 merges first, then on #104 merging: `internal/agent/agent.go:270` →
    `:285` in §1.4 and T1's and T5's evidence cells; `:285-293` in §1.3, T1 and
    T5 no longer exists and is replaced by the refusal at `:314-325`; `:266-268`
    in T7 becomes `:272-283`; and the `BundleFetcher` register row moves to
    `Yes — E1` with `internal/agent/verify_bundles_test.go:291` and
    `cmd/strata-agent/s3_bundle_contract_test.go:22`. The `#92 (decision), #93
    (fix)` row stays `No`: #93's nil-verifier fail-open is untouched. If #104
    merges first, the same list applies to this document before it lands.

**Placement.** This file sits at the repository root beside `STRATA.md` rather
than under `docs/`. `CLAUDE.md` hygiene rule 1 sends documentation to `docs/`;
the judgment here is that a specification of the system's claimed properties is
a peer of `STRATA.md`, which is also at the root. Recorded as a decision rather
than assumed, because rule 1 also forbids standalone tracking files and §0.1
exists to explain why this is not one.

# Strata — Security and Correctness Properties

**Status: first population, 2026-08-21; amended the same day after review.**
Every proposition below carries a verdict from an adversarial review of the
proposition *text*, an authored `Basis`, and a **generated** `Status` — see §3
and §6, and do not edit a Status cell by hand. The bibliography in §5 has been
verified against primary sources. Of the 34 propositions, 27 are `REFUTED`, 2
are `ENFORCED E1`, 1 is `ASSERTED (E0)`, 2 are `UNPOPULATED` and 2 are
`WITHDRAWN`; that distribution is derived, not counted by eye —
`go run ./cmd/propgen` prints the totals it was taken from. The principal
finding is §0.2.

> **The distribution is not a progress measure, and must not be quoted as one.**
> It counts propositions by status, and a proposition already `REFUTED` stays
> `REFUTED` whether it has one live counterexample or four. Demonstrated, not
> hypothesised: on 2026-08-21 two rows moved from discharged to live — `P4` from
> `(1 of 2 live)` to `(3 of 3 live)`, `X3` to `(2 of 3 live)` — and **every number
> in the paragraph above stayed the same.** Movement lives in the `(n of m live)`
> counts and in §4, not in the totals. A reader watching the distribution for
> change would have seen a clean bill on the day the register got worse.
>
> The converse also holds, demonstrated the same day: restating `R1`, `R2` and
> `R6` truthfully (§7) moved `ENFORCED E1` from 4 to 2 and moved `REFUTED` **not
> at all**. Nothing about the code changed. A falling `ENFORCED` count here means
> propositions stopped claiming more than their evidence supports, which is the
> instrument working — so the totals cannot be read as a direction of travel in
> either direction.
>
> **This is a property of the generator, not a caveat about four occasions.** It
> has now been rediscovered four times — §7 items 46, 55 and 62, and item 79 on
> 2026-08-22, where `T7` moved from `REFUTED (1 of 4 live)` to
> `REFUTED (3 of 4 live)` and every number above held — so it is stated once here
> as what it is. `Distribution` tallies `Kind(status)`
> (`internal/propdoc/propdoc.go:339`) and `Kind` discards the parenthesised detail
> by construction (`:348`), so a movement expressible only in an `(n of m live)`
> count **cannot** appear in these totals. No future amendment demonstrates this
> again; a movement that leaves the totals unchanged is the expected reading, and
> the number to check is the cell.

Provenance for every measurement in this document unless stated otherwise:

| | |
|---|---|
| measured at | `339329fb08f2876c5d08405d25f40540f1609268`, tree `457cd8e5350336aa2fe197eed388162f267b10a2` |
| refreshed for | #104, which moves `internal/agent/agent.go` — see §7 items 33–36 |
| citations into files #104 changes | updated where they support a **live** property, pinned in the past tense to `339329fb` where they are the evidence for a **discharged** refutation |
| re-derive | `git rev-parse HEAD; git rev-parse HEAD^{tree}; git status --porcelain`, and `go run ./cmd/propgen` for the Status column |

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

**Twenty of thirty-six refutations need no adversary at all.** They are
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
**at `339329fb`** dropped any layer whose `Bundle` field was empty from the set to
be verified, and an empty set returned success — so A1 (write the manifest) plus
A5 (omit the bundle field) produced a clean boot with a fully configured verifier
that was never consulted. The probe is on #92.

**Closed by #104.** That block no longer exists; `internal/agent/agent.go:310-323`
now collects every bundle-less layer and refuses, naming all of them. The citation
above is retained in the past tense and pinned to the commit where the measurement
was made, because a refutation's evidence is the tree it was taken on — renumbering
it to a line that now refuses would make the counterexample unreproducible. §7
records the refresh.

### 1.4 What the model cannot express — and the class that fills the gap

**Five of the six fail-open paths found in this codebase need no adversary at
all.** `internal/agent/agent.go:285` skips verification when the caller supplied
no verifier — still open, tracked by #93; the historical `strata run --no-verify`
disabled a check that was never performed (#55). No capability in §1.1 describes "the deployment's default
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

Each proposition carries a **basis**, and a basis is a **pair, not an ordinal**:
how much of a stated domain the evidence reached, and what the evidence was
*about*. The two vary independently, so no single number holds both.

**Coverage — how much of the declared domain the evidence reached.**

| Coverage | What it establishes |
|----------|---------------------|
| **`asserted`** | Nothing was executed. The property is stated in documentation, or follows from the structure of the code. An honest basis, not a failure — but it must be labelled. |
| **`chosen`** | Example tests exercise the property on inputs the author picked, and the site the property is about is executed by them. Establishes the property *for those inputs*. |
| **`sampled`** | A generated, property-based or metamorphic run exercises the property over inputs drawn from a declared domain. Establishes it over what was drawn, which is not the domain. |
| **`exhaustive`** | Every member of a declared, bounded domain was exercised. Establishes the property over that domain with nothing left to sample. |

**Subject — what the evidence was about.**

| Subject | What it establishes |
|---------|---------------------|
| **`implementation`** | The evidence executed the code that ships. |
| **`model`** | The evidence executed an abstraction of it. What holds of the model holds of the code only so far as a *separately cited* faithfulness argument carries it — rule 1. |

Coverage is totally ordered: `asserted` < `chosen` < `sampled` < `exhaustive`.
Subject is not an ordering at all. The pair is what resists a total order, and
"E1 or better" is therefore a well-formed phrase only about coverage.

### The seven bases, and the four the old ladder named

`asserted` takes no subject. Where nothing was executed there is no artifact the
evidence was *about*, so `asserted/implementation` and `asserted/model` would
distinguish nothing measurable. The grid is 1 + 3 × 2 = **seven** cells, not
4 × 2 = eight — a reader who counts the dimensions will expect eight, so the
collapse is stated rather than left to be noticed.

| Spelling in a Basis cell | Coverage | Subject | Legacy name |
|---|---|---|---|
| `asserted` | `asserted` | — | **E0** |
| `chosen/model` | `chosen` | `model` | *none* |
| `chosen/implementation` | `chosen` | `implementation` | **E1** |
| `sampled/model` | `sampled` | `model` | *none* |
| `sampled/implementation` | `sampled` | `implementation` | **E2** |
| `exhaustive/model` | `exhaustive` | `model` | **E3** |
| `exhaustive/implementation` | `exhaustive` | `implementation` | *none* |

Both spellings are accepted in a Basis cell. Where a legacy name exists it is the
canonical one, so `chosen/implementation` in a cell derives the same Status as
`E1` and the Status column stays uniform; `internal/propdoc` normalises at parse
and `internal/propdoc/basis_pair_test.go` checks this table against the parser's
registry in **both** directions, so a spelling the document defines and the tool
rejects — or accepts and the document never defines — fails the build.

**Why the ladder was one-dimensional and the evidence isn't.** E0 < E1 < E2 < E3
ranked by *technique* — did you use a generator, did you build a model — where
what matters is *what was established*. Traced onto the grid, the old ladder is
one path that switches subject at its top step: `asserted` →
`chosen/implementation` → `sampled/implementation` → `exhaustive/`**`model`**.
Three consequences, all of which bit:

- **Three of the seven bases had no rung**, so evidence of those kinds had to be
  filed as something it wasn't.
- **The strongest of the three had to file below a model check.** #129 enumerated
  all 112 cells of the four-dimensional (`Verifier` × bundle bytes × names-a-bundle
  × digest) domain of `internal/agent.verifyBundles` — an `exhaustive/implementation`
  result — and found four blocks that no test in the module had ever executed, one
  of them `Verifier.Verify` returning an error, at 81.3% package coverage with three
  CI jobs green. The ladder's top rung had already spent itself on a change of
  subject, so the only tiers left for that result were E1 and E2: *beneath* an
  exhaustive walk of an abstraction.
- **The perverse ordering, which is the tell:** an exhaustive walk of a declared
  domain filed below a sampled walk of an undeclared one.

### What is reachable here today

```
$ grep -rn '^func Fuzz' --include='*_test.go' .
spec/environment_id_r7_fuzz_test.go:445:func FuzzR7NoSpuriousDistinctions(f *testing.F) {
$ grep -rn 'testing/quick\|gopter\|pgregory\|rapid' --include='*.go' .   # no output
```

`sampled/implementation` is reachable: a fuzz target exists, without a
property-testing library. The paragraph this replaces said the opposite — *"there
are no fuzz targets … no proposition may claim E2 until one exists"* — and it
was false from the day that target merged, which is the same relocated-rule
failure it was written to prevent: the section asserting a technique's absence is
a site that the technique's arrival has to update (§7).

No proposition claims `sampled/implementation` yet. R7's basis is `none` despite
that target's 46 million executions, for the reason rule 2's second clause now
states as a rule.

### Ranking, where a worklist needs one

Order by coverage, breaking ties by subject `model` < `implementation`. Over the
seven that is a total order. It is a **sorting convention, not an evidence
comparison**, and these are the places where it ranks two bases the evidence does
not:

1. **`sampled/implementation` vs `exhaustive/model`** — legacy E2 vs E3. Sampling
   the code that ships, against exhausting an abstraction of it. Neither
   dominates. The order puts `exhaustive/model` higher; nothing establishes that.
2. **`exhaustive/implementation` vs `exhaustive/model`** — strictly stronger on
   faithfulness, and **incomparable on domain size**: a bounded model state space
   can be orders of magnitude larger than any implementation domain that can be
   enumerated. The order ranks it higher, which is right on the first count and
   unwarranted on the second. This pair is why no single number could hold #129.
3. **Any two bases with different declared bounds.** The pair says *how* a domain
   was covered; only the bound says *which* domain. Comparing the Basis cells of
   two propositions whose bounds are not stated and comparable compares nothing.
4. **`chosen/model` and `sampled/model` against `asserted`.** The order puts them
   higher because something ran. Without a faithfulness argument they establish
   nothing about the code, and `asserted` at least does not imply otherwise.

**A column that cannot be totally ordered is more useful than one that can be and
lies about it.**

**Something now consumes part of this ordering, and the guard promised here has
come due** (2026-08-22, #135; §7 item 80). The paragraph that stood here said
nothing in the repository consumed the ranking, that the list above was therefore
a note and notes do not fire, and that the first consumer would bring the list into
`internal/propdoc` with a guard making it non-vacuous. `propdoc.Reduce` is that
consumer, and the guard is
`internal/propdoc/basis_scope_test.go:192 TestReduceRefusesSection2sIncomparablePairs`.
What it consumes is narrower than "sorting by basis", and the difference is the
whole reason the four cases above survive intact:

- **`Reduce` consumes the order on `Coverage` alone** — `BasisKind.Rank`,
  `internal/propdoc/basis.go:102` — which §2 states is total and means it. It never
  consults the tie-break by subject, because it refuses to compare two bases whose
  subjects differ instead of ranking them.
- **It computes a meet, not a sort.** A floor over a set of scopes is not a ranking
  of one scope against another: it reports the weakest thing true anywhere in the
  union, which is well defined even where the scopes cover different domains.
- **Items 1 and 2 are refused**, and the guard asserts the refusal names both bases
  rather than discarding one. **Item 4 is not refused**, and the guard states why in
  the narrowest terms available: `asserted` is the bottom of the *coverage* order, so
  a floor over coverage is defined for it. Item 4's complaint is about *usefulness* —
  whether something that ran but was not about the code beats a bare assertion — and
  `Reduce` does not compute a usefulness ranking and does not claim to.
- **Item 3 is not expressible as a pair of bases at all.** It says that comparing the
  Basis cells of two propositions whose bounds differ compares nothing, which is a
  statement about two *cells*. `Reduce`'s answer to it is structural rather than a
  verdict: every entry carries its own scope, and a multi-entry cell that declines to
  name them is rejected
  (`internal/propdoc/basis_scope_test.go:364 TestParseBasisRejectsUnscopedMultiEntryCells`).

Still true, and still a note: **no tool sorts propositions by basis.** Nothing
ranks one proposition against another, and the seven-way order remains
unconsumed — a triage script, a dashboard or a release gate would be the first to
consume *that*, and would owe the same guard for the part it used.

### 2.1 Rules

1. **A `model` subject does not transfer to the implementation, and a faithfulness
   argument does not rewrite the subject.** A model check proves a property of the
   model. The claim that the model represents the code is a separate claim, made
   separately, with its own evidence and its own basis, cited on the row beside
   the model result. This is what the rule reached for when it read *"E3 without a
   faithfulness argument is E0"*, and the pair states it better in both
   directions. **What it now permits:** recording an unargued model check honestly
   as `exhaustive/model` — a real result about a real artifact — rather than
   erasing it to `asserted`. **What it now forbids:** reading a model check *with*
   a faithfulness argument as evidence about the implementation. The old wording
   implied the argument promoted the tier; it does not, because there is no rung
   to promote it to. Subject is not an ordering.
2. **The declared bound is part of the claim, in both dimensions.** For coverage
   the bound is the domain: "holds for all profiles" is not established by a
   generator that only emits single-layer profiles, nor by an exhaustive walk of a
   domain the walk itself declared. For subject the bound is what the model
   abstracts away. State both; a property established over a stated bound is a
   real result, and a bound that is not stated makes the coverage `asserted`
   whatever ran.

   **And a bound no member of which can satisfy the property's premise makes the
   coverage `asserted` too, whatever ran.** R7 — *two lockfiles that assemble the
   same environment have equal `EnvironmentID`* — was fuzzed for 46,546,540
   executions, green, over a domain in which no two *distinct* lockfiles can
   assemble the same environment at all: `internal/overlay` marshals the whole
   lockfile into `/etc/strata/active.lock.yaml` inside the assembled root, so the
   premise holds only for byte-identical pairs and the property is vacuously true.
   An empty search space and a clean search are indistinguishable in every number
   the run reports — exec count, corpus reach and transformation counters were all
   green. So a `sampled` or `exhaustive` claim states not only its bound but that
   the bound is **satisfiable** by something the generator can produce. For an
   implication-shaped property that question is distinct from "did the
   transformation fire" and from "was it non-identity". (Added 2026-08-21; see §7.)
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
   `internal/agent/agent.go:285-293` **at `339329fb`** left all seven tests in the
   package green with zero coverage hits on the inverted statements (measurement on
   #93): the suite was green and the change unexecuted. #104 both deletes that
   block and supplies the missing tests, so the measurement is no longer
   reproducible on `main` — which is the point of pinning it. Reachability is shown with a
   coverage delta on the lines the property is about, not with a passing suite.
   (Added 2026-08-21; see §7.)
7. **A documented claim that is false is not E0 — it is a refutation, and the
   documentation is where the counterexample lives.** E0 as originally worded
   admitted a doc comment stating an invariant the code does not have, and rated
   it the same as an honest silence. `internal/trust/verify.go:90-91` asserts
   that `filepath.Join` prevents a `..` escape; it does not. A false invariant is
   worse than an absent one, because it gives a future auditor a reason to stop
   looking. (Added 2026-08-21; see §7.)
8. **Every status is dated.** A basis is a claim about a commit. §7 carries the
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
10. **The scope of a query is part of its claim.** Rule 2 says this for a
    generator's input domain. It holds for every command whose output becomes
    evidence: a `grep` is scoped by its path filter, a `git` history question is
    scoped by the ref classes enumerated, a per-consumer analysis is scoped by
    what "consumer" was taken to mean. Where a result depends on a set the author
    selected, **the selection is an assertion and is stated beside the finding.**

    Three instances, all from this project, all with the command run and the
    output real:
    - *Ref classes chosen to make a question tractable.* "Does pruning merged
      branches close the stale-code-in-a-grep hazard?" was answered over
      `refs/heads` + `refs/remotes` and came back "reduced, not removed." Over
      `git for-each-ref` with no filter the old fail-open is in twelve release
      tags — the one ref class that must never be deleted — so the hazard is
      irreducible by ref deletion and the prune's premise was wrong, not merely
      incompletely served.
    - *The conflict set substituted for the diff set.* "No Go file differs between
      the coverage-measured head and the merged head" was derived from the merge's
      one conflicted path; a merge commit's diff necessarily includes everything
      the other side brought in, and three Go files did differ.
    - *Consumers enumerated where producers were the question.* The #92
      `CHANGELOG.md` entry's per-consumer analysis was built from three greps for
      callers of the changed code, while the refusal's trigger is a property of
      the *lockfile* — so the one command that would have found the affected path
      (`grep -rn 'ResolvedLayer{' … | grep -v _test.go`) was never run, and the
      entry claimed a shipped producer that does not exist. Corrected by #109.

    The failure mode is not a wrong command. It is a right command over a set the
    author picked for a reason that never entered the finding. (Added 2026-08-21;
    see §7.)
11. **Closure-discharged is not evidence-discharged.** An issue closing is a fact
    about the tracker. A property holding is a fact about the code. A register row
    may be marked `Discharged: Yes` only on **re-derived evidence whose subject is
    `implementation` and whose coverage is `chosen` or stronger**, and the row cites
    that evidence rather than the closure. "Closed completed" is not a citation; the
    issue's title is not the row's counterexample.

    That wording replaces *"at E1 or better"*, and it is narrower on purpose: read
    against the old ladder, E3 was "better than E1", so a discharge could have been
    claimed on an exhaustive check of a *model* — evidence that a counterexample in
    the shipping code is gone, obtained without executing that code. No row was
    discharged that way; the ordinal permitted it, which is enough. Subject is not
    an ordering, so it cannot be traded for coverage (§2).

    This rule is retrospective, not hypothetical: three of the register's nine
    `Yes` rows at the time it was written had been discharged on closure alone, and
    one of the three (`P4` / #54) was discharged against a claim wider than the row
    made, while the row's counterexample went on reproducing and the defect sat
    stated in a comment in the tree (§7 item 38).

    **Deriving a column does not make it true.** The Status column is generated from
    the register precisely so that the two cannot drift (§7 items 23–24), and
    `propgen` reported agreement on every run from the day the register was written.
    Two representations agreeing is not evidence that either is right; a generated
    field removes drift, not error. The audit obligation in §6.1 exists because
    nothing mechanical can close this gap.

    **Half of this rule is now enforced, and the boundary matters more than the
    enforcement.** `propdoc.Doc.DischargeDefects` (`internal/propdoc/propdoc.go:450`)
    reports every `Yes` row that names no basis or cites no artifact; `propgen`
    refuses to run and `TestPropertiesRegisterMeetsRule11` fails the build
    (`internal/propdoc/discharge_citation_test.go:143`). It is a report over a parsed
    document rather than a parse error, because a policy violation must not make the
    document unreadable by the tool that would fix it. The occasion was that this rule
    was written in one session and never run against the rows already in the table:
    two cells reading verbatim `Yes — closed completed` survived the amendment that
    names that exact string, for a day, and were found by an audit rather than by a
    check (§7 item 78).

    **What no check here can see: subject divergence.** A row can name a basis, cite a
    real path, and track a genuinely closed issue whose fix is real and whose closure
    is correct — and still be false, because the issue is about a *different claim*
    than the counterexample beside it. Every component is true and the row is not.
    Two instances are known, and each was found by a different accident rather than by
    any check:

    | Row | Tracking | What the row claimed | What the closed issue was about |
    |---|---|---|---|
    | `P4` | #54 | Stage 7 refuses profiles resolved against the **shipped catalog** | `file://` **scheme dispatch**, which a fixture registry exercises (§7 item 38) |
    | `T7` | #48 | `packages:` entries are **unattested** | that those entries **will not be installed** by `strata run` (§7 item 78) |

    A syntactic check cannot reach this: the discharge is well-formed by every
    criterion available to a parser, and the divergence lives between the row's
    subject and the issue's. Finding it means reading the tracked artifact and
    comparing subjects, which is the §6.1 audit obligation and nothing else. So the
    green from `TestPropertiesRegisterMeetsRule11` bounds the closure-for-citation
    conflation only, and must not be read as covering the class above.
12. **A proposition covered unevenly across its domain reduces to its weakest
    covered scope, not its strongest.** Where a proposition quantifies over several
    execution routes, one basis for the whole cell cannot say that one route was
    enumerated and another merely exercised. The Basis cell therefore takes one
    entry per scope — `tier @ scope — citation`, entries separated by semicolons —
    and the basis it claims is the **meet** over them.

    The reason is that the old definition was a **max**. §3 defined the Basis cell as
    *the strongest basis claimed*, so a proposition covered exhaustively on one route
    and by three examples on another reported `exhaustive/implementation`, and nothing
    in the column said which route that came from. The overstatement is silent, which
    is the property that makes it worth a rule rather than a correction.

    **A meet needs an order, and the seven bases do not have one.** Coverage is
    totally ordered; Subject is not (rule 1). So:

    - Where every covered scope shares a Subject, the reduction is a **pair**: the
      lowest coverage reached on any of them, with that Subject.
    - Where they do not, there is **no meet**, and the reduction is the **set** of
      bases claimed. Status renders the set rather than choosing from it.
    - In both cases the reduction is over the **union of the declared bounds**, and
      says nothing about a part of the domain no entry names. A scope known to be
      uncovered may be declared with `none`, which collapses the whole cell: nothing
      is established over the union however well covered the rest is.

    Four things a multi-scope cell must do, each enforced with its own error so the
    author is told which one was missed: name every scope, name none of them twice,
    cite every scope that claims a basis, and **lead with the entry it reduces to**.
    The last is not cosmetic. The reduction is derived and appears only in the
    generated Status column, so the first entry is the only basis a reader of the
    Basis column actually sees — a cell leading with its strongest scope reproduces
    the overstatement with every derived number correct.

    Enforced by `internal/propdoc.parseBasis` and `Reduce`
    (`internal/propdoc/basis.go:182`, `internal/propdoc/propdoc.go:582`), tested at
    `internal/propdoc/basis_scope_test.go:42`. Two cells are scoped today, `T1` and
    `T5`, each reducing from `exhaustive/implementation` to `chosen/implementation`;
    the other 32 are single-scope and unchanged. **What the document's own green does
    not show:** both scoped propositions are `REFUTED`, and a live refutation
    outranks any basis (rule 5), so neither Status cell displays its reduction and
    the generated column is byte-identical before and after this rule. The
    renderings are covered by constructed cells, not by this document — see §7
    item 80. (Added 2026-08-22, #135.)

---

## 3. Propositions

Three columns, two authored and one generated:

- **Verdict** — *authored.* The result of attacking the proposition's *text*:
  `SOUND`, `TOO WEAK` (satisfiable by an implementation that still has the defect
  the proposition was written to exclude — the broken implementation is named),
  `TOO STRONG`, or `ILL-FORMED` (with the falsifiable replacement).
- **Basis** — *authored.* What the proposition's evidence establishes across the
  domain the cell declares: a (coverage, subject) pair from §2, written either in
  pair notation or under its legacy E-name, and the citation carrying it; `none` if
  nothing is cited, or `withdrawn` naming what superseded it. A basis with no
  citation is not a basis.

  Where the cell names one scope it claims one basis. Where it names several —
  `tier @ scope — citation`, semicolon-separated — the basis is the **meet** over
  them, the weakest reached on any scope the cell covers, and the cell leads with
  that entry. This was *the strongest basis claimed* until 2026-08-22, which is a
  **max** and overstates a proposition covered unevenly across its routes; §2.1
  rule 12 states the replacement and §7 item 80 the correction. Neither "strongest"
  nor "weakest" is a total order over the seven bases: Coverage is ordered and
  Subject is not, so where the scopes' subjects differ there is no meet and the set
  is reported instead. See §2 on which pairs the ranking convention ranks and the
  evidence does not.
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
3. Otherwise the Basis decides — the **meet** over the cell's scopes, per §2.1
   rule 12, not the strongest of them. `asserted` coverage renders `ASSERTED (…)`,
   naming the basis in parentheses — `ASSERTED (E0)` for the legacy spelling.
   Anything else renders `ENFORCED` followed by the canonical spelling:
   `ENFORCED E1`, `ENFORCED E2`, `ENFORCED E3`, or
   `ENFORCED exhaustive/implementation` for a pair with no legacy name. A cell
   citing nothing renders `UNPOPULATED` whatever it claims.
4. A cell naming more than one scope says so in the status, because a basis that is
   the weakest of several routes must not read as a claim about all of them:
   `ENFORCED E1 (weakest of 2 scopes)`, or
   `ASSERTED (E0, weakest of 2 scopes)`. Where the scopes share no Subject there is
   no meet and the set is named:
   `ENFORCED (no meet over 2 scopes: chosen/model, exhaustive/implementation)`.
   Where one scope is declared uncovered: `UNPOPULATED (a scope of 2 is uncovered)`.
   All of this is parenthesised detail, so the distribution the header quotes tallies
   these with their single-scope counterparts and does not grow a bucket — except the
   no-meet case, whose class is `ENFORCED` with no basis, since that is what it is.
   No cell renders any of these today; see §2.1 rule 12 on why, and §7 item 80.

A `SOUND` verdict and a `REFUTED` status are the healthy combination: the
proposition is worth having and the system does not yet satisfy it.

Where a proposition's verdict is `TOO STRONG`, Status measures the *repaired*
form named in its evidence cell; that the as-written form is unsatisfiable is the
verdict, not a measurement of the implementation.

### Group R — Resolution

| # | Proposition | Verdict | Basis | Status | Evidence |
|---|-------------|---------|-------|--------|----------|
| **R1** | *Determinism.* Resolution is a function of (profile, registry state, resolver version). Two resolutions with identical inputs produce lockfiles that are byte-identical after eliding exactly one field, `resolved_at`, which records the wall-clock time of resolution. Every other byte is identical — including every field that does not participate in `environment_id`. The set of elided fields is enumerated here and is not deferred to the specification. | SOUND | none | UNPOPULATED | **Restated 2026-08-21** (was: *"...produce byte-identical lockfiles"*, verdict `TOO STRONG`). The old form was unsatisfiable — a lockfile records `ResolvedAt` from the wall clock, so byte-identity fails on input one and satisfying R1 literally meant abandoning a field Strata deliberately records. **What the old statement permitted that this one forbids:** an unsatisfiable proposition generates no checkable obligation, so what stood in for R1 was whatever its citation happened to assert. `internal/resolver/resolver_test.go:574 TestEnvironmentID_Stability` compares `EnvironmentID` and `ProfileSHA256` — two derived hashes, the first over the five members of `envHashInput` (`spec/lockfile_hash.go:16-22`). Every field outside that hash was therefore free to differ between two identical resolutions with nothing objecting: `profile_name`, `strata_version`, `rekor_entry`, `bundle`, `mutable_layer`, `mount_order`, `satisfied_by`, `from_formation`. The new form forbids all of that. **What can now refute it that could not before:** a differential resolve comparing the full canonical serialisation with `resolved_at` elided. Under the old form no such test could be R1 evidence, because an honest attempt fails on the timestamp; under the new form it is the direct instrument, and it is cheap. Basis is deliberately `none`: the cited test does not exercise the restated property (proof-standard rule 6), and the old cell's `ENFORCED E1` measured a *repaired form* — "identical on the canonical content projection" — that deferred the word *canonical* to the implementation, which is rule 9. Stating R1 truthfully moved it from `ENFORCED E1` to `UNPOPULATED`. |
| **R2** | *Order-independence.* Permuting the entries of `software:` in a profile does not change the resulting lockfile in **any** field, `mount_order` included. Where the dependency graph leaves two layers mutually unordered, their relative order is fixed by a total order computed from layer content — name, then version, then digest — and never by position in the input. This proposition grants no exception and the specification cannot grant one on its behalf. | SOUND | none | REFUTED | **Restated 2026-08-21** (was: *"...does not change the resulting lockfile, except in fields whose specification says they record declaration order"*, verdict `TOO WEAK`). **What the old statement permitted that this one forbids:** one sentence added to the spec — *"`mount_order` records declaration order"* — satisfied the old R2 exactly, while leaving YAML key order to decide OverlayFS shadowing between two layers that both ship `bin/python`. The old exception clause was also open-ended: any field could be exempted later by amending the spec, so the proposition's content was whatever the implementation's own document said it was (rule 9). The new form forbids both — `mount_order` must be permutation-invariant, the tie-break is named in the proposition rather than referenced, and no spec amendment can widen it. **What can now refute it that could not before:** a permutation generator over `software:`. Under the old form that generator **confirms** R2 no matter what the implementation does, because every difference it can find lands in `mount_order`, which the spec exempts — a property test of a too-weak proposition is a tautology. Under the new form the same generator refutes on input one, and two artifacts already in hand become refutations of R2 itself rather than of a doc comment. Counterexample, executed: two lockfiles differing only in the slice order of two layers that share `MountOrder` yield different `EnvironmentID`s (`ef104908…` vs `4535c5c4…`), with the distinct-`MountOrder` control returning equal (`ef104908…` twice); permuting `Packages` likewise changes the ID (`789830da…` vs `fd576991…`). The doc comment at `spec/lockfile_hash.go:27-28` claims determinism "regardless of the order they appear in the lockfile YAML" — true only when `MountOrder` is distinct. Upstream, the resolver's tie-break *is* declaration order: `internal/resolver/stages.go:301` `sort.Ints(queue) // deterministic tie-breaking` orders by input index. Filed as #95. |
| **R3** | *Totality.* Resolution yields either a complete lockfile or an error. No execution produces a partially-populated lockfile. | **TOO WEAK** | none | REFUTED (2 of 2 live) | Broken-but-satisfying implementation: drop every request that cannot be resolved and return a fully-populated lockfile for the remainder. Every layer it names is complete, so R3 holds, and the environment is silently not the one that was asked for. "Complete" must be quantified over the *request*: every element of `software:` is either represented in the lockfile or named in an error. Realised instance: #79 — a null entry in a software list is silently dropped and the profile resolves without it. Second instance, untracked: `Profile.Instance` and `Profile.Storage` are parsed and then referenced nowhere (see X1). |
| **R4** | *Provider soundness.* If the dependency graph contains an edge from a consumer to a provider for capability *c*, that provider satisfies the consumer's declared version constraint on *c*. | SOUND | chosen/implementation — `internal/resolver/provider_matrix_test.go:239` | REFUTED | #67. `internal/resolver/stages.go:266-270` builds `capProviderIdx[cap.Name] = i` in a nested loop with no guard, so the highest-indexed provider of a capability wins the edge irrespective of version. **Reproduced 2026-08-21** by `TestStage6ProviderSoundness`, which runs all 24 input orders of a four-layer set: a `yy` provider; `mpi@3.0.0` requiring `yy`; `mpi@1.0.0`; a consumer requiring `mpi>=2.0.0`. **Four layers are needed to make R4 falsifiable at all**, and that is the reason this row had no executable basis for as long as it did: with a consumer and two providers the consumer has in-degree 1 and both providers have in-degree 0, so the consumer sorts last whichever provider owns the edge and a wrong edge is invisible. Blocking the *satisfying* provider behind a dependency of its own is what turns the wrong edge into an observable ordering — the consumer mounts before the layer it requires. **12 of the 24 orders reach stage 6** — stage 4's half of #67 rejects the other 12, so one half of this issue masks the other and the reachable count is asserted rather than assumed — **and 3 of those 12 violate R4.** The count is a count of known defects: it goes to zero when #67 is fixed and this test must then be changed on purpose, which is what makes this row dischargeable rather than merely describable. Basis is **`chosen`, not `exhaustive`**: input order is exhausted, and every other dimension — provider count, capability graph shape, constraint form — is a fixture this test picked, which rule 2 says an exhaustive walk of does not establish. Mutation-checked, each count predicted before the run: making stage 6 keep the *first* provider of a capability moves the 3 to 0, and reversing its tie-break (`sort.Ints(queue)` → `sort.Sort(sort.Reverse(sort.IntSlice(queue)))`) moves it to 9. |
| **R5** | *Provider completeness.* If some layer in the resolved set satisfies a consumer's constraint on *c*, resolution does not fail for want of a provider of *c*. | SOUND | exhaustive/implementation — `internal/resolver/provider_matrix_test.go:116` | REFUTED | #67, same issue, different half: `spec/layer.go:192-205 SatisfiesRequirement` is version-aware but first-match, so a satisfiable profile is rejected when a non-satisfying provider of the same capability name is encountered first. **Reproduced 2026-08-21** by `TestStage4ProviderCompleteness`, calling the shipping `stage4ValidateGraph` over the full cross product of two `mpi` providers × three versions each × four constraint forms (unconstrained, min, max, both) × both slice orders = **72 cells, of which 12 violate R5** — a satisfiable set rejected for want of a provider it contains. **The declared bound, per rule 2:** the axes are enumerated from `SatisfiesRequirement`'s own inputs rather than picked — a `Provides` list and a `Requirement`, whose only version fields are `MinVersion` and `MaxVersion`, so the four constraint forms are the complete set — while the *values* on the version axis are three majors, chosen because that makes an integer comparison a complete oracle. So the claim is exhaustive over that bound and says nothing about three providers, minor versions, or prereleases. Satisfaction is decided by an oracle written for the table (`satisfiesOracle`), deliberately **not** `semverGTE`/`semverLT`: expectations computed with the code under test hold by construction. The converse is asserted in the same loop — no provider satisfies ⇒ stage 4 must reject — which is what stops R5 being "fixed" by never rejecting anything; mutating stage 4 to accept unconditionally therefore fails on totality rather than passing, and mutating `SatisfiesRequirement` to scan every same-name provider moves the 12 to 0. |
| **R6** | *Environment identity is functional.* `EnvironmentID` is a function of exactly the fields the specification enumerates: changing any enumerated field changes the ID, and changing any non-enumerated field does not. | **TOO WEAK** | withdrawn — R7, X2 | WITHDRAWN (superseded by R7, X2) | **Withdrawn 2026-08-21, not restated — the attempt to restate it is what retired it.** The defect is proof-standard rule 9: *both* of R6's conjuncts defer to an enumeration the implementation owns, so hashing `base_ami_sha256` alone and enumerating `base_ami_sha256` alone satisfies R6 exactly while the identity distinguishes nothing. **What the old statement permitted:** the identity depending on an attestation pointer. Enumerate `rekor_entry`, and re-signing a layer changes `environment_id` — invalidating every cache entry for an environment whose *content* did not change — and old-R6 calls that correct, because the enumeration lists it. **What forbids it now, and why that is not a new R6:** removing the deferral means stating both conjuncts in terms of environment content, and both are already propositions here. Conjunct (a), *changing any enumerated field changes the ID*, becomes *a behaviour that can alter the assembled environment participates in the identity* — that is **X2**. Conjunct (b), *changing any non-enumerated field does not*, becomes *two lockfiles that assemble the same environment have equal `EnvironmentID`* — that is **R7**, verbatim. The enumeration was not incidental to R6; it was the only thing giving R6 content distinct from X2 ∧ R7. R6 in fact became redundant the moment R7 was added (§7 *Propositions restated or added* item 2, which already recorded that “R6 and X2 constrain only the direction behaviour → identity”); the deferral disguised it, and restating R6 is what exposed it. **What could now refute it that could not before — the question that decided withdrawal over restatement:** the one restatement that keeps R6 independent is to move the enumeration into this document — *the ID is a function of exactly these five fields*, `spec/lockfile_hash.go:16-22`. That form is sound, and falsifiable only by **drift between the list here and the struct there**: two representations bound to each other, which is exactly what rule 11 and §6.1 were adopted to stop counting as evidence. It would render `ENFORCED` while asserting nothing about whether either representation is right. A proposition whose only refuting instrument is a consistency check against itself has not moved, so R6 is withdrawn rather than narrowed. **Both citations transfer rather than lapse:** `spec/spec_test.go:542 TestEnvironmentID` asserts that `RekorEntry` does **not** change the ID — that is R7's direction, not functional-on-an-enumeration, so it was always evidence for R7 and never for R6 as written. `spec/packages_test.go:208 TestEnvironmentIDIncludesPackages` witnesses that `Packages` *do* participate, which is X2's direction. R6 carried `ENFORCED E1` on two tests, neither of which tested it. |
| **R7** | *(new)* *No spurious distinctions.* Two lockfiles that assemble the same environment have the same `EnvironmentID`. | SOUND | none | REFUTED (4 of 4 live) | Added 2026-08-21 (§7). R6 and X2 together constrain only one direction — that behaviour reaches the identity. Nothing forbids the identity distinguishing environments that are identical. `OnReady` is hashed (`spec/lockfile_hash.go:20,50`) and executed by nothing (#69), so two lockfiles that differ only in a never-run command list get different IDs and the same environment. **A generator exists and the Basis column is still `none`, deliberately.** `spec/environment_id_r7_fuzz_test.go` searches R7 over an enumerated set of environment-preserving transformations — permuting the layer slice under distinct `MountOrder`, re-signing, varying resolution metadata, appending an advisory `requires_host` entry, rebuilding the `Env` map — justified from the specification rather than from `envHashInput`'s membership, because deferring the domain to the code under test is what made the retired R6 unfalsifiable. 46,546,540 executions on `048aea4` found no failing input. That is evidence for **R7 restricted to that domain**, and R7 as stated quantifies over all environment-preserving pairs, four classes of which refute it — so citing a tier here would be the overclaim this document exists to catch. `TestR7SeedsReachEveryTransform` holds the floor that the seed corpus, which is all `go test ./...` runs, actually reaches every transformation; the search itself needs an explicit `-fuzztime` and runs in `.github/workflows/fuzz.yml`. **Two corrections to the generator's own domain, found after it was written and before it merged.** `vary-attestation` mutated `RekorEntry` and `vary-identity-and-timing` mutated `ProfileName`, on justifications copied from `spec/lockfile_hash.go:11-13`; both fields reach the assembled environment (#120), so R7's premise failed for those pairs and asserting an unchanged identity across them asserted that an X2 violation is *correct*. Both fields are now excluded and the transformations renamed. This is the R6 error one level down — not deferring the domain to `envHashInput`'s membership, but to a comment beside it. **And the whole domain is conditional on a scope clause this document does not yet state:** `internal/overlay/overlay.go:207-211` writes the entire lockfile into the assembled root, under which literal reading no two distinct lockfiles assemble the same environment, R7's premise is unsatisfiable and every execution above searched an empty domain (#121). |

R4 and R5 are separate and a system can fail either independently. R4 failing
means a consumer is wired to a provider that does not satisfy it; R5 failing
means a satisfiable profile is rejected. #67 does both, which is why the register
in §4 is many-to-many.

**And the two halves interact in one direction, measured 2026-08-21: R5's defect
masks R4's.** Stage 4 rejects any set whose first same-name provider does not
satisfy, so the input orders that most readily expose the wrong edge in stage 6
never reach stage 6 — exactly half of them, 12 of 24, in the fixture R4's basis
cites. Two consequences for whoever fixes this. Fixing R5 alone **widens** R4's
observable violation set rather than leaving it unchanged, so a fix to the first
half must not be read as leaving the second half where it was. And a test that
demonstrates R4 has to assert that its cells reached stage 6 at all, because a
stage-4 rejection and a correct topological order are indistinguishable in the
result: both produce no misordering to report.

### Group I — Integrity

| # | Proposition | Verdict | Basis | Status | Evidence |
|---|-------------|---------|-------|--------|----------|
| **I1** | *Mount integrity.* Every byte made visible in an assembled environment belongs to a layer whose content hashes to the digest recorded for it in the lockfile. | **TOO STRONG** | withdrawn — I1′, I6 | WITHDRAWN (superseded by I1′, I6) | Satisfying I1 as a universal claim over every visible byte requires abandoning two things Strata deliberately does: `packages:` installs (`internal/agent/agent.go:178`) put bytes from PyPI, conda and CRAN into the merged overlay, and Path B mounts a writable EBS upper (`MutableLayerSpec`). Neither set of bytes belongs to any layer. I1 is therefore restricted to layer-derived bytes, and I6 below covers the remainder. |
| **I1′** | *(restriction of I1)* Every byte made visible **from a layer** hashes to that layer's recorded digest. | SOUND | E1 — `cmd/strata/layer_cache_integrity_test.go:74,123,185,249`, `internal/agent/agent_test.go:181` | ENFORCED E1 | `strata run` route: `cmd/strata/run.go:426` builds the cache path only through `spec.LayerCachePath`, and both return paths hash the file against the declared digest first (`:436`, `:481`). Witnessed by `cmd/strata/layer_cache_integrity_test.go:74 TestLayerCacheAcceptsHonestLayers` (the control, showing the harness can pass) with `:185 TestLayerCacheRejectsPlantedCacheHit`, `:123 TestLayerCacheRejectsTraversalDigest`, `:249 TestLayerCacheRejectsEmptyDigest`. Agent route: `internal/agent/agent.go:237-247` hashes every path `Fetch` returns and compares unconditionally, with no `!= ""` guard; witnessed by `internal/agent/agent_test.go:181 TestRun_SHA256Mismatch`. Domain: these two routes. |
| **I2** | *Cache soundness.* Content obtained from a local cache is used only after its bytes have been hashed and compared against the declared digest, on every use. Under **A2** this must hold for cache hits, not only for fresh downloads. | SOUND | E1 — `cmd/strata/layer_cache_integrity_test.go:74,185` | ENFORCED E1 | Three cache-hit sites, all routed through validation: `cmd/strata/run.go:426` + `spec.VerifyFileDigest`, `internal/registry/s3client.go:366-371`, `internal/registry/localclient.go:268-273`. Witnessed by `TestLayerCacheRejectsPlantedCacheHit` (plants different content under a correct digest and requires refusal) against `TestLayerCacheAcceptsHonestLayers` as the control. `#83` records the standing cost objection — re-hashing every hit is O(environment size) per invocation — and is a design question, not a refutation. |
| **I3** | *Digest well-formedness.* Any value used as a digest — in a comparison, a filesystem path, or a cache key — is syntactically a SHA-256 digest before it is so used. Absent and malformed are both rejected (**A5**). | SOUND | E1 — `spec/digest_test.go:16`, `spec/digest_test.go:53` | REFUTED (2 of 3 live) | The rule exists and is enforced where it is called: `spec/digest.go:30-47 ValidateLayerDigest` requires exactly 64 lowercase hex and rejects empty explicitly, witnessed at `spec/digest_test.go:16 TestValidateLayerDigest` and `:53 TestLayerCachePathRejectsEscape`. Two sites do not call it. (a) `cmd/strata-agent/s3_fetcher.go:73` builds `filepath.Join(f.cacheDir, layer.SHA256+".sqfs")` from an unvalidated digest and `os.Rename`s into it — #81. (b) `LockFile.IsFrozen()` and `EnvironmentID()` treat any non-empty string as a digest: the repository's own `spec/spec_test.go:544` passes `SHA256: "bbbbbb"` and `internal/resolver/resolver_test.go:597` passes `AMISHA256: "sha256-ami-test123456789"`, and both tests pass. So "frozen" is satisfied by a lockfile with no digests in it. (b) (b) untracked → filed as #96. |
| **I4** | *Path confinement.* No field of a lockfile or manifest can cause a filesystem operation outside the directory designated for it, for any field value. | SOUND | E1 — `spec/digest_test.go:53` | REFUTED (3 of 4 live) | The mechanism is understood and stated correctly in one place — `spec/digest.go:52-55`: *"`filepath.Join` calls `Clean`, which resolves `..` rather than rejecting it, so joining an unvalidated digest can name a file outside `cacheDir`. Every site that builds a layer cache path must go through here."* Executed confirmation: `filepath.Join("/var/cache/strata/layers", "../../../../etc/cron.d/evil.sqfs")` → `/etc/cron.d/evil.sqfs`. Four sites build a path from an unvalidated lockfile field: `internal/trust/verify.go:92` (`layer.ID`, and its own comment at `:90-91` asserts the opposite — #58); `internal/overlay/mount_linux.go:141` (`layer.ID` into `os.MkdirAll` then `MountSquashfs` — a **write** primitive, untracked); `internal/export/oci.go:60` (`"layer-"+lp.ID`, untracked); `cmd/strata-agent/s3_fetcher.go:73` (`layer.SHA256`, #81). Untracked pair → filed as #97. |
| **I5** | *Distinctness.* Two layers with distinct content never share a cache location. | **TOO WEAK** | E1 — `spec/digest_test.go:80` | REFUTED (1 of 2 live) | Broken-but-satisfying implementation: key the cache on a fresh UUID per fetch. Distinct content never collides, so I5 holds — and nothing is ever a cache hit, because the useful property is the converse one I5 does not state: *the cache location is a function of the content*. Restated, that is what makes a digest a cache key rather than a label. Counterexample to I5 even as written: `cmd/strata-agent/s3_fetcher.go:73` with `layer.SHA256 == ""` yields the filename `.sqfs` for every hashless layer, so distinct content collides (#81). The confined route rejects it — `spec/digest_test.go:80 TestLayerCachePathEmptyDigestDoesNotCollide`. |
| **I6** | *(new)* *Non-layer byte accountability.* Every byte made visible in an assembled environment that does not come from a layer is either (a) produced inside the environment after assembly, or (b) named in the lockfile with a digest that is checked at the moment it is installed. | SOUND | none | REFUTED (2 of 2 live) | Added 2026-08-21 (§7), as the remainder I1 had to give up. `spec.ResolvedPackageEntry` carries a `SHA256` field (`spec/packages.go`), and the installer never passes it: `internal/agent/package_installer.go:101` runs `pip install --quiet <name>==<version>` with no `--require-hashes` (`grep -rn 'require-hashes' . --exclude=PROPERTIES.md` → no output; see §7 item 22), `:115` runs `conda install` and treats a recorded version of `latest` as "whatever is current". The recorded digest is read only by `strata verify --packages` out of band (`internal/packages/resolve.go:272-287`). Refuted for the empty adversary; A3 and A7 make it an attack rather than a hazard. Filed as #98. |

### Group T — Authenticity and trust

This group is the core of Strata's differentiating claim.

| # | Proposition | Verdict | Basis | Status | Evidence |
|---|-------------|---------|-------|--------|----------|
| **T1** | *No unverified mount.* There exists no execution path from *layer declared in a lockfile* to *layer mounted in an assembled environment* that does not pass a successful signature verification under the trust policy in force. | **TOO WEAK** | chosen/implementation @ the `strata run` route — `cmd/strata/run_verify_test.go:370,519`; exhaustive/implementation @ the agent boot route — `internal/agent/boot_matrix_test.go:464` | REFUTED (1 of 4 live) | *The most important finding in this review.* Broken-but-satisfying implementation: ship an agent whose default policy is `allow-unverified`, and route every layer through a verifier that returns success when a layer names no bundle. Every mount then "passes a successful verification under the trust policy in force" and T1 holds — which is precisely the shape of the five fail-opens this document was written about. T1 defers its content to "the policy in force" and never says what the policy is when the operator selects nothing. It is only worth having when read together with T5's second sentence, and it should be restated to quantify over the *default* configuration (see H1, §1.4). Status under that reading: refuted at `internal/agent/agent.go:285` (nil verifier ⇒ skip — still open, #93) and, at `339329fb`, `:285-293` (layer with `Bundle: ""` dropped from the set; empty set ⇒ success — #92, **closed by #104**, which replaces the filter with a refusal naming every bundle-less layer at `:310-323`; witnessed at `internal/agent/verify_bundles_test.go:187,252,291` and `cmd/strata-agent/s3_bundle_contract_test.go:22`). Probe 3 on #92 boots READY with a verifier that refuses every artifact, `VerifierCalled=false`. Closed on the `strata run` route by #55: `cmd/strata/run_verify_test.go:519 TestRunRun_MissingKeyIsARefusalNotASkip`, `:370 TestVerifyRunLayers_ReportsEveryFailure`. **Basis raised 2026-08-21 (#129), and it is route-scoped for a reason T1's own text creates.** `TestBootMatrix_DecisionSurface` enumerates all 112 cells of the agent boot route's decision surface — `Config.Verifier` × the bytes a bundle fetch yields × `layer.Bundle` × content-against-`SHA256` — asserting per cell which check fired, whether `Mount` was reached, and how many times the verifier was called. That last is what makes it T1 evidence rather than T5-only: *"`Run` returned nil"* does not distinguish verification succeeding from verification never happening, which is the whole of #92. **20 of the 112 cells reach the mount with nothing verified**, pinned as a literal so the hole cannot shrink without a diff. Two limits on what this cites. (a) It covers the **agent** route; T1 quantifies over *every* execution path, and the `strata run` route is covered separately and only at `chosen`. The Basis cell says so per route rather than in prose, and reduces to the weaker of the two — §2.1 rule 2 and rule 12, the declared bound is part of the claim (#135). (b) The table's expectation ladder makes a boot with no verifier a **refusal**, so it measures T1 under the strengthened reading rule 9 prescribes and not under T1's literal text, which a fail-open still satisfies — the `TOO WEAK` verdict, unchanged. The 20 cells therefore carry `knownOpen` and assert *today's* behaviour, failing loudly when #93(a) closes rather than certifying it. |
| **T2** | *Verification soundness.* Verification succeeds for a layer only if a signature exists, by an identity admitted by the trust policy, over the digest of the exact bytes that will be mounted. | SOUND | E1 — `cmd/strata/run_verify_test.go:466` | REFUTED | The "exact bytes" clause is the load-bearing part and it is satisfied deliberately: `cmd/strata/run.go` adds a check absent from `trust.VerifyLayer` — that the bundle attests *this lockfile's* digest — with the reason stated in its doc comment (*"a missing cosign must not be the difference between 'wrong layer's bundle' and 'accepted'"*). Witnessed by `cmd/strata/run_verify_test.go:466 TestRunRun_RefusesLayerWhoseBundleAttestsAnotherArtifact`. For the *lockfile*, no signature verification exists at all (#60), so T2 does not hold of the artifact that names the layer set. Weakness worth recording: "an identity admitted by the trust policy" is satisfied here by possession of one `--key`; there is no identity policy to admit or refuse anything. |
| **T3** | *Transparency binding.* A transparency-log entry accepted as evidence for artifact *A* has a body that corresponds to *A*'s attestation. Under **A4**, the existence of the referenced entry is not itself evidence about *A*. | SOUND | E1 — `cmd/strata/verify_rekor_test.go:80,147,217` | REFUTED (2 of 3 live) | Discharged for the verify command by #59, which replaced a log-index existence check that discarded the bundle: `cmd/strata/verify_rekor_test.go:80 TestVerifyRekorEntries_PassesTheBundle`, with `:217 TestVerifyRekorEntries_BadIndexIsNotVerified` and `:147 TestVerifyRekorEntries_UnfetchableBundleIsAFailure` as the failing controls. Refuted on the resolver path: stage 7 holds a bundle *URI*, not bundle bytes, so it cannot compare anything (#85). Two open objections to the *evidence*, not to the proposition: #88 — the `hashedrekord` body shape is inferred from our own `Log`, so writer and verifier can be wrong together, which is rule 3's tautology at the level of a design; #86 — `TestStage7_RekorVerification` asserts a negative case its body never exercises, which is rule 6. |
| **T4** | *Trust-anchor independence.* The root of trust used to verify artifacts is not obtained from the authority that serves those artifacts. Under **A1**, an attacker who can replace an artifact cannot also replace the material that decides who may sign it. | SOUND | none | REFUTED (2 of 2 live) | #62 — the agent fetches its cosign public key from the same S3 bucket that serves the layers, so A1 alone replaces both. #63 — `ec2runner` downloads the cosign binary itself with no checksum or signature check, which is the §1.2 clause "obtaining the verification binary is in scope under A3" paying out. |
| **T5** | *Fail-closed.* Any inability to complete verification — absent tool, absent key, absent bundle, absent log entry, network failure, unparseable material — results in refusal. Degradation to a weaker check occurs only when a weaker policy has been explicitly selected by the operator. | SOUND | chosen/implementation @ the `strata run` route — `cmd/strata/run_verify_test.go:264`; chosen/implementation @ verifier construction — `cmd/strata-agent/cosign_verifier_test.go:127,187,249`; exhaustive/implementation @ the agent boot route — `internal/agent/boot_matrix_test.go:464` | REFUTED (1 of 4 live) | The strongest proposition in this document: it names absence alongside invalidity, so A5 cannot slip past it, and it fixes the default in its second sentence, which is what T1 fails to do. Refuted at `internal/agent/agent.go:285` (#93, still open) and, at `339329fb`, `:285-293` (#92) — an absent bundle was a skip, not a refusal. **#104 closes the #92 half**: absence is now refused at `:310-323`, witnessed at `internal/agent/verify_bundles_test.go:187,252,291` and `cmd/strata-agent/s3_bundle_contract_test.go:22`. Enforced at E1 on the routes already fixed: `cmd/strata/run_verify_test.go:264 TestNewRunVerifier_NeverReturnsANilVerifier` (#55) and `cmd/strata-agent/cosign_verifier_test.go:127 TestResolveVerifier_NilVerifierOnlyWithAnExplicitOptOut`, `:187 TestAllowUnverified_DefaultsToClosed`, `:249 TestProductionPrereqs_ClosedByDefault` (#56). **Basis raised 2026-08-21 (#129), and scoped per route 2026-08-22 (#135): the cell now reduces to `chosen/implementation`, the weakest of its three routes, rather than reporting the strongest.** T5's list of failure modes — *absent tool, absent key, absent bundle, absent log entry, network failure, unparseable material* — is close to an enumeration of a domain, and `TestBootMatrix_DecisionSurface` now runs one: the bundle-bytes dimension covers no fetcher, valid-for-this-content, valid-for-*other*-content, `(nil, nil)`, non-JSON, well-formed JSON with the wrong media type, and a fetch error, crossed with four verifier dispositions, `layer.Bundle` empty or named, and content matching or not matching its declared `SHA256` — 112 cells, each asserting *which* check fired rather than merely that one did. Where T5's own list stops short of the shipping surface, the enumeration is derived from the code's inputs and covers more (valid-for-other-content is a substitution, not an inability), and where it goes beyond this route it is not covered here at all: **an absent cosign binary and an absent Rekor entry are neither of them dimensions of this table**, so T5's "absent tool" and "absent log entry" clauses remain at `chosen` on the routes cited above. **20 of the 112 cells reach the mount unverified** because `Verifier` or `BundleFetcher` is nil (#93(a), open), which is the same live half that keeps T5 refuted; they assert today's behaviour and fail when it changes. |
| **T6** | *Policy explicitness.* The trust policy under which a result was produced is recorded alongside that result, so that a verification outcome is interpretable without knowledge of the invoking environment. | **ILL-FORMED** | none | REFUTED | "Recorded alongside that result" names neither an artifact nor a lifetime. A line on stderr satisfies it on one reading and no durable record satisfies it on another, so as written it cannot be failed. Falsifiable form: *the lockfile, or an attestation deposited with it, carries a field naming the trust policy in force, and a consumer reading only the artifact can determine what was checked.* Under that form: refuted by absence. `grep -rn 'TrustPolicy\|VerifiedAt' --include='*.go' . \| grep -v _test.go` returns 0 lines, and no lockfile field records what was checked. Filed as #100. |
| **T7** | *Command-name honesty.* A command's name and documented behaviour do not claim a stronger check than it performs. A flag that purports to disable a check disables a check that was otherwise performed. | **TOO WEAK** | E1 — `cmd/strata/run_verify_test.go:350,497` | REFUTED (3 of 4 live) | Broken-but-satisfying implementation: keep the fail-open and fix the *documentation*. T7 constrains names and docs, so the cheapest way to satisfy it is to document the weakness while leaving the command called `verify` and the behaviour unchanged — the operator's attention is displaced exactly as before. Repair: pair T7 with a requirement that a weaker check announces itself **at use time**, not only in prose. `strata run --no-verify` already meets the repaired form (`cmd/strata/run.go:184` prints the warning; `cmd/strata/run_verify_test.go:350 TestVerifyRunLayers_NoVerifyAnnouncesTheSkip` with `:497 TestRunRun_NoVerifyReachesTheMount` as its pair), and so does `STRATA_AGENT_ALLOW_UNVERIFIED` (#56). `strata verify` does not: without `--rekor` it performs presence checks only (`cmd/strata/verify.go:82-98 collectPresenceFailures`) under a name that claims verification — #60. And `verifyBundles`'s doc comment stated the skip as intended design (`internal/agent/agent.go:272-283`) — T7's broken-but-satisfying implementation occurring in the wild rather than as a hypothetical: the weakness was documented accurately and the behaviour left alone, so T7 as written was *satisfied* by the artifact that recorded the fail-open. #104 rewrites the comment and inverts the behaviour together, which is the only combination that discharges anything. |
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
| **P3** | *Referent stability.* The environment identity attested by a published record cannot subsequently denote different bytes. | SOUND | none | REFUTED (2 of 2 live) | `internal/agent/package_installer.go:115` treats a recorded conda version of `latest` (or empty) as "resolve at boot", and `:101` installs pip packages from PyPI with no hash, so one `EnvironmentID` denotes different bytes on different days. Refuted for the empty adversary; A7 makes it steerable. Same root as I6; filed together as #98. Second instance, measured on `93df7ca` and structurally different: the identity is a *storage key*. `PutLockfile` writes `locks/<environmentID>.yaml` unconditionally in both clients, so two lockfiles that share an `EnvironmentID` and differ in content cannot coexist — the second silently replaces the first, and `strata-agent` boots whichever was published last from the URI the caller was told to tag the instance with. A published record's referent therefore changes on someone else's publish, with no adversary and no error (#124). |
| **P4** | *Freeze attainability.* A lockfile produced by ordinary resolution can satisfy the system's own definition of frozen, without manual editing. | SOUND | none | REFUTED (3 of 3 live) | #64 — `strata freeze` structurally cannot succeed because nothing populates `ami_sha256`, and `IsFrozen()` requires `Base.AMISHA256 != ""`. The draft's own note anticipated the consequence and it holds: P1 is presently vacuous, because the precondition P1 guards cannot be reached by ordinary output. |

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
| **X1** | *Specification completeness.* Every behaviour the profile schema permits an author to specify is either executed by the runtime or rejected at parse time. A field that is accepted, recorded, and never acted upon is a defect. | SOUND | none | REFUTED (5 of 5 live) | Instances, each named with the issue it is filed as. No count is restated here: the register rows are the source and `Status` derives the total, and the lettering does not map one-to-one onto rows — `Profile.Instance` and `Profile.Storage` are two instances under one issue. (a) `OnReady` — parsed at `spec/profile.go:49`, copied into the lockfile at `internal/resolver/stages.go:434`, hashed at `spec/lockfile_hash.go:50`, executed nowhere (#69). (b) `Profile.Instance` and (c) `Profile.Storage` — `grep -rn 'InstanceConfig\|StorageMount' --include='*.go' . \| grep -v _test.go` returns only the declarations at `spec/profile.go:39,42,310,328` and no use; both are parsed, dropped at resolution, and never reach the lockfile. (d) `ResolvedPackageEntry.SHA256` — recorded and hashed into the identity, and never used by the component that installs the package (see I6). `RequiresHost` is a further candidate and is exempted only because the schema says so in as many words — *"Advisory only in v0.21.0"* (`spec/lockfile.go:57-60`), which is X1's rule applied honestly rather than a violation of it. (e) `validated_on` — a verification claim no code reads, and false where it can be checked (#113, added below). (f) the twelve `pending-initial-build` placeholders occupying `rekor_entry` and `bundle` (#46, added below). Filed as: (a) #69, (b) and (c) #102, (d) #98, (e) #113, (f) #46. |
| **X2** | *Identity captures behaviour.* If a specified behaviour can alter the state of the assembled environment, that behaviour's specification participates in the environment identity — or the identity does not determine the environment. | SOUND | none | REFUTED (4 of 4 live) | Third instance, and the one measured rather than read off the source: `LockFile.Defaults` decides the contents of `/etc/profile.d/strata-defaults.sh` in the assembled root and is absent from `envHashInput`, so three lockfiles that load no module, `python/3.11.9` and `python/3.9.18` respectively all share `09f451dc…` (#118). Refuted in writing, by design: `spec/lockfile_hash.go:14-15` excludes `MutableLayer` from the hash — *"it is metadata about the build process, not the environment content itself (the upper is not content-addressed)"* — and a writable EBS upper mounted over the stack is exactly a behaviour that alters the assembled environment's state. Second instance: `Packages` *do* participate in the identity and still fail to determine the environment, because the bytes they name are fetched from a moving upstream at boot (I6, P3). The draft's sharp consequence stands and is now concrete. **Two further instances from the same enumeration, both measured:** `ProfileName`/`RekorEntry` reach the assembled root and the child process environment at five sites while `spec/lockfile_hash.go:11-13` asserts in writing that they "do not affect what runs" (#120), and `PATH`/`LD_LIBRARY_PATH` are built entirely from `InstallLayout`, `Name` and `Version`, none hashed, so one identity covers a layer being on `PATH` and being absent from it (#122). **X2 is also asserted as fact in the source it is refuted by:** `spec/lockfile.go:139` reads *"Two lockfiles with the same EnvironmentID describe identical environments"* (#123). **And the consequence is destructive, not merely ambiguous:** the identity is a registry storage key written with no conditional put, so a colliding publish silently overwrites (#124, filed against P3, which is the cheapest mitigation for this whole class because it makes every instance loud at publish time without waiting for the identity question to be settled). |
| **X3** | *(new)* *Documented-form acceptability.* Every profile form the documentation presents as valid is accepted by the parser, and every artifact the documentation presents as runnable resolves against the shipped catalog. | SOUND | E1 — `spec/softwareref_yaml_test.go:22-24`, `spec/docsnippets_test.go` | REFUTED (2 of 3 live) | Added 2026-08-21 (§7) because two tracker issues refuted nothing in §3 and both were the *converse* of X1. X1 lets a system satisfy it by rejecting everything at parse time; nothing said the documented forms must be accepted. #53 — the inline software-ref form `- python@3.13`, presented in the documentation, did not parse because `SoftwareRef` had no `UnmarshalYAML`; discharged at E1 by `spec/profile.go:237` with `spec/softwareref_yaml_test.go:22-24` and `spec/docsnippets_test.go`, the latter testing the documentation sites themselves. #70 — open, and confirmed live: `examples/alphafold3.yaml:10`, `examples/pytorch-jupyter.yaml:10` and `examples/r-quarto-workstation.yaml:10` name `@2024.03` formations while `cmd/strata/formations/` ships only `@2026.03`, and `go test ./examples/` passes. Four tests parse those files (`examples/examples_test.go:21,48`, `examples/catalog_test.go:19,101`) and none crosses a profile's reference against the catalog — rule 6 in the documentation dimension. |

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
removed from this register only when its discharge is evidenced at coverage
`chosen` or stronger with subject `implementation` (§2.1 rule 11).
`H1` in the capability column means no adversary is required (§1.4).
**A `Yes` requires re-derived evidence, not a closed issue** — §2.1 rule 11 — and
`Yes` rows are audited on the cadence in §6.1, because a discharged row is the one
nobody has a reason to look at again.

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
| P4 | Stage 7 refuses every profile resolved offline against the shipped catalog, with `BUNDLE_MISSING` (originally worded *"no profile could resolve offline with the shipped catalog"*; reworded 2026-08-21 for the reason in §7 item 43, meaning unchanged) | H1 | #54 | No — **reopened 2026-08-21**. Was `Yes — closed completed`; the counterexample as *written here* still reproduces. What #54 fixed is scheme dispatch, so a `file://` registry now works: `STRATA_REGISTRY_URL=file://…/strata-fixture/registry strata resolve …/offline-minimal.yaml` exits 0, and CI's `Offline resolve (no AWS)` job asserts exactly that. The **shipped catalog** is a different registry — `buildRegistryClient` returns the embedded `MemoryStore` when `STRATA_REGISTRY_URL` is unset (`cmd/strata/resolve.go:105`), and its recipe-derived manifests carry no bundle, so stage 7 refuses every profile: `env -u STRATA_REGISTRY_URL strata resolve <single-layer profile>` → `[stage=stage7 code=BUNDLE_MISSING] layer "python-3.13.2-linux-gnu-2.34-x86_64" has no Sigstore bundle`. `cmd/strata/resolve.go:96-97` says so in a comment. The discharge matched a claim about the fixture registry to a row about the shipped catalog — the same referent slippage the citation-tense rule guards against, here in this register (§7 item 38) |
| I6 | pip SHA256 pins were validated against nothing | A3 | #51 | Partially — `strata verify --packages` validates against PyPI out of band; the *install* still ignores the pin (see the I6 row below) |
| T7 | `strata run` did not warn that `packages:` entries are unattested | H1 | #48 | No — **re-derived 2026-08-22** under §2.1 rule 11; was `Yes — closed completed`, which is the closure-for-evidence conflation that rule names (#137). The row and the code answer two different questions. #48 asked for a warning that the packages *will not be installed* by `strata run`; that shipped at `cmd/strata/run.go:92-105`, counts entries rather than sets, and names both the installer and the consequence — E1 at `cmd/strata/run_packages_warning_test.go:78,132`. The row says "unattested", which is a claim about the attestation chain that **no** warning on either route makes: the agent installs these entries from PyPI/conda/CRAN with no bundle and no Rekor entry (`internal/agent/package_installer.go:37-45`), and `spec.ResolvedPackageEntry`'s `SHA256` is optional and unchecked at install time (#51). Filed as #139, with `cmd/strata/run_packages_warning_test.go:156` asserting today's silence so closing it fails a test. The row's wording is repaired under #140 and travels alone: rewording a counterexample in the change that discharges it is narrowing-until-satisfied |
| T7 | The resolver expanded unattested formations without warning | H1 | #49 | Partially — **re-derived 2026-08-22** under §2.1 rule 11; was `Yes — closed completed` (#137). The warning exists and fires, naming the formation, the reason and the placeholder value (`internal/resolver/stages.go:71-75`) — E1 at `internal/resolver/formation_attestation_warning_test.go:132`, with `:159` the control that an attested formation is *not* warned about. Three of the four resolver constructions in the tree deliver it: `cmd/strata/resolve.go:63`, `update.go:49`, `freeze.go:34` all set `Warnings: os.Stderr`. The fourth is the live half — `resolver.warn` returns silently when `cfg.Warnings` is nil (`internal/resolver/resolver.go:65-70`), `pkg/strata` builds its `resolver.Config` without that field (`pkg/strata/strata.go:103-107`) and `pkg/strata.Options` exposes none to set, so the public library route is silent on the same input, on formations the shipped catalog actually contains (#46). Filed as #138; `formation_attestation_warning_test.go:193` asserts that silence, so wiring it up fails a test. `Partially` counts as live, so T7 stays refuted |
| X3 | The inline software-ref form `- python@3.13` was documented and did not parse | H1 | #53 | Yes — E1, `spec/softwareref_yaml_test.go:22-24`, `spec/docsnippets_test.go` |
| X3 | All three shipped examples name formation versions the catalog does not contain, and `go test ./examples/` passes | H1 | #70 | No |
| X3, P4 | None of the six shipped formations resolves against the shipped catalog, at the versions that do exist. Measured on `87f02fe`: 6 of 6 exit 1, 0 lockfiles produced — five at stage 7 `BUNDLE_MISSING`, and `hpc-mpi@2026.03` at stage 4 `UNSATISFIED_REQUIREMENT`. That last is **four** missing layer dependencies, not one: `openmpi`'s recipe declares `runtime_requires` on `ucx@>=1`, `hwloc@>=2`, `pmix@>=5` and `libfabric@>=1`, all four shipped as recipes and none listed in the formation, which lists only `gcc` and `openmpi`. Stage 4 returns on the first, so the reported `ucx` understates it — enumerated by probe against `spec.BaseCapabilities.SatisfiesRequirement`, the predicate `stage4ValidateGraph` itself uses (originally worded *"because `openmpi` requires `ucx@>=1` and the formation omits a `ucx` layer"*; corrected 2026-08-21). Distinct from #70: these profiles name `@2026.03`, which exists. The one CI job that resolves end to end points at the `file://` fixture, where bundles exist by construction, and its profile names layers rather than formations | H1 | #108 | No |
| X1 | `validated_on` is a verification claim that no code reads and that is false where it can be checked. `grep -rn 'ValidatedOn' --include='*.go' .` returns its declaration and doc comment at `spec/layer.go:223,225` and nothing else — no reader in non-test code and none in tests. All six shipped formations assert it. For `hpc-mpi@2026.03` the assertion cannot ever have held: its four missing dependencies fail `stage4ValidateGraph`, which checks requirements against base capabilities plus *the resolved set only* and never adds a layer to satisfy one, and stage 4 (`internal/resolver/resolver.go:119`) runs before stage 7 (`:135`) — so the failure is independent of bundle, registry and attestation state, and no registry serving the shipped `openmpi` recipe resolves it | H1 | #113 | No |
| X1 | Twelve `pending-initial-build` values across the six shipped formations occupy `rekor_entry` and `bundle` — fields whose names assert an attestation that does not exist. The resolver responds with `r.warn` (`internal/resolver/stages.go:71-75`, the deliberate outcome of #49) and never propagates either field into a lockfile: `stages.go:409-413` copies `FromFormation`, the formation *name*, alone. So `internal/agent.verifyBundles` cannot see the placeholder and nothing downstream can act on it. Recorded here for the first time on 2026-08-21; previously the register mentioned #46 only in §4.1's "same category" list, which is the silent state the register exists to prevent | H1 | #46 | No |
| R4, R5 | Stage 4 validates against one provider, stage 6 wires the edge to another; stage 4 is itself first-match. **Boundaries measured 2026-08-21** (`internal/resolver/provider_matrix_test.go`): the stage-4 half rejects 12 of 72 satisfiable provider/constraint/order cells; the stage-6 half misorders 3 of the 12 input orders that reach it, the other 12 of 24 being rejected by the stage-4 half first. Recorded here because a counterexample with no reproduction cannot be discharged — only closed | H1 | #67 | No |
| T1, T5 | `verifyBundles` skips when the verifier is nil **and** when a layer names no bundle | A1 + A5; H1 for the nil half | #93 (fix); the decision half was #92, closed | Partially — the absent-bundle half is closed by #104 at `internal/agent/agent.go:310-323`, E1 at `internal/agent/verify_bundles_test.go:187,252`; the nil-verifier half at `:285` is open under #93. `Partially` counts as live, so T1 and T5 stay refuted |
| T1, T5 | `BundleFetcher`'s godoc **specified** the fail-open: implementations were told to return `(nil, nil)` for a layer naming no bundle, and `verifyBundles` treated no bytes as nothing to verify. The shipped `s3LayerFetcher` conforms. A defect in an interface's *specification* is inherited by every conforming implementation, so no amount of testing the implementations finds it | H1 | #92 | Yes — E1, `internal/agent/verify_bundles_test.go:291` (empty bytes refused) and `cmd/strata-agent/s3_bundle_contract_test.go:22` (the shipped `s3LayerFetcher` held to the corrected contract). #104 rewrote the godoc and inverted the behaviour together |
| T2, T7, T9 | `strata verify` and `IsSigned()` are presence checks; no lockfile verification exists | A1 | #60 | No |
| T4 | Agent fetches its cosign public key from the bucket that serves the layers | A1 | #62 | No |
| T4 | `ec2runner` downloads the cosign binary with no checksum or signature | A3 | #63 | No |
| P4, P1 | `strata freeze` cannot succeed — nothing populates `ami_sha256` | H1 | #64 | No |
| P1 | `strata publish` accepts unsigned lockfiles and dirty mutable layers | H1 | #66 | No |
| B2, B3 | Recipes fetch sources with no digest pinning; the recipe schema has no field for one | A3 | #68 | No |
| X1, R7 | `OnReady` is specified, hashed into the identity, and never executed. Declared `spec/lockfile.go:39-40`, copied `internal/resolver/stages.go:434`, hashed `spec/lockfile_hash.go:20,50`, executed nowhere. It refutes **R7** as well as X1, and for the same reason read in the opposite direction: a command list that runs nowhere cannot change the assembled environment, so two lockfiles differing only in `on_ready` assemble the same environment and get different identities. R7's evidence cell has cited this row as its demonstrating instance since R7 was added; the row named only X1 until 2026-08-21 (§7), so the attribution existed in prose and not in the table that derives R7's status | H1 | #69 | No |
| I3, I4, I5 | `strata-agent` fetcher builds a cache path from an unvalidated digest; `""` collides on `.sqfs` | A1 + A5 | #81 | No |
| I4 | `trust.VerifyLayers` builds squashfs paths from an unvalidated `layer.ID`; the comment claims `Join` prevents escape | A1 | #58 | No |
| T3 | Stage 7 holds a bundle URI, not bundle bytes, so it cannot Rekor-verify | A4 | #85 | No |
| T3 | `hashedrekord` body shape inferred from our own `Log` — writer and verifier can be wrong together | A4 | #88 | No |
| R3 | A null entry in a software list is silently dropped and the profile resolves without it | H1 | #79 | No |
| R2, R7 | `EnvironmentID` depends on YAML slice order when `MountOrder` ties, and on `Packages` order always | H1 | #95 | No |
| I3, P1 | `IsFrozen()`/`EnvironmentID()` accept any non-empty string as a digest | A5 | #96 | No |
| I4 | `overlay/mount_linux.go:141` and `export/oci.go:60` build paths from an unvalidated `layer.ID` | A1 | #97 | No |
| I6, P3, X1, X2, R7 | `packages:` installs ignore the recorded SHA256; conda `latest` resolves at boot — so `Packages` participate in the identity and still fail to determine the environment. The row carries **both** directions and they are worth separating, because until 2026-08-21 (§7) it named only the second: an install that ignores the recorded `sha256` means two lockfiles differing only in that digest fetch identical bytes, so the environment is the same and the identity is not — that is **R7**. Conda `latest` resolving at boot is the converse, and that is X2 | A3, A7, H1 | #98 | No |
| P2 | The Zenodo deposit contains the lockfile only — no bundles, no key, no layers | H1 | #99 | No |
| T6 | Nothing records the trust policy a result was produced under | H1 | #100 | No |
| T8, T9 | No freshness bound and no set-level attestation: rollback, freeze and mix-and-match all succeed | A6, A1 | #101 | No |
| X1, R3 | `Profile.Instance` and `Profile.Storage` are parsed and referenced nowhere else | H1 | #102 | No |
| R7 | `EnvironmentID` distinguishes a package set whose inner `Packages` slice is `nil` from one where it is empty. A set with no entries installs nothing either way, so the environment is identical. `spec/packages.go:49` carries `json:"packages"` with no `omitempty` while the **outer** `LockFile.Packages` field has it, so the inner slice marshals as `null` in one case and `[]` in the other and `computeEnvironmentID` hashes different bytes. Measured on `048aea4`: `59cc9349bf262f0ac84ae8ec74b3c768af016efe128dc66ae3a93179ffbdd9e4` versus `57620b0c2fa034e49fd39f507a180f28d42819a3996f7300a91d0a0d7b6c5076`. Reachable from ordinary input, not only hand-built structs: YAML `packages: []` decodes to an empty non-nil slice while an omitted key leaves it nil. Distinct from #95, which is about order rather than emptiness | H1 | #117 | No |
| X2 | `LockFile.Defaults` alters the assembled environment and does not participate in the identity, so two lockfiles that assemble **materially different** environments share one `EnvironmentID`. `Defaults` is declared `spec/lockfile.go:42-44`, copied `internal/resolver/stages.go:435`, and consumed at `internal/overlay/overlay.go:164-182`, which writes `module load <name>/<version>` lines into `/etc/profile.d/strata-defaults.sh` inside the assembled root — so it decides which module versions are active in every login shell. `envHashInput` (`spec/lockfile_hash.go:16-22`) has five members and `Defaults` is not one. Measured on `048aea4`: three lockfiles identical but for `Defaults` — none, `python/3.11.9`, `python/3.9.18` — all hash to `09f451dc302d078526dfcd8f1d50ab245ab1ba6f888f9693933db687c7f2c339`. This is the **converse** direction to #95/#69/#98/#117: same ID, different environment, so anything keying a cache or an attestation on `EnvironmentID` can vouch for the wrong environment. R7's generator cannot find this class — it asserts equal-environment-implies-equal-ID, and this is the other implication | H1 | #118 | No |
| X2 | `ProfileName` and `RekorEntry` alter the assembled environment at five sites and do not participate in the identity — and unlike `Defaults`, the exclusion is *argued for* in the source: `spec/lockfile_hash.go:11-13` says attestation and identity fields "do not affect what runs". They affect what runs at `internal/overlay/overlay.go:142-143` (`export STRATA_PROFILE`, `export STRATA_REKOR_ENTRY` in `/etc/profile.d/strata.sh`), `:197-198` (the same two in `/etc/strata/environment`, a systemd `EnvironmentFile`), `cmd/strata/run.go:370,372` (the child process environment), `internal/fold/eject.go:193-194,210-211` (the ejected artifact), and `internal/export/oci.go:390,403-404` (OCI image `Env` and labels). Measured on `93df7ca`: two frozen lockfiles differing only in these two fields both return `6680a2f4279081f86b759e179ae35a873c47730e352c61f23b6427c7317d35d1` while `/etc/profile.d/strata.sh` and `/etc/strata/environment` differ. The false comment is the direct cause of two unsound justifications in the R7 generator, recorded in §7 | H1 | #120 | No |
| X2 | `PATH` and `LD_LIBRARY_PATH` — the entire mechanism by which a Strata environment is the environment it claims to be — are built from three `LayerManifest` fields outside the identity. `internal/overlay/overlay.go:108-127` reads `InstallLayout`, `Name` and `Version`; `envHashInput` holds only `LayerSHA256s`. `InstallLayout` decides whether a layer contributes to `PATH` **at all** (`if layer.InstallLayout == "flat" { continue }`), so one identity covers both "this layer is on `PATH`" and "this layer is absent from it". Measured on `93df7ca`: three pairs identical in every hashed field all share `a5f6e562af5a9a1999231f6c6d0c3b250598f3f1ec425ec163272976ba961edf`, and the `flat`-versus-empty pair produces one root with no `PATH` line and one with `/strata/env/python/3.11.9/bin`. The `Name`/`Version` pairs are additionally *broken*, since the mount path comes from the manifest while the directory structure lives inside the squashfs | H1, A1 | #122 | No |
| P3 | `EnvironmentID` is a storage key, so a collision is destructive rather than merely ambiguous. `internal/registry/s3client.go:617` and `localclient.go:374` write `locks/<environmentID>.yaml` with no conditional put and no existence check, and `PutLockfile`'s godoc directs the caller to tag an instance with the returned URI so `strata-agent` fetches it at boot. Measured on `93df7ca` against `LocalClient`: two frozen lockfiles differing only in `Defaults` both return `a5f6e562…`, `locks/` holds **one** file after both puts, and the stored `Defaults` is the second writer's — so which environment an instance boots depends on publish order, not on the identity it was launched with. Same function: `EnvironmentID()` returns `""` for an unfrozen lockfile and `pkg/strata.Client.UploadLockfile` has no `IsFrozen` guard where `cmd/strata/publish.go:73` does, so an unfrozen lockfile is accepted and written to `locks/.yaml`. That second claim was held by reading until 2026-08-22 and is now **executed** against `LocalClient`: `internal/registry/lockfile_key_test.go` puts two *different* unfrozen lockfiles and asserts both premises (neither is frozen, both return `""`), that both puts return the same URI, that `locks/` holds exactly **one** entry and that its name is the literal `.yaml`, and that `ListLockfiles` returns the **second** writer's lockfile. **What remains held by reading is the *S3* half. Rule 4, and rule 6:** what runs is the local client, `internal/registry/s3client.go` is uncovered (§7), and the claim that the shipping path behaves the same rests on `s3client.go:617` building the key with the same expression as `localclient.go:374` — read, not run. An `s3client` integration run is the only thing that closes it | H1, A1 | #124 | No |

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

**When a proposition's evidence covers its routes unequally, the Basis cell says so
per route.** A proposition quantifying over every execution path is rarely covered
the same way on all of them, and one basis for the whole cell reports the
best-covered route's strength as the proposition's. So write one entry per route —
`tier @ scope — citation`, separated by `;` — lead with the entry the cell reduces
to, and let the Status column derive the floor (`weakest of N scopes`). A route known
to be uncovered is declared `none @ that route`, which collapses the cell to
`UNPOPULATED` rather than leaving the gap to be inferred from the Evidence prose.
§2.1 rule 12 states the reduction and the four obligations; `propgen` refuses a cell
that breaks any of them. Recording only the best-covered route is a rejected
document, not a style choice.

A change to a proposition's text is an amendment with a date and a reason, not a
silent edit: a proposition that quietly narrows until it is satisfied is not
evidence of progress. The verdict column exists to make that visible — a
proposition whose verdict moves from `SOUND` to nothing while its status moves to
`ENFORCED` has been narrowed, not satisfied.

**Obligation on any change that moves a proposition.** A pull request that
changes behaviour states which propositions it moves and from what to what, in
the form *"T5 moves from REFUTED to ENFORCED at E1; T1 remains REFUTED pending
the agent-side counterexample."* A change that moves no proposition says so.

**Referent stability applies to the records, not only to the environments.** P3
states it for a published environment identity: what a record denotes cannot
change after the record is made. The same failure occurs in this document, in the
tracker, and in milestones — artifact classes P3 does not range over and the
citation-tense rule (§7 item 35) does not cover, because that rule pins line
numbers and issue numbers rather than the *sets* a claim quantifies over.

So: **a claim recorded against a named set names the set as it stood when the
claim was made.** If the set is renamed, re-dated, or re-scoped afterwards, the
claim is restated or withdrawn — it is not left to be read against the new set.
Two instances, both live at the time of writing:

- **A discharge matched to the wrong registry.** The `P4 / #54` register row reads
  *"No profile could resolve offline with the shipped catalog"* and was marked
  `Yes — closed completed`. What #54 established is that a `file://` registry
  resolves; the shipped catalog is the embedded `MemoryStore`, and it still fails
  stage 7 for every profile. Row reopened above.
- **A closed milestone whose completion claim moved under it.** `v0.21.0` records
  #46 (*formation catalog refresh to 2026.03*) as in-scope work; #46's checklist
  and table name `data-science`, `bioinformatics`, `quarto-publishing`, and
  `alphafold3`, and the catalog now ships `bio-seq`, `genomics-python`,
  `r-research`, and `jupyter-gpu`. The set was **renamed rather than re-dated**,
  so items 1 and 4 of that checklist are satisfied for six formations other than
  the six they name. The milestone's completeness claim does not refer to what it
  appears to refer to. (#46 has since been unmilestoned, with the history recorded
  on the issue; the point survives the bookkeeping, which is why it is written
  here as a rule rather than only as a comment.)

This is deliberately **not** a new numbered proposition. §0 puts process out of
scope and §0.1 says this document is not a second tracker; a proposition about
milestone hygiene would be both. It is an obligation on maintaining the record,
which is what §6 is for.

### 6.1 Audit the `Yes` rows, not the `No` rows

**Discharged rows are where error accumulates unobserved.** A refuted row keeps
getting attention: it is in a milestone, it is on someone's list, and somebody is
trying to make it go away. A discharged row leaves the queue and is never looked at
again. So the register's reliability is not limited by the rows under active
dispute — it is limited by the rows nobody has a reason to revisit.

`P4` / #54 is the demonstration. It sat `Yes — closed completed` while its
counterexample reproduced on every run, and the mechanisms in place did not and
could not object: `propgen` checked the Status column against the register, the
register agreed with itself, and no test re-derives a counterexample. It was found
by re-running a command, and only a re-run could have found it.

**Cadence.** At every minor release, and on any change that touches the register:

1. Sample **at least three** `Yes` rows, choosing the **oldest-discharged first**
   and rotating so no row goes more than two minor releases unaudited.
2. For each, **re-derive the counterexample as the row states it** — run the
   command, boot the probe, execute the test. Reading the row is not an audit;
   reading the closing issue is not an audit.
3. Check the row's *scope* against what the evidence establishes, which is the
   failure #54 exhibited: the counterexample reproduced, the discharge was real for
   a narrower claim, and the row was wider than the claim. Rule 10 in the register.
4. Record the audit with its date on the row, whether or not the row changes. A
   `Yes` with no audit date is a `Yes` that has never been checked, and that is
   worth being able to see.

Every audit outcome is one of three, and all three get written down: the row holds
(date it), the row is wrong (reopen it, per rule 11), or the row is **right but
overbroad** (restate the counterexample to what the evidence covers, and file the
remainder).

**Audit log.**

| Date | Rows audited | Outcome |
|---|---|---|
| 2026-08-21 | `P4`/#54, `T7`/#48, `T7`/#49 | #54 reopened — counterexample reproduces, discharge belonged to a narrower claim (§7 item 38). #48 and #49 found discharged on closure with no cited tier; filed as #110 rather than flipped, since applying rule 11 to them is a standard-application decision (§7 item 42). |

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

### 2026-08-21 — #103 merged; the refresh executed, and the enumeration scored

#103 merged first (`3206ca4`), then this refresh, then #104. The prediction in
item 32 above is left as written so it can be scored; it was **incomplete in one
class and wrong in one detail**.

33. **The enumeration missed every citation that shifts merely because the file
    grew.** Item 32 listed the citations *about* the fail-open and none of the
    four that simply live in the same file below the insertion point: I1's
    `agent.go:172` → `:178`, I1′'s `agent.go:231-241` → `:237-247`, and two
    historical entries in this log. Nothing in item 32's reasoning would have
    found them, because it reasoned from *what the PR changes* rather than from
    *what cites the file it changes*. The correct query is mechanical:

    ```
    for f in $(git diff --name-only <base>...HEAD); do
      grep -c "$(basename $f):" PROPERTIES.md; done
    ```

    which reports 10 citations into `agent.go`, not the 4 item 32 anticipated. It
    also surfaced 3 citations into `cmd/strata-agent/s3_fetcher.go`, a file #104
    also edits — those turned out to need no change, because the edit is at `:130`
    and the citations are at `:73`, but *that* was established by checking rather
    than by assuming. This is the relocated-rule failure in its documentation
    form: I enumerated the sites I had been thinking about, not the sites that
    refer to what moved.

34. **`:314-325` was wrong; the refusal is at `:310-323`.** Predicted by arithmetic
    on a diff, corrected by reading the merged file. `:325` lands inside the
    comment for the *next* branch — the no-fetched-layers case, which is
    explicitly not the absent-bundle case — so the prediction cited a boundary
    that would have argued against the point it was cited for.

35. **Three citations were pinned in the past tense rather than renumbered**, in
    §1.3, §2 rule 6, and T1/T5: `agent.go:285-293` at `339329fb`. A refutation's
    evidence is the tree it was taken on, and renumbering it to the line that now
    *refuses* would make the counterexample unreproducible — the reader would go
    looking for a fail-open at a refusal and conclude the register was wrong. Rule
    for future refreshes: **a citation supporting a discharged refutation is
    pinned, not updated; a citation supporting a live property is updated.**

36. **T1 and T5 each dropped one live row**, so their statuses moved from
    `REFUTED (2 of 4 live)` to `REFUTED (1 of 4 live)` — regenerated with
    `go run ./cmd/propgen -write`, not typed. This is the movement that was
    invisible in the first population and is the reason the column was made
    generated at all: the propositions correctly stay `REFUTED` (#93's
    nil-verifier fail-open is untouched, and its row stays `No`), and the progress
    is nonetheless legible in the column.

    #104 is therefore the first change in this repository whose claimed
    proposition movements were **checked by CI rather than reviewed by eye** — with
    the scope of that check stated precisely, because it is narrower than it
    sounds. Two independent mechanisms, not one:

    - **The bookkeeping** is bound. Reverting the `Yes` in the `BundleFetcher` row
      to `No`, changing nothing else, makes `propgen` report drift on T1 and T5 and
      `TestPropertiesStatusColumnIsDerived` fail. Probed, not assumed.
    - **The behaviour** is bound separately, by `internal/agent`'s tests.

    What CI still does **not** check is that a citation names a test which actually
    *exercises* the property — a plausible-looking `_test.go:NNN` in an evidence
    cell is accepted as long as the suite is green. That is rule 6's question, and
    it is answered by a coverage delta on the line, by hand, per refutation. The
    generator removed the drift between two columns; it did not remove the need to
    show reachability. Filed as **#105**, with the obstacle measured: 45 of 154
    citation instances in this file are not machine-resolvable as written (10
    basename-only, 35 with no path at all), and only 13 of 49 test-file citations
    name the test they cite.

37. **#92 closed; the live register row's issue reference moved to #93, and the
    discharged one did not.** #92 held both halves of the `verifyBundles`
    fail-open; its absent-bundle half is discharged by #104 and its nil-verifier
    half is #93, so nothing lived in it that was not tracked elsewhere. Two rows
    in §4 named it and they were treated differently, which is the citation-tense
    rule of item 35 applied to issue references rather than line numbers:

    - The `verifyBundles skips…` row is **`Partially` — live** — so its issue cell
      was updated to name #93 as the tracking issue. A live refutation pointing at
      a closed issue reads as settled work.
    - The `BundleFetcher's godoc specified the fail-open` row is **`Yes` —
      discharged** — so its `#92` stays. The issue is where that counterexample was
      established, and repointing it would lose the provenance for no gain.

    Bookkeeping that transferred with the closure, recorded because a state
    transition is cheap and a queue label is a claim about the world: #92's
    decision-needed marker lived in its *title* (`Decision request: …`) and in no
    label, so it was invisible to every label query. #93 now carries
    `status: needs-design`, with the open question written out — whether
    `agent.Config{Verifier: nil}` becomes an error or a spelled `AllowUnverified`
    opt-out. Labels, milestone, both proposition references (T1 and T5) and the
    zero-coverage measurement were already on #93 and were checked field by field
    before the transition, not after.

### 2026-08-21 — a discharge matched to the wrong registry, and referent stability applied to the records

Measurements in this entry were taken at
`87f02fe10716f1f68423d8ab5cd84a280f289899`, working tree clean, with a binary
built from it (`go build -o /tmp/strata-probe ./cmd/strata`). Re-derive:
`git rev-parse HEAD; git status --porcelain`.

**Propositions moved**, per the §6 obligation: **P4** moves from
`REFUTED (1 of 2 live)` to `REFUTED (3 of 3 live)`; **X3** moves from
`REFUTED (1 of 2 live)` to `REFUTED (2 of 3 live)`. No proposition's text, verdict
or Basis changes, and no Status cell was edited by hand — both moved because
register rows moved, and `go run ./cmd/propgen -write` rewrote the cells:

```
PROPERTIES.md:401: P4 written "REFUTED (1 of 2 live)", register implies "REFUTED (3 of 3 live)"
PROPERTIES.md:429: X3 written "REFUTED (1 of 2 live)", register implies "REFUTED (2 of 3 live)"
PROPERTIES.md: rewrote 2 Status cell(s)
```

The distribution is unchanged — `ASSERTED (E0) 1, ENFORCED E1 4, REFUTED 27,
UNPOPULATED 1, WITHDRAWN 1` — because both propositions were already `REFUTED`.
That is worth stating rather than passing over: **a row moving from discharged to
live is invisible in the distribution.** It is visible only in the `(n of m live)`
counts, which is the property §6 claims for the generated column, now exercised in
the direction that costs something.

The §0.2 headline count moved with the new row, from *seventeen of thirty-three*
to *eighteen of thirty-four*, re-derived with the awk command printed there.

38. **A discharge was matched to a claim about a different registry, and the
    counterexample still reproduces.** The `P4 / #54` row read *"No profile could
    resolve offline with the shipped catalog (stage 7 `BUNDLE_MISSING`)"*,
    discharged `Yes — closed completed`. What #54 established is that **`file://`
    scheme dispatch works**: with a materialised fixture registry,
    `STRATA_REGISTRY_URL=file://…/strata-fixture/registry strata resolve
    …/profiles/offline-minimal.yaml` exits 0, and CI's `Offline resolve (no AWS)`
    job asserts exactly that and nothing more. The **shipped catalog** is a
    different registry: `buildRegistryClient` returns the embedded `MemoryStore`
    when `STRATA_REGISTRY_URL` is unset (`cmd/strata/resolve.go:105`), and its
    recipe-derived manifests carry no bundle, so stage 7 refuses every profile —

    ```
    $ env -u STRATA_REGISTRY_URL /tmp/strata-probe resolve <single-layer profile>
    strata: resolve: [stage=stage7 code=BUNDLE_MISSING] layer "python-3.13.2-linux-gnu-2.34-x86_64" has no Sigstore bundle — unsigned layers cannot be used
    exit=1
    ```

    — which `cmd/strata/resolve.go:96-97` states in a comment, in the tree, the
    whole time. Row reopened to `No`.

    The mechanism is worth separating from the outcome. Nobody edited the row and
    nobody misread the issue. The row's subject ("the shipped catalog") and the
    fix's subject ("a `file://` registry") were close enough that "offline
    resolution works now" covered both, and the register recorded the discharge
    against the wider of the two. **A discharge is a claim about a specific
    counterexample, and the counterexample is the row's text, not the issue's
    title.** Rule 10 is the general form; this is the instance that produced it in
    the register rather than in a report.

    Note what did *not* catch it: `propgen` verified that the Status column agreed
    with the register on every run since the register was written, and would have
    gone on doing so forever. A generated column enforces consistency between two
    places in this document; it says nothing about whether either one is true. The
    control §2.1 rule 3 asks for — a checker that fails when it reads nothing,
    implemented as `internal/propdoc.TestPropertiesGuardCanFail` — has a companion
    this instance names: a checker that passes because both of its inputs are wrong
    in the same direction.

39. **#108 added as a register row against X3 and P4** — none of the six shipped
    formations resolves against the shipped catalog, at the versions that *do*
    exist. 6 of 6 exit 1 with 0 lockfiles produced; five fail at stage 7
    `BUNDLE_MISSING` and `hpc-mpi@2026.03` fails earlier, at stage 4
    `UNSATISFIED_REQUIREMENT`, because `openmpi` declares `ucx@>=1` and the
    formation lists only `gcc` and `openmpi` — while the same file asserts
    `validated_on: [al2023/x86_64, al2023/arm64]`, a field `spec/layer.go:223-225`
    documents as "has been smoke-tested against". A formation that cannot pass
    stage 4 was not smoke-tested on anything.

    It is a **separate row from #70** rather than an extension of it, because the
    two fail for unrelated reasons and either can be fixed without the other: #70
    is `examples/*.yaml` naming `@2024.03` formations that do not exist, and these
    profiles name `@2026.03`, which does. Recorded against X3's second clause
    ("every artifact the documentation presents as runnable resolves against the
    shipped catalog") and against P4, since a lockfile that cannot be produced
    cannot be frozen.

    On why the existing tests are green: `examples/catalog_test.go` asserts the
    formation YAML **parses**, and CI's one end-to-end resolve is pointed at the
    `file://` fixture — where bundles exist by construction — with a profile that
    names layers rather than formations. Rule 6 in the catalog dimension: the job
    that would catch this is aimed at the one registry where it cannot happen.

40. **Rule 10 added — the scope of a query is part of its claim.** Rule 2 already
    said it for a generator's input domain; three instances this session said it
    for ordinary commands, in both directions, every time with the command run and
    the output real. The third is the one that reached a shipped artifact: the #92
    `CHANGELOG.md` entry enumerated "what changes per consumer" from three greps
    for callers of the changed code, when the refusal's trigger is a property of
    the *lockfile* — so the affected producer (`strata scan --output-lockfile`,
    the only non-resolver constructor of a `spec.ResolvedLayer` in non-test code)
    was never in the search, and the entry asserted a shipped producer of
    bundle-less lockfiles that does not exist. Corrected in a separate PR.

    Stated as a rule because the countermeasure is cheap and specific: the
    selection travels with the finding. Not "I grepped for X" as method
    boilerplate, but "this holds over *this set*, which I chose *because*" — at
    which point the reason is inspectable and a reader can name the set that was
    left out.

41. **§6 gains a referent-stability obligation on the records themselves.** P3
    states referent stability for a published environment identity. The same
    failure happens to records, in artifact classes P3 does not range over and the
    citation-tense rule (item 35) does not cover — that rule pins line numbers and
    issue numbers, not the *sets* a claim quantifies over. Item 38 is one instance,
    inside this register. The other is `v0.21.0`: a **closed** milestone whose
    completion claim covers #46, whose checklist and table name `data-science`,
    `bioinformatics`, `quarto-publishing` and `alphafold3`, while the catalog ships
    `bio-seq`, `genomics-python`, `r-research` and `jupyter-gpu`. The set was
    renamed rather than re-dated, so two of that checklist's items are satisfied
    for six formations other than the six they name, and a closed record asserts
    completeness of something it no longer denotes.

    Deliberately **not** a new numbered proposition. §0 puts process out of scope
    and §0.1 says this document is not a second tracker; a proposition about
    milestone hygiene would be both. It is an obligation on maintaining the record,
    which is §6's subject.

42. **Two register rows remain discharged on issue closure, and are filed rather
    than flipped here.** After item 38, `T7 / #48` and `T7 / #49` still read
    `Yes — closed completed`, which §4's own standard ("removed only when its
    discharge is evidenced at E1 or better") does not admit and §2.1 rule 4 rules
    out directly — a closed issue is not executed evidence. Both describe a warning
    that **is** present: `internal/resolver/stages.go:71-75` emits the unattested-
    formation warning (observed by command in item 38's transcript) and
    `cmd/strata/run.go:104` prints the `packages:` warning. Neither is asserted by
    any test —

    ```
    $ grep -rln 'no Rekor attestation' --include='*_test.go' .
    (no output)
    ```

    — so both are E0, and deleting either `warn` call leaves the suite green.

    Not flipped in this amendment on purpose. Item 38 is a **measurement** — the
    counterexample reproduces — whereas these two turn on *what `Yes` is allowed to
    mean* when the behaviour is right and the evidence is absent. That is a
    standard-application decision affecting T7's count, and it deserves its own
    review rather than travelling inside a PR whose claim is something else. Filed
    as #110, with the suggestion that `internal/propdoc` reject a `Yes` carrying no
    tier and no citation, which converts this whole class from something a reader
    must notice into a build failure.

43. **The `#54` row's counterexample was reworded, because the guard's own control
    could not mutate it.** `internal/propdoc.TestPropertiesGuardCanFail` is the
    control §2.1 rule 3 asks for: it discharges a live row and requires that some
    proposition's status move, so that a green `propgen` cannot mean "the parser
    read nothing." (No numbered §7 item introduces it; it arrived unremarked in the
    same commit as item 23, `d9d1b76`, which is its own small instance of the class
    — the mechanism that makes the guard non-vacuous was the one change in that
    commit the log did not claim.) It performs the mutation by string replacement —

    ```go
    strings.Replace(doc.lines[target.lineIdx], "| "+target.Discharged, "| "+DischargedYes, 1)
    ```

    — and `strings.Replace` with count 1 takes the **first** match in the line. The
    row's counterexample cell began with the words *"No profile could resolve
    offline…"*, so `| No` matched the **counterexample** column, not the
    `Discharged` column. The mutation produced `| Yes profile could resolve
    offline…` with the `Discharged` cell untouched, no status moved, and the control
    failed — correctly, and for a reason that had nothing to do with what it tests.

    Two properties of that control are worth recording rather than fixing in this
    amendment, since the test file is inherited and this PR touches no `.go` file:

    - It locates a cell by **text**, not by column index, so any counterexample
      whose prose begins with `Yes`, `No` or `Partially` redirects the mutation.
      This failed loudly, which is the good case; the bad case is a redirect that
      still leaves a parseable document and a moved status, where the control would
      pass while exercising the wrong column.
    - It selects the **first** live row and `break`s, so which row the control
      exercises is a function of register order. Reopening a row near the top of the
      table silently changed the subject of the test.

    The counterexample text now reads *"Stage 7 refuses every profile resolved
    offline against the shipped catalog, with `BUNDLE_MISSING`"* — same claim,
    different first word — with the original wording preserved in the cell. Filed as
    #111. Noted here because a reader comparing this row against the register's
    history should know the wording moved for a mechanical reason and the claim did
    not.

**Placement.** This file sits at the repository root beside `STRATA.md` rather
than under `docs/`. `CLAUDE.md` hygiene rule 1 sends documentation to `docs/`;
the judgment here is that a specification of the system's claimed properties is
a peer of `STRATA.md`, which is also at the root. Recorded as a decision rather
than assumed, because rule 1 also forbids standalone tracking files and §0.1
exists to explain why this is not one.

### 2026-08-21 — the discharge standard, and an audit obligation on the rows nobody revisits

This entry follows directly from item 38. That item found one wrong `Yes`; this one
states the standard it violated and puts a cadence on finding the next.

44. **Rule 11 added — closure-discharged is not evidence-discharged.** An issue
    closing is a fact about the tracker; a property holding is a fact about the
    code. Three of the register's nine `Yes` rows had conflated them. A `Yes` now
    requires re-derived evidence at E1 or better, cited on the row, and "closed
    completed" is not a citation.

    The rule carries the reason it was needed, because the reason is not obvious:
    **deriving a column does not make it true.** §7 items 23–24 made Status a
    function of the register precisely so the two could not drift, and `propgen`
    reported agreement on every run from the day the register was written. Two
    representations agreeing is not evidence that either is right. A generated field
    removes drift, not error — and the failure it cannot see is the one where both
    inputs are wrong in the same direction.

45. **§6.1 added — audit the `Yes` rows, not the `No` rows.** The sharp form of item
    38's finding. A refuted row keeps getting attention: it is in a milestone, it is
    someone's work, somebody wants it gone. A discharged row leaves the queue and is
    never looked at again. **So the register's reliability is limited not by the rows
    under dispute but by the rows nobody has a reason to revisit**, and those are
    exactly the `Yes` rows.

    The cadence is: at every minor release and on any change touching the register,
    sample at least three `Yes` rows oldest-discharged first, rotating so none goes
    more than two minor releases unaudited; **re-derive the counterexample as the row
    states it** rather than reading the row or its closing issue; check the row's
    scope against what the evidence actually establishes; and date the row whether or
    not it changes, so that a `Yes` with no audit date is visibly unchecked.

    Three outcomes are possible and all three are written down: the row holds, the
    row is wrong (reopen), or the row is **right but overbroad** — restate the
    counterexample to what the evidence covers and file the remainder. That third
    outcome is what #54 actually was, and a binary audit would have recorded it
    wrongly in either direction.

46. **The distribution is now marked as not a progress measure**, in a block quote
    beside the headline where it is most likely to be quoted from. This is not a
    caution about a hypothetical: on this date two rows moved from discharged to live
    and every number in that paragraph stayed the same, because a proposition already
    `REFUTED` stays `REFUTED` whether it has one live counterexample or four. **A
    reader watching the distribution for change would have seen a clean bill on the
    day the register got worse.** This amendment adds two more refutation rows and
    the distribution again does not move, which makes the point twice on one day.

47. **`validated_on` gets a register row against X1 (#113), split out of #108.** Five
    formations that do not resolve is a broken catalog. One that *asserts it was
    validated* is a false verification claim shipped in data, and repairing the
    catalog does not by itself make the assertion honest. X1 is the exact fit: "a
    field that is accepted, recorded, and never acted upon is a defect."
    `grep -rn 'ValidatedOn' --include='*.go' .` returns the declaration at
    `spec/layer.go:225` and its doc comment at `:223`, and **nothing else** — no
    reader in non-test code, and none in tests either.

    What makes `hpc-mpi@2026.03`'s assertion false rather than merely unverified is
    that its failure is registry-independent. `stage4ValidateGraph` checks each
    requirement against base capabilities plus the capabilities of *the resolved set*
    and never adds a layer to satisfy one; stage 4 (`internal/resolver/resolver.go:119`)
    runs before stage 7 (`:135`). So no registry serving the shipped `openmpi`
    recipe's `runtime_requires` resolves this formation, whatever the bundle state —
    which means `validated_on: [al2023/x86_64, al2023/arm64]` cannot have been true
    when it was written.

48. **The #108 row's count was wrong and is corrected: four missing dependencies, not
    one.** The row said `hpc-mpi@2026.03` fails "because `openmpi` requires `ucx@>=1`
    and the formation omits a `ucx` layer." `openmpi`'s recipe declares
    `runtime_requires` on `ucx@>=1`, `hwloc@>=2`, `pmix@>=5` and `libfabric@>=1` —
    **all four shipped as recipes, none listed in the formation.** Stage 4 returns on
    the first unsatisfied requirement, so the error message names `ucx` and the row
    reproduced the error message as if it were the finding.

    This is rule 10 again, in its narrowest form: the *set* the claim quantified over
    was "requirements stage 4 reported", not "requirements unsatisfied". Enumerated by
    probe against `spec.BaseCapabilities.SatisfiesRequirement` — the same predicate
    `stage4ValidateGraph` uses, so the probe cannot disagree with the resolver about
    what satisfaction means. `glibc` is satisfied by base capabilities, evidenced by
    the observed error naming `ucx` rather than `glibc` even though `gcc`'s `glibc`
    requirement is iterated first.

49. **The `pending-initial-build` placeholders get a register row against X1 (#46),
    for the first time.** They had been recorded only in §4.1's list of "open issues
    in the same category", which is precisely the silent state item 27 established the
    register to prevent — a defect described in prose with no row to move. The row
    states what the placeholders do and do not do: the resolver warns
    (`internal/resolver/stages.go:71-75`, the deliberate outcome of #49) and never
    propagates either field into a lockfile (`stages.go:409-413` copies
    `FromFormation` alone), so nothing downstream can act on them. **They are a trust
    defect and not a liveness defect**, which is worth having written down, because a
    static grep for the constant invites the opposite conclusion and this review drew
    it before tracing the field.

    **And adding a row obliges extending the proposition it refutes.** Items 47 and 49
    each add a register row against X1, which took X1 from three rows to five — while
    X1's own evidence cell still opened "Four instances." and enumerated `(a)`–`(d)`.
    The count was inherited and was honest when written; adding rows beneath it is what
    made it wrong, and nothing mechanical objected because the count lives in prose that
    no derivation reads. `validated_on` and the placeholders are now `(e)` and `(f)` in
    that cell, and the opening count is **deleted rather than corrected** — the register
    rows are the source and `Status` derives the total, per the ruling behind rule 11.
    The lettering deliberately does not claim to map onto rows: `Profile.Instance` and
    `Profile.Storage` are two instances under one issue, which is the row-for-case
    substitution this review has made before.

50. **§0.2's headline is re-derived: twenty of thirty-six.** Two rows added, both
    `H1` with no §1.1 capability, and the `awk` in §0.2 moves 18 → 20 as the row count
    moves 34 → 36.

    Note for anyone comparing against the record, because two similar figures are in
    play and the resemblance is a trap. Item 30 corrected a review figure of "20 of
    32" by reporting that at the first population the register held 32 rows, "of which
    18 mention H1 and 16 exercise no adversary capability." Those are two different
    counts, and **§0.2's headline is the second one** — its `awk` requires `H1` present
    *and* no `A1`–`A7`. Run against each historical version rather than extrapolated —
    `git show "<ref>":"PROPERTIES.md" | awk '<the §0.2 command>'`, quoting the ref
    because zsh reads a bare `$ref:P...` as its realpath modifier and silently prints
    `0 of 0`:

    | ref | rows | no-capability | mentions H1 |
    |---|---|---|---|
    | `d9d1b76^` (first population) | 32 | **16** | 18 |
    | `d9d1b76` (X2's row added, item 27) | 33 | 17 | — |
    | `ff5cd80` (#112) | 34 | 18 | — |
    | here | 36 | **20** | — |

    So the present "20 of 36" shares a numerator with the original non-reproducible
    "20 of 32" by coincidence, and is not a vindication of it.

51. **§2.1 rule 10's third instance now names #109 as its correction.** The instance
    described the #92 `CHANGELOG.md` entry's consumers-for-producers substitution; the
    entry has since been rewritten. Under the citation-tense rule (item 35) a
    historical instance stays in the past tense, but naming the correcting artifact
    makes it checkable rather than merely asserted.

52. **R1 restated: `TOO STRONG` → `SOUND`, and its status falls from `ENFORCED E1`
    to `UNPOPULATED`.** The old form demanded byte-identical lockfiles from two
    identical resolutions. A lockfile records `ResolvedAt` from the wall clock, so
    that fails on input one, and satisfying R1 literally meant abandoning a field
    Strata deliberately records. The new form elides exactly one enumerated field,
    `resolved_at`, and enumerates it **here** rather than deferring the elision set
    to the specification (rule 9).

    *What the old statement permitted that this one forbids.* An unsatisfiable
    proposition generates no checkable obligation, so what stood in for R1 was
    whatever its citation happened to assert.
    `internal/resolver/resolver_test.go:574 TestEnvironmentID_Stability` compares
    `EnvironmentID` and `ProfileSHA256` — two derived hashes, the first over the five
    members of `envHashInput` (`spec/lockfile_hash.go:16-22`). Every field outside
    that hash was free to differ between two identical resolutions with nothing
    objecting: `profile_name`, `strata_version`, `rekor_entry`, `bundle`,
    `mutable_layer`, `mount_order`, `satisfied_by`, `from_formation`.

    *What can now refute it that could not before.* A differential resolve comparing
    the full canonical serialisation with `resolved_at` elided. Under the old form no
    such test could be R1 evidence, because an honest attempt fails on the timestamp;
    under the new form it is the direct instrument, and it is cheap. `Basis` is
    deliberately `none`: the cited test does not exercise the restated property (rule
    6), and the old `ENFORCED E1` was measuring a *repaired* form — "identical on the
    canonical content projection" — which deferred the word *canonical* to the
    implementation, which is rule 9 again.

53. **R2 restated: `TOO WEAK` → `SOUND`; it stays `REFUTED`.** The old form carried
    the spec's own exception clause, *"except in fields whose specification says they
    record declaration order"* — the second instance of the rule 9 pattern. The new
    form names its own tie-break (name, then version, then digest) and grants no
    exception.

    *What the old statement permitted that this one forbids.* One sentence added to
    the specification exempted `mount_order`, which is what decides OverlayFS
    shadowing — so YAML key order could determine which layer's copy of a file a user
    sees, and R2 would call it conformant.

    *What can now refute it that could not before.* A permutation generator over
    profile key order. Under the old form that generator **confirms** R2 no matter what
    the implementation does, because any order-dependence it finds is in a field whose
    specification can be said to record declaration order. Under the new form the same
    generator is a refutation instrument, and it already has a counterexample:
    `sort.Ints(queue)` at `internal/resolver/stages.go:301` breaks `MountOrder` ties by
    slice position (#95).

54. **R6 withdrawn, not restated — the attempt to restate it is what retired it.**
    R6 said `EnvironmentID` is a function of *exactly the fields the specification
    enumerates*, in both directions. Removing the deferral means stating both conjuncts
    in terms of environment content, and both are already propositions here: conjunct
    (a) becomes **X2** (*a behaviour that can alter the assembled environment
    participates in the identity*) and conjunct (b) becomes **R7** (*two lockfiles that
    assemble the same environment have equal `EnvironmentID`*), verbatim. The
    enumeration was the only thing giving R6 content of its own.

    R6 was in fact redundant from the moment R7 was added — item 2 of *Propositions
    restated or added* above already recorded that "R6 and X2 constrain only the
    direction behaviour → identity", which is the observation, one step short of the
    conclusion. Withdrawal follows the I1 precedent: statement and verdict are left
    standing as the historical record and the retirement is expressed in `Basis`.

    *Why withdrawn rather than narrowed*, which is the question the restatement
    criterion decides. One restatement does keep R6 independent: move the enumeration
    into this document — *the ID is a function of exactly these five fields*,
    `spec/lockfile_hash.go:16-22`. That form is sound, and falsifiable **only by drift
    between the list here and the struct there** — two representations bound to each
    other, which is what rule 11 and §6.1 exist to stop counting as evidence. It would
    render `ENFORCED` while asserting nothing about whether either representation is
    right. A proposition whose only refuting instrument is a consistency check against
    itself has not moved, so it is retired instead.

    Both of R6's citations transfer rather than lapse, and neither ever tested it:
    `spec/spec_test.go:542 TestEnvironmentID` asserts that `RekorEntry` does **not**
    change the ID, which is R7's direction; `spec/packages_test.go:208
    TestEnvironmentIDIncludesPackages` witnesses that `Packages` *do* participate,
    which is X2's. R6 held `ENFORCED E1` on two tests of two other propositions.

55. **The distribution caveat gains its converse, and the header is re-derived.**
    Item 46 recorded the caveat's first demonstration — rows moving discharged → live
    with the totals unmoved. This entry is the other direction: three propositions
    restated truthfully, no change to any code, and `ENFORCED E1` falls 4 → 2 while
    `REFUTED` does not move at all. Measured on both sides —
    `git show origin/main:PROPERTIES.md` into the tree and `go run ./cmd/propgen`, then
    the same on this branch. A falling `ENFORCED` count is the instrument working, so
    the totals are not a direction of travel either way.

    This is also the evidence against the standing worry that restatement is
    *narrowing until satisfied*. If these three had been narrowed to fit, the count of
    enforced propositions would have risen. Not one of the three came out satisfied:
    R1 lost its only citation, R2 stayed refuted, R6 was retired.

56. **`go run ./cmd/propgen` reporting "no drift" is narrower than it sounds, and
    said so falsely once.** propgen compares authored `Status` cells against the
    derivation. It does **not** read §0's header sentence, so with the R-group changes
    in place it reported `no drift` while the header still stated the previous
    distribution. `TestPropertiesDistributionMatchesHeader`
    (`internal/propdoc/propdoc_test.go:287-301`) is what caught it, by flattening
    whitespace and requiring the document to contain each derived count as prose. Rule
    10 applied to an instrument rather than a query: the scope of a checker is part of
    its clean bill, and `go test ./internal/propdoc/` is the gate here, not propgen
    alone.

### 2026-08-21 — R7 gets a generator, and two counterexamples the register did not name

57. **R7 has a search, and the `Basis` column stays `none` on purpose.**
    `spec/environment_id_r7_fuzz_test.go` asserts R7 over an enumerated set of
    environment-preserving transformations. The enumeration is the whole design
    decision: R7 quantifies over *pairs* of lockfiles, so a generator needs a supply
    of pairs that provably assemble the same environment, and the one source it must
    not consult is `envHashInput` — asking the code under test which fields matter is
    what made the retired R6 unfalsifiable (item 54). So each transformation carries a
    specification-level reason recorded beside it, and the reason, not the field list,
    is what makes the file a test of R7 rather than a restatement of the hash input.

    46,546,540 executions found no failing input, and that buys a tier for **R7
    restricted to that domain**, which is not R7. Four classes of environment-
    preserving pair refute R7 as stated, so they are excluded from the live set — a
    target that fails on every input searches nothing — and citing `E1` in `Basis`
    would be precisely the overclaim §2.1 exists to catch. The exclusion is recorded
    in the document and in the code, not silently avoided.

    **Amended 2026-08-22 (#148, item 81):** three of those four classes no longer have a
    control in that file, and one of them is no longer a class — #117 was fixed by #147. The
    count in this paragraph is the count as it stood on 2026-08-21.

58. **The exclusion list cannot outlive the defects it describes.**
    `spec/environment_id_r7_exclusions_test.go` holds one control per exclusion,
    each asserting the spurious distinction **still reproduces**. Those tests fail when
    a defect is *fixed*, and the failure message is the instruction: move the
    transformation into the live set, delete the control, update the rows here. This
    answers a hazard the register has no other guard against — a scope limit that was
    honest when written and quietly becomes a lie, which is the shape of the count
    deleted in item 51 and of the `Yes` rows §6.1 audits.

    The same file is the control for the fuzz target's machinery, by construction
    rather than by intention: it uses the same clone-and-compare path. If `clone` were
    shallow, or `EnvironmentID` returned a constant, or the comparison were inverted,
    the fuzz target would pass on every input and read exactly as it does now — and
    these five tests would fail. A generator with no such control cannot distinguish
    holding from being unable to see.

    **Amended 2026-08-22 (#148, item 81):** *one control per exclusion* and *these five tests*
    were both true when written and are neither now. Two controls remain, both #95, and the
    machinery-control argument in the paragraph above rests on those two — which is why they were
    kept rather than deleted with the others.

59. **Two counterexamples filed, and they run in opposite directions.**
    **#117** (R7): a package set's inner `Packages` slice hashes differently when
    `nil` than when empty, because `spec/packages.go:49` lacks the `omitempty` the
    outer field has. Same environment, different identity — `59cc9349…` versus
    `57620b0c…`. **#118** (X2): `LockFile.Defaults` decides the contents of
    `/etc/profile.d/strata-defaults.sh` (`internal/overlay/overlay.go:164-182`) and is
    not in `envHashInput`, so three lockfiles that load no module, `python/3.11.9` and
    `python/3.9.18` all hash to `09f451dc…`. Same identity, **different environment**.

    The asymmetry is the finding, not a presentational note. #118 is the class that
    matters more — a cache or an attestation keyed on `EnvironmentID` can vouch for
    the wrong environment — and **R7's generator structurally cannot find it.** R7
    asserts equal-environment ⇒ equal-ID; #118 is unequal-environment ∧ equal-ID. No
    amount of searching R7 reaches it. A generator for X2 has to run the enumeration
    the other way: find the fields that reach the assembled root and check each
    arrives at the hash. `Defaults` appeared nowhere in this document until today,
    which is what an unenumerated direction costs.

60. **Two register rows named fewer propositions than the prose did.** #69
    (`OnReady` hashed and executed nowhere) and #98 (`packages:` installs ignore the
    recorded `sha256`) are both R7 counterexamples, and neither row said so; #69's
    named X1, #98's named I6, P3, X1, X2. R7's own evidence cell has cited #69 as its
    demonstrating instance since R7 was added. So the attribution lived in prose while
    the table that **derives** R7's status did not carry it, and R7 rendered `REFUTED`
    off one row when four applied.

    This is item 54's borrowed-evidence finding read from the other end. There, R6
    stood `ENFORCED E1` on two tests that measured other propositions; here two
    refutations were attributed in prose to a proposition the register did not connect
    them to. Both are the same missing instrument — nothing checks that a citation
    supports what cites it — and #105 remains open for it. Neither row's `Discharged`
    cell was touched: this changes which propositions a counterexample is recorded
    against, not whether it is answered.

61. **A fuzz target's characteristic failure is to pass without searching, and it
    happens two ways.** Both were live here and both are now checked.

    *The seeds reached three of the five transformations.* `go test ./spec/` runs the
    seed corpus only, so the ordinary run was asserting R7 over three fifths of its
    stated domain and reporting `PASS`; the two that declined every seed were the only
    two with guards, needing two layers and a two-entry `Env` respectively.
    `TestR7SeedsReachEveryTransform` failed on its first execution and named both. It
    also exposed that `permute-layers` composed a rotation with a reversal, which for
    two layers **is the identity** — it returned "permuted" having permuted nothing,
    and compared a lockfile against a copy of itself. Both branches are now provably
    non-identity, and `apply` returning false now means *nothing changed* rather than
    *nothing was available to change*.

    *`go test -fuzz` exits 0 when the pattern matches no target*, printing
    `testing: warning: no fuzz tests to fuzz` and then `PASS`. Measured. A renamed
    target would leave the nightly job green indefinitely while searching nothing, so
    `.github/workflows/fuzz.yml` asserts non-vacuity **before** it reads the search
    result — on that warning, on a missing exec count, and on an exec count below a
    floor — because a vacuous green outlasts a crasher, which gets fixed. This is the
    same shape as item 52's absent CI run: not a check that cannot fail, an **absent**
    check that is visually identical to a passing one.

62. **The distribution did not move, for the third time.** Two counterexamples added
    and two rows re-attributed — 36 register rows to 38 — and `go run ./cmd/propgen`
    reports the same distribution as before, because R7 and X2 were both already
    `REFUTED`. The movement is visible only in the `(n of m live)` suffixes: R7
    `REFUTED` → `REFUTED (4 of 4 live)`, X2 `REFUTED` → `REFUTED (2 of 2 live)`. §0's
    header sentence required no edit, which is the correct behaviour and also the
    reason item 46's caveat is stated beside the number rather than left to a reader
    watching totals.

### 2026-08-21 — the enumeration paid four more times, and turned on the generator that prompted it

63. **The sweep that #118 suggested found four more defects, and one of them was in
    the instrument.** #118 was found by writing down `envHashInput`'s five members
    and noticing `Defaults` was not among them. Asking the bounded version of that
    question — *which `LockFile` fields are named by no proposition here?* — produced
    #120 (`ProfileName`/`RekorEntry`), #122 (`InstallLayout`/`Name`/`Version`), #123
    (the godoc), #124 (the registry key), and #121 (the scope clause both R7 and X2
    are missing). The method was `awk` over the struct plus `grep` for each field
    name across the assemblers, and it is worth naming as distinct from search: a
    generator explores a domain someone already stated, and enumeration is how the
    domain gets stated. R7's generator could not have found any of these, because
    R7 asserts the converse implication.

64. **Two of the generator's five transformations were unsound, and the unsoundness
    was inherited rather than invented.** `vary-attestation` mutated `RekorEntry`
    and `vary-identity-and-timing` mutated `ProfileName`, each with a `why` string
    paraphrasing `spec/lockfile_hash.go:11-13`: *"they do not affect what runs"*.
    Both fields are written into `/etc/profile.d/strata.sh`, `/etc/strata/environment`
    and the child process environment of `strata run`. So R7's premise —
    *environment-preserving* — was false for those pairs, and the target asserted
    that the identity **must not** change across a change that alters the
    environment. That is not a weak test; it is a test asserting the negation of X2.
    The R6 lesson was *do not let the implementation define the domain*, and the
    lesson was applied one level too shallow: the domain was taken not from
    `envHashInput`'s membership but from a comment sitting beside it, which is the
    same deferral wearing prose. **The check that would have caught it earlier:** for
    each transformation, grep the field it mutates against every consumer that writes
    into the assembled root — the same enumeration as item 63, run per transformation
    instead of per field.

65. **The reachability guard caught the fix breaking a transformation, which is what
    it was for.** Removing `RekorEntry` left `vary-attestation` mutating only
    `Bundle`, which `buildLockFile` leaves empty and `s.str()` returns empty for on
    an exhausted stream — so the transformation correctly reported "nothing changed"
    and fired on zero seeds. `TestR7SeedsReachEveryTransform` failed and named it.
    The repair was the same one `permute-layers` needed: a rewrite that is *provably*
    non-identity — append a byte, so the result is always one longer than the input —
    rather than a redraw that may coincide. Two of five transformations have now
    needed that repair, which suggests "declined" and "no-op" are easy to conflate
    and the guard is load-bearing rather than decorative. Post-fix reachability
    across 7 seeds: `permute-layers` 2, `rebuild-env-map` 2, `vary-bundle` 7,
    `vary-provenance-and-timing` 5, `vary-requires-host` 7.

66. **R7 is vacuous and X2 is trivially refuted, both for one reason, and neither
    verdict currently means what the table implies (#121).**
    `internal/overlay/overlay.go:207-211` marshals the whole lockfile into
    `/etc/strata/active.lock.yaml`, inside the assembled root. Read literally: every
    field alters the assembled environment, so X2 is refuted by all nine
    non-participating fields for a reason unrelated to any recorded defect, and R7's
    premise — *two lockfiles that assemble the same environment* — is satisfiable
    only by byte-identical lockfiles, making R7 vacuously true and the 10,943,695
    executions measured on the corrected domain a search of an empty set. The
    intended scope (mounted content plus exported process environment, excluding the
    provenance record) is what separates `STRATA_PROFILE` — in scope, a variable a
    process reads — from `ProfileName`'s copy inside `active.lock.yaml`. That scope is
    an assumption of the generator's file and a statement in no document. **The
    exclusion cannot be blanket, and that is the part worth keeping:**
    `cmd/strata/cache_prune.go:35` defaults `--lockfile` to `active.lock.yaml` and
    **deletes** every cached `.sqfs` whose digest is absent from it. The provenance
    record is a live input to a destructive command. It is safe today only because
    the single field it reads, `Layers[].SHA256`, happens to be in `envHashInput` —
    so the scope clause has to be per-field, and a new reader of `active.lock.yaml`
    depending on an unhashed field silently widens the environment.

67. **The identity is a storage key, which turns every collision in this class from
    ambiguous into destructive (#124).** Tracing all ten non-test call sites of
    `EnvironmentID()` — a `grep` that should have run the day X2 was written — shows
    it is not only compared but used as an S3 object key, a DOI description, an OCI
    `revision` label and an EC2 tag. `PutLockfile` writes `locks/<id>.yaml` with an
    unconditional `PutObject`, so #118's three colliding lockfiles cannot coexist:
    measured against `LocalClient`, `locks/` holds one file after two puts and the
    survivor is the second writer's. Because the returned URI is what the caller is
    told to tag an instance with, **which environment a machine boots depends on
    publish order.** The same function accepts an unfrozen lockfile from
    `pkg/strata.Client.UploadLockfile` — whose `EnvironmentID()` is `""` — and writes
    it to `locks/.yaml`, where `cmd/strata/publish.go:73` guards exactly that case
    for the CLI. Guard at one call site, hazard at the other: the relocated-rule
    shape, and the reason the guard belongs in `PutLockfile`.

68. **A conditional put is worth more than a verdict here.** #118, #120 and #122 all
    turn on a hard question — putting the missing fields into `envHashInput` changes
    every published identity, and #121 says the propositions lack the scope that
    would make either answer checkable. None of that blocks the cheap fix: an
    `IfNoneMatch` put that errors when the key exists with different content makes
    every collision in the class loud at publish time, and would have surfaced #118
    years before a property document did. Recording it because the review standard
    rewards refutations and this is the first place where the useful output was a
    mitigation that decides nothing.

### 2026-08-21 — the ladder ranked technique, and the strongest evidence had no rung

69. **`Basis` records a pair, not an ordinal, and §2 is rewritten around it.**
    Coverage ∈ {`asserted`, `chosen`, `sampled`, `exhaustive`} × Subject ∈
    {`implementation`, `model`}. The old E0 < E1 < E2 < E3 ranked by *technique* —
    did you use a generator, did you build a model — where what matters is what was
    *established*. On the grid it is one path that switches subject at the top step:
    `asserted` → `chosen/implementation` → `sampled/implementation` →
    `exhaustive/`**`model`**. Three of the seven bases therefore had no rung, and
    the strongest of the three, `exhaustive/implementation`, could only be filed as
    E1 or E2 — *beneath* an exhaustive walk of an abstraction — because the top rung
    had spent itself on a change of subject. The tell was the perverse ordering: an
    exhaustive walk of a declared domain filing below a sampled walk of an
    undeclared one. Filed as #130 off the #129 enumeration, which produced exactly
    that result and had no honest cell to sit in.

    Seven bases, not eight: `asserted` takes no subject, because where nothing was
    executed there is no artifact the evidence was about. The collapse is stated in
    §2 rather than left to be noticed, since a reader who counts the dimensions
    expects eight — and it is what makes `ASSERTED (E0)` the only rendering asserted
    coverage can produce, so `propdoc.Kind` needed no change.

70. **The pair is strictly stronger than the ordinal in one direction and
    incomparable in another, and §2 now says which.** `exhaustive/implementation`
    beats `exhaustive/model` on faithfulness and is **incomparable to it on domain
    size**: a bounded model state space can be orders of magnitude larger than any
    implementation domain that can be enumerated. That is precisely why no single
    number could hold #129. §2 states the total order triage wants *and* the four
    places it ranks two bases the evidence does not — E2 vs E3, the pair above,
    differing declared bounds, and the model subjects against `asserted`. A column
    that cannot be totally ordered is more useful than one that can be and lies
    about it.

    Recorded honestly: that half is a **note, not a check**. Nothing in this
    repository sorts propositions by basis, so the incomparability list cannot
    fire, and §2 says what has to happen the first time something does — the order
    and the list move into `internal/propdoc` together, and the list gets the guard
    that makes it non-vacuous, namely that every pair it names must be one the order
    actually ranks.

71. **Removing the ordinal made three rules ill-formed, which is why they moved
    with it.** Rule 1 was *"E3 without a faithfulness argument is E0"* — it demoted
    a real result about a real artifact to "nothing was executed". It now says a
    `model` subject does not transfer and a faithfulness argument does not rewrite
    the subject: it *permits* recording an unargued model check as
    `exhaustive/model`, and *forbids* reading an argued one as evidence about the
    implementation. Rule 11 and §4's preamble said **"at E1 or better"**, and under
    the old ladder E3 was better than E1 — so a discharge could have been claimed on
    an exhaustive check of a *model*, declaring a counterexample in shipping code
    gone without executing that code. No row was discharged that way; the ordinal
    permitted it, which is enough. Both now read *coverage `chosen` or stronger,
    subject `implementation`*, which is well formed because coverage alone **is**
    totally ordered. Rule 8 and §3's legend: "tier" → "basis".

72. **§2's own availability paragraph had been false since the day a fuzz target
    merged, and the paragraph existed to prevent exactly that.** It asserted *"there
    are no fuzz targets … no proposition may claim E2 until one exists"*, quoting two
    commands. Re-run, the first now prints
    `spec/environment_id_r7_fuzz_test.go:445:func FuzzR7NoSpuriousDistinctions`.
    Item 14 of the first population added that paragraph so the tier could not be
    "mistaken for available"; item 57 added the target; nothing connected them. A
    section that asserts a technique's absence is a site the technique's arrival has
    to update, and this is the relocated-rule failure committed by the author of the
    rule against it. Replaced with the commands and their real output. R7 still
    claims no basis, for the reason rule 2's second clause now states.

73. **Rule 2 gains the R7 finding as a rule: a bound whose premise nothing in it can
    satisfy makes the coverage `asserted`, whatever ran.** 46,546,540 executions,
    green, over a domain in which no two *distinct* lockfiles can assemble the same
    environment at all. An empty search space and a clean search are
    indistinguishable in every number a run reports. So a `sampled` or `exhaustive`
    claim states not only its bound but that the bound is **satisfiable** by
    something the generator can produce — a question distinct, for an
    implication-shaped property, from whether the transformation fired and whether
    it was non-identity.

74. **What fires, and what a green run here would not have caught.** No Basis cell
    changed and no Status cell changed: E0–E3 stay canonical, `propgen` reports no
    drift, and the distribution is identical, which is the evidence that this is a
    change to what the column *means* and not to what it *says*.
    `internal/propdoc/basis_pair_test.go` checks §2's seven-basis table against the
    parser's registry in **both** directions — a spelling the document defines and
    the tool rejects breaks an author who followed the document; one the tool accepts
    and the document never defines is a basis nobody can look up, and only the first
    is loud on its own. It states its own cardinality (7) rather than deriving it
    from either side, per the rule that a generated table's cardinality is stated and
    only its balance derived.

    And it closes a hole the pair notation opens: the inherited distribution test
    names **five** status kinds by hand, a bare list reads as the complete set, and a
    pair-spelled cell with no legacy name derives a sixth that the prose would omit
    with nothing objecting. The new test asserts the derived direction — whatever
    kinds the register produces, the document states them — with a control proving
    the search can fail.

75. **Nine mutations, and the ninth was void in a way the #129 rule does not cover.**
    The suite passed on its first run, so each check was probed by breaking what it
    is about, with the failing check predicted first: drop a documented row, drop a
    registry entry, misname a legacy tier, spell a pair inconsistently, break the
    table header, stop canonicalising, revert `DeriveStatus`'s lookup, delete a kind
    from the prose, and state a kind nothing claims. All nine went red, and the tree
    was verified clean after each. But the canonicalisation mutant **did not
    compile** (`declared and not used: k`), and `go test` exits 1 for a build failure
    exactly as it does for a detected defect — so the first pass recorded it as red
    with zero failing tests, and 8 of 9 nearly went into the record as 9 of 9. #129's
    rule was *assert the patch applied*; the patch did apply. **The rule needs its
    second half: assert the mutant builds.** Re-run as `k.Canonical()` →
    `k.Spelling()`, which compiles, it produced the predicted 8 errors in the
    canonicalisation test.

76. **R4 and R5 get executable bases, and they get *different* ones — the first cells
    to use the pair notation for something the ordinal could not have said** (#133).
    Both had
    stood `SOUND` on a Basis of `none` since this document was written: asserted here,
    executed nowhere, refuted by #67 in prose alone.
    `internal/resolver/provider_matrix_test.go` now calls the shipping stage functions
    directly (it is `package resolver`, so no inherited test file is touched) and
    reproduces both halves with stated boundaries — 12 of 72 cells for R5, 3 of the 12
    reachable orders of 24 for R4. **R5 is `exhaustive/implementation`; R4 is
    `chosen/implementation`**, and rule 2 is what separates them: R5's axes are
    enumerated from `SatisfiesRequirement`'s own inputs (a `Provides` list and a
    `Requirement` whose only version fields are `MinVersion` and `MaxVersion`), so the
    constraint dimension is complete rather than sampled, whereas R4's four-layer graph
    is a fixture the test picked and only *input order* is exhausted — an exhaustive
    walk of a domain the walk itself declared, which rule 2 says establishes the
    narrower claim and not the wider one. Under the retired ladder both would have been
    filed `E1` and the difference would have been invisible.

    **Three findings the enumerations produced that the prose did not have.** (a) R5's
    defect **masks** R4's: stage 4 rejects the orders that would most readily expose
    stage 6's wrong edge, exactly half of them, so fixing the first half widens the
    second half's observable violation set — recorded in §3 beside R4/R5. (b) R4 is
    **unobservable in a three-layer graph**, which is the likeliest reason it had no
    executable basis for so long: with one consumer and two providers the consumer
    always sorts last, so the wrong edge produces the right order. It takes a fourth
    layer, blocking the *satisfying* provider behind a dependency of its own, before
    the property has a falsifying instance at all. (c) Stage 5 permits the domain only
    because `InstallLayout: ""` counts as versioned in `canCoexist`; if it did not,
    R4's table would be vacuous with every number in it green, so the premise is
    asserted per rule 2's satisfiability clause rather than assumed.

    **Six mutations, each with its failing test and its failing *count* predicted
    before the run, all six red.** The counts are what the predictions are worth:
    keeping stage 6's first provider moves R4's 3 to 0; reversing its tie-break moves
    it to 9 (derived from an independent model of Kahn's queue, not from the
    implementation — and the first prediction was **wrong at 4** because the model
    assumed FIFO where `sort.Ints(queue)` gives min-index, caught by reading the rest
    of the function before writing the test); scanning every same-name provider in
    stage 4 moves R5's 12 to 0; disabling the layer check empties the reachable set;
    swallowing `errUnsatisfiedRequirement` breaks totality; and tightening
    `canCoexist` makes R4's domain unreachable, which the table reports as *unreachable*
    rather than as zero violations. The tie-break mutation had to be re-cut as a call
    *swap* rather than a deletion: removing `sort.Ints(queue)` orphans the `sort`
    import, and per item 75 a mutant that does not compile is **void, never red**.
    That guard fired here on its first use after being written down, which is the
    difference between a note and a check.

77. **T1 and T5 cite #129's enumeration, and doing so found a hole in the `Basis`
    column itself: it cannot say *which execution path* was covered.** §3 defines the
    cell as *the strongest basis claimed and the citation carrying it*, which is a
    max over the evidence. `internal/agent/boot_matrix_test.go` enumerates all 112
    cells of the **agent boot route's** decision surface — an
    `exhaustive/implementation` result — while `cmd/strata/run_verify_test.go` covers
    the **`strata run`** route at `chosen/implementation`. T1 quantifies over *every*
    execution path from *declared in a lockfile* to *mounted*. So taking the max
    silently reports the agent route's strength as the proposition's, and the reader
    cannot tell that the other route is weaker. **The max is the wrong operation for a
    proposition whose domain is a union of paths, and the failure is invisible in
    exactly the direction that matters: it overstates.** Both cells therefore name the
    route beside the pair, which §2.1 rule 2 already required — the declared bound is
    part of the claim, and *which path* is a bound as much as *which inputs* is. Filed
    as #135 rather than fixed here, because the fix is either a per-route Basis or a
    rule that a route-scoped claim states its route, and that is a schema decision for
    `internal/propdoc`, not a cell edit. The parser is what forced the shape: it
    reads the tier as the first space-delimited token, so a cell cannot begin
    `exhaustive/implementation, agent route only` — the scope has to follow the
    citation, and that is the better place for it anyway.

    **A second limit, specific to T1, and it is rule 9 resurfacing.** The matrix's
    expectation ladder makes a boot with **no** verifier a refusal, so what it
    measures is T1 under the strengthened reading rule 9 prescribes — quantified over
    the *default* configuration — and not T1's literal text, which a fail-open still
    satisfies. That is precisely why T1's verdict stays `TOO WEAK`: the evidence for
    it is stronger than the proposition it is filed under, and 20 of the 112 cells
    reach the mount with nothing verified while T1 as written has no complaint. Those
    cells carry `knownOpen` and assert today's behaviour, so they fail when #93(a)
    closes instead of certifying the hole they exist to measure.

    **What T5's enumeration does *not* reach, stated because T5's own text reads like
    a domain.** T5 lists *absent tool, absent key, absent bundle, absent log entry,
    network failure, unparseable material*. The table covers absent bundle
    (`Bundle: ""`, and `(nil, nil)` bytes for a named one), network failure (fetch
    error), and unparseable material (non-JSON, and well-formed JSON with the wrong
    `mediaType`), and it adds one the list omits — a valid bundle over *different*
    content, which is a substitution rather than an inability. But **an absent cosign
    binary and an absent Rekor log entry are not dimensions of this table**
    (`grep -in 'rekor\|cosign' internal/agent/boot_matrix_test.go` returns one hit, a
    string literal used as non-JSON bytes), so those two clauses of T5 remain at
    `chosen` on the routes already cited. No status changes: both propositions keep a
    live register row on #93, so rule 2 of the status function dominates, and the
    distribution is unchanged.

78. **Rule 11 gets run against the rows that were already in the table, and two of the
    eight `Discharged: Yes` cells read verbatim the string it forbids** (#137). Rule 11
    was added the day before — *"'Closed completed' is not a citation"* — replacing an
    older *"at E1 or better"*. It was written to govern rows added after it, and nothing
    re-read the rows already there. Measured on `cc6e0c4`, this branch's parent for the
    register edits, because the query no longer returns anything after them:

    ```
    $ git show cc6e0c4:PROPERTIES.md | awk -F'|' \
        '/^\| /{c=$6; gsub(/^ +| +$/,"",c); if (c ~ /^Yes/) n++} END{print n}'
    8
    $ git show cc6e0c4:PROPERTIES.md | awk -F'|' \
        '/^\| /{c=$6; gsub(/^ +| +$/,"",c); if (c ~ /^Yes/ && c !~ /`/) print NR": "c}'
    685: Yes — closed completed
    686: Yes — closed completed
    ```

    Two of eight, both on T7, both now decided on **re-derived evidence** rather than
    flipped to `No` as bookkeeping — the distinction the audit exists to force, since one
    of them might have been genuinely discharged.

    **#49 is `Partially`.** The warning exists, fires, and names the formation, the
    reason and the value (`internal/resolver/stages.go:71-75`), which is E1 at
    `internal/resolver/formation_attestation_warning_test.go:132`. What the closure did
    not cover is *delivery*: `resolver.warn` is a no-op when `cfg.Warnings` is nil, and
    of the four `resolver.New` constructions in the tree, three pass `os.Stderr` and
    `pkg/strata/strata.go:103-107` passes nothing, with no `Options` field a caller could
    use. Filed as #138. The gap is reachable on the shipped catalog, not hypothetical —
    all six formations still carry `pending-initial-build` (#46).

    **#48 is `No`, and the reason is that the row and the issue are about different
    things.** #48's title is *"strata run should warn when lockfile has packages:
    entries"* and its body is about non-installation; the register row says the entries
    *"are unattested"*. The shipped warning (`cmd/strata/run.go:92-105`) does what #48
    asked and says nothing about attestation, and neither does any other route — the
    agent installs these entries from PyPI/conda/CRAN with no bundle and no Rekor entry
    (`internal/agent/package_installer.go:37-45`). So the row as written is not
    discharged by anything. The attestation gap is #139; the row's wording is #140 and
    travels alone, because rewording a counterexample in the change that discharges it
    is narrowing-until-satisfied.

    **T7's live count moves 1 → 3 of 4, and this is the failure mode the derived column
    was supposed to prevent.** `Refutation.Live()` is `Discharged != "Yes"`
    (`internal/propdoc/propdoc.go:117`), so both cells counted as discharged and §3
    reported `REFUTED (1 of 4 live)` — a false statement, in a trust proposition, sitting
    inside a *generated* cell. Deriving the column removed the drift between §3 and §4
    and could not remove the error, because the error was in the input the derivation
    trusts. A prose rule and a derived column disagreed and the column won silently.
    Distribution unchanged (27 REFUTED before and after): what moved is a count inside
    one cell, which is exactly the class of claim a distribution table cannot see.

    **Two things this cost.** The forecast written before the re-derivation said `2 of
    4`, on the assumption that #48's row would discharge on the new evidence — the
    evidence said otherwise, and the honest number is the one a plain flip-to-`No` would
    also have produced. And the mismatch between a row and the issue it tracks is not
    machine-checkable: `#48` is a valid, closed issue whose *subject* differs from the
    counterexample beside it, so no parser can find the next one. The guard that **is**
    mechanical — a `Discharged` cell beginning `Yes` with no citation — is filed as the
    next change, stated as failing on 2 of 8 rows before these two were fixed.

79. **The mechanical half of rule 11 becomes a check, the unmechanical half gets a
    name, and putting the check in the wrong place was caught by a test written for
    something else.** Rule 11 has been enforceable since the day it was written and was
    not enforced, which is why item 78 exists. `Doc.DischargeDefects`
    (`internal/propdoc/propdoc.go:450`) now reports every `Yes` row naming no basis or
    citing no artifact, `propgen` refuses before it rewrites the Status column
    (`cmd/propgen/main.go:51-57`), and `TestPropertiesRegisterMeetsRule11`
    (`internal/propdoc/discharge_citation_test.go:143`) fails the build.

    **Measured before it was scoped**, on `1a7646f`, because a check whose false-positive
    rate is unknown is a hypothesis:

    ```
    $ awk -F'|' '/^\| /{c=$6; gsub(/^ +| +$/,"",c);
        if (c ~ /^(Yes|No|Partially)/){split(c,a," "); n[a[1]]++}} END{for(k in n) print k, n[k]}' PROPERTIES.md
    Partially 3
    Yes 6
    No 32
    ```

    Six `Yes` rows, **0 failing today** and **2 of 8 failing at `cc6e0c4`** — the check is
    introduced against the population it was built for, and that population was fixed one
    commit earlier, which is why the corpus test is the weaker of the two claims in its
    file. Of the three `Partially` rows one (`I6` / #51) names neither a basis nor a path
    for its discharged half and would be reported; the check is scoped to `Yes` and that
    row is **#142**, because a `Discharged` change travels alone.

    **The placement was wrong and an unrelated test said so.** The check first went into
    `parseRefutation`, where the cell was already being split — placement by convenience.
    `TestPropertiesGuardCanFail` then went red: that test proves the derivation reads the
    register by taking the first live row and flipping its cell to `Yes`, and the row it
    picks is `P4`/#54, whose reopened cell names no basis. A rule-11 guard in the parser
    makes a document containing an unjustified discharge **unreadable** — by `propgen`,
    which is the tool that would fix it, and by any test constructing a hypothetical
    register. `Unknown` already had the right shape: a report over a parsed document, with
    the enforcement in a test. Two things worth keeping: a policy check does not belong in
    a parser, and the control that caught it was written for a different purpose, which is
    the second time in two days that a control outside the instrument caught the
    instrument (§7 item 78's mutant, stopped by the pre-commit gate's named-path staging).

    **Ten mutation probes, each failing set predicted before the run, 10 for 10** — full
    output at `/tmp/mutout_rule11.txt`, restore in a `trap ... EXIT` per item 78:

    | Probe | Mutation | Failing tests |
    |---|---|---|
    | A | scope widened from `Yes` to `Yes`+`Partially` | `CheckDischargeCitation`, `PropertiesRegisterMeetsRule11` |
    | B | basis check condition inverted | `CheckDischargeCitation`, `PropertiesRegisterMeetsRule11`, `DischargeReportRejectsTheRegisterAsItWas` |
    | C | rule 11's two halves checked in the other order | `CheckDischargeCitation`, `DischargeReportRejectsTheRegisterAsItWas` |
    | D | error message reports the 0-based line | `DischargeGuardReportsItsLine` |
    | E | `DischargeDefect.Line` reports the 0-based line | `DischargeReportRejectsTheRegisterAsItWas` |
    | F | citation pattern loosened to any backticked span | `CheckDischargeCitation`, `DischargeCitationPatternIsNotSatisfiedByProse` |
    | G | basis pattern accepts any E-digit | `DischargeBasisPatternCoversEveryBasis` |
    | H | basis pattern loses the three pair-only spellings | `CheckDischargeCitation`, `DischargeBasisPatternCoversEveryBasis` |
    | I | `DischargeDefects` stops at the first defect | `DischargeReportRejectsTheRegisterAsItWas` |
    | J | `DischargeDefects` collects the rows that passed | `PropertiesRegisterMeetsRule11`, `DischargeReportRejectsTheRegisterAsItWas` |

    Two of these are about the tests rather than the guard. **I** is why the fixture
    carries two offending rows: with one, a report that stopped after the first defect was
    undetectable. **H** is a stated bound — `PropertiesRegisterMeetsRule11` does **not**
    fail under H, because all six live `Yes` cells spell `E1` and none uses pair notation,
    so the corpus cannot witness a basis pattern that has lost `chosen/model`,
    `sampled/model` or `exhaustive/implementation`. That is what
    `TestDischargeBasisPatternCoversEveryBasis` is for, and it iterates `Bases()` rather
    than a hand-written list of E-names, which is the only form that covers the three
    spellings with no legacy name.

    **Subject divergence gets its name in rule 11, and the boundary is stated where the
    green is read.** The class: a row marked discharged against a valid closed issue whose
    fix is real and whose closure is correct, where the issue's subject differs from the
    counterexample beside it. Every component true, the row false. Both known instances
    are now tabulated in rule 11 — `P4`/#54 (item 38) and `T7`/#48 (item 78) — and each
    was found by a different accident, neither by a check. No parser can reach it, so
    `TestPropertiesRegisterMeetsRule11`'s green bounds the closure-for-citation conflation
    only. Recorded in rule 11 itself rather than only here, because the place a bound has
    to be legible is beside the thing whose green invites the wider reading.

    **The distribution caveat is restated as a property of the generator.** This is the
    fourth session in which a movement failed to appear in the header's totals (items 46,
    55, 62, and 78's `T7` count). `Distribution` tallies `Kind(status)`
    (`internal/propdoc/propdoc.go:339`) and `Kind` discards parenthesised detail by
    construction (`:348`), so a movement expressible only in an `(n of m live)` count
    **cannot** appear there — it is derivable, not a tally of four coincidences. The block
    quote in the header now says so and says that no future amendment demonstrates it
    again. Four rediscoveries is the cost of recording a trap as a note: a note does not
    fire.
80. **The Basis column was a max, and a max over unevenly covered routes overstates.**
    §3 defined the cell as *the strongest basis claimed*. `T1` and `T5` each quantify over
    every execution path and are covered unequally across them — 112 enumerated cells on
    the agent boot route, example tests on the `strata run` route — so both cells read
    `exhaustive/implementation`, and nothing in the column said which route that came from.
    The overstatement is silent, which is what made it worth a rule rather than an edit
    (#135, filed after #129's PR worked around it by naming the route in prose beside each
    pair).

    **The replacement is a meet, and the correction to the obvious form of it is the point.**
    A min over covered routes is the honest reduction, but a min needs an order and the seven
    bases do not have one: Coverage is totally ordered, Subject is not (rule 1). So the rule
    is **a pair where all covered scopes share a Subject, the set otherwise, over the union of
    the declared bounds** — and a scope known to be uncovered may be declared with `none`,
    which collapses the cell, because a floor over a union says nothing about a part of the
    domain no entry names. Stated as §2.1 rule 12; implemented as `Reduce`
    (`internal/propdoc/basis.go:182`) and `parseBasis` (`internal/propdoc/propdoc.go:582`).

    | Cell | Scopes | Old (max) | New (meet) | Status before | Status after |
    |---|---|---|---|---|---|
    | `T1` | 2 | `exhaustive/implementation` | `chosen/implementation` | `REFUTED (1 of 4 live)` | `REFUTED (1 of 4 live)` |
    | `T5` | 3 | `exhaustive/implementation` | `chosen/implementation` | `REFUTED (1 of 4 live)` | `REFUTED (1 of 4 live)` |

    **The two right-hand columns are the finding about this change's own evidence.** Both
    scoped propositions are `REFUTED`, and a live refutation outranks any basis (rule 5), so
    neither Status cell moves. `go run ./cmd/propgen` reports `no drift` and the identical
    distribution — `ASSERTED (E0) 1, ENFORCED E1 2, REFUTED 27, UNPOPULATED 2, WITHDRAWN 2` —
    before and after. **The document's own green therefore says nothing about whether the
    reduction is right**, and a corpus test asserting today's rows pass would have been
    vacuous in the precise sense: it would pass equally if `Reduce` returned the max. So the
    corpus test asserts the *parsed reduction* and asserts it **differs from the max**
    (`internal/propdoc/basis_scope_test.go:460`), and the renderings are covered by
    constructed cells (`:376`). Cardinality stated: 2 multi-scope cells, 32 single-scope,
    34 propositions.

    **The migration was forced, not offered.** Both cells carried their second route as prose
    after a semicolon, and under the new grammar neither parses — six inherited tests went red
    on the unmigrated document, which is how a half-migrated document is prevented rather
    than discouraged. Recorded as a test in its own right (`:560`).

    **The guard §2 promised in advance came due, and the promise was well drafted.** The
    ranking note said no tool consumed the basis ordering, that the list of pairs the order
    ranks and the evidence does not was therefore "a note, and notes do not fire", and that
    the first consumer would owe a guard making the list non-vacuous. `Reduce` is that
    consumer. The guard exists (`:192`) and the answers are narrower than the promise
    anticipated: `Reduce` consumes the **Coverage** order only and refuses items 1 and 2
    outright; item 4 is *not* refused, because `asserted` is the bottom of the coverage order
    and a floor over coverage is defined for it, while item 4's complaint is about usefulness,
    which `Reduce` does not compute; and **item 3 is not expressible as a pair of bases at
    all** — it is a statement about two cells — so its answer is structural, every entry
    carrying its own scope, enforced separately (`:364`). Three of four ranked pairs
    machine-checked, the fourth stated as inexpressible rather than silently dropped.

    **Nine mutation probes, all nine red, eight failing sets predicted exactly.** The two
    departures are worth more than the eight matches. Probe C ranked `chosen` as high as `exhaustive`
    and I predicted one failing test; three failed, because the tie also disables the
    lead-with-the-reduction rule (the meet equals the first entry, so a strongest-first cell
    is accepted) and because two separate tests assert their own fixtures *discriminate* — each
    row records whether its meet and max coincide and fails if a supposedly discriminating row
    stops discriminating. That assertion, not any expected value, is what caught the probe: a
    `Reduce` returning the max passes every value assertion on any row where the two agree.
    Probe H was **VOID** on its first cut — removing `uncovered ||` orphaned the variable and
    the mutant did not compile, which `go test` reports with exit 1 exactly as it reports a
    caught defect — and went red at the predicted set after being re-cut as a value swap, per
    the standing corollary that a probe should swap an expression rather than delete the only
    use of something.

    `zsh /tmp/mutprobe_135.sh` (baseline green asserted first; patch asserted to apply exactly
    once; mutant asserted to build; tree restored from a `trap`):

    | Probe | Mutation | Predicted | Failing | Verdict |
    |---|---|---|---|---|
    | A | meet becomes max | 10 | 10 | RED |
    | B | subject agreement inverted | 4 | 4 | RED |
    | C | `chosen` ranks as high as `exhaustive` | 1 | 3 | RED |
    | D | weakest-first check disabled | 1 | 1 | RED |
    | E | scope-uniqueness never records | 1 | 1 | RED |
    | F | `@` recognised anywhere, not as a prefix | 1 | 1 | RED |
    | G | scope count dropped from the status | 1 | 1 | RED |
    | H | uncovered scope no longer collapses the cell | 2 | 2 | RED |
    | I | the reported set is built wrong | 2 | 2 | RED |

    Nine probes, seven distinct failing sets, sizes 1 to 10 — output that varies with its
    input, which is the instrument check. The three that coincide are D, E and F, each caught
    by the grammar table alone. Probe A's 10 is the whole package, the shape to expect from
    inverting the reduction itself, since every corpus test parses the document through it.

    **And a defect in the practice, found by this change and not fixed by it: seven
    `path:line` citations elsewhere in this document were invalidated by inserting code above
    their targets, and nothing checks them.**

    | Anchor as written | Target | Actually at | Cited | Broken by |
    |---|---|---|---|---|
    | `propdoc.go:252` | `out[Kind(status)]++` | `:339` | 2× | this change |
    | `propdoc.go:261` | `func Kind` | `:348` | 2× | this change |
    | `propdoc.go:363` | `DischargeDefects` | `:450` | 2× | this change |
    | `propdoc.go:87` | `Refutation.Live` | `:117` | 1× | never correct |

    Seven citation instances across four cited lines, enumerated by extracting every anchor the
    diff removes:

    ```sh
    git diff -U0 4d5ba47^..4d5ba47 -- PROPERTIES.md | grep '^-' \
      | grep -o '[a-z_]*\.go:[0-9]*\|`:[0-9]*`' | sort | uniq -c
    ```

    The range is the repair commit alone, and it has to be: widened to include this change's
    first commit the same command reports twelve anchors, because rewriting the `T1` and `T5`
    cells re-emits every citation inside them as a removal. A diff-based enumeration of
    citations counts *lines touched*, not citations broken, unless the range is the one that
    touched only pointers.

    Six were broken by inserting code above their targets in this change. The seventh,
    `propdoc.go:87` for `Refutation.Live`, was **already wrong when it was written**: at
    `041e50f` `Live()` was at line 95. All are repaired here, and only the
    pointers changed — every claim they support is unchanged, which is why repairing them
    inside §7 is not rewriting the log.

    **This is not a new issue; it is a measurement that contradicts an existing one's
    premise.** #105 already scopes a citation checker and states that its level 1 — the
    path resolves, the line is within the file, a `func <Name>(` sits at the line where a
    test name is given — **finds zero defects today**, and therefore that level 1 is
    insurance against future drift rather than a bug-finder. Five of these seven instances
    are written repo-relative and land inside the file and within its length, and none of
    the seven names a symbol; the remaining two are written bare (`` `:261` ``), the form
    #105 measures as not machine-resolvable at all. So **level 1 as scoped is green on every
    one of them** — silent on five and unable to read two. The class is not a dangling
    pointer but a
    pointer at the wrong valid line, and its base rate is not zero: seven instances in this
    document, six of them created by one commit, in the one file this change happened to
    touch. What separates a checkable citation from an uncheckable one is whether it names
    what it points at, which is why #105's *"make test names mandatory alongside line
    numbers"* is the part that buys the detection — recorded on #105 rather than filed
    again here. (2026-08-22, #135.)

### 2026-08-22 — three controls that could not fail, and one that failed with the wrong instruction

81. **The exclusion list's guard against rot was itself unguarded.** §58 claimed the file holds one
    control per exclusion, each failing when its defect is fixed. #148 probed all 46 assertions of
    that shape in the repo by applying each fix locally and reading the verdict, and four could not
    do the job — three of them in this file. The classes, and the treatment each got:

    | Class | Instance | Failure mode | Treatment |
    |---|---|---|---|
    | A | `TestR7Exclusion_NilVersusEmptyInnerPackages` (#117) | mutated the whole `Packages` field, not nil against empty — true before *and* after #147 | deleted; `TestEnvironmentID_NilAndEmptyInnerPackagesAgree` is the assertion the fix holds |
    | B | `TestR7Exclusion_MutateOnReady` (#69), `TestR7Exclusion_MutatePackageDigest` (#98) | read `EnvironmentID()`; the fixes land in an executor and an installer argv | deleted, not repaired — the observable each needs is named on its issue |
    | C | `assertStillSpurious`'s message (#95 ×2) | goes red correctly, then instructs the reader to apply #95's *refuted* prescription | message repaired; controls kept |
    | D | `knownOpenCells` comment (`internal/agent/boot_matrix_test.go:87`) | claimed a tripwire role; both sides of the check derive from `wantFor()` | comment repaired, constant untouched |

    **Class B is the finding this document has to absorb, because it is not a repairable control.**
    A mutation of a lockfile's identity cannot observe a fix that lands in an installer's argv. There
    is no better mutation; writing one means a different control against a different observable,
    which is a new control wearing the old one's name. So the treatment is deletion plus a **named
    absence** on the issue — and the rule that follows is that *a control aimed at an instrument that
    cannot observe its subject reads as coverage and supplies none*, which no green run can tell you.

    **Class C is the sharper one, and it is not machine-detectable.** *Does it go red* is answerable
    by a probe; *does the red say the right thing* is not. Both #95 controls fail correctly and then
    say *"appears to be FIXED … move this transformation into liveTransforms()"* — widening R7 over
    package order, which `TestEnvironmentID_PackageOrderIsContent` refutes and #95 records as
    refuted. A control that fails correctly and prescribes wrongly is worse than one that never
    fires, because it converts a real signal into a wrong action. The review rule adopted from it:
    **when a control goes red, read what it tells the reader to do.**

    **Class D is R6's error, alive inside the document that retired R6.** The comment claimed the
    constant was the tripwire against the code. Check (5) compares `fired[reasonNoVerifier]` against
    `const knownOpenCells = 20`, and `fired` is built from `c.want` — so **both sides derive from
    `wantFor()`**, the expectation table. It is a consistency check between two representations of
    one claim, refutable only by the two disagreeing, which is exactly the ground on which R6 was
    withdrawn rather than narrowed (item 54) and the ground rule 11 was adopted on (item 79). Fixing
    #93(a) leaves both sides at 20 and check (5) green. The real tripwire is the `knownOpen` branch
    in `runCell` — `Run` refuses where the cell says it boots — and it fires **twenty times**, once
    per cell, each failure carrying the instruction to lower the constant. The treatment is the
    comment, because the constant and the check are both correct for what they actually do: they
    bound the *table*. What is worth recording is the date. This shape was named in items 54 and 79
    on 2026-08-21, and the table carrying it was added the **same day**:

    ```sh
    git log --diff-filter=A --format='%h %ad %s' --date=short -- internal/agent/boot_matrix_test.go
    # a76ef34 2026-08-21 test(agent): enumerate the boot verification decision surface (112 cells)
    ```

    Knowing the pattern did not prevent it, and no green run could have said so. What found it was
    asking, of a check that passes, *where does each side of the comparison come from* — which is
    rule 11's question pointed at an instrument instead of at a register row.

    Two consequences beyond the repairs. The `why` strings these two controls pass are documented as
    *the specification-level reason the environment is unchanged*, and they assert something the
    suite contradicts — so **the R7 half of #95's register row rests on a premise
    `TestEnvironmentID_PackageOrderIsContent` denies**, and a transformation whose premise fails
    belongs in the opposite-reason list beside `Defaults` (#118) rather than among R7's
    counterexamples. That is a refutation-register change and travels alone; raised on #95. And
    #117's fix left three artifacts describing it as live — this register's #117 R7 row, §59's *"two
    counterexamples filed"*, and a transformation that is now neither excluded nor in the live set —
    filed as #150.

    **Two stale citations were created and corrected inside this change**, which is item 80's class
    arriving one item later: `runCell (:513-517)` was already wrong at `origin/main` before the
    branch existed, and `environment_id_scope_test.go:838` was correct when one commit wrote it and
    one line off after the next commit added a line above the function. Both are the *valid wrong
    line* form an existence check reads as green. Both replacements name the symbol they point at —
    #105's prescription — which is what makes them checkable rather than merely correct today.

    Neither the sweep nor this change fixes a defect. Both are apparatus, and the apparatus/defect
    purpose counter does not advance: 46 assertions probed, 3 controls deleted, 10 comment or string
    sites corrected across 4 files — one of them the failure message that prescribed the defect — 0
    lines of shipped code touched, 0 issues closed. The two counts a reader can re-derive on `main`:

    ```sh
    grep -c 'assertStillSpurious(t,' spec/environment_id_r7_exclusions_test.go   # 2, was 5
    grep -c '^func Test' spec/environment_id_r7_exclusions_test.go                # 3, was 6
    ```

    (2026-08-22, #148.)

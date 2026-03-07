# Strata

**Composable, reproducible, cryptographically attested compute environments for cloud-based research.**

---

## The Problem

A researcher asks: *"If I want a full functioning R + LaTeX + Quarto + Pandoc + Git environment
for use with both command line and RStudio in the browser — which template should I build on?
I see r-research-workstation.yml, rstudio-desktop.yml, rstudio-server.yml and
community/ultimate-research-workstation.yml having r-base installed. Right now
r-research-workstation doesn't actually have a version of R installed. And the rstudio-server
template has an old version of R."*

This is a systems administration problem. The researcher should be doing science.

The failure modes visible in that single request:

- Templates have broken software (`r-research-workstation` — R not installed)
- Templates have outdated software (`rstudio-server` — old R version)
- Four overlapping templates with no clear composition model
- Researcher must read YAML files to understand what they get
- A human must be consulted before work can begin

These failures are not unique to one tool. They are structural properties of the
template-and-bootstrap model: templates are static descriptions of install scripts, install
scripts break, versions drift, and no mechanism enforces that what is described is what is
delivered.

Strata is a different model.

---

## The Approach

A researcher declares what they want:

```yaml
name: r-quarto-workstation
base:
  os: al2023

software:
  - formation:r-research@2024.03
  - quarto@1.4
  - pandoc@3.1
  - texlive@2024
  - git@2.43

instance:
  type: r7i.2xlarge
```

The system guarantees:

- R is installed at the version in the formation
- RStudio Server is running in the browser
- Every declared piece of software is present at the declared version
- The environment is identical every time this profile is resolved
- The environment is cryptographically attested and independently auditable

The question *"which template should I build on?"* does not exist in Strata. You declare
what you want. The system composes it.

---

## Core Invariants

These are not design goals. They are enforced properties:

```
1. Same profile + same registry state = identical environment, always
2. Conflicts are build-time errors, never runtime surprises
3. The environment is fully described by the profile; no runtime state matters
4. Every environment is auditable: cryptographic chain from profile → layers → files
5. Failure is loud and early; partial environments never run
```

---

## Concepts

### Layer

An atomic, immutable software unit. A squashfs filesystem image containing a specific piece
of software installed into a clean prefix. Signed with Sigstore at build time. Content-addressed
by SHA256. Once pushed to the registry, never modified.

```
gcc@13.2.0    →  a squashfs containing GCC 13.2.0 installed into /usr/local
cuda@12.3.2   →  a squashfs containing the CUDA 12.3.2 runtime
python@3.11.9 →  a squashfs containing the Python 3.11.9 interpreter
```

Layers are not packages. They are pre-built, pre-validated binary artifacts. A layer built
against `gcc@13` on `al2023/x86_64` is a different artifact than the same software on
`ubuntu24/x86_64`. The registry tracks this distinction.

### Formation

A named, versioned, pre-validated group of layers that are known to compose correctly.
Geological formations are named assemblages of strata that always appear together. The
metaphor is precise.

```yaml
name: r-research
version: "2024.03"
layers:
  - R@4.3.3
  - rstudio-server@2024.09
  - pandoc@3.1
  - r-tidyverse@2.0
validated_on:
  - al2023/x86_64
  - al2023/arm64
```

A formation is conflict-checked and smoke-tested as a unit before entering the registry.
When a profile references a formation, the resolver treats it as a pre-solved subgraph —
the internals are trusted, not re-solved.

### Profile

What users write. A declaration of intent: which software, on which base OS, on which
instance type. Version constraints are expressed as semver prefixes. The system resolves
the rest.

```yaml
name: alphafold3
base:
  os: al2023
  arch: x86_64

software:
  - formation:cuda-python-ml@2024.03
  - alphafold@3.0

instance:
  type: p4d.24xlarge
  spot: true

storage:
  - type: s3
    bucket: my-af3-databases
    mount: /data
```

A profile is a version-controllable, shareable, human-readable description of a compute
environment. It is the artifact researchers share in supplementary materials.

### Lockfile

What the system produces. A fully pinned, cryptographically attested description of exactly
what was resolved from the profile. Contains the exact AMI ID, the SHA256 of every layer,
and the Rekor transparency log entry for each. Machine-generated, never hand-written.

```yaml
profile: alphafold3
profile_sha256: abc123...
resolved_at: 2026-03-06T10:00:00Z
strata_version: 0.1.0
rekor_entry: xyz789...   # lockfile itself is logged

base:
  declared_os: al2023
  ami_id: ami-0abc123
  ami_sha256: def456...
  capabilities:
    family: rhel
    glibc: "2.34"
    kernel: "5.15"
    arch: x86_64

layers:
  - id: python-3.11.9-rhel-x86_64
    name: python
    version: 3.11.9
    source: s3://strata-layers/python/3.11.9-rhel-x86_64.sqfs
    sha256: aaa111...
    rekor_entry: bbb222...
    mount_order: 1
    satisfied_by: "formation:cuda-python-ml@2024.03"
  # ...
```

The lockfile is a citable research artifact. `strata freeze` pins a profile to a lockfile.
`strata publish` mints a DOI via Zenodo. A methods section can cite the environment as
`doi:10.5281/zenodo.xxxxxxx` and a reviewer can reproduce it exactly.

### Registry

The translation layer between user intent (profile) and system artifacts (lockfile). An
S3-backed catalog of signed layer manifests, formation manifests, and base capability probes.
Never touched by users directly.

### Base Capabilities

Every base AMI is probed once and its capabilities recorded: glibc version, kernel version,
OS family, architecture, system libraries. Capability probes are cached by AMI ID. Layers
declare their requirements against capabilities, not OS names — `glibc@>=2.34` not `al2023`.
This means a single layer artifact runs on AL2023, Rocky 9, Rocky 10, and RHEL 9 without
separate builds.

---

## Trust Model

Trust is cryptographic, not assumed.

### Layer signing (build time)

Every layer is signed with cosign immediately after build. The signature bundle and a
Rekor transparency log entry are stored alongside the squashfs in the registry. The Rekor
entry commits the layer's identity to a public, append-only log.

### Layer verification (pull time)

The agent verifies the cosign bundle against the Rekor transparency log before mounting
any layer. SHA256 of the pulled squashfs is verified against the manifest. Unsigned layers
will not mount. This is unconditional — there is no flag to skip verification.

### Lockfile signing

The lockfile itself is signed and logged to Rekor. The Rekor entry is recorded in the
lockfile. The chain is: profile SHA256 → layer SHA256s + Rekor entries → lockfile Rekor
entry. Every element of the environment is independently auditable without trusting Strata
or its registry.

### Trust tiers

```
Tier 0  Strata core     gcc, cuda, python, R, MPI
                        Built and signed by Strata maintainers
                        Strata signing key, Rekor logged

Tier 1  Community       Domain software: AlphaFold, GROMACS, BLAST, PyTorch
                        Recipes contributed via PR, built by Strata CI after review
                        Signed by Strata CI key

Tier 2  Institutional   Institution-built layers, institution signing key
                        Strata verifies signature validity, not identity
                        "bring your own registry"

Tier 3  User/local      strata layer build --recipe myjob.sh --local
                        Signed with user's cosign key
                        For custom code, proprietary software, one-offs
```

Profiles declare which registries to trust and with which keys.

---

## The Build Pipeline

Layers are pre-built artifacts, not built at launch time. Building at launch time introduces
non-determinism, latency, and hidden state. Resolution fails loudly if a layer does not exist
in the registry — it never silently builds one.

### Recipe contract

A recipe is a shell script and a metadata file. The script installs software into
`$STRATA_PREFIX`. The metadata declares what the layer provides and what it requires at
runtime.

```bash
# recipes/openmpi/4.1.6/build.sh
set -euo pipefail
./configure --prefix=$STRATA_PREFIX --with-cuda=$STRATA_PREFIX
make -j${STRATA_NCPUS} && make install
```

```yaml
# recipes/openmpi/4.1.6/meta.yaml
name: openmpi
version: 4.1.6
provides:
  - openmpi@4.1.6
  - mpi@3.1
build_requires:   # available during build, NOT in the layer
  - gcc@>=13
  - cuda@>=12.0
runtime_requires: # must exist on target at runtime
  - glibc@>=2.34
family: rhel
```

### Build environment

The build environment is itself a Strata environment. The compiler layer that built openmpi
is recorded in the layer manifest. Strata uses itself to build layers. The build is
reproducible because the build environment is a pinned, attested set of layers.

Squashfs images are produced with reproducible options (`-mkfs-time 0`, deterministic file
ordering). Same recipe + same build environment = same SHA256. This property is what makes
content-addressing meaningful.

### Build pipeline stages

```
1.  Resolve build environment from registry (build_requires)
2.  Launch clean EC2 instance matching target base
3.  Mount build environment via Strata overlay
4.  Execute recipe with STRATA_PREFIX=/strata/out
5.  Capture /strata/out → squashfs (reproducible options)
6.  Probe squashfs: what does it actually provide?
7.  Validate: declared provides ⊆ probed provides
8.  Generate content manifest (every file path + SHA256)
9.  Sign with cosign → push bundle to registry → log to Rekor
10. Push squashfs + manifest + bundle to S3 registry
11. Terminate build instance
```

---

## The Resolver

The resolver takes a Profile and produces a LockFile. Every stage is a clean pass or a
hard stop. There is no partial lockfile.

```
Stage 1  Base resolution     OS alias → AMI ID → probe capabilities (cached)
Stage 2  Formation expansion Formations unwrapped into layer refs, signatures verified
Stage 3  Software resolution Each SoftwareRef matched against catalog (family + arch filtered)
Stage 4  Graph validation    All requires satisfied by base + resolved layers
Stage 5  Conflict detection  File-level and capability-level, formation-aware
Stage 6  Topological sort    Dependency order → mount order
Stage 7  Sigstore verify     Every layer, parallel, hard fail on any miss
Stage 8  Lockfile assembly   Signed, Rekor logged
```

Errors are specific and actionable:

```
ERROR: no layer found for "alphafold@4.0"
  Available versions: 3.0.0, 3.0.1
  Run: strata search alphafold

ERROR: unsatisfied requirements for "openmpi@4.1"
  Requires: cuda@>=12.0
  Not provided by base or resolved layers
  Fix: add "cuda@12.3" to your software list

ERROR: conflict detected
  Both "openmpi@4.1" and "mpich@4.0" provide "mpi@3.1"
  Use only one MPI implementation
```

---

## The Agent

The agent runs on the instance. It has one job: take the lockfile, assemble the overlay,
signal ready. It runs as a systemd service early in boot, before spored, before user services.

```
Acquire lockfile    user-data → S3 → instance tag (priority order)
Verify lockfile     Sigstore bundle against Rekor
Pull layers         Parallel, cache-aware (SHA256 verified on cache hit)
Verify layers       cosign bundle + Rekor for every layer
Mount squashfs      Read-only loopback mounts
Assemble overlay    OverlayFS: lower=layers (ordered), upper=tmpfs
Configure env       /etc/profile.d/strata.sh, STRATA_* env vars
Signal ready        EC2 tag, CloudWatch event, systemd notify
```

Partial environments never run. Any failure halts the instance with a CloudWatch event
and an EC2 tag describing the failure. spored does not start until the agent succeeds.

### Boot timeline (cold start, no cache)

```
t+0s   Instance boots
t+6s   strata-agent starts
t+7s   lockfile acquired and verified
t+7s   parallel layer pulls begin
t+35s  all layers pulled and verified
t+36s  squashfs mounts complete
t+36s  OverlayFS assembled at /strata/env
t+37s  environment configured
t+37s  strata:status=ready tag applied
t+38s  spored starts, instance available
```

Warm start (all layers cached): under 15 seconds.

---

## Reproducibility and Provenance

```
strata resolve profile.yaml    →  profile.lock.yaml
strata freeze profile.yaml     →  fully pinned lockfile (all sha256)
strata publish profile.lock.yaml  →  doi:10.5281/zenodo.xxxxxxx
```

The lockfile is a complete provenance record:
- The exact profile that produced this environment (SHA256)
- The exact base AMI (ID + SHA256)
- Every layer (SHA256 + Rekor transparency log entry)
- The lockfile itself (Rekor entry)
- The Strata version that resolved it

A reviewer can independently verify every layer against the Rekor transparency log without
trusting Strata, the registry, or the researcher. A researcher can reproduce an environment
from a DOI a year later. An institution can audit environments for compliance without access
to the original researcher's machine.

This addresses NIH's computational reproducibility requirements directly. The environment
is not described in a methods section — it is cited as a verified artifact.

---

## Integration Points

### spore-host (spawn + spored)

```bash
spawn launch \
  --name my-analysis \
  --instance-type c6a.xlarge \
  --environment alphafold3.yaml \
  --ttl 4h \
  --on-complete terminate
```

spored gains a Strata phase in its startup sequence. The lockfile path is recorded in
spored's status. On termination, spored optionally pushes the active lockfile to S3
alongside job outputs — provenance travels with results.

### Prism

Prism templates embed a Strata profile as an optional field. Prism manages workspaces,
budgets, users, and lifecycle. Strata manages what is in the environment. Two concerns,
cleanly separated.

```yaml
# Prism template
name: r-workstation
strata:
  profile: r-quarto-workstation
  registry: s3://my-institution-layers/
# ... Prism-specific fields: budget, users, hibernation policy
```

---

## Initial Layer Catalog (AL2023, x86_64 + arm64)

```
Infrastructure    gcc@13, python@3.11, python@3.12, R@4.3, R@4.4
GPU               cuda@12.3, cuda@12.4, cudnn@8.9
MPI               openmpi@4.1, openmpi@5.0
Bioinformatics    blast@2.15, samtools@1.21, bwa@0.7, gatk@4.5
ML                pytorch@2.2, tensorflow@2.15
Tools             miniforge@24.3, jupyterlab@4.1, rstudio-server@2024.09
Publishing        quarto@1.4, pandoc@3.1, texlive@2024, git@2.43
```

Initial formations:

```
cuda-python-ml     cuda + python + miniforge
r-research         R + RStudio Server + pandoc + quarto + texlive
hpc-mpi            gcc + openmpi + ucx
bio-seq            samtools + bwa + blast
genomics-python    python + miniforge + bio-seq
jupyter-gpu        cuda-python-ml + jupyterlab
```

---

## What Strata Is Not

- Not a package manager (conda/pip/R handle user packages on top of Strata layers)
- Not a container runtime (no namespacing, no cgroups — just filesystem composition)
- Not a replacement for Spack or Lmod (composable with both; operates at a different level)
- Not an on-premises HPC tool (designed for cloud ephemerality; Warewulf adaptation is future work)
- Not a template system (you declare intent; the system composes — there are no templates to inherit or debug)

---

## Design Principles

**Determinism is designed in, not retrofitted.** The complexity of capability probing,
Sigstore attestation, and content-addressed layers is fixed cost paid once. Adding it
later means paying it on every layer, every user, every lockfile that shipped without it.

**The environment is read-only, always.** Software layers are immutable. The writable
upper layer (tmpfs) exists only for runtime noise. If you need something installed,
add a layer.

**Failure is loud and early.** Partial environments never run. Unsigned layers never
mount. Resolution errors name the problem and suggest the fix. The system never silently
succeeds at something that will fail later.

**Users think in software, not infrastructure.** Profile authors declare `cuda@12.3`,
not `s3://strata-layers/cuda/12.3.2-rhel-x86_64.sqfs`. The registry, the DAG, the
capability matching — these are the system's problem.

**A researcher should be able to write a profile in ten minutes and launch in under two.**

---

*Apache License 2.0 — Copyright 2026 Scott Friedman*

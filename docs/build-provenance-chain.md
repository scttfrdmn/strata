# Build Provenance Chain

How Strata establishes and records what built what, from the system compiler up through
MPI libraries, math libraries, and scientific applications.

---

## The Bootstrapping Problem

Every build chain has to start somewhere. Strata's answer is explicit:

**Tier 0 layers** (gcc, LLVM, CUDA) are built with the system compiler — whatever
`dnf install gcc` provides on the target AMI. This is the ground truth. You cannot prove
the system compiler's provenance without going all the way down to the C library, binutils,
and eventually hardware microcode. Every reproducible build system has a bootstrap
boundary; Strata's is the AL2023 (or Rocky 9/10) base AMI.

Tier 0 layers are marked `bootstrap_build: true` in their LayerManifest:

```yaml
# layers/rhel/x86_64/gcc/13.2.0/manifest.yaml
name: gcc
version: 13.2.0
bootstrap_build: true                              # Tier 0: chain root
bootstrap_compiler: "gcc-11.4.1-2.amzn2023.0.1.x86_64"  # exact RPM package, independently verifiable
build_env_lock_id: sha256:abc123...               # SHA256 of the BaseCapabilities probe record
built_with: []                                    # empty — no Strata layers used
rekor_entry: "12345"
sha256: d4e5f6...
```

`bootstrap_compiler` records the exact RPM package version of the system compiler used —
not just the version number, but the full package NVR (Name-Version-Release). On AL2023 this
is always `gcc-11.x.y-N.amzn2023.z.arch`. A reviewer can verify this against the AL2023
package repos or the AMI's RPM database without trusting Strata.

`build_env_lock_id` for bootstrap builds is the SHA256 of the `BaseCapabilities` YAML record
stored in the registry. That record contains the AMI ID, OS family, arch, and the full list
of system-provided capabilities including the system gcc version. The combination of
`bootstrap_compiler` + `build_env_lock_id` → `BaseCapabilities` → AMI ID gives a complete,
independently verifiable chain root.

This is not a weakness — it's an honest declaration of where the chain starts. A reviewer
knows exactly what to trust: the AL2023 AMI (independently verifiable via AWS), the recipe
`build.sh` (SHA256 recorded), and the Rekor transparency log entry (independently
verifiable). That's the same ground truth every Linux distro starts from.

---

## Why MPI Cannot Be a Bootstrap Build

OpenMPI links against `libgcc`, `libstdc++`, `libgfortran`, and `libgomp` from the
compiler that built it. If you build OpenMPI with the AL2023 system gcc (11.5.0) and then
run it in a Strata environment where the Strata gcc@13.2.0 layer is mounted, you have a
mismatch: the MPI library was compiled against gcc 11.5 runtime libs but the environment
provides gcc 13.2 runtime libs. This may or may not work at runtime (usually it does,
due to backwards compatibility), but it is **not provably correct** and the chain is broken.

More importantly: a user who declares `build_requires: gcc@>=13` in their own recipe to
build a Fortran MPI application expects that the MPI library they link against was compiled
with the same gcc generation. If it wasn't, the Fortran ABI may not match.

The clean answer: **OpenMPI must be built inside a mounted Strata gcc layer.**

---

## The Provenance Chain in Practice

```
AL2023 AMI ami-0c421724a94bba6d6
  └─ gcc@13.2.0 (bootstrap_build: true)
       sha256: d4e5f6...  rekor: 12345

  └─ openmpi@5.0.6
       built_with:
         - gcc@13.2.0  sha256: d4e5f6...  rekor: 12345
       sha256: a7b8c9...  rekor: 23456

  └─ openblas@0.3.26
       built_with:
         - gcc@13.2.0  sha256: d4e5f6...  rekor: 12345
       sha256: b1c2d3...  rekor: 34567

  └─ numpy@1.26.4
       built_with:
         - gcc@13.2.0   sha256: d4e5f6...  rekor: 12345
         - openblas@0.3.26  sha256: b1c2d3...  rekor: 34567
       sha256: e4f5a6...  rekor: 45678
```

A reviewer can reconstruct this chain from any DOI. Every `sha256` can be independently
computed from the artifact. Every `rekor` entry can be verified against the public Rekor
transparency log at rekor.sigstore.dev without trusting Strata, the registry, or the
researcher.

---

## How the Pipeline Records `built_with`

The v0.10.0 EC2 build pipeline (deferred from v0.9.0):

1. Resolver resolves `build_requires` from the recipe's `meta.yaml`
2. Strata downloads and verifies those layers from the registry
3. The layers are mounted as OverlayFS lower layers on the build EC2 instance
4. `STRATA_INSTALL_PREFIX` (and the layer's compilers on `PATH`) are configured
5. `build.sh` runs inside this environment — it literally uses the Strata gcc binary
6. After `make install`, the pipeline populates `manifest.BuiltWith` from the resolved
   build environment lockfile:

```go
// v0.10.0 pipeline (not yet implemented)
manifest.BootstrapBuild = false
manifest.BuiltWith = []spec.LayerRef{
    {Name: "gcc", Version: "13.2.0", SHA256: "d4e5f6...", Rekor: "12345"},
}
manifest.BuildEnvLockID = buildEnvLockfile.SHA256()
```

The v0.9.0 local pipeline marks all builds `bootstrap_build: true` and leaves `BuiltWith`
empty. This is intentional — v0.9.0 only builds Tier 0 layers (gcc itself), for which
`bootstrap_build: true` is the correct declaration.

---

## MPI Landscape in Strata

There are four significant MPI implementations:

| Implementation | Strata layer name | provides |
|---|---|---|
| Open MPI | `openmpi` | `mpi@4.0` |
| MPICH | `mpich` | `mpi@4.0` |
| Intel MPI | `intel-mpi` | `mpi@4.0` |
| MVAPICH2 | `mvapich2` | `mpi@3.1` |

All declare `provides: mpi@<standard-version>`. Applications that need MPI declare
`requires: mpi@>=3.1`. The resolver picks whichever implementation is in the profile;
the conflict detector prevents loading two at once.

**Why this matters for provenance**: Because the `provides: mpi@4.0` capability is
identical across implementations, the lockfile records *which implementation* was resolved
(e.g., `openmpi@5.0.6`) and its `built_with` field records the exact compiler. A research
group using Intel MPI on their cluster and Open MPI in the registry gets different
environments — and Strata makes that visible in the lockfile, not silently different.

### Tier 1 MPI build order

```
Tier 0:  gcc@13.2.0  (bootstrap_build: true)
              │
Tier 1a: openmpi@5.0.6  (built_with: gcc@13.2.0)
         openblas@0.3.26 (built_with: gcc@13.2.0)
         fftw@3.3.10     (built_with: gcc@13.2.0)
              │
Tier 1b: scalapack@2.2   (built_with: gcc@13.2.0 + openmpi@5.0.6 + openblas@0.3.26)
         hdf5@1.14.3     (built_with: gcc@13.2.0 + openmpi@5.0.6)
              │
Tier 2:  petsc, wrf, gromacs, alphafold, ...
```

Tier 1a layers require only gcc. Tier 1b layers require gcc + MPI (and/or BLAS). Each
level can only be built after the previous level's layers are in the registry and verified.

### Toolchain formations for build environments

Once Tier 1a is in the registry, a toolchain formation packages them for reuse:

```yaml
# formations/foss-2024a.yaml
name: foss-2024a
type: toolchain
software:
  - gcc@13.2.0
  - openmpi@5.0.6
  - openblas@0.3.26
  - fftw@3.3.10
```

Tier 1b and Tier 2 recipes declare:

```yaml
# meta.yaml for hdf5
build_requires:
  - formation: foss-2024a
```

The pipeline resolves the formation, mounts all four layers, and the `built_with` field
in the resulting manifest records all four with their SHA256s.

---

## Build Sequencing Constraint

This is the fundamental ordering rule:

> **A layer at tier N can only be built after all layers it `build_requires` exist in the
> registry and have been verified.**

This means the full Tier 0 → Tier 1 → Tier 2 catalog build is a sequential pipeline, not
a parallel one (within a dependency chain). Across independent chains it can be parallel:
openmpi and openblas can build simultaneously once gcc is in the registry, since they only
require gcc.

The v0.10.0 build pipeline will need a build graph resolver that understands this ordering
and can schedule builds across multiple EC2 instances efficiently — essentially the same
DAG resolution logic as the runtime resolver, but applied to the build dependency graph.

---

## What `bootstrap_build: true` Means for Trust

A layer marked `bootstrap_build: true`:
- Was built with the OS system compiler (version recorded in `build_env_lock_id`)
- Has no `built_with` entries
- Is a valid chain root — the provenance starts here
- Can still be verified via `rekor_entry` (the binary itself is signed and logged)

A layer with `bootstrap_build: false` (or absent) and empty `built_with`:
- Is an error — the pipeline should always populate one or the other
- The registry should reject it
- `strata verify` should flag it

This invariant makes the chain complete and auditable with no gaps.

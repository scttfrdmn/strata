# Strata Ecosystem Integration

Design notes on how Strata relates to, differs from, and can integrate with adjacent tools
in the HPC and research computing ecosystem.

---

## Strata vs Spack vs LMod

These three tools are frequently mentioned together and are often confused. They operate at
different levels of the stack and are more complementary than competitive.

### LMod

LMod is an **environment switcher**. It manipulates `PATH`, `LD_LIBRARY_PATH`,
`MANPATH`, and similar environment variables so that software already installed on a shared
POSIX filesystem (NFS, Lustre, GPFS) can be used and swapped at runtime. It has no opinions
about how software got there, no isolation model, no cryptographic guarantees. LMod is a
very thin shim on top of the traditional HPC sysadmin model: a human administrator installs
software into `/software/gcc/13.2.0/` and writes a module file; LMod exposes it.

LMod's weaknesses in a cloud research context:
- Requires a persistent shared filesystem (NFS/Lustre), which is expensive and complex on AWS
- Provides no isolation — module A can shadow symbols from module B in unpredictable ways
- Software is mutable; there is no guarantee the bits you load today are the bits you loaded
  last month
- No cryptographic attestation, no audit trail, no DOI

**How Strata relates**: Strata replaces the shared filesystem + LMod model entirely for
layer composition. OverlayFS at boot time does what module load does at runtime — but
immutably, atomically, and with full attestation. However, LMod could be run *inside* a
Strata environment to handle fine-grained switching between software exposed by mounted
layers. For example, a Strata environment containing both `gcc@13.2` and `gcc@14.2` layers
could use LMod to switch between them without remounting.

**Integration idea**: LMod modulefiles are generated automatically by `strata build` from
the content manifest and bundled inside the squashfs layer. When the layer mounts at
`/strata/env/modulefiles/<name>/<version>.lua`, LMod can immediately use it. The
strata-agent writes `/etc/profile.d/strata-modules.sh` to auto-register the module path.
See `docs/multi-version-layers.md` for the full design including version-prefixed install
paths, auto-generated modulefile content, resolver changes for coexistence, and the
`defaults:` profile key for auto-activation.

---

### Spack

Spack is a **source-based package manager** with a deep dependency solver, thousands of
packages, and extensive support for build variants (`+cuda`, `%gcc@13`, `target=zen3`). It
can install multiple versions of the same package simultaneously and generate LMod module
files for them. Spack is very good at what it does.

Spack's weaknesses in a cloud research context:
- Reproducibility is at the *recipe* level, not the *binary* level. The same Spack spec on
  different hardware, with different ambient compilers, or after a Spack version upgrade can
  produce different bits. There is no mechanism to verify that what you built is what someone
  else built.
- No cryptographic attestation of build outputs. You can share a `spack.lock` file but not a
  signed, Rekor-logged proof that the binary matches it.
- Installs to a mutable shared prefix — you can `spack install` more things and potentially
  break existing environments.
- Assumes a persistent shared filesystem. Pulling a Spack environment from scratch on an
  ephemeral EC2 instance means rebuilding from source every time.
- No concept of squashfs layers, OverlayFS, or content-addressed binary distribution.

**How Strata relates**: Strata and Spack occupy different parts of the lifecycle. Spack is
excellent at *describing how to build* complex software with many configuration variants.
Strata is excellent at *attesting what was built* and *distributing the result reproducibly*.
The two can be combined:

**Integration idea — Spack as a recipe backend**:
A Strata recipe's `build.sh` could invoke Spack to build the software, then copy the Spack
install prefix into `$STRATA_PREFIX`. The squashfs, signing, and registry push are all
Strata's responsibility regardless. This gives you Spack's huge package library and variant
system while gaining Strata's attestation, binary distribution, and OverlayFS composition.

```yaml
# meta.yaml (future)
name: hdf5
version: 1.14.3
build_backend: spack
spack_spec: "hdf5@1.14.3+mpi+fortran %gcc@13.2.0"
```

The `strata build` pipeline would resolve the spack spec, run `spack install`, relocate the
prefix to `$STRATA_PREFIX`, then proceed with squashfs + signing as normal.

**Integration idea — Spack buildcache as input**:
Spack has a binary cache format (`spack buildcache`). A future `strata import-spack` command
could take a Spack buildcache tarball, wrap it in a squashfs layer, generate a content
manifest, and sign + push it to the Strata registry. This would let institutions with
existing Spack investments get Strata's attestation and distribution model without rebuilding
from source.

**What Strata adds that Spack cannot**:
- Binary-level reproducibility: same squashfs SHA256 = same bits, always
- Sigstore/Rekor transparency log entries for every layer
- DOI minting via Zenodo for environment lockfiles
- OverlayFS composition at boot — no shared filesystem required
- Sub-60-second cold start on EC2 from S3 (vs. hours of Spack builds)

---

## Compiler Toolchains

### The concept

In HPC, a **compiler toolchain** is a named, versioned, internally-consistent set of:
- A compiler (gcc, Intel, LLVM)
- An MPI library (OpenMPI, MPICH, Intel MPI)
- Core math libraries (OpenBLAS/BLAS/LAPACK, FFTW, ScaLAPACK)
- Optionally: CUDA, UCX

Spack formalizes this with named toolchains: `foss` (Free and Open Source Software stack),
`intel`, `gompi` (GCC + OpenMPI), etc. Software built with `foss-2024a` is guaranteed to
link against a specific, compatible set of the above. You cannot mix `foss-2023b` and
`foss-2024a` libraries safely.

### Toolchains in Strata

Compiler toolchains are **not** the same as Tier 0, though they are composed from Tier 0
layers.

**Tier 0** = individual atomic layers built using only the OS system compiler (whatever
`dnf install gcc` provides on AL2023). Tier 0 layers have no `build_requires` pointing to
other Strata layers. They are the bottom of the dependency graph.

Current Tier 0 catalog: gcc, python@3.11, python@3.12, R@4.3, R@4.4, cuda, openmpi,
miniforge, jupyterlab, samtools, quarto.

**Toolchain Formations** = named compositions of Tier 0 layers that form a coherent build
environment. They are a special class of Formation with `type: toolchain`.

```yaml
# formations/foss-2024a.yaml
name: foss-2024a
version: "2024.1"
type: toolchain
description: "GCC 13.2 + OpenMPI 5.0 + OpenBLAS + FFTW — standard HPC toolchain"
software:
  - gcc@13.2.0
  - openmpi@5.0
  - openblas@0.3.26
  - fftw@3.3.10
```

**Tier 1** = layers built *within* a toolchain formation. Their `build_requires` field in
`meta.yaml` references a toolchain formation. This creates a traceable chain:

```
numpy@1.26 was built with foss-2024a
foss-2024a = gcc@13.2.0 (sha256:a1b2...) + openmpi@5.0 (sha256:c3d4...) + ...
```

**Tier 2** = applications built on Tier 1 layers (alphafold, pytorch, etc.).

### Why this matters

1. **ABI compatibility** — software linked against OpenBLAS@0.3.26 from `foss-2024a` may
   segfault if loaded alongside OpenBLAS@0.3.23 from an older toolchain. Strata's capability
   system (`provides`/`requires`) can encode this: openblas layers provide `blas@3.12` and
   numpy requires `blas@>=3.10`. The resolver detects conflicts.

2. **Reproducible builds** — when the v0.10.0 EC2 build pipeline mounts a toolchain
   formation as the OverlayFS environment before running `build.sh`, the build environment
   itself is cryptographically attested. You can prove not just "what we built" but "what we
   built it with."

3. **Named toolchains for users** — a researcher can write:
   ```yaml
   software:
     - formation:foss-2024a
     - hdf5@1.14
     - netcdf@4.9
   ```
   and get a coherent, validated environment without understanding ABI compatibility rules.

4. **Toolchain versioning** — `foss-2024a` and `foss-2025a` can coexist in the registry.
   A user pinning `foss-2024a` in their lockfile gets reproducibility across years.

### Recommended layer tier structure

```
Tier 0 (system compiler):   gcc, python, R, cuda, openmpi, OpenBLAS, FFTW
                                          ↓
Toolchain Formations:       foss-2024a, cuda-toolchain-12.3, intel-2024a
                                          ↓
Tier 1 (toolchain build):   numpy, scipy, hdf5, netcdf, petsc, mpi4py
                                          ↓
Tier 2 (application):       alphafold, pytorch, tensorflow, wrf, gromacs
                                          ↓
Runtime Formations:         r-research, jupyter-gpu, hpc-mpi, bio-seq
```

Note: Tier 0 layers (especially gcc) are also available as runtime layers — a researcher
who needs to compile code in their environment can declare `gcc@13.2` directly.

---

## gcc Version Strategy

The gcc major version determines ABI compatibility for C++ code. Minor versions within a
major are generally ABI-compatible.

Recommended catalog targets:

| Version | Status | Priority |
|---------|--------|----------|
| gcc@13.2.0 | Current stable | High — build now |
| gcc@14.2.0 | Current stable (2024) | High — add recipe |
| gcc@12.3.0 | Maintenance | Low — skip unless requested |
| gcc@15.x | Development | Track, build when stable |

The recipe is identical across versions — only `VERSION` changes in `build.sh`. Each version
gets its own recipe directory: `recipes/gcc/14.2.0/`, `recipes/gcc/15.1.0/`, etc.

The toolchain formation `foss-2024a` would pin `gcc@13.2.0`; `foss-2025a` would pin
`gcc@14.2.0`. Users who need reproducibility pin the toolchain formation; users who want
"latest gcc@>=13" get whichever the resolver finds.

---

## Provenance Traceability Chain

This is Strata's deepest differentiator from Spack, LMod, or any other approach. Every
artifact in the system carries a complete, independently verifiable chain of custody:

```
alphafold@3.0.0 (sha256:a1b2c3..., rekor:98765)
  built with: foss-2024a
    gcc@13.2.0        (sha256:d4e5f6..., rekor:12345)
    openmpi@5.0       (sha256:a7b8c9..., rekor:23456)
    openblas@0.3.26   (sha256:d0e1f2..., rekor:34567)
    fftw@3.3.10       (sha256:a3b4c5..., rekor:45678)
  requires at runtime:
    python@3.12.0     (sha256:d6e7f8..., rekor:56789)
    cuda@12.3         (sha256:a9b0c1..., rekor:67890)

lockfile: doi:10.5281/zenodo.1234567
```

A reviewer can reconstruct this chain from the DOI alone, verify every Rekor entry against
the public transparency log, and confirm that the binary they receive today is the binary
that was signed when the paper was published — without trusting Strata, the registry, or
the original researcher.

Spack can tell you what recipe produced a binary. Strata can prove it.

---

## Summary: The Stack

```
Institution / researcher
        ↓  declares intent via Profile
Strata resolver
        ↓  matches capabilities, validates graph, verifies signatures
Lockfile (DOI-minted, Rekor-logged)
        ↓  pulled by strata-agent at boot
OverlayFS environment at /strata/env
        ↓  software exposed in PATH
[optional] LMod for runtime switching within the environment
        ↓  user runs code
Results + provenance (lockfile travels with outputs)
```

Spack lives to the left of this diagram — it can produce the content of Tier 0/1 layers.
LMod lives inside the environment. Strata owns the middle: attestation, distribution,
composition, and boot-time assembly.

# Layer Tier Structure

How Strata classifies the software stack from compilers through domain applications.
The tier system is **machine-enforced**: `tier` is a required field in `meta.yaml`,
validated by `RecipeMeta.Validate()`, and the directory structure must match.

---

## The Single Criterion

> **A layer's tier is determined by one question: do other compiled layers link
> against it at the C/ABI level, and if so, how deep in the stack is it?**

If swapping versions would silently break binary compatibility for something else
in the catalog, it is infrastructure. If nothing in the catalog compiles against
it, it is a leaf node (Tier 2).

Package managers (conda, pip, npm, cargo) are **build tools**, not layers. They
appear in `build.sh` scripts — the same role as `make`, `cmake`, or `autoconf`.
They do not appear in `build_requires` or `provides`. See
[Package Managers as Build Tools](#package-managers-as-build-tools) below.

---

## The Invariants

Each tier has hard constraints that the build pipeline and validator enforce:

| Tier | `bootstrap_build` | `build_requires` constraint | Other layers depend on it |
|------|--------------------|----------------------------|--------------------------|
| 0 | **true** — built with OS system compiler | **must be empty** | Yes — it is the chain root |
| 0.5 | false | only Tier 0 layers | Yes — MPI/comm libs depend on this |
| 1.0 | false | Tier 0 and/or 0.5 only | Yes — applications build against these |
| 1.5 | false | Tier 0–1.0 only | Sometimes — framework-to-framework deps |
| 2 | false | any tier | **No** — leaf nodes; never in another layer's `build_requires` |

**The leaf rule**: if any other layer in the catalog lists this layer in its own
`build_requires`, it cannot be Tier 2. Move it up.

**The bootstrap rule**: if the recipe has `build_requires`, it cannot be Tier 0.
`RecipeMeta.Validate()` rejects any Tier 0 recipe with a non-empty `build_requires`.

---

## Tier 0 — Compilers and Language Runtimes

`bootstrap_build: true`. The chain root. Built using only the OS-provided toolchain.

**Invariant**: `build_requires` must be empty. Every other layer's provenance chain
traces back to a Tier 0 layer. The defining characteristic is that Tier 0 layers
produce `.so` files (or GPU device libraries) that other compiled code links against
at build time, and where the version is encoded in the binary — swap versions and
things silently break at the linker or at runtime.

### Compilers

Every compiler that produces ABI-specific object code belongs here. Binaries built
with Intel's compiler link against Intel runtime libraries; those built with ARM's
compiler link against ARM performance libraries. This is the same ABI-lock concern
as gcc.

| Layer | Provides | Note |
|---|---|---|
| `gcc@13`, `gcc@14` | `gcc@N` | C/C++/Fortran; gfortran included |
| `llvm@17`, `llvm@18` | `llvm@N` | Clang/C++/OpenMP offload |
| `intel-oneapi@2024` | `intel-compiler@2024` | icx/icpx/ifort; links against libsvml, libifcore, libirng |
| `aocc@4` | `amd-compiler@4` | AMD-optimized clang/flang; links against AMD runtime libs |
| `arm-compiler@24` | `arm-compiler@24` | armclang/armflang + ARM Performance Libraries runtime |
| `nvhpc@24` | `nvhpc@24` | nvc/nvfortran/OpenACC; links against NVIDIA HPC runtime |

### GPU Runtimes

GPU device code is compiled for a specific runtime ABI. A binary built for CUDA 12
will not load under CUDA 11.

| Layer | Provides | Note |
|---|---|---|
| `cuda@12.3`, `cuda@12.4` | `cuda@12` | NVIDIA GPU runtime + nvcc + device library headers |
| `rocm@6.0` | `rocm@6` | AMD GPU runtime (libhip, librocblas, libamdhip64) |
| `intel-gpu@2024` | `intel-gpu@2024` | Intel oneAPI compute runtime (Level Zero) for Xe/PVC |

### Language Runtimes

These belong in Tier 0 for the same reason compilers do: compiled extensions link
against the interpreter's `.so` at build time, and the version is baked in.
`libpython3.12.so` and `libpython3.11.so` are different ABI surfaces — a `.so`
extension built for 3.12 will not load under 3.11.

| Layer | Provides | Note |
|---|---|---|
| `python@3.11`, `python@3.12` | `python@3.N` | CPython; compiled extensions link against libpython3.N.so |
| `R@4.4`, `R@4.5` | `R@4.N` | R interpreter; compiled packages link against libR.so |
| `julia@1.10` | `julia@1.N` | Julia; compiled packages link against libjulia.so |
| `jdk@21` | `java@21` | JVM; native extensions (JNI) link against libjvm.so; required by GATK, Cromwell |

**What does NOT belong in Tier 0**: package managers (conda, pip, npm, cargo).
These are build tools. See [Package Managers as Build Tools](#package-managers-as-build-tools).

---

## Tier 0.5 — Parallel Communication Infrastructure

`build_requires: gcc` (Tier 0). Must enter the registry before any MPI-linked code.

**Invariant**: `build_requires` contains only Tier 0 layers. These layers exist
to provide capabilities (`mpi@4.0`, `ucx@1`, `hwloc@2`) that Tier 1.0+ recipes
declare in their own `build_requires`.

| Layer | Provides | Note |
|---|---|---|
| `ucx@1.16` | `ucx@1` | Unified Communication X; transport layer MPI sits on |
| `libfabric@1.21` | `libfabric@1` | OFI fabric layer (EFA, Slingshot, InfiniBand) |
| `pmix@5.0` | `pmix@5` | Process management interface for MPI runtimes |
| `hwloc@2.10` | `hwloc@2` | Hardware locality; NUMA/CPU topology for MPI binding |
| `openmpi@5.0` | `mpi@4.0`, `openmpi@5` | Needs ucx + pmix + hwloc |
| `mpich@4.1` | `mpi@4.0`, `mpich@4` | Reference MPI implementation |
| `mvapich2@3.0` | `mpi@3.1`, `mvapich2@3` | InfiniBand-optimized MPI |

All MPI implementations `provides: mpi@4.0`. Applications declare
`requires: mpi@>=3.1`. The conflict detector prevents two MPI implementations
from loading simultaneously.

---

## Tier 1.0 — Math and I/O Foundations

`build_requires` contains only Tier 0 and/or Tier 0.5 layers.

**Invariant**: these layers appear in `build_requires` of Tier 1.5 and Tier 2
recipes. Swapping BLAS or HDF5 versions affects binary compatibility of everything
that links against them.

| Layer | Provides | Needs |
|---|---|---|
| `openblas@0.3.26` | `blas@3`, `lapack@3` | gcc |
| `fftw@3.3.10` | `fftw@3` | gcc (serial); gcc + mpi (parallel) |
| `scalapack@2.2` | `scalapack@2` | gcc + mpi + blas |
| `suitesparse@7.6` | `suitesparse@7` | gcc + blas |
| `mumps@5.6` | `mumps@5` | gcc + mpi + blas + scalapack |
| `superlu@6.0` | `superlu@6` | gcc + mpi + blas |
| `zlib@1.3`, `lz4@1.9`, `zstd@1.5` | `zlib@1`, etc. | gcc |
| `hdf5@1.14.3` | `hdf5@1.14` | gcc (serial) or gcc + mpi (parallel) |
| `netcdf@4.9` | `netcdf@4` | gcc + hdf5 |
| `pnetcdf@1.12` | `pnetcdf@1` | gcc + mpi |
| `adios2@2.9` | `adios2@2` | gcc + mpi + hdf5 |

---

## Tier 1.5 — Scientific Frameworks

`build_requires` contains Tier 0 through Tier 1.0 layers.

**Invariant**: these layers appear in `build_requires` of some Tier 2 codes.
They are not end-user tools — no researcher runs PETSc directly; they run FEniCS
which was compiled against PETSc.

| Layer | Needs | Used by |
|---|---|---|
| `petsc@3.21` | gcc + mpi + blas + hdf5 | FEniCS, OpenFOAM, many CFD codes |
| `trilinos@16` | gcc + mpi + blas + hdf5 | Sierra, Albany |
| `magma@2.7` | gcc + cuda + blas | GPU-accelerated LAPACK; some ML codes |
| `slepc@3.21` | petsc | Eigenvalue solvers |

---

## Tier 2 — Domain Applications

**Invariant**: no other layer in the Strata catalog lists a Tier 2 layer in
`build_requires`. These are leaf nodes in the dependency DAG — end-user tools
that researchers run directly.

| Domain | Examples |
|---|---|
| Molecular dynamics | GROMACS, NAMD, LAMMPS, AMBER |
| Climate / weather | WRF, CESM, ICON |
| Quantum chemistry | VASP, NWChem, GAMESS |
| Machine learning | PyTorch, TensorFlow, JAX |
| Bioinformatics | SAMtools, BLAST, BWA, GATK, bcftools |
| Compiled Python stacks | NumPy, SciPy, h5py, mpi4py, pySCF |
| Notebooks / publishing | JupyterLab, Quarto, RStudio Server |
| Package manager environments | miniforge (frozen conda base) |

Note on compiled Python packages: NumPy/SciPy link against BLAS at compile time,
so their `built_with` chain is as deep as a Tier 1.0 layer's — but they are still
Tier 2 because no other Strata layer compiles against them at the C level.

---

## Package Managers as Build Tools

conda, pip, npm, cargo, and similar package managers are **build tools** in
Strata's model. They have the same role as `make`, `cmake`, or `autoconf`: they
appear in `build.sh` scripts, not in `build_requires` or `provides`.

This is a critical conceptual distinction because it determines how scientific
reproducibility works in Strata.

### The question: how do I memorialize pySCF@2.5.0 and numpy@1.26.4?

The answer is not to make conda a Strata layer. The answer is to build a **frozen
environment layer** — a Tier 2 Strata layer whose `build.sh` runs `conda install`
or `pip install` with pinned versions, then squashes the result:

```bash
# build.sh for a pyscf-2.5.0 layer
conda create -p "${STRATA_INSTALL_PREFIX}" python=3.12 \
  numpy=1.26.4 scipy=1.13.0 pyscf=2.5.0 --yes
conda clean -a --yes
```

The resulting squashfs contains the entire frozen environment. The layer manifest
records:

```yaml
built_with:
  - name: python@3.12.13    # Tier 0 — libpython3.12.so
  - name: openblas@0.3.26   # Tier 1.0 — numpy links against BLAS
content_manifest:            # every installed file + SHA256
  - /pyscf/2.5.0/lib/python3.12/site-packages/pyscf/__init__.py  sha256: a1b2...
  - /pyscf/2.5.0/lib/python3.12/site-packages/numpy/__init__.py  sha256: c3d4...
  - /pyscf/2.5.0/lib/python3.12/site-packages/numpy/core/_multiarray_umath.so  sha256: e5f6...
```

This gives you exactly the provenance you want:
- pySCF@2.5.0 is memorialized by the SHA256 of its installed files
- numpy@1.26.4 is memorialized the same way
- The BLAS it was compiled against is in `built_with`
- The entire layer is signed with cosign and logged to Rekor

### Why not make conda itself a Strata layer?

Nothing in the catalog *links against* conda at the C level. There is no
`libconda.so`. conda is a Python application that installs packages; the packages
it installs are what matter, and those are captured by the content manifest.

Making conda a Strata `build_requires` would add a dependency edge in the DAG
without adding any meaningful provenance — you already know which conda you used
because the layer was built with a specific Strata Python (which is Tier 0 and
therefore fully attested).

### When does a frozen environment layer change tier?

The frozen environment layer is Tier 2 if nothing else compiles against it. If
another Strata layer declares `build_requires: pyscf@2.5` (because it wraps pySCF
with a compiled C extension), then pySCF moves to Tier 1.5. The criterion never
changes — only the catalog facts do.

---

## Directory Structure and Enforcement

Recipes live at:

```
cmd/strata/recipes/
  tier0/    <name>/<version>/   ← gcc, llvm, cuda, rocm, python, R, julia, jdk
  tier0.5/  <name>/<version>/   ← openmpi, ucx, libfabric, pmix, hwloc
  tier1.0/  <name>/<version>/   ← openblas, fftw, hdf5, ...
  tier1.5/  <name>/<version>/   ← petsc, trilinos, ...
  tier2/    <name>/<version>/   ← samtools, GROMACS, JupyterLab, miniforge, ...
```

`meta.yaml` must declare `tier: "N"` matching the directory. The test in
`examples/catalog_test.go` enforces this: a tier mismatch is a CI failure.

`RecipeMeta.Validate()` enforces:
- `tier` must be one of: `0`, `0.5`, `1.0`, `1.5`, `2`
- Tier 0 recipes with non-empty `build_requires` are rejected

---

## Build Order

```
Tier 0    gcc, llvm, intel-oneapi, aocc, arm-compiler, nvhpc   ← parallel, no deps
          cuda, rocm, intel-gpu
          python, R, julia, jdk
              │
Tier 0.5  ucx, libfabric, pmix, hwloc                          ← parallel, deps on Tier 0
          openmpi, mpich, mvapich2                              ← after ucx+pmix+hwloc
              │
Tier 1.0  openblas, fftw                                       ← parallel with Tier 0.5
          zlib, lz4, zstd
          hdf5, netcdf, adios2, pnetcdf                        ← after HDF5
          scalapack, mumps, superlu                            ← after MPI + BLAS
              │
Tier 1.5  petsc, trilinos, magma, slepc
              │
Tier 2    gromacs, wrf, pytorch, samtools, ...                 ← conda/pip are build tools here
          numpy, scipy, pyscf, h5py, mpi4py
          miniforge, jupyterlab, quarto
```

Within each tier, independent chains build in parallel. Across tiers, the DAG
scheduler enforces ordering. See [build-provenance-chain.md](build-provenance-chain.md)
for the `built_with` / `bootstrap_build` manifest spec.

---

## Toolchain Formations

Once Tier 0.5 is in the registry, a toolchain formation packages the core build
environment for reuse. Tier 1.0, 1.5, and 2 recipes reference a formation rather
than listing individual layers:

```yaml
# meta.yaml for gromacs
build_requires:
  - formation: foss-2024a   # gcc@13.2.0 + openmpi@5.0.6 + openblas@0.3.26 + fftw@3.3.10
  - name: cuda
    min_version: "12.0"
```

| Formation | Contents |
|---|---|
| `foss-2024a` | gcc@13.2.0 + openmpi@5.0.6 + openblas@0.3.26 + fftw@3.3.10 |
| `foss-2025a` | gcc@14.2.0 + openmpi@5.0.6 + openblas@0.3.26 + fftw@3.3.10 |
| `cuda-2024a` | cuda@12.3 + gcc@13.2.0 |

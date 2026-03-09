# Layer Tier Structure

How Strata classifies the software stack from compilers through domain applications.
The tier system is **machine-enforced**: `tier` is a required field in `meta.yaml`,
validated by `RecipeMeta.Validate()`, and the directory structure must match.

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
traces back to a Tier 0 layer.

| Layer | Provides | Note |
|---|---|---|
| `gcc@13.2.0`, `gcc@14.2.0` | `gcc@N` | C/C++/Fortran; gfortran included |
| `llvm@17`, `llvm@18` | `llvm@N` | Clang/OpenMP offload |
| `cuda@12.3`, `cuda@12.4` | `cuda@12` | GPU runtime + nvcc + cuBLAS headers |
| `rocm@6.0` | `rocm@6` | AMD GPU runtime |
| `python@3.11`, `python@3.12` | `python@3.N` | CPython interpreter + pip |
| `R@4.3`, `R@4.4` | `R@4.N` | R interpreter + base packages |

Python and R belong here because compiled C extensions link against the interpreter
ABI directly. Swapping Python 3.11 for 3.12 breaks `.so` extensions — same binary
compatibility concern as swapping a compiler.

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
| Compiled Python | NumPy, SciPy, h5py, mpi4py |
| Notebooks / publishing | JupyterLab, Quarto, RStudio Server |
| Package managers | miniforge (conda + mamba) |

Note on compiled Python packages: NumPy/SciPy link against BLAS at compile time,
so their `built_with` chain is as deep as a Tier 1.0 layer's — but they are still
Tier 2 because no other Strata layer compiles against them at the C level. The
Python *interpreter* is Tier 0; pure-Python packages are conda/pip's domain.

---

## Directory Structure and Enforcement

Recipes live at:

```
cmd/strata/recipes/
  tier0/    <name>/<version>/   ← gcc, python, R, cuda, miniforge
  tier0.5/  <name>/<version>/   ← openmpi (and ucx, libfabric, pmix, hwloc)
  tier1.0/  <name>/<version>/   ← openblas, fftw, hdf5, ...
  tier1.5/  <name>/<version>/   ← petsc, trilinos, ...
  tier2/    <name>/<version>/   ← samtools, GROMACS, JupyterLab, ...
```

`meta.yaml` must declare `tier: "N"` matching the directory. The test in
`examples/catalog_test.go` enforces this: a tier mismatch is a CI failure.

`RecipeMeta.Validate()` enforces:
- `tier` must be one of: `0`, `0.5`, `1.0`, `1.5`, `2`
- Tier 0 recipes with non-empty `build_requires` are rejected

---

## Build Order

```
Tier 0    gcc, LLVM, CUDA, Python, R                   ← parallel, no deps
              │
Tier 0.5  ucx, libfabric, pmix, hwloc                 ← parallel, deps on Tier 0
          openmpi, mpich, mvapich2                     ← after ucx+pmix+hwloc
              │
Tier 1.0  openblas, fftw                              ← parallel with Tier 0.5
          zlib, lz4, zstd
          hdf5, netcdf, adios2, pnetcdf               ← after HDF5
          scalapack, mumps, superlu                   ← after MPI + BLAS
              │
Tier 1.5  petsc, trilinos, magma, slepc
              │
Tier 2    gromacs, wrf, pytorch, samtools, ...
          numpy, scipy, h5py, mpi4py
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

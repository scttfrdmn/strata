# Layer Tier Structure

How Strata classifies the software stack from compilers through domain applications.

---

## The Rule

> **Infrastructure layer**: appears in another layer's `build_requires`, or provides a
> named capability (`mpi@4.0`, `blas@3`) that the resolver matches against.
>
> **Application layer**: only appears in user profiles. Never in another layer's
> `build_requires`.

Practical corollary: if swapping versions would silently break binary compatibility for
something else in the catalog, it's infrastructure.

---

## Tier 0 — Compilers and Language Runtimes

Built with the OS system compiler (`bootstrap_build: true`). The chain root.

| Layer | Provides | Note |
|---|---|---|
| `gcc@13.2.0`, `gcc@14.2.0` | `gcc@N` | C/C++/Fortran |
| `llvm@17`, `llvm@18` | `llvm@N` | Clang/OpenMP |
| `cuda@12.3`, `cuda@12.4` | `cuda@12` | GPU runtime + nvcc |
| `rocm@6.0` | `rocm@6` | AMD GPU |
| `python@3.11`, `python@3.12` | `python@3.N` | CPython interpreter |
| `R@4.3`, `R@4.4` | `R@4.N` | R interpreter |

Python and R belong here because compiled extensions link against the interpreter ABI.
Everything above Tier 0 records which Tier 0 layer(s) built it in `built_with`.

---

## Tier 0.5 — Parallel Communication Infrastructure

`build_requires: gcc`. The foundation MPI sits on. Must enter the registry before any
MPI implementation can build.

| Layer | Provides | Note |
|---|---|---|
| `ucx@1.16` | `ucx@1` | Transport layer; MPI sits on this |
| `libfabric@1.21` | `libfabric@1` | Alternative MPI transport (Slingshot, EFA) |
| `pmix@5.0` | `pmix@5` | Process management across nodes |
| `hwloc@2.10` | `hwloc@2` | Hardware locality; NUMA topology for MPI |
| `openmpi@5.0.6` | `mpi@4.0`, `openmpi@5` | needs ucx + pmix + hwloc |
| `mpich@4.1` | `mpi@4.0`, `mpich@4` | Reference implementation |
| `intel-mpi@2021` | `mpi@4.0`, `intel-mpi@2021` | Intel clusters |
| `mvapich2@3.0` | `mpi@3.1`, `mvapich2@3` | InfiniBand optimized |

All MPI implementations declare `provides: mpi@4.0`. Applications declare
`requires: mpi@>=3.1`. The resolver picks whichever implementation is in the profile;
the conflict detector prevents two from loading simultaneously.

---

## Tier 1.0 — Math and I/O Foundations

Appear in `build_requires` of nearly every serious scientific application.

| Layer | Provides | Needs |
|---|---|---|
| `openblas@0.3.26` | `blas@3`, `lapack@3` | gcc |
| `fftw@3.3.10` | `fftw@3` | gcc (serial); gcc + mpi (parallel) |
| `scalapack@2.2` | `scalapack@2` | gcc + mpi + blas |
| `suitesparse@7.6` | `suitesparse@7` | gcc + blas |
| `mumps@5.6` | `mumps@5` | gcc + mpi + blas + scalapack |
| `superlu@6.0` | `superlu@6` | gcc + mpi + blas |
| `zlib@1.3`, `lz4@1.9`, `zstd@1.5` | `zlib@1`, etc. | gcc (needed by HDF5) |
| `hdf5@1.14.3` | `hdf5@1.14` | gcc (serial) or gcc + mpi (parallel) |
| `netcdf@4.9` | `netcdf@4` | gcc + hdf5 |
| `pnetcdf@1.12` | `pnetcdf@1` | gcc + mpi |
| `adios2@2.9` | `adios2@2` | gcc + mpi + hdf5 |

---

## Tier 1.5 — Scientific Frameworks

Libraries that appear in `build_requires` of some Tier 2 codes but are also used
directly. They need everything in Tier 1.0.

| Layer | Needs | Used by |
|---|---|---|
| `petsc@3.21` | gcc + mpi + blas + hdf5 (+ mumps, superlu optionally) | FEniCS, OpenFOAM, many CFD codes |
| `trilinos@16` | gcc + mpi + blas + hdf5 | Sierra, Albany |
| `magma@2.7` | gcc + cuda + blas | GPU-accelerated LAPACK; needed by some ML codes |
| `slepc@3.21` | petsc | Eigenvalue solvers |

GROMACS doesn't need PETSc. FEniCS does. Both are Tier 2. PETSc is Tier 1.5.

---

## Tier 2 — Domain Applications

Appear only in user profiles. Strata builds, signs, and records full `built_with`
provenance, but no other catalog layer builds against them.

| Domain | Examples |
|---|---|
| Molecular dynamics | GROMACS, NAMD, LAMMPS, AMBER |
| Climate / weather | WRF, CESM, ICON |
| Quantum chemistry | VASP, NWChem, GAMESS |
| ML frameworks | PyTorch, TensorFlow |
| Bioinformatics | BLAST, SAMtools, BWA, GATK |
| Compiled Python | NumPy, SciPy, h5py, mpi4py (\*) |

(\*) NumPy/SciPy link against BLAS at compile time. Their `built_with` chain is as deep
as a Tier 1.0 layer's even though they're Tier 2 in dependency terms — no other catalog
layer builds against them at the C level. h5py needs HDF5; mpi4py needs MPI.
The Python *interpreter* is Tier 0; pure-Python packages are conda/pip's domain.
Same logic for R: the interpreter is Tier 0; renv manages pure-R packages.

---

## Build Order

```
Tier 0    gcc, LLVM, CUDA, Python, R
              │
Tier 0.5  ucx, libfabric, pmix, hwloc    ← must precede MPI
          openmpi, mpich, mvapich2
              │
Tier 1.0  openblas, fftw                 ← can parallel with Tier 0.5 builds
          zlib, lz4, zstd
          hdf5, netcdf, adios2, pnetcdf  ← needs HDF5
          scalapack, mumps, superlu       ← needs MPI + BLAS
              │
Tier 1.5  petsc, trilinos, magma
              │
Tier 2    gromacs, wrf, pytorch, ...
          numpy, scipy, h5py, mpi4py
```

Within each tier, independent chains build in parallel. Across tiers, the DAG scheduler
enforces ordering. See [build-provenance-chain.md](build-provenance-chain.md) for the
`built_with` / `bootstrap_build` manifest spec.

---

## Toolchain Formations

Once Tier 0.5 is in the registry, a toolchain formation packages the core build
environment for reuse. Tier 1.0, 1.5, and 2 recipes reference a formation rather than
listing individual layers:

```yaml
# meta.yaml for gromacs
build_requires:
  - formation: foss-2024a   # gcc@13.2.0 + openmpi@5.0.6 + openblas@0.3.26 + fftw@3.3.10
  - name: cuda
    min_version: "12.0"
```

The `built_with` field in the resulting manifest records all formation layers plus CUDA.

| Formation | Contents |
|---|---|
| `foss-2024a` | gcc@13.2.0 + openmpi@5.0.6 + openblas@0.3.26 + fftw@3.3.10 |
| `foss-2025a` | gcc@14.2.0 + openmpi@5.0.6 + openblas@0.3.26 + fftw@3.3.10 |
| `cuda-2024a` | cuda@12.3 + gcc@13.2.0 |

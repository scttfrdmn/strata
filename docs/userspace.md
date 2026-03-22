# Strata on HPC and Userspace Systems

## Overview

`strata run` lets you run commands inside a Strata environment **without root privileges**
on HPC login nodes, shared workstations, or any Linux system with FUSE tools available.

## Requirements

| Tool | Purpose | Install |
|------|---------|---------|
| `squashfuse` | Mount squashfs layers as a regular user | `conda install -c conda-forge squashfuse` |
| `fuse-overlayfs` | Merge layers without CAP_SYS_ADMIN | See distro/module system |

Both tools must be in `PATH`. Many HPC systems offer them via modules:

```sh
module load squashfuse fuse-overlayfs
```

## Quickstart

```sh
# Download a frozen lockfile
curl -O https://example.com/my-env.lock.yaml

# Run a command in the environment
strata run --lockfile my-env.lock.yaml -- python train.py --epochs 100
```

Layers are downloaded to `~/.cache/strata/layers/` and cached for subsequent runs.

## User-local Layer Cache

Strata uses the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/latest/) for the layer cache:

| Condition | Cache location |
|-----------|----------------|
| Root | `/strata/cache` |
| `$XDG_CACHE_HOME` set | `$XDG_CACHE_HOME/strata/layers` |
| Default | `~/.cache/strata/layers` |

Layers are named by their SHA256 hash (e.g., `abc123...sqfs`). To free space:

```sh
strata cache-prune --lockfile my-env.lock.yaml --dry-run
strata cache-prune --lockfile my-env.lock.yaml
```

## Air-gapped Systems

If the compute cluster has no outbound internet access, pre-download layers on
a connected node and copy them to the cluster:

```sh
# On a connected node — populate the cache
strata run --lockfile my-env.lock.yaml -- true

# Copy cache to cluster (rsync example)
rsync -av ~/.cache/strata/layers/ cluster:~/.cache/strata/layers/

# On the cluster — run without signature check (Rekor unreachable)
strata run --lockfile my-env.lock.yaml --no-verify -- python train.py
```

## SLURM Example

```bash
#!/bin/bash
#SBATCH --job-name=strata-ml
#SBATCH --nodes=1
#SBATCH --ntasks=1
#SBATCH --cpus-per-task=8
#SBATCH --time=04:00:00

module load squashfuse fuse-overlayfs

strata run \
  --lockfile /shared/envs/ml-training.lock.yaml \
  -- \
  python train.py --data /scratch/$USER/data --output /scratch/$USER/results
```

## Interactive Shell

```sh
strata run --lockfile my-env.lock.yaml -- bash --login
```

This starts a Bash shell with `PATH`, `LD_LIBRARY_PATH`, and `STRATA_*` variables
set appropriately. Use `exit` to leave the environment.

## Environment Variables

`strata run` sets the following variables in the child process:

| Variable | Value |
|----------|-------|
| `PATH` | Per-layer bin directories + original `$PATH` |
| `LD_LIBRARY_PATH` | Per-layer lib directories |
| `STRATA_PROFILE` | Profile name from lockfile |
| `STRATA_ENV` | Path to the merged overlay |
| `STRATA_REKOR_ENTRY` | Rekor log entry for the lockfile |

Additional variables from the profile's `env:` block and `--env` flags are also set.

## Mount Strategy Auto-detection

`strata run` automatically selects the best mount strategy:

1. **Syscall (privileged)**: uses `mount(2)` syscalls — requires `CAP_SYS_ADMIN`
2. **FUSE (unprivileged)**: uses `squashfuse` + `fuse-overlayfs` — no privileges needed

Inside containers, the FUSE strategy is preferred to avoid nested privilege issues.
Container detection checks:
- `$container` environment variable (systemd-nspawn, Flatpak)
- `/run/.containerenv` (Podman)

To force FUSE mode explicitly, ensure `squashfuse` and `fuse-overlayfs` are in PATH
and run Strata inside a container or without `CAP_SYS_ADMIN`.

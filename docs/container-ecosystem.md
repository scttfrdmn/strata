# Strata and Container Ecosystems

## Overview

Strata environments can be used in three container-related ways:

1. **Strata inside a container** — run `strata run` within Docker/Podman/Apptainer
2. **Export to OCI image** — convert a lockfile to an OCI Image Layout for Docker/Podman
3. **Strata as a build environment for containers** — use Strata layers as container build stages

## Strata inside a Privileged Container

Strata works unchanged in privileged containers because `CAP_SYS_ADMIN` is available:

```sh
docker run --privileged -v /path/to/lockfile.yaml:/env.lock.yaml \
  your-base-image strata run --lockfile /env.lock.yaml -- python -c "import torch; print(torch.__version__)"
```

## Strata inside an Unprivileged Docker Container

Use FUSE tools so Strata can mount layers without `CAP_SYS_ADMIN`. The container
needs `/dev/fuse` access:

```sh
docker run \
  --device /dev/fuse \
  --security-opt apparmor:unconfined \
  -v /path/to/lockfile.yaml:/env.lock.yaml \
  your-strata-image \
  strata run --lockfile /env.lock.yaml -- python train.py
```

### Example Containerfile (multi-stage)

```dockerfile
FROM public.ecr.aws/amazonlinux/amazonlinux:2023 AS base

# Install FUSE tools
RUN dnf install -y fuse3 fuse3-libs && \
    dnf clean all

# Install squashfuse from source (not in AL2023 repos)
RUN dnf install -y gcc make zlib-devel && \
    curl -fsSL https://github.com/vasi/squashfuse/releases/download/0.5.2/squashfuse-0.5.2.tar.gz | tar xz && \
    cd squashfuse-0.5.2 && \
    ./configure --prefix=/usr/local && make install && \
    cd .. && rm -rf squashfuse-0.5.2

# Install fuse-overlayfs
RUN curl -fsSL -o /usr/local/bin/fuse-overlayfs \
    https://github.com/containers/fuse-overlayfs/releases/latest/download/fuse-overlayfs-$(uname -m) && \
    chmod +x /usr/local/bin/fuse-overlayfs

# Install strata binary
COPY --from=strata-builder /usr/local/bin/strata /usr/local/bin/strata

ENTRYPOINT ["/usr/local/bin/strata", "run"]
```

## Strata inside Rootless Podman

Rootless Podman provides a user namespace with UID mapping. Strata's FUSE strategy
works without any extra flags because user namespaces give FUSE permission:

```sh
podman run \
  -v $(pwd)/lockfile.yaml:/env.lock.yaml:ro \
  your-strata-image \
  strata run --lockfile /env.lock.yaml -- bash
```

Podman automatically writes `/run/.containerenv`, which Strata detects to skip
the CAP_SYS_ADMIN probe and use FUSE directly.

## Strata inside Apptainer / Singularity

Apptainer runs unprivileged by default. Use `--bind` to mount the lockfile and
pre-populate the layer cache:

```sh
apptainer exec \
  --bind ~/.cache/strata/layers:/root/.cache/strata/layers \
  --bind lockfile.yaml:/env.lock.yaml \
  strata.sif \
  strata run --lockfile /env.lock.yaml -- python -c "import numpy; print(numpy.__version__)"
```

## Exporting to OCI Image Layout

Convert a lockfile's layers directly to an OCI Image Layout tar archive:

```sh
# Export to OCI
strata export \
  --lockfile my-env.lock.yaml \
  --format oci \
  --output my-env.tar \
  --tag my-env:v1.0

# Load into Docker
docker load -i my-env.tar

# Load into Podman
podman load -i my-env.tar

# Run in Docker
docker run --rm my-env:v1.0 python -c "import torch; print(torch.__version__)"
```

### OCI Export Requirements

- `unsquashfs` (from `squashfs-tools`) must be in PATH
- Layers must be locally cached (run `strata run` first, or fetch manually)

### OCI Image Contents

The exported image contains:
- One OCI layer per Strata squashfs layer (in mount order)
- `PATH` and `LD_LIBRARY_PATH` set in the image config
- `STRATA_*` provenance variables
- OCI labels recording SHA256 and Rekor entries for each layer

### Provenance Labels

```
org.opencontainers.image.revision = <environment_id>
strata.lockfile.rekor_entry       = <rekor_log_entry>
strata.layer.<name>.sha256        = <sha256>
strata.layer.<name>.rekor_entry   = <rekor_log_entry>
```

## Comparison: `strata run` vs `strata export`

| | `strata run` | `strata export` |
|---|---|---|
| **Ephemeral use** | Yes | No — produces a static image |
| **Writable upper layer** | Yes (tmpfs) | No |
| **Requires FUSE or root** | Yes | No (CPU-only unpack) |
| **Docker/Podman compatible** | Via `--privileged` or FUSE | Yes, natively |
| **Cache reuse** | Yes — layers shared | Layers unpacked to image |
| **Provenance in image** | Via env vars | Via OCI labels |

Use `strata run` for interactive work and HPC jobs. Use `strata export` when you
need to distribute an environment as a container image to others who don't have Strata.

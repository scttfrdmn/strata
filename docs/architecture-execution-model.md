# Architecture: Execution Model

## Current model: OverlayFS, host libc

The agent assembles layers into an OverlayFS merged view at `/strata/env`.
PATH and LD_LIBRARY_PATH are set to point into the merged view.
The host glibc and ld.so remain at `/lib64/ld-linux-x86-64.so.2`.

Programs spawned in this environment use the host glibc, regardless of
whether a glibc layer is mounted. This is the practical model for most
research software: compiled against a known glibc version, running on a
compatible host.

## Proposed model (unimplemented): bwrap + glibc layer

> **Status (v0.23.0): a proposal, not built.** `grep -rn bwrap` over the tree
> returns nothing — there is no bubblewrap runtime. The current runtime is the
> OverlayFS model above, with a FUSE-overlay fallback where the kernel refuses an
> unprivileged overlay mount (`internal/overlay/mount_fuse_linux.go`). Making
> glibc itself a reproducible layer (rather than relying on a compatible host
> glibc) is still open; whether bwrap, a user-namespace pivot_root, or another
> mechanism provides it has not been decided. The sketch below is kept as the
> statement of the problem, not a description of shipped behaviour.

To make glibc itself a reproducible layer, the agent must present the
assembled layer stack as a complete root filesystem. The Linux kernel
hardcodes the ELF interpreter path: `/lib64/ld-linux-x86-64.so.2`.
This path cannot be changed without recompiling the binary.

The solution: create a private mount namespace where the glibc layer
(which installs to flat `/`) appears at the root.

```
bwrap \
  --ro-bind /strata/layers/glibc/2.34/merged / \   # glibc layer as rootfs
  --ro-bind /strata/layers/gcc/13.2.0 /gcc/13.2.0 \
  --ro-bind /strata/layers/python/3.11.9 /python/3.11.9 \
  --bind /home /home \
  --bind /tmp /tmp \
  -- /python/3.11.9/bin/python3 "$@"
```

### Layer layout requirements

Under the bwrap model:
- glibc layer: `install_layout: flat` — installs to `/` (lib64/ld-linux...)
- All other layers: `install_layout: versioned` — installs to `/<name>/<version>/`
- The merged view of all layers forms a complete, self-contained POSIX rootfs

### Why not containers?

Containers (Docker, Podman) solve a related but different problem: process
isolation, image distribution, network namespacing. Strata's execution model
is purely about filesystem composition — no cgroups, no network namespacing,
no image layers. bwrap provides the minimal kernel primitive (mount namespaces)
needed for filesystem composition without the container overhead.

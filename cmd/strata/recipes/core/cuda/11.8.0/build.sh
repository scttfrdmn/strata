#!/usr/bin/env bash
set -euo pipefail

VERSION="11.8.0"
DRIVER_VER="520.61.05"

# NVIDIA distributes CUDA as a self-extracting .run installer.
# The filename differs between x86_64 and aarch64.
case "${STRATA_ARCH:-x86_64}" in
    x86_64)
        RUNFILE="cuda_${VERSION}_${DRIVER_VER}_linux.run"
        ;;
    arm64|aarch64)
        # CUDA on aarch64 server (AWS Graviton etc.) uses the sbsa runfile.
        RUNFILE="cuda_${VERSION}_${DRIVER_VER}_linux_sbsa.run"
        ;;
    *)
        echo "unsupported arch: ${STRATA_ARCH}" >&2
        exit 1
        ;;
esac

# Installer SHA256-verified into $STRATA_SOURCES by the pipeline (per-arch, meta.yaml, #68).
cp "${STRATA_SOURCES}/cuda_installer.run" cuda_installer.run
chmod +x cuda_installer.run

# Install toolkit-only components to STRATA_PREFIX.
# --no-drm skips the driver, --toolkit installs compilers and libraries only.
./cuda_installer.run \
  --silent \
  --toolkit \
  --toolkitpath="${STRATA_PREFIX}" \
  --no-opengl-libs \
  --no-drm

rm cuda_installer.run

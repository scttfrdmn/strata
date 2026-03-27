#!/usr/bin/env bash
set -euo pipefail

VERSION="12.6.0"
DRIVER_VER="560.28.03"

# NVIDIA distributes CUDA as a self-extracting .run installer.
# The filename differs between x86_64 and aarch64.
case "${STRATA_ARCH:-x86_64}" in
    x86_64)
        RUNFILE="cuda_${VERSION}_${DRIVER_VER}_linux.run"
        ;;
    arm64|aarch64)
        # CUDA 12.x on aarch64 server uses the sbsa (Server Base System
        # Architecture) runfile, not linux_aarch64.run (which is for Jetson).
        RUNFILE="cuda_${VERSION}_${DRIVER_VER}_linux_sbsa.run"
        ;;
    *)
        echo "unsupported arch: ${STRATA_ARCH}" >&2
        exit 1
        ;;
esac
CUDA_RUN_URL="https://developer.download.nvidia.com/compute/cuda/${VERSION}/local_installers/${RUNFILE}"

curl -fsSL -o cuda_installer.run "${CUDA_RUN_URL}"
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

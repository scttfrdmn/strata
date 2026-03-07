#!/usr/bin/env bash
set -euo pipefail

VERSION="12.3.2"
# NVIDIA distributes CUDA as a self-extracting .run installer.
CUDA_RUN_URL="https://developer.download.nvidia.com/compute/cuda/${VERSION}/local_installers/cuda_${VERSION}_545.23.08_linux.run"

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

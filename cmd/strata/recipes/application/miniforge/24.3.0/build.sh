#!/usr/bin/env bash
set -euo pipefail

VERSION="24.3.0-0"
ARCH="${STRATA_ARCH}"

# Map Strata arch names to Miniforge installer arch suffixes.
case "${ARCH}" in
  x86_64) INSTALLER_ARCH="x86_64" ;;
  arm64)  INSTALLER_ARCH="aarch64" ;;
  *) echo "Unsupported arch: ${ARCH}" >&2; exit 1 ;;
esac

URL="https://github.com/conda-forge/miniforge/releases/download/${VERSION}/Miniforge3-${VERSION}-Linux-${INSTALLER_ARCH}.sh"

curl -fsSL -o miniforge_installer.sh "${URL}"
chmod +x miniforge_installer.sh

# Install in batch mode to STRATA_PREFIX. -u allows install into a pre-existing directory
# (the build pipeline pre-creates STRATA_PREFIX before running the recipe).
./miniforge_installer.sh -b -u -p "${STRATA_PREFIX}"

rm miniforge_installer.sh

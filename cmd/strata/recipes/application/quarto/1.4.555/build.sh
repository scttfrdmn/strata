#!/usr/bin/env bash
set -euo pipefail

VERSION="1.4.555"
ARCH="${STRATA_ARCH}"

# Map Strata arch names to Quarto installer arch suffixes.
case "${ARCH}" in
  x86_64) PKG_ARCH="amd64" ;;
  arm64)  PKG_ARCH="arm64" ;;
  *) echo "Unsupported arch: ${ARCH}" >&2; exit 1 ;;
esac


# Source SHA256-verified into $STRATA_SOURCES by the pipeline (per-arch, meta.yaml, #68).
tar xf "${STRATA_SOURCES}/quarto.tar.gz"

# Move extracted tree into STRATA_PREFIX.
mv "quarto-${VERSION}" "${STRATA_PREFIX}/quarto"

# Symlink the binary for PATH access.
mkdir -p "${STRATA_PREFIX}/bin"
ln -sf "${STRATA_PREFIX}/quarto/bin/quarto" "${STRATA_PREFIX}/bin/quarto"

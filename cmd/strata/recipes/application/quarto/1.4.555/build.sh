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

URL="https://github.com/quarto-dev/quarto-cli/releases/download/v${VERSION}/quarto-${VERSION}-linux-${PKG_ARCH}.tar.gz"

curl -fsSL "${URL}" | tar -xz

# Move extracted tree into STRATA_PREFIX.
mv "quarto-${VERSION}" "${STRATA_PREFIX}/quarto"

# Symlink the binary for PATH access.
mkdir -p "${STRATA_PREFIX}/bin"
ln -sf "${STRATA_PREFIX}/quarto/bin/quarto" "${STRATA_PREFIX}/bin/quarto"

#!/usr/bin/env bash
# Julia 1.11.3 build script for Strata
# Uses the official pre-built binary tarball.
set -euo pipefail

VERSION="1.11.3"
MAJOR_MINOR="1.11"

case "${STRATA_ARCH}" in
    x86_64)  ARCH_TAG="linux-x86_64"; URL_ARCH="x64" ;;
    arm64)   ARCH_TAG="linux-aarch64"; URL_ARCH="aarch64" ;;
    *)       echo "Unsupported arch: ${STRATA_ARCH}"; exit 1 ;;
esac

TARBALL="julia-${VERSION}-${ARCH_TAG}.tar.gz"
URL="https://julialang-s3.julialang.org/bin/linux/${URL_ARCH}/${MAJOR_MINOR}/${TARBALL}"

cd /tmp
curl -fsSL "$URL" -o "$TARBALL"
tar xf "$TARBALL"

# Install by copying the pre-built layout.
cp -a "julia-${VERSION}/." "${STRATA_INSTALL_PREFIX}/"

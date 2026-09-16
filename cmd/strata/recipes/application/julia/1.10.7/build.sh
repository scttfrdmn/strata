#!/usr/bin/env bash
# Julia 1.10.7 build script for Strata
# Uses the official pre-built binary tarball.
set -euo pipefail

VERSION="1.10.7"
MAJOR_MINOR="1.10"

case "${STRATA_ARCH}" in
    x86_64)  ARCH_TAG="linux-x86_64"; URL_ARCH="x64" ;;
    arm64)   ARCH_TAG="linux-aarch64"; URL_ARCH="aarch64" ;;
    *)       echo "Unsupported arch: ${STRATA_ARCH}"; exit 1 ;;
esac


cd /tmp
# Source SHA256-verified into $STRATA_SOURCES by the pipeline (per-arch, meta.yaml, #68).
tar xf "${STRATA_SOURCES}/julia.tar.gz"

# Install by copying the pre-built layout.
cp -a "julia-${VERSION}/." "${STRATA_INSTALL_PREFIX}/"

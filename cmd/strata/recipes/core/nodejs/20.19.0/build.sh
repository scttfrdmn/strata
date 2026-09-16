#!/usr/bin/env bash
# Node.js 20.19.0 (LTS) build script for Strata
# Uses the pre-built binary tarball for speed and reproducibility.
set -euo pipefail

VERSION="20.19.0"

case "${STRATA_ARCH}" in
    x86_64)  ARCH_TAG="linux-x64" ;;
    arm64)   ARCH_TAG="linux-arm64" ;;
    *)       echo "Unsupported arch: ${STRATA_ARCH}"; exit 1 ;;
esac


cd /tmp
# Source SHA256-verified into $STRATA_SOURCES by the pipeline (per-arch, meta.yaml, #68).
tar xf "${STRATA_SOURCES}/node.tar.xz"

# Install by copying the pre-built layout into the install prefix.
cp -a "node-v${VERSION}-${ARCH_TAG}/." "${STRATA_INSTALL_PREFIX}/"

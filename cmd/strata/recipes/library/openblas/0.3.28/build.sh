#!/usr/bin/env bash
# OpenBLAS 0.3.28 build script for Strata
set -euo pipefail

VERSION="0.3.28"
TARBALL="OpenBLAS-${VERSION}.tar.gz"
URL="https://github.com/OpenMathLib/OpenBLAS/releases/download/v${VERSION}/${TARBALL}"

cd /tmp
curl -fsSL "$URL" -o "$TARBALL"
tar xf "$TARBALL"
cd "OpenBLAS-${VERSION}"

# Use the detected number of CPUs and disable affinity for reproducible builds.
make -j"${STRATA_NCPUS}" \
    USE_THREAD=1 \
    NUM_THREADS=64 \
    NO_AFFINITY=1 \
    DYNAMIC_ARCH=1

make install PREFIX="${STRATA_INSTALL_PREFIX}"

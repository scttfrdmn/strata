#!/usr/bin/env bash
# OpenBLAS 0.3.28 build script for Strata
set -euo pipefail

VERSION="0.3.28"
TARBALL="OpenBLAS-${VERSION}.tar.gz"

cd /tmp
# Source SHA256-verified into $STRATA_SOURCES by the pipeline (meta.yaml, #68).
tar xf "${STRATA_SOURCES}/OpenBLAS-0.3.28.tar.gz"
cd "OpenBLAS-${VERSION}"

# Use the detected number of CPUs and disable affinity for reproducible builds.
make -j"${STRATA_NCPUS}" \
    USE_THREAD=1 \
    NUM_THREADS=64 \
    NO_AFFINITY=1 \
    DYNAMIC_ARCH=1

make install PREFIX="${STRATA_INSTALL_PREFIX}"

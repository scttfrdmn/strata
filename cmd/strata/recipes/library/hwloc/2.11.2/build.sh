#!/usr/bin/env bash
set -euo pipefail

VERSION="2.11.2"

# Source SHA256-verified into $STRATA_SOURCES by the pipeline (meta.yaml, #68).
tar xf "${STRATA_SOURCES}/hwloc-2.11.2.tar.bz2"
cd "hwloc-${VERSION}"

./configure \
  --prefix="${STRATA_INSTALL_PREFIX}" \
  --enable-shared \
  --disable-static \
  --disable-opencl \
  --disable-cuda \
  --disable-nvml \
  --disable-gl \
  --with-pic

make -j"${STRATA_NCPUS}"
make install

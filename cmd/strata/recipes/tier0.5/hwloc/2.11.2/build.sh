#!/usr/bin/env bash
set -euo pipefail

VERSION="2.11.2"
URL="https://download.open-mpi.org/release/hwloc/v2.11/hwloc-${VERSION}.tar.bz2"

curl -fsSL "${URL}" | tar -xj
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

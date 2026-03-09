#!/usr/bin/env bash
set -euo pipefail

VERSION="5.0.10"
URL="https://download.open-mpi.org/release/open-mpi/v5.0/openmpi-${VERSION}.tar.bz2"

curl -fsSL "${URL}" | tar -xj
cd "openmpi-${VERSION}"

# STRATA_BUILD_ENV is the OverlayFS merged view of all build_requires layers.
# ucx, hwloc, pmix, and libfabric are all mounted there.
./configure \
  --prefix="${STRATA_INSTALL_PREFIX}" \
  --enable-mpi-fortran \
  --enable-shared \
  --disable-static \
  --with-pic \
  --with-ucx="${STRATA_BUILD_ENV}" \
  --with-hwloc="${STRATA_BUILD_ENV}" \
  --with-pmix="${STRATA_BUILD_ENV}" \
  --with-ofi="${STRATA_BUILD_ENV}" \
  --with-verbs=no

make -j"${STRATA_NCPUS}"
make install

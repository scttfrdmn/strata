#!/usr/bin/env bash
set -euo pipefail

VERSION="5.0.10"
URL="https://download.open-mpi.org/release/open-mpi/v5.0/openmpi-${VERSION}.tar.bz2"

curl -fsSL "${URL}" | tar -xj
cd "openmpi-${VERSION}"

# Per-package build env vars point to each dependency's install prefix within
# the OverlayFS merged view: STRATA_BUILD_ENV_<NAME>=<merged>/<name>/<version>
./configure \
  --prefix="${STRATA_INSTALL_PREFIX}" \
  --enable-mpi-fortran \
  --enable-shared \
  --disable-static \
  --with-pic \
  --with-ucx="${STRATA_BUILD_ENV_UCX}" \
  --with-hwloc="${STRATA_BUILD_ENV_HWLOC}" \
  --with-pmix="${STRATA_BUILD_ENV_PMIX}" \
  --with-ofi="${STRATA_BUILD_ENV_LIBFABRIC}" \
  --with-verbs=no

make -j"${STRATA_NCPUS}"
make install

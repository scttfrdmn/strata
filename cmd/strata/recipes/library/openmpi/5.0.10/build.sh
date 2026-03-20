#!/usr/bin/env bash
set -euo pipefail

VERSION="5.0.10"
URL="https://download.open-mpi.org/release/open-mpi/v5.0/openmpi-${VERSION}.tar.bz2"

curl -fsSL "${URL}" | tar -xj
cd "openmpi-${VERSION}"

# .la files in build-env layers embed the build-machine's /tmp/strata-build-*/
# path as libdir=. Libtool on this instance can't follow those paths. Remove
# them so libtool falls back to the .so files directly.
find "${STRATA_BUILD_ENV}" -name '*.la' -delete 2>/dev/null || true

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

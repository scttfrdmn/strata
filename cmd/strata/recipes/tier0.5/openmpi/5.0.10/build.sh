#!/usr/bin/env bash
# NOTE: This script must run inside a Strata environment with gcc mounted.
# The build pipeline (v0.10.0+) mounts the gcc layer via OverlayFS before
# invoking this script. Do NOT run this with the system compiler.
set -euo pipefail

VERSION="5.0.10"
URL="https://download.open-mpi.org/release/open-mpi/v5.0/openmpi-${VERSION}.tar.bz2"

curl -fsSL "${URL}" | tar -xj
cd "openmpi-${VERSION}"

./configure \
  --prefix="${STRATA_INSTALL_PREFIX}" \
  --enable-mpi-fortran \
  --enable-shared \
  --disable-static \
  --with-pic

make -j"${STRATA_NCPUS}"
make install

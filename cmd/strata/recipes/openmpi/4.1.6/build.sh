#!/usr/bin/env bash
set -euo pipefail

VERSION="4.1.6"
URL="https://download.open-mpi.org/release/open-mpi/v4.1/openmpi-${VERSION}.tar.bz2"

curl -fsSL "${URL}" | tar -xj
cd "openmpi-${VERSION}"

./configure \
  --prefix="${STRATA_PREFIX}" \
  --enable-shared \
  --enable-static=no \
  --with-slurm

make -j"${STRATA_NCPUS}"
make install

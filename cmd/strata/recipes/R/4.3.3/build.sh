#!/usr/bin/env bash
set -euo pipefail

VERSION="4.3.3"
URL="https://cran.r-project.org/src/base/R-${VERSION}.tar.gz"

curl -fsSL "${URL}" | tar -xz
cd "R-${VERSION}"

./configure \
  --prefix="${STRATA_PREFIX}" \
  --enable-R-shlib \
  --with-blas \
  --with-lapack \
  --enable-memory-profiling

make -j"${STRATA_NCPUS}"
make install

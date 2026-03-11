#!/usr/bin/env bash
set -euo pipefail

VERSION="4.4.3"
URL="https://cran.r-project.org/src/base/R-4/R-${VERSION}.tar.gz"

curl -fsSL "${URL}" | tar -xz
cd "R-${VERSION}"

./configure \
  --prefix="${STRATA_INSTALL_PREFIX}" \
  --enable-R-shlib \
  --with-blas \
  --with-lapack \
  --without-x \
  --without-recommended-packages \
  --enable-memory-profiling

make -j"${STRATA_NCPUS}"
make install

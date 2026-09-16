#!/usr/bin/env bash
set -euo pipefail

VERSION="4.5.2"

# Source fetched + SHA256-verified into $STRATA_SOURCES by the pipeline (meta.yaml, #68).
tar -xzf "${STRATA_SOURCES}/R-${VERSION}.tar.gz"
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

#!/usr/bin/env bash
set -euo pipefail

VERSION="14.2.0"

# Source SHA256-verified into $STRATA_SOURCES by the pipeline (meta.yaml, #68).
tar xf "${STRATA_SOURCES}/gcc-14.2.0.tar.xz"
cd "gcc-${VERSION}"

# Download GCC prerequisites (GMP, MPFR, MPC).
contrib/download_prerequisites

mkdir -p ../gcc-build
cd ../gcc-build

"../gcc-${VERSION}/configure" \
  --prefix="${STRATA_INSTALL_PREFIX}" \
  --enable-languages=c,c++,fortran \
  --disable-multilib \
  --with-system-zlib

make -j"${STRATA_NCPUS}"
make install

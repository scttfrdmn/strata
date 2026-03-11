#!/usr/bin/env bash
set -euo pipefail

VERSION="13.2.0"
URL="https://ftp.gnu.org/gnu/gcc/gcc-${VERSION}/gcc-${VERSION}.tar.xz"

curl -fsSL "${URL}" | tar -xJ
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

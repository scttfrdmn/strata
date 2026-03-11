#!/usr/bin/env bash
set -euo pipefail

VERSION="3.11.11"
URL="https://www.python.org/ftp/python/${VERSION}/Python-${VERSION}.tgz"

curl -fsSL "${URL}" | tar -xz
cd "Python-${VERSION}"

./configure \
  --prefix="${STRATA_INSTALL_PREFIX}" \
  --enable-optimizations \
  --with-lto \
  --enable-shared \
  LDFLAGS="-Wl,-rpath,${STRATA_INSTALL_PREFIX}/lib"

make -j"${STRATA_NCPUS}"
make install

# Ensure pip is available under the versioned name.
"${STRATA_INSTALL_PREFIX}/bin/python3.11" -m ensurepip --upgrade

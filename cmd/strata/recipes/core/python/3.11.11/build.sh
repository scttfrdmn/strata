#!/usr/bin/env bash
set -euo pipefail

VERSION="3.11.11"

# Source fetched + SHA256-verified into $STRATA_SOURCES by the pipeline (meta.yaml, #68).
tar -xzf "${STRATA_SOURCES}/Python-${VERSION}.tgz"
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

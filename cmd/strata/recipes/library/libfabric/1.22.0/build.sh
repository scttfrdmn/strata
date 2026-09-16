#!/usr/bin/env bash
set -euo pipefail

VERSION="1.22.0"

# Source SHA256-verified into $STRATA_SOURCES by the pipeline (meta.yaml, #68).
tar xf "${STRATA_SOURCES}/libfabric-1.22.0.tar.bz2"
cd "libfabric-${VERSION}"

./configure \
  --prefix="${STRATA_INSTALL_PREFIX}" \
  --enable-shared \
  --disable-static \
  --with-pic

make -j"${STRATA_NCPUS}"
make install

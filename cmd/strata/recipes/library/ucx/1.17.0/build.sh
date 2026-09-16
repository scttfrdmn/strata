#!/usr/bin/env bash
set -euo pipefail

VERSION="1.17.0"

# Source SHA256-verified into $STRATA_SOURCES by the pipeline (meta.yaml, #68).
tar xf "${STRATA_SOURCES}/ucx-1.17.0.tar.gz"
cd "ucx-${VERSION}"

./configure \
  --prefix="${STRATA_INSTALL_PREFIX}" \
  --enable-shared \
  --disable-static \
  --enable-optimizations \
  --disable-logging \
  --disable-debug \
  --disable-assertions \
  --disable-params-check \
  --with-pic

make -j"${STRATA_NCPUS}"
make install

#!/usr/bin/env bash
set -euo pipefail

VERSION="1.17.0"
URL="https://github.com/openucx/ucx/releases/download/v${VERSION}/ucx-${VERSION}.tar.gz"

curl -fsSL "${URL}" | tar -xz
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

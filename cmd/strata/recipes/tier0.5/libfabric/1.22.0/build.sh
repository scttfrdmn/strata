#!/usr/bin/env bash
set -euo pipefail

VERSION="1.22.0"
URL="https://github.com/ofiwg/libfabric/releases/download/v${VERSION}/libfabric-${VERSION}.tar.bz2"

curl -fsSL "${URL}" | tar -xj
cd "libfabric-${VERSION}"

./configure \
  --prefix="${STRATA_INSTALL_PREFIX}" \
  --enable-shared \
  --disable-static \
  --with-pic

make -j"${STRATA_NCPUS}"
make install

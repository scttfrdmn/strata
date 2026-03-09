#!/usr/bin/env bash
set -euo pipefail

VERSION="5.0.3"
URL="https://github.com/pmix/pmix/releases/download/v${VERSION}/pmix-${VERSION}.tar.bz2"

curl -fsSL "${URL}" | tar -xj
cd "pmix-${VERSION}"

./configure \
  --prefix="${STRATA_INSTALL_PREFIX}" \
  --enable-shared \
  --disable-static \
  --with-hwloc="${STRATA_BUILD_ENV_HWLOC:-/usr}" \
  --with-pic

make -j"${STRATA_NCPUS}"
make install

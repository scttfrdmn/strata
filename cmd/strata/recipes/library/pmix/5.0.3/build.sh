#!/usr/bin/env bash
set -euo pipefail

VERSION="5.0.3"

# Source SHA256-verified into $STRATA_SOURCES by the pipeline (meta.yaml, #68).
tar xf "${STRATA_SOURCES}/pmix-5.0.3.tar.bz2"
cd "pmix-${VERSION}"

./configure \
  --prefix="${STRATA_INSTALL_PREFIX}" \
  --enable-shared \
  --disable-static \
  --with-hwloc="${STRATA_BUILD_ENV_HWLOC:-/usr}" \
  --with-pic

make -j"${STRATA_NCPUS}"
make install

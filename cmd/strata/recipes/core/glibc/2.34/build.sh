#!/usr/bin/env bash
# glibc 2.34 build recipe (STUB — requires bwrap agent support, v0.13.0+)
# This recipe will NOT succeed until the agent's pivot_root execution model is implemented.
# See docs/architecture-execution-model.md for the required execution context.
set -euo pipefail

VERSION="2.34"

# glibc must be built outside its own source tree.
mkdir -p build-glibc
# Source fetched + SHA256-verified into $STRATA_SOURCES by the pipeline (meta.yaml, #68).
tar -xJf "${STRATA_SOURCES}/glibc-${VERSION}.tar.xz"
cd build-glibc

# Install to STRATA_INSTALL_PREFIX (= STRATA_PREFIX for flat layout).
# The flat layout places lib64/ld-linux-x86-64.so.2 at the expected kernel path.
../glibc-${VERSION}/configure \
  --prefix="${STRATA_INSTALL_PREFIX}" \
  --enable-shared \
  --disable-werror

make -j"${STRATA_NCPUS}"
make install

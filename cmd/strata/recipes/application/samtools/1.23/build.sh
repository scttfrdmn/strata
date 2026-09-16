#!/usr/bin/env bash
set -euo pipefail

# Upstream samtools uses two-part version tags (e.g. "1.23", not "1.23.0").
VERSION="1.23"

# Build htslib first (samtools depends on it).
tar xf "${STRATA_SOURCES}/htslib-1.23.tar.bz2"
cd "htslib-${VERSION}"
./configure --prefix="${STRATA_INSTALL_PREFIX}"
make -j"${STRATA_NCPUS}"
make install
cd ..

# Build samtools linked against the installed htslib.
tar xf "${STRATA_SOURCES}/samtools-1.23.tar.bz2"
cd "samtools-${VERSION}"
./configure \
  --prefix="${STRATA_INSTALL_PREFIX}" \
  --with-htslib="${STRATA_INSTALL_PREFIX}"
make -j"${STRATA_NCPUS}"
make install

#!/usr/bin/env bash
set -euo pipefail

# Upstream samtools uses two-part version tags (e.g. "1.23", not "1.23.0").
VERSION="1.23"
SAMTOOLS_URL="https://github.com/samtools/samtools/releases/download/${VERSION}/samtools-${VERSION}.tar.bz2"
HTSLIB_URL="https://github.com/samtools/htslib/releases/download/${VERSION}/htslib-${VERSION}.tar.bz2"

# Build htslib first (samtools depends on it).
curl -fsSL "${HTSLIB_URL}" | tar -xj
cd "htslib-${VERSION}"
./configure --prefix="${STRATA_INSTALL_PREFIX}"
make -j"${STRATA_NCPUS}"
make install
cd ..

# Build samtools linked against the installed htslib.
curl -fsSL "${SAMTOOLS_URL}" | tar -xj
cd "samtools-${VERSION}"
./configure \
  --prefix="${STRATA_INSTALL_PREFIX}" \
  --with-htslib="${STRATA_INSTALL_PREFIX}"
make -j"${STRATA_NCPUS}"
make install

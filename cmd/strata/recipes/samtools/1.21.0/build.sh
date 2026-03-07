#!/usr/bin/env bash
set -euo pipefail

# Upstream samtools uses version "1.21" (no patch segment).
SAMTOOLS_VERSION="1.21"
HTSLIB_VERSION="1.21"
SAMTOOLS_URL="https://github.com/samtools/samtools/releases/download/${SAMTOOLS_VERSION}/samtools-${SAMTOOLS_VERSION}.tar.bz2"
HTSLIB_URL="https://github.com/samtools/htslib/releases/download/${HTSLIB_VERSION}/htslib-${HTSLIB_VERSION}.tar.bz2"

# Build htslib first (samtools depends on it).
curl -fsSL "${HTSLIB_URL}" | tar -xj
cd "htslib-${HTSLIB_VERSION}"
./configure --prefix="${STRATA_PREFIX}"
make -j"${STRATA_NCPUS}"
make install
cd ..

# Build samtools linked against the installed htslib.
curl -fsSL "${SAMTOOLS_URL}" | tar -xj
cd "samtools-${SAMTOOLS_VERSION}"
./configure \
  --prefix="${STRATA_PREFIX}" \
  --with-htslib="${STRATA_PREFIX}"
make -j"${STRATA_NCPUS}"
make install

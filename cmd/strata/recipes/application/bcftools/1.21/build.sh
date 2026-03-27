#!/usr/bin/env bash
# BCFtools 1.21 build script for Strata.
# Compiles from source; ships its own htslib (bundled in the release tarball).
set -euo pipefail

VERSION="1.21"
URL="https://github.com/samtools/bcftools/releases/download/${VERSION}/bcftools-${VERSION}.tar.bz2"

curl -fsSL "$URL" -o bcftools.tar.bz2
tar xf bcftools.tar.bz2
cd "bcftools-${VERSION}"

# Disable optional GSL and Perl-filter dependencies to keep the build
# self-contained on a plain AL2023 instance.
./configure \
  --prefix="${STRATA_INSTALL_PREFIX}" \
  --enable-libgsl=no \
  --enable-perl-filters=no

make -j"${STRATA_NCPUS}"
make install

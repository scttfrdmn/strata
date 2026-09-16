#!/usr/bin/env bash
# BCFtools 1.21 build script for Strata.
# Compiles from source; ships its own htslib (bundled in the release tarball).
set -euo pipefail

VERSION="1.21"

# Source is fetched and SHA256-verified by the build pipeline into $STRATA_SOURCES
# (pinned in meta.yaml), so the build is reproducible from the recipe (#68).
tar xf "${STRATA_SOURCES}/bcftools-${VERSION}.tar.bz2"
cd "bcftools-${VERSION}"

# Disable optional GSL and Perl-filter dependencies to keep the build
# self-contained on a plain AL2023 instance.
./configure \
  --prefix="${STRATA_INSTALL_PREFIX}" \
  --enable-libgsl=no \
  --enable-perl-filters=no

make -j"${STRATA_NCPUS}"
make install

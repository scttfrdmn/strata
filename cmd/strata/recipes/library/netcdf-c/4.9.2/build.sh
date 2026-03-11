#!/usr/bin/env bash
# NetCDF-C 4.9.2 build script for Strata
# Requires HDF5 in the build environment (via build_requires).
set -euo pipefail

VERSION="4.9.2"
TARBALL="netcdf-c-${VERSION}.tar.gz"
URL="https://github.com/Unidata/netcdf-c/archive/refs/tags/v${VERSION}.tar.gz"

cd /tmp
curl -fsSL "$URL" -o "$TARBALL"
tar xf "$TARBALL"
cd "netcdf-c-${VERSION}"

./configure \
    --prefix="${STRATA_INSTALL_PREFIX}" \
    --enable-shared \
    --enable-netcdf-4 \
    --enable-hdf5 \
    --with-hdf5="${STRATA_BUILD_ENV_HDF5}" \
    --disable-dap \
    CPPFLAGS="-I${STRATA_BUILD_ENV_HDF5}/include" \
    LDFLAGS="-L${STRATA_BUILD_ENV_HDF5}/lib -L${STRATA_BUILD_ENV_HDF5}/lib64"

make -j"${STRATA_NCPUS}"
make install

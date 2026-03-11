#!/usr/bin/env bash
# HDF5 1.14.4 build script for Strata
set -euo pipefail

VERSION="1.14.4"
# GitHub tag for latest 1.14.4.x patch; tarball dir is hdf5-hdf5_1.14.4.3
TAG="hdf5_1.14.4.3"
TARBALL="${TAG}.tar.gz"
URL="https://github.com/HDFGroup/hdf5/archive/refs/tags/${TAG}.tar.gz"

dnf install -y cmake gcc-c++ gcc-gfortran zlib-devel

cd /tmp
curl -fsSL "$URL" -o "$TARBALL"
tar xf "$TARBALL"
cd "hdf5-${TAG}"

mkdir build && cd build
cmake .. \
    -DCMAKE_INSTALL_PREFIX="${STRATA_INSTALL_PREFIX}" \
    -DBUILD_SHARED_LIBS=ON \
    -DHDF5_BUILD_CPP_LIB=OFF \
    -DHDF5_BUILD_FORTRAN=ON \
    -DHDF5_BUILD_HL_LIB=ON \
    -DHDF5_ENABLE_Z_LIB_SUPPORT=ON \
    -DHDF5_ENABLE_SZIP_SUPPORT=OFF \
    -DHDF5_WARNINGS_ARE_ERRORS=OFF \
    -DCMAKE_BUILD_TYPE=Release

cmake --build . -j"${STRATA_NCPUS}"
cmake --install .

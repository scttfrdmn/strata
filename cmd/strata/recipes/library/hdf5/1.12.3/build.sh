#!/usr/bin/env bash
# HDF5 1.12.3 build script for Strata
set -euo pipefail

VERSION="1.12.3"
TAG="hdf5-1_12_3"
TARBALL="${TAG}.tar.gz"
# HDF5 source is on GitHub; tag format for 1.12.x: hdf5-1_12_3

dnf install -y cmake gcc-c++ gcc-gfortran zlib-devel

cd /tmp
# Source SHA256-verified into $STRATA_SOURCES by the pipeline (meta.yaml, #68).
tar xf "${STRATA_SOURCES}/hdf5-1_12_3.tar.gz"
cd "hdf5-${TAG}"

mkdir build && cd build
cmake .. \
    -DCMAKE_INSTALL_PREFIX="${STRATA_INSTALL_PREFIX}" \
    -DBUILD_SHARED_LIBS=ON \
    -DHDF5_BUILD_CPP_LIB=ON \
    -DHDF5_BUILD_FORTRAN=ON \
    -DHDF5_BUILD_HL_LIB=ON \
    -DHDF5_ENABLE_Z_LIB_SUPPORT=ON \
    -DHDF5_ENABLE_SZIP_SUPPORT=OFF \
    -DCMAKE_BUILD_TYPE=Release

cmake --build . -j"${STRATA_NCPUS}"
cmake --install .

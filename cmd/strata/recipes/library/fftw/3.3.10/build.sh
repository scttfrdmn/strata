#!/usr/bin/env bash
# FFTW 3.3.10 build script for Strata
# Builds double, single (--enable-float), and long-double (--enable-long-double)
# precision libraries in a single pass for maximum application compatibility.
set -euo pipefail

VERSION="3.3.10"
TARBALL="fftw-${VERSION}.tar.gz"
URL="http://www.fftw.org/${TARBALL}"

cd /tmp
curl -fsSL "$URL" -o "$TARBALL"
tar xf "$TARBALL"

# Common configure flags.
COMMON_FLAGS="--prefix=${STRATA_INSTALL_PREFIX} --enable-shared --enable-threads --enable-openmp"

case "${STRATA_ARCH}" in
    x86_64)
        SIMD_FLAGS="--enable-avx --enable-avx2 --enable-avx512 --enable-sse2"
        ;;
    arm64)
        SIMD_FLAGS="--enable-neon"
        ;;
    *)
        SIMD_FLAGS=""
        ;;
esac

build_fftw() {
    local dir="fftw-${VERSION}-${1}"
    cp -a "fftw-${VERSION}" "$dir"
    pushd "$dir"
    # shellcheck disable=SC2086
    ./configure $COMMON_FLAGS $SIMD_FLAGS ${@:2}
    make -j"${STRATA_NCPUS}"
    make install
    popd
}

# Double precision (default)
build_fftw double

# Single precision
build_fftw float --enable-float

# Long double precision (x86 only; not available on ARM).
# Long double is not compatible with SSE2/AVX; build without SIMD flags.
if [ "${STRATA_ARCH}" = "x86_64" ]; then
    SIMD_FLAGS_SAVE="$SIMD_FLAGS"
    SIMD_FLAGS=""
    build_fftw longdouble --enable-long-double
    SIMD_FLAGS="$SIMD_FLAGS_SAVE"
fi

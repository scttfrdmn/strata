#!/usr/bin/env bash
# Lmod 8.7.37 build script for Strata
# Lmod requires Lua (available on AL2023) and tcl (optional, for TCL module compat).
set -euo pipefail

VERSION="8.7.37"
SRC="Lmod-${VERSION}.tar.bz2"
URL="https://github.com/TACC/Lmod/archive/refs/tags/${VERSION}.tar.gz"

dnf install -y tcl lua lua-posix lua-devel

cd /tmp
curl -fsSL "$URL" -o "Lmod-${VERSION}.tar.gz"
tar xf "Lmod-${VERSION}.tar.gz"
cd "Lmod-${VERSION}"

./configure \
    --prefix="${STRATA_INSTALL_PREFIX}" \
    --with-module-root-path="${STRATA_INSTALL_PREFIX}/modulefiles" \
    --with-fastTCLInterp=no

make -j"${STRATA_NCPUS}"
make install

# Lmod installs a shell init script; create a convenience symlink.
ln -sf "${STRATA_INSTALL_PREFIX}/lmod/lmod/init/bash" \
    "${STRATA_INSTALL_PREFIX}/init.sh" 2>/dev/null || true

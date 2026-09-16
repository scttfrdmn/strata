#!/usr/bin/env bash
# Rust 1.82.0 build script for Strata.
# Installs the pinned official standalone toolchain (rustc + cargo + rust-std +
# rustfmt + clippy) from the SHA256-verified tarball in $STRATA_SOURCES, replacing
# the old unpinned `curl https://sh.rustup.rs | sh` bootstrap (#68). rustc, cargo
# and friends land in $STRATA_INSTALL_PREFIX/bin via the tarball's install.sh.
set -euo pipefail

VERSION="1.82.0"

case "${STRATA_ARCH}" in
    x86_64)  TARGET="x86_64-unknown-linux-gnu" ;;
    arm64)   TARGET="aarch64-unknown-linux-gnu" ;;
    *)       echo "Unsupported arch: ${STRATA_ARCH}"; exit 1 ;;
esac

# Source SHA256-verified into $STRATA_SOURCES by the pipeline (per-arch, meta.yaml).
tar xf "${STRATA_SOURCES}/rust.tar.gz"
cd "rust-${VERSION}-${TARGET}"

./install.sh --prefix="${STRATA_INSTALL_PREFIX}" --disable-ldconfig

#!/usr/bin/env bash
# Rust 1.82.0 build script for Strata.
# Uses the official rustup installer to bootstrap a pinned stable toolchain.
# rustup writes tool binaries to CARGO_HOME/bin and toolchain files to RUSTUP_HOME.
set -euo pipefail

VERSION="1.82.0"

# Point rustup and cargo at the layer install prefix so bin/rustc, bin/cargo,
# etc. land at the expected squashfs paths.
export CARGO_HOME="${STRATA_INSTALL_PREFIX}"
export RUSTUP_HOME="${STRATA_INSTALL_PREFIX}/.rustup"

# Install rustup + the pinned stable toolchain.
# --no-modify-path: skip modifying ~/.bashrc / ~/.profile (not needed on build instance).
# --profile minimal: rustc + cargo + rust-std + rustfmt + clippy only.
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | \
  sh -s -- -y \
    --default-toolchain "${VERSION}" \
    --profile minimal \
    --no-modify-path

#!/usr/bin/env bash
set -euo pipefail

VERSION="4.2.0"

# Install JupyterLab into the Strata prefix using pip from the python layer.
"${STRATA_PREFIX}/bin/pip" install \
  --prefix="${STRATA_PREFIX}" \
  --no-cache-dir \
  "jupyterlab==${VERSION}"

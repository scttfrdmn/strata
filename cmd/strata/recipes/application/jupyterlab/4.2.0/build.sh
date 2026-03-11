#!/usr/bin/env bash
set -euo pipefail

VERSION="4.2.0"

# PYTHONHOME lets the build-env python3 binary find its own stdlib at the
# overlay path (shebangs in pip3 point to the compiled-in prefix, but
# calling python3 -m pip with PYTHONHOME set avoids the shebang issue).
export PYTHONHOME="${STRATA_BUILD_ENV_PYTHON}"
"${STRATA_BUILD_ENV_PYTHON}/bin/python3" -m pip install \
  --prefix="${STRATA_INSTALL_PREFIX}" \
  --no-cache-dir \
  "jupyterlab==${VERSION}"

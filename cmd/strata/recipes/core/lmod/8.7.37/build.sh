#!/usr/bin/env bash
# Lmod 8.7.37 build script for Strata
# Bundles the Lua interpreter into the layer so target instances do not need
# lua installed on the host OS.
set -euo pipefail

LMOD_VERSION="8.7.37"
URL="https://github.com/TACC/Lmod/archive/refs/tags/${LMOD_VERSION}.tar.gz"

# Install system lua for the build. We will bundle a copy in the layer.
dnf install -y tcl lua lua-posix lua-devel readline-devel

# ── Build Lmod ────────────────────────────────────────────────────────────────
cd /tmp
curl -fsSL "$URL" -o "Lmod-${LMOD_VERSION}.tar.gz"
tar xf "Lmod-${LMOD_VERSION}.tar.gz"
cd "Lmod-${LMOD_VERSION}"

./configure \
    --prefix="${STRATA_INSTALL_PREFIX}" \
    --with-module-root-path="${STRATA_INSTALL_PREFIX}/modulefiles" \
    --with-fastTCLInterp=no

make -j"${STRATA_NCPUS}"
make install

# ── Bundle the Lua interpreter and posix library ──────────────────────────────
# Copy system lua into the layer so the lmod scripts work without a host lua.
# The lua binary is small (~200KB) and has no dynamic-library deps beyond libc.
# lua-posix (.so) is also bundled because lmod requires it at runtime (require 'posix').
mkdir -p "${STRATA_INSTALL_PREFIX}/bin"
cp "$(which lua)" "${STRATA_INSTALL_PREFIX}/bin/lua"

# Bundle lua-posix so lmod works on target instances without lua-posix installed.
# AL2023 lua-posix installs a directory of .so sub-modules (/usr/lib64/lua/5.4/posix/)
# plus Lua wrappers (/usr/share/lua/5.4/posix/). Both must be bundled.
LUA_VER=$(lua -e 'print(string.match(_VERSION, "%d+%.%d+"))')
mkdir -p "${STRATA_INSTALL_PREFIX}/lib/lua/${LUA_VER}" \
         "${STRATA_INSTALL_PREFIX}/share/lua/${LUA_VER}"
# Copy C extension directory
[[ -d "/usr/lib64/lua/${LUA_VER}/posix" ]] && \
    cp -r "/usr/lib64/lua/${LUA_VER}/posix" \
         "${STRATA_INSTALL_PREFIX}/lib/lua/${LUA_VER}/"
[[ -d "/usr/lib/lua/${LUA_VER}/posix" ]] && \
    cp -r "/usr/lib/lua/${LUA_VER}/posix" \
         "${STRATA_INSTALL_PREFIX}/lib/lua/${LUA_VER}/"
# Copy Lua wrapper directory
[[ -d "/usr/share/lua/${LUA_VER}/posix" ]] && \
    cp -r "/usr/share/lua/${LUA_VER}/posix" \
         "${STRATA_INSTALL_PREFIX}/share/lua/${LUA_VER}/"

# ── Patch build-time paths to runtime paths ───────────────────────────────────
# Lmod bakes --prefix into its init scripts. At runtime the layer is mounted at
# /strata/env/lmod/VERSION via OverlayFS; replace every occurrence.
RUNTIME_PREFIX="/strata/env/lmod/${LMOD_VERSION}"
grep -rl "${STRATA_INSTALL_PREFIX}" "${STRATA_INSTALL_PREFIX}" 2>/dev/null | \
    while IFS= read -r f; do
        [[ -f "$f" ]] && sed -i "s|${STRATA_INSTALL_PREFIX}|${RUNTIME_PREFIX}|g" "$f"
    done

# Patch the lua shebang in lmod's scripts from the system path to our bundled lua.
# Without this the scripts contain #!/usr/bin/lua which won't exist at runtime.
grep -rl '#!/usr/bin/lua' "${STRATA_INSTALL_PREFIX}" 2>/dev/null | \
    while IFS= read -r f; do
        [[ -f "$f" ]] && sed -i "s|#!/usr/bin/lua|#!${RUNTIME_PREFIX}/bin/lua|g" "$f"
    done

# Patch lmod's init/bash to export LUA_CPATH and LUA_PATH pointing to bundled posix.
# LUA_CPATH: for C extension sub-modules (posix/ctype.so etc)
# LUA_PATH:  for Lua wrapper files (posix/init.lua, posix/compat.lua etc)
LMOD_INIT_BASH="${STRATA_INSTALL_PREFIX}/lmod/lmod/init/bash"
if [[ -f "${LMOD_INIT_BASH}" ]]; then
    sed -i "1a export LUA_CPATH=\"${RUNTIME_PREFIX}/lib/lua/${LUA_VER}/?.so;;\"" \
        "${LMOD_INIT_BASH}"
    sed -i "1a export LUA_PATH=\"${RUNTIME_PREFIX}/share/lua/${LUA_VER}/?.lua;${RUNTIME_PREFIX}/share/lua/${LUA_VER}/?/init.lua;;\"" \
        "${LMOD_INIT_BASH}"
fi

# Patch the lmod Lua main script to prepend bundled posix to package.path/cpath.
# lmod hardcodes sys_lua_path/sys_lua_cpath at configure time and then explicitly
# assigns them to package.path/cpath, ignoring LUA_PATH/LUA_CPATH env vars and any
# earlier prepends. We must patch the assignment lines themselves.
LMOD_SCRIPT="${STRATA_INSTALL_PREFIX}/lmod/lmod/libexec/lmod"
if [[ -f "${LMOD_SCRIPT}" ]]; then
    sed -i "s|package.path   = sys_lua_path|package.path   = \"${RUNTIME_PREFIX}/share/lua/${LUA_VER}/?.lua;${RUNTIME_PREFIX}/share/lua/${LUA_VER}/?/init.lua;\" .. sys_lua_path|" \
        "${LMOD_SCRIPT}"
    sed -i "s|package.cpath  = sys_lua_cpath|package.cpath  = \"${RUNTIME_PREFIX}/lib/lua/${LUA_VER}/?.so;\" .. sys_lua_cpath|" \
        "${LMOD_SCRIPT}"
fi

# ── Relative init.sh symlink ──────────────────────────────────────────────────
ln -sf "lmod/lmod/init/bash" \
    "${STRATA_INSTALL_PREFIX}/init.sh" 2>/dev/null || true

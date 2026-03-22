local base = "/opt/apps/python/3.11.11"
local version = "3.11.11"

prepend_path("PATH", pathJoin(base, "bin"))
prepend_path("LD_LIBRARY_PATH", pathJoin(base, "lib"))

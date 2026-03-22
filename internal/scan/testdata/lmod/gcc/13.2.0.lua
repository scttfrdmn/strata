local base = "/opt/apps/gcc/13.2.0"

prereq("binutils")

prepend_path("PATH", pathJoin(base, "bin"))
prepend_path("LD_LIBRARY_PATH", pathJoin(base, "lib64"))

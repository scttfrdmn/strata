#%Module1.0

set root /opt/apps/openmpi/5.0.10

prereq gcc

prepend-path PATH $root/bin
prepend-path LD_LIBRARY_PATH $root/lib

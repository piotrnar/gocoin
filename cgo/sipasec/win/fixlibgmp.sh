# For windows
# 1) build libgmp.a using mingw64, or download from here: 
#    http://sourceforge.net/projects/mingw-w64/files/External%20binary%20packages%20(Win64%20hosted)/Binaries%20(64-bit)/
# 2) Place libgmp.a and libsecp256k1.a (see README.md) in this folder
# 3) Execute this script to remove symbols that cgo does not like
# 4) go build
ar x libgmp.a
rm libgmp.a
strip --strip-unneeded *.o
ar q libgmp.a *.o
rm *.o

# For windows
# 1) build libcrypto.a without zlib using mingw64
# 2) use this script to remove symbols that cause conflicts with cgo 
ar x libcrypto.a
rm libcrypto.a
strip --strip-unneeded *.o
ar q libcrypto.a *.o
rm *.o

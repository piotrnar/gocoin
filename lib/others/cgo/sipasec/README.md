Building
==============


Unix
--------------

1. Execute "sudo apt-get install libgmp3-dev"

2. Download secp256k1:
 * https://github.com/bitcoin/secp256k1

3. Follow "Build steps" section from README.md, iuncluding the install part



Windows
--------------

Use mingw(64) and msys.

1. Download GMP and build libgmp.a, eventually use a pre-compiled binaries:
 * http://sourceforge.net/projects/mingw-w64/files/External%20binary%20packages%20(Win64%20hosted)/Binaries%20(64-bit)/

2. Download forked secp256k1:
 * https://github.com/piotrnar/secp256k1

3. Create folder "secp256k1/gmp" and place there "libgmp.a" and "gmp.h"

4. Execute "bash winconfig.sh" in "secp256k1/"

5. Execute "make -f Makefile.w64" in "secp256k1/" (for 64 bit windows/mingw)

Note that instead of this cgo, you may prefer to use "secp256k1.dll" (that should have been built)
together with "client/speedup/sipadll.go". In such case take the dll and do not proceed further.

6. Edit "sipasec.go" and fix the paths to "libsecp256k1.a", "libgmp.a" and "include/secp256k1.h"

7. Both "go build" and "go test" shoudl be working from now on.

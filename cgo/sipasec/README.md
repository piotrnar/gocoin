Building
==============


Unix
--------------

1. Execute "sudo apt-get install libgmp3-dev"

2. Download secp256k1:
 * https://github.com/sipa/secp256k1

3. Execute "./configure" inside "secp256k1/"

4. Execute "make" and copy the "libsecp256k1.a" to "/lib/" (you will need a root access)

5. Copy "include/secp256k1.h" to the current (sipasec) folder.

6. Both "go build" and "go test" shoudl be working from now on.



Windows
--------------

Use mingw(64) and msys.

1. Download GMP and build libgmp.a, eventually use a pre-compiled binaries:
 * http://sourceforge.net/projects/mingw-w64/files/External%20binary%20packages%20(Win64%20hosted)/Binaries%20(64-bit)/

2. Place libgmp.a in the "win/" folder and execute "bash fixlibgmp.sh"

3. Download secp256k1:
 * https://github.com/sipa/secp256k1

4. Replace Makefile inside secp256k1 with "win/Makefile"

5. Create folder "secp256k1/gmp" and copy there fixed libgmp.a and gmp.h

6. Copy "win/winconfig.sh" to "secp256k1/" and execute "bash winconfig.sh"

7. Execute "make" in "secp256k1/"

8. Copy "libsecp256k1.a", "include/secp256k1.h" and fixed "libgmp.a" to the current (sipasec) folder.

9. Both "go build" and "go test" shoudl be working from now on.

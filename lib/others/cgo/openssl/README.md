Building
==============


Unix
--------------

The wrapper should build smoothly, as long as you have libssl-dev installed.

Just run "go build" and if it gives no errors, you can also try "go test".



Windows
--------------

Use mingw(64) and msys.

You will need libcrypto.a build for your architecture and the header files.

Before doing "go build" edit openssl.go and fix the paths to "libcrypto.a" and openssl include dir.

If you want to build the lib yourself, leek here:
 * http://stackoverflow.com/questions/9379363/how-to-build-openssl-with-mingw-in-windows#9379476

 > perl Configure mingw64 no-shared no-asm --prefix=/C/OpenSSL-x64
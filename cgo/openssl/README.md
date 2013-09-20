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

Having the libcrypto.a, you will need to "fix" it by executing bash script “win_fix_libcrypto.sh”.

Before doing "go build" edit openssl.go and fix the paths to "libcrypto.a" and openssl include dir.

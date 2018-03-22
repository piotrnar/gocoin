// +build windows

package sipasec

/*
See these pages for some info on how to install MSYS2 and mingw64:
 * http://www.msys2.org/
 * https://stackoverflow.com/questions/30069830/how-to-install-mingw-w64-and-msys2#30071634

After having MSYS2 and Mingw64 installed, you will have to manually install (pacman -S <pkgname>)
the following packages:
 * make
 * autoconf
 * libtool
 * automake
 * ... probably something else, which I don't remember ATM

At this point you should be able to use "MSYS2 MinGW 64-bit" shell to make and install ("make
install"):
 * sipa's secp256k1 library - https://github.com/bitcoin/bitcoin/tree/master/src/secp256k1
 * libgmp - https://gmplib.org/

Having the two packages proparly installed withing your MSYS MinGW 64-bit envirionment, you shoule
be able to execute "go test" in this folder without any problems.

*/

// #cgo LDFLAGS: -lsecp256k1 -lgmp
import "C"

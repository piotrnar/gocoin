//go:build windows
// +build windows

package sipasec

/*
1. MSYS2 + MinGW64
See the following web pages for info on installing MSYS2 and mingw64 for your Windows OS.
Please note that you will need the 64-bit compiler.
 * http://www.msys2.org/
 * https://stackoverflow.com/questions/30069830/how-to-install-mingw-w64-and-msys2#30071634


2. Dependencies
After having MSYS2 and Mingw64 installed, you have to install dependency packages.
Just execute the following command from within the "MSYS2 MSYS" shell:

 > pacman -S make autoconf automake libtool lzip gmp


3. secp256k1
Now use "MSYS2 UCRT64" shell and execute:

 > cd ~
 > git clone https://github.com/bitcoin/bitcoin.git
 > cd bitcoin/src/secp256k1/
 > ./autogen.sh
 > ./configure
 > make --enable-shared  # <- this will also build the DLL that can be used with sipadll
 > make install



If everything went well, you should see "PASS" executing "go test" in this folder.
Then copy "gocoin/client/speedups/sipasec.go" to "gocoin/client/" to boost your client.
*/

// #cgo LDFLAGS: -lsecp256k1 -lgmp
import "C"

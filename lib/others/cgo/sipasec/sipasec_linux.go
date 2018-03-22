// +build linux

package sipasec

/*
To build the library, on Debian based Linux system, execute the following steps:
 * sudo apt-get install gcc autoconf libtool make
 * git clone https://github.com/bitcoin/bitcoin.git
 * cd bitcoin/src/secp256k1/
 * ./autogen.sh
 * ./configure
 * make
 * sudo make install

When the library is properly installed, executing "go test" in this folder says PASS.
*/

// #cgo LDFLAGS: /usr/local/lib/libsecp256k1.a
import "C"

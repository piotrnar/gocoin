// +build linux

package sipasec

/*
To build and install secp256k1 lib on Debian Linux system, execute the following steps:

 * sudo apt-get install gcc autoconf libtool make
 * git clone https://github.com/bitcoin/bitcoin.git
 * cd bitcoin/src/secp256k1/
 * ./autogen.sh
 * ./configure
 * make
 * sudo make install

When the lib is properly installed, executing "go test" in this folder will say "PASS".
Then copy "gocoin/client/speedups/sipasec.go" to "gocoin/client/" to boost your client.
*/

// #cgo LDFLAGS: /usr/local/lib/libsecp256k1.a
import "C"

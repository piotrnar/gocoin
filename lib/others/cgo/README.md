EC_Verify
==============
The "sipasec" folder contains a cgo wrapper to boost our native lib/secp256k1.
On a regular PC it executes EC_Verify operations about 2 to 3 times faster.

Windows
--------------
A preferred solution to boost EC verify operations under Windows is to not use
cgo wrapper, but a DLL solution instead (see “client/speedup/sipadll.go”).
This will require "libsecp256k1-5.dll" to be placed within your PATH.
To build the DLL, follow the instructions in "sipasec/sipasec_windows.go"

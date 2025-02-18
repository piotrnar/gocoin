EC_Verify
==============
The "sipasec" folder contains a cgo wrapper to boost our native lib/secp256k1.
On a regular PC it executes EC_Verify operations about 2 to 3 times faster.

Windows
--------------
A preferred solution to boost EC verify operations under Windows is to not use
cgo wrapper, but a DLL solution instead (see “client/speedup/sipadll.go”).
However, if you manage to build the cgo solution, it performs just as well.
EC_Verify
==============
The sub-folders contain cgo wrappers for native libs, that significantly speed up EC_Verify operations.
"sipasec" is at least 5 times faster than "openssl", though sipa warns that it's still in an experimental phase and advises to be careful.

Windows
--------------
A preferred solution to boost EC verify operations under Windows is to not use a cgo wrapper, but a DLL solution instead (see “client/speedup/sipadll.go”).

If you still want to use a cgo method under Windows, note that there is a known issue with linking libs:
 * https://code.google.com/p/go/issues/detail?id=5740
Until the fix is released, there are workarounds that you will need to apply (read inside the sub-folders).



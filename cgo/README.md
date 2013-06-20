EC_Verify
==============
The sub-folders contain cgo wrappers for native libs, that significantly speed up EC_Verify operations.
"sipasec" is at least 5 times faster than "openssl", though sipa warns that it's still in an experimantal phase and advises to be careful.

Windows
--------------
Please note that there is a known issue with linking libs on Windows:
 * https://code.google.com/p/go/issues/detail?id=5740

Until the fix is released, there are work arounds that you can use (read inside sub-folders).

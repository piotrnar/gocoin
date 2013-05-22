This is a TCP server service which does ECDSA verifications using OpenSSL.

If you can build the ../openssl package, you don't need this package.
Otherwise use it, since it speeds up the crypto ops by about 10 times.

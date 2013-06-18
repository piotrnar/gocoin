1)
Build the libsecp256k1.a for your platform from these sources:
 * https://github.com/sipa/secp256k1

2)
Copy secp256k1.h and libsecp256k1.a to /usr/local/secp256k1
Eventually change the path to this files in sipasec.go

3)
Make sure the linker will be able to find the gmp lib
You may need to add another "-L /path/to/it" in sipasec.go

Thank you very much, @sipa

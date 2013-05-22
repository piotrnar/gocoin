gcc -DWINDOWS ecdsa_server.c -o ecdsa_server.exe -lws2_32 -lcrypto -lgdi32 -lz -I . -L .

# How to use SSL secured WebUI

In order to have a SSL secured access to your node's WebUI, place here the following files:
* ca.crt
* server.key
* server.crt

If all the three files are in place, SSL server will be started at port 4433, in parallell to the regular HTTP server.

The SSL server will accept connections from any IP address, regardless of the WebUI setting in `gocoin.conf` file.

In order to access it you will need `client.p12` certificate imported into your browser's Personal certificates.

Then use URL like **https://your.hostname.or.ip:4433/**


# How to generate needed files

Use `openssl` command to generate all the required files.

## Generate ca.key and ca.crt
	openssl genrsa -out ca.key 4096
	openssl req -new -x509 -days 365 -key ca.key -out ca.crt

Place `ca.crt` in the current folder.

If you plan to use self-signed SSL certificate, additionally import `ca.crt` into your browser's Trusted Root CA list.

## Generate server.key and server.crt

You can use one of the CA vendors to acquire SSL certificate for your WebUI hostname.
Both the files are expected to be in the PEM format.
`server.crt` may contain a chain of certificates.

Otherwise, the method below guides you through creating a self-signed SSL certificate.
Using self-signed certificate, make sure to have `ca.crt` imported into your browser's Trusted Root CA list, to avoid security alerts.

### Create v3.ext file
Create file named `v3.ext` with the following content:

	authorityKeyIdentifier=keyid,issuer
	basicConstraints=CA:FALSE
	keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment
	subjectAltName = @alt_names

	[alt_names]
	DNS.1 = domain.com

Replace **domain.com** with your node's hostname or IP.

### Generate server.key and server.crt
	openssl genrsa -out server.key 2048
	openssl req -new -key server.key -out server.csr

When asked for **Common Name** give your node's hostname or IP (same value as **DNS.1** in `v3.ext` file)

	openssl x509 -req -days 365 -in server.csr -CA ca.crt -CAkey ca.key -set_serial 01 -out server.crt -sha256 -extfile v3.ext

## Installing server.key and server.crt

Place `server.key` and `server.crt` in the current folder.

## Generate client.p12
	openssl genrsa -out client.key 2048
	openssl req -new -key client.key -out client.csr
	openssl x509 -req -days 365 -in client.csr -CA ca.crt -CAkey ca.key -set_serial 01 -out client.crt
	openssl pkcs12 -export -clcerts -in client.crt -inkey client.key -out client.p12

Import `client.p12` into your browser's Personal certificates.


# Security pracautions

In order to assure the security of the WebUI, make sure to keep the `ca.key` and all the `client.*` files secret.
Whoever gets access to any of these files, will be able to access your node's WebUI.

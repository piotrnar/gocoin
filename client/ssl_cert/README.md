In order to have a TLS secured access to your node's WebUI, place here the following files:
* ca.crt
* server.key
* server.crt

### Generate ca.key and ca.crt
> openssl genrsa -out ca.key 4096
> openssl req -new -x509 -days 365 -key ca.key -out ca.crt

Import `ca.crt` into your browser's Trusted Root CA list and place its copy in the current folder.

### Create v3.ext file
Create file named `v3.ext` with the following content:
	authorityKeyIdentifier=keyid,issuer
	basicConstraints=CA:FALSE
	keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment
	subjectAltName = @alt_names

	[alt_names]
	DNS.1 = domain.com

Replace *domain.com* with your node's hostname or IP.

### Generate server.key and server.crt
	openssl genrsa -out server.key 2048
	openssl req -new -key server.key -out server.csr

When asked for **Common Name** give your node's hostname or IP (same value as **DNS.1** in `v3.ext` file)
	openssl x509 -req -days 365 -in server.csr -CA ca.crt -CAkey ca.key -set_serial 01 -out server.crt -sha256 -extfile v3.ext

When finished, place `server.key` and `server.crt` in the current folder.

### Generate client.p12
	openssl genrsa -out client.key 2048
	openssl req -new -key client.key -out client.csr
	openssl x509 -req -days 365 -in client.csr -CA ca.crt -CAkey ca.key -set_serial 01 -out client.crt
	openssl pkcs12 -export -clcerts -in client.crt -inkey client.key -out client.p12

Import `client.p12` into your browser's Personal certificates.


### Security pracautions

In order to assure the security of the WebUI, make sure to keep the `ca.key` and all the `client.*` files secret.
Whoever gets access to any of these files, will be able to access your node's WebUI.

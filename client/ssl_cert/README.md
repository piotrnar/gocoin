Place here the following files:
* ca.crt
* server.key
* server.crt

### Generate ca.key
> openssl genrsa -out ca.key 4096

### Generate ca.crt
> openssl req -new -x509 -days 365 -key ca.key -out ca.crt

Import `ca.crt` into your broweser's Trusted Root CA list and place its copy in this (`ssl_cert/`) folder.

### Generate server.key
> openssl genrsa -out server.key 2048

Place `server.key` in this (`ssl_cert/`) folder.

### Create v3.ext file

	authorityKeyIdentifier=keyid,issuer
	basicConstraints=CA:FALSE
	keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment
	subjectAltName = @alt_names

	[alt_names]
	DNS.1 = domain.com

Replace domain.com with your node's hostname or IP.

### Generate server.crt
> openssl req -new -key server.key -out server.csr
> openssl x509 -req -days 365 -in server.csr -CA ca.crt -CAkey ca.key -set_serial 02 -out server.crt -sha256 -extfile v3.ext

When asked for *Common Name* give your node's hostname or IP.

When finished, place `server.crt` in this (`ssl_cert/`) folder.

### Generate client.p12
> openssl genrsa -out client.key 2048
> openssl req -new -key client.key -out client.csr
> openssl x509 -req -days 365 -in client.csr -CA ca.crt -CAkey ca.key -set_serial 01 -out client.crt
> openssl pkcs12 -export -clcerts -in client.crt -inkey client.key -out client.p12

Import `client.p12` into your browser's Personal certificates.

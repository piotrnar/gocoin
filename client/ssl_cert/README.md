Place here the following files:
* ca.crt
* server.key
* server.crt


# Generate ca.key
> openssl genrsa -out ca.key 4096

# Generate ca.crt
> openssl req -new -x509 -days 365 -key ca.key -out ca.crt
Import this certificate into your broweser Root Authorities

# Generate server.key
> openssl genrsa -out ca.key 2048

# Generate server.crt
> openssl req -new -key server.key -out server.csr
> openssl x509 -req -days 365 -in server.csr -CA ca.crt -CAkey ca.key -set_serial 02 -out server.crt -sha256 -extfile v3.ext

# Generate client.p12 (to be inported into the client browser)
> openssl genrsa -out client.key 2048
> openssl req -new -key client.key -out client.csr
> openssl x509 -req -days 365 -in client.csr -CA ca.crt -CAkey ca.key -set_serial 01 -out client.crt
> openssl pkcs12 -export -clcerts -in client.crt -inkey client.key -out client.p12
Import client.p12 into your browsers Personal certificates

# Content of v3.ext file

	authorityKeyIdentifier=keyid,issuer
	basicConstraints=CA:FALSE
	keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment
	subjectAltName = @alt_names

	[alt_names]
	DNS.1 = domain.com

Make sure to replace the value of DNS.1 to whatever you need

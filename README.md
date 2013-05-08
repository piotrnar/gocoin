Gocoin
==============
Gocoin is a bictoin software written in Go langage (golang).
At the current version it only works with entire blocks and
does not rely transactions, except from announcing own transactions
to the netowrk.

The entire tree consists of:
* btc - bitcoin client library
* client - bitcoin node
* wallet - a deterministic wallet


Dependencies
==============
Alow go to fetch the dependencies for you:

	go get github.com/piotrnar/qdb
	go get code.google.com/p/go.crypto/ripemd160


Requirements
==============

Client
--------------
Becasue of the required memory space, the client side (network node)
is only supposed to be used with a 64-bit architecture (OS), though
for testnet only purposes, 32-bit arch should also be enough.

When the full bitcoin blockchain is loaded, the node needs about 2GB
of system memory, so make sure that there is enough of it.

The blockchain is stored in one large file, so make sure that your file
system supports files larger than 4GB.


Wallet
--------------
The wallet app has very little requirements and it should work with
literelly any platform where you have a working Go compiler.

Also, though it has not been tested, there is no reason why it would
not work on ARM (i.e. Raspberry Pi)

Please keep in mind that the wallet uses unencrypted files to store
the private keys, so run it always from an encrypted disk.
Also use an encrypted swap file.


Building
==============
For anyone familiar with Go language, building shoudl not be a problem.
You can fetch the souce code using i.e.:
	go get github.com/piotrnar/gocoin/btc

After you have the sorces in your local disk, building them is usually
as simple as executing "go build" in either the client, or the wallet,
directory.


Running
==============
Both the applications (client and wallet) are command line only.
The client, after run has an additional interactive text interface.


Client
--------------
Command line switches for executing the client:
	-t - use testnet (insted of regular bitcoin network)
	-l - listen for incomming TCP connections (no UPnP support)
	-ul=NN - set maximu upload speed to NN kilobytes per second
	-dl=NN - set maximu download speed to NN kilobytes per second
	-r - rescan the blockchain and re-create unspent database
	-c="1.2.3.4" - connect only to this host

When the client is running you have an interactive interface where
you can issue commands. Type "help" to see the while list.



Wallet
--------------
Wallet is deterministic and the only thing you need to setup it with
is the seed password. You do not need to backup it, as long as you
remember the password.

Store your master seed password in file named wallet.sec
Make sure that there are no accidential end-of-line characters,
because they count while calculating the seed.

You can also import keys from your existing bitcoin wallet.
To do this use dumprivkey RPC and store the base58 encoded value in
a file named others.sec - each key in a separate line.
You can also place a key's label in each line, after the space.


Spending money
--------------

After you setup your wallet with a proper content of wallet.sec file,
you should export your public addresses. In order to to this, just run
	wallet -l

The wallet's public keys will be written to wallet.txt - you can take
this file safely to your client's PC and place it in the forder where it
looks for wallet.txt (the path is printed while are starting the client).

After you place your wallet.txt in the right folder, you can check your
balance using the clients interactive command:
	bal

Each time you execute it, a directory "balance/" is created in the folder
where you run your client from.

To spend your money, move the most recent "balance/" folder to the PC
with your wallet. If you execute wallet app without any paremeters, it
should show you how many bitcoins you are able to spend.
To spend them, order the wallet to make and sign a transaction using:
	wallet -send 1JbdKe4eBwtexisGTbCKY5v5CfphtdZXJs=1.0

There are also additional switches you can use. To see them all ty:
	wallet -h

If evrything goes well with the "-send ...", the wallet should create
a text file with a signed transaction (i.e. 01234567.txt).

Now move this transaction file to your running client and use it's
interactive interface to execute the following command:
	tx 01234567.txt

The node should decode the transaction and show it to you for verification.
It will also output some <transactionid>.

After making sure that this is what you wanted to send, you can broadcast
the transaction to the network:
	stx <transactionid>

Please note that the current version of client does not re-broadcast
transactions, so if your transaction does not appear in the chain soon
enough, you may need to repeat the "stx ..." command.
There may be of course other reasons, like i.e. your fee was not big enough,
in which case repeating "stx ..." will not help much.

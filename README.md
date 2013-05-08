Gocoin
==============
Gocoin is a bictoin software written in Go langage (golang).

The entire tree consists of:
* btc - bitcoin client library
* client - bitcoin node
* wallet - a deterministic wallet


Dependencies
==============
* go get github.com/piotrnar/qdb
* go get code.google.com/p/go.crypto/ripemd160


Requirements
==============

Client
--------------
Becasue of the required memory space, the client side (network node)
is only supposed to be used with a 64-bit architecture (OS), though
if you want to try it with testnet only, 32-bit arch should be enough.

When the full bitcoin blockchain is loaded, the node needs about 2GB
of system memory, so make sure you have enough.

The blocks are stored in one large file, so make sure that your file
system supports files larger than 4GB.


Wallet
--------------
The wallet app has very little requirements and it should work with
literelly any platform where you have a working Go compiler.

Aslo, though it has not been tested, there is no reason why it would
not work on ARM (i.e. raspberry pi)


Building
==============
For anyone familiar with Go language, building shoudl not be a problem.
You can fetch the souce code using i.e.:
* go get github.com/piotrnar/gocoin/btc

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
* -t - use testnet (insted of regular bitcoin network)
* -l - listen for incomming TCP connections (no UPnP support)
* -ul=NN - set maximu upload speed to NN kilobytes per second
* -dl=NN - set maximu download speed to NN kilobytes per second
* -r - rescan the blockchain and re-create unspent database
* -c="1.2.3.4" - connect only to this host

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
* wallet -l
The wallet's public keys will be written to wallet.txt - you can take
this file safely to your client's PC and place it in the forder where it
looks for wallet.txt (the path is printed while are starting the client).

After you place your wallet.txt in the right folder, you can check your
balance using the clients interactive command "bal".
Each time you execute "bal" a directory "balance/" is created in your
client's folder.
To spend your money move the most recent "balance/" folder to the PC
with your wallet. If you execute wallet app without any paremeters, it
should show you how many bitcoins you are able to spend currently.
To spend them, first you need to make a transaction using:
* wallet -fee 0.0001 -send addres_to_send=amount
If wverything goes well, the wallet should create a file with a signed
transaction. Now move this file to your client and use the interacive
interface to issue:
* tx <trans.txt>
Where <trans.txt< is path to the file created by your wallet.
The node should decode the transaction and show it to you for verification.
It will also output the transaction id. After making sure that this is
waht you wanted to send, you broadcast the transaction to the network:
* stx <transactionid>
Where <transactionid> is the value that was printed previously when you
executed "tx .."

Please note that the current version of client does not re-broadcast,
so if your transaction deos not apprear in the chain soon enough, you
may need to repeat the stx command.

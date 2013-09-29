Gocoin
==============
Gocoin is a full bitcoin client solution (node + wallet) written in Go language (golang).


Architecture
--------------
The two basic components of the software are:

* **client** - a bitcoin node that must be connected to Internet
* **wallet** - a wallet app, that is designed to be used (for security) on a network-free PC

There are also some additional tools, like:

* **versigmsg** - verify a messages signed with bitcoin address.
* **importblocks** - import block database from the satoshi client
* **type2determ** - generate public adresses for type-2 deterministic wallets

Main features
--------------
* Deterministic, cold wallet that is based on a seed-password (does not require any backups).
* Fast and easy switching between different wallets.
* Support for a bandwidth limit used by the node, configured separately for upload and download.
* Block database is compressed on your disk (it takes almost 30% less space).
* Optimized network latency and bandwidth usage.


Limitations of the client node
--------------
* No mining
* No UPnP
* No IPv6
* No GUI
* No multisig


System Requirements
==============
The online client node has much higher system requirements than the wallet app.


Client / Node
--------------
As for the current bitcoin block chain (around block #250000), it is recommended to have at least 4GB of RAM. Because of the required memory space, the node will likely crash on a 32-bit system, so build it using 64-bit Go compiler. For testnet only purposes 32-bit arch is enough.

The entire block chain is stored in one large file, so your file system must support files larger than 4GB.


Wallet
--------------
The wallet application has very little requirements and it should work with literally any platform where you have a working Go compiler. That includes ARM (e.g. for Raspberry Pi).

The wallet uses unencrypted files to store the private keys, so **make sure that these files are stored on an encrypted disk**, with a solid (unbreakable) password. Also **use an encrypted swap file** since there is no guarantee that no secret part would end up in the swap file at some point.


Installation
==============

Required tools
--------------
In order to build Gocoin you need the following tools installed in your system:

* Go - http://golang.org/doc/install
* Git - http://git-scm.com/downloads
* Mercurial - http://mercurial.selenic.com/

If they are all properly installed you should be able to execute `go`, `git` and `hg` from your OS's command prompt without a need to specify their full path.

Note: Git and Mercurial are needed only for `go get` command to work properly. If you can manually download and install (in a proper `$GOPATH/src/..` folder) the required dependency packages (`ripemd160` and `snappy`), then you do not need Mercurial. If you manually download Gocoin sources from GitHub (e.g. using a web browser) and extract them to a proper folder (`$GOPATH/src/github.com/piotrnar/gocoin`), you do not need Git.


Dependencies
--------------
Two additional packages are needed, both of which are provided by Google, though are not included in the default set of Go libraries, therefore you need to install them explicitly before building Gocoin.

The easiest way to do it is using your Go toolset (that will use Mercurial) to download the libraries for you. Just execute the following commands:

	go get code.google.com/p/go.crypto/ripemd160
	go get code.google.com/p/snappy-go/snappy



Building Gocoin
--------------
For anyone familiar with Go language, building should not be a problem. If you have Git installed, the simplest way to fetch the source code is by executing:

	go get github.com/piotrnar/gocoin

After you have the sources in your local disk, building them is usually as simple as going to either the `client` or the `wallet` directory and executing `go build`. This will create `client` and `wallet` executable binaries, in their respective folders.


To build any of the additional tools, go to the `tools` folder and execute `go build toolname.go` to build a tool of your choice.

ECDSA Verify speedups
--------------
The performance of EC operations provided by the standard `crypto/ecdsa` package is poor and therefore a new package (`btc/newec`) has been developed to address this issue. This package is used by Gocoin as a default option, although you have a choice to use a different solutions.

In folder `tools/ec_bench` you can find benchmarks that compare a speed of each of the available speedups.

To use the native go implementation, just remove the file `speedup.go` from the `./client` folder.

In order to use a different speedup module, after removing `speedup.go`, copy any of the .go files (but never more than one) from `client/speedup/` to the `client/` folder and redo `go build` there.  For cgo wrappers, it does not always go smoothly, because building them requires a working C toolset and additional external libraries. If it does not build out of the box, see `README.md` in the wrapper's folder (`./cgo/wrapper/`) for some help.

### sipasec (cgo)
The "sipasec" option is about four times faster than the default speedup, and over a hundred times faster than using no speedup at all. To build this wrapper, follow the instructions in `cgo/sipasec/README.md`. It is the advised speedup for non-Windows systems.

### sipadll (Windows only)
This is the advised speedup for Windows. It should have a similar performance as the cgo option and needs `secp256k1.dll` in order to work (follow the instructions from `cgo/sipasec/README.md` to build it).

If you struggle with building the DLL yourself, you can use pre-compiled binary from `tools/sipa_dll/secp256k1.dll` that should work with any 64-bit Windows OS.

Make sure the DLL can be found (executed) by the system, from where you run your client. The most convenient solution is to copy the DLL to one of the folders from your PATH.

### openssl (cgo)
OpenSSL seems to be performing a bit worse than the built-in native Go speedup, but it is based on the library that is a well recognized and widely approved standard, so you might prefer to use it for security reasons.


Initial chain download
==============
Gocoin's client node has not been really optimized for the initial chain download, but even if it was, this is still a very time consuming operation. Waiting for the entire chain to be fetched from the network and verified can be enough of a pain to discourage you from seeing how good gocoin is while working with an already synchronized chain.


Import block from Satoshi client
--------------
When you run the client node for the first time, it will look of the satoshi's blocks database in its default location (e.g. `~/.bitcoin/blocks` or `%appdata%\Bitcoin\blocks`) and if found, it will ask you whether you want to import these blocks - you should say 'yes'. Choosing to verify the scripts is not necessary, since it is very time consuming and all the blocks in the input database should only contain verified scripts anyway.

There is also a separate tool called `importblocks` that you can use for importing blocks from the satoshi's database into gocoin. Start the tool without parameters and it will tell you how to use it. While importing the blockchain using this tool, make sure that your node is not running.

In both cases, the operation might take like an hour, or even more, but it is one time only.


Fetching blockchain from local host
--------------
When starting the client, use command line switch "-c=<addr>" to instruct your node to connect to a host of your choice. For the time of the the chain download do not limit do download, nor upload speed.


User Manual
==============
Both the applications (client and wallet) are console only (no GUI window). The client node has a quite functional web interface though, which can be used to control it with your web browser, even from a remote PC (if you've allowed a remote access first).


Client / Node
--------------
Run `client -h` to see all available command line switches. Some example ones are:

	-t - use Testnet (instead of regular bitcoin network)
	-ul=NN - set maximum upload speed to NN kilobytes per second
	-dl=NN - set maximum download speed to NN kilobytes per second

### Text UI
When the client is already running you have the UI where you can issue commands. Type `help` to see the possible commands.

### Web UI
There is also a web interface that you can operate with a web browser.

Make sure that wherever you launch the client executable from, there is the "webht" and "webui" folder, along with its content. They are an important part of the WebUI application. You can edit these files to satisfy your preferences.

By default, for security reasons, WebUI is available only from a local host, via http://127.0.0.1:8833/

If you want to have access to it from different computers:

 * Create `gocoin.conf` file, using either TextUI command `configsave` or the "Show Configuaration" -> "Apply & Save" buttons, at the Home tab of the WebUI.
 * Change `Interface` value in the config file to `:8833`
 * Change `AllowedIP` value to allow access from all the addresses you need (i.e. `127.0.0.1,192.168.0.0/16`)
 * Remember to save the new config and then restart your node.

### Incoming connections
There is no UPnP support, so if you want to accept incoming connections, make sure to setup your NAT for routing TCP port 8333 (or 18333 for testnet) to the PC running the node.

It is possible to setup a different incomming port (`TCPPort` config value), but the satoshi clients refuse connecting to non-default ports, so if you get any incoming connections in such case, they will only be from alternative clients, which there are barely few out there.


Wallet
--------------
Run `wallet -h` to see all available command line switches.


### Setup your seed
Wallet is deterministic and the only thing you need to setup it is the seed password. As long as you remember the password, you do not need to backup the wallet ever.

You can either enter this password each  time  when  running  the  wallet  (not advised since characters are shown on the screen) - or you can store  it,  once and for all, in a file called `.secret`.


### Export public addresses
After you setup your wallet in a secured environment, you should export its public addresses. In order to to this, just run `wallet -l` and your wallet's public addresses will be written to `wallet.txt`. Now you can take this text file safely to your client's PC and place it in the folder where it looks for wallet files (i.e. `~/.bitcoin/gocoin/btcnet/wallet/`).


#### Multiple wallets
It is advised to rename the `wallet.txt` to `DEFAULT` - this will be the wallet that is always loaded at startup.
Additionally in the gocoin's `wallet` folder you can have other text files (representing wallets), that contain different sets of bitcoin addresses.
This will allow you to quickly switch between the wallets, using WebUI's *Wallets* tab. The tab allows you to also edit your wallets, as well as to create a new ones.

### Security precautions
Make sure that the disk with `.secret` file is encrypted.

Keep in mind that `.secret` should not contain any special characters, nor new line characters, because they count while calculating the seed. You do not see such characters on screen, so you may have problems re-creating the same file after you would loose it.

### Importing other private keys
You can import keys from your existing bitcoin wallet, as well as keys generated by other tools (all kind of key/address generators).

The key that you want to import must be in base58 encoded format, which looks somehow like `5KJvsngHeMpm884wtkJNzQGaCErckhHJBGFsvd3VyK5qMZXj3hS`.
To export a private key from the official bitcoin wallet use `dumprivkey` RPC command. To import such a key into your Gocoin wallet, just store the base58 encoded value in a text file named `.others` (each key must be in a separate line). You can also place a key's label in each line, after a space.

The imported keys will extend the key pool of the deterministic ones (that come from your password-seed). After Importing each new key, you should redo `wallet -l` to get an updated `wallet.txt` for your client.


Spending money
--------------
Spending money is a secured process that requires several steps and moving files between your online client PC and your  offline wallet machine. USB flash disk is probably the most convenient medium to use for this purpose.

Never move any files others than the ones that you actually need to move, which are:

 * The `balance/` folder containing your unspent outputs - you move it from client to wallet
 * Text files with signed transactions - these you move from wallet to client

Assuming that you would not sign a wrong transaction, nothing in the files you move between the two points is security sensitive, so  you do not  need to worry about protecting the medium (USB disk).

### Checking your balance
From the moment when `wallet.txt` is properly loaded into your client, you can check your balance, either by executing `bal sum` from textUI, or via the WebUI (using a web browser).

### Exporting balance via TextUI

Each time you execute `bal` command (from TextUI), a directory `balance/` is (re)created, in the folder where you run your client from. The `balance` folder is the one that you shall move to the wallet's PC in order to spend your coins.


### Download balance via WebUI
Alternatively, you can use the WebUI to download `balance.zip` file, which contains the `balance` folder. The zip file contains the same folder as the TextUI command would have given you, so just extract it at the wallet's PC before spending your coins.


### Sign your transaction
As mentioned before, to spend your money you need to move the most recent `balance/` folder, from the client node to the PC with your wallet. Now, on the wallet PC, when you execute `wallet` without any parameters, it should show you how much BTC you are able to spend, from the current balance.

To spend your coins, order the wallet to make and sign a new transaction, using a command like:

	wallet -send 1JbdKe4eBwtexisGTbCKY5v5CfphtdZXJs=0.01

Please note no spaces between the address, the equal sign and the value. The value represents the amount in BTC, which you request to send to the given address.

There are also other switches which you may find useful at this stage (i.e. a fee control). To see all your options, just run `wallet -h`.

### Coin control
You can choose which coins you want to spend, by editing the file `balance/unspent.txt`, before ordering the wallet to make a transaction.

Each line in that file represents an unspent output. Whichever output you remove from "unspent.txt" will not be spent. Whatever will be spent, will be taken in the order that appears in this file. Any change will go back to the address from your first input, unless you use `-change` parameter.

### MakeTx via WebUI
Starting from version 0.7.6 the web interface gives you a page where you can select the exact inputs to be spent and prepare a text command to be executed at the wallet PC in order to send these inputs to a specified set of addresses. The form allows you to download `payment.zip` file that contains the `balance/` folder and `pay_cmd.txt` with the command that needs to executed on the wallet's PC in order to generate a requested transaction.


### Signed transaction file
If everything goes well with the `wallet -send ...` command, it will create a text file with a signed transaction. The file is named like `01234567.txt` - this is the file you need to move to your online node in order to notify the network about your payment.

### Load transaction using TextUI
Move the signed transaction file to your online PC and use the node's TextUI to execute `tx 01234567.txt` (where `01234567.txt` is the path to your transaction file). The node will decode the transaction and display it's details (inputs, outputs, fee, TXID).

### Load transaction using WebUI
You can also use WebUI and it's "Upload Transaction File" form. After you load the transaction, its details will be shown in your browser, for your verification.

### Verify transaction carefully
Verify that the information shown on the screen (after loading it) matches exactly what you intended to spend.

If what you see would not be what you wanted, do not broadcast the transaction, but immediately restart the node (to unload the transaction) and destroy the transaction file, so you would not broadcast it accidentally in a future.

If you follow this simple rule, you should be able to keep you money safe, even in case if there was a critical bug in the wallet application that would destroy your coins (which we hope there isn't). But also (more likely) it allows you to double check that you did not make any mistake when making your transaction, giving you the last chance to fix it.


### Broadcast your transaction

After making sure that the transaction does indeed what you  intended, you must broadcast it to the network, in order to actually move your coins.

#### TextUI
Wse `stx` command with the transaction ID as its parameter (you had the ID printed, when loading it).

#### WebUI
Click on the envelope icon with a green arrow to broadcast a loaded transaction to the network.
Coming back to *Transactions* tab later, press the button in the "Accepted transactions" row, to see all the transactions you've loaded (those with a red background are yours). You can also unload a previously loaded transaction by clicking on the red X icon.

### Re-broadcasting transactions
The client never broadcasts transactions unrequested, so if your transaction does not appear in the chain soon enough, you may want  to re-broadcast it, using the same method as for the initial broadcasting. There may of course be other reasons why your transaction does not get confirmed (i.e. the fee was to low), in which case re-broadcasting will not help much.

There is also a TextUI command `stxa` that re-broadcasts all the transaction that have been loaded, but not yet confirmed by the network. Note that when a transaction gets mined into a block it gets removed from the list automatically.


Known issues
==============

Go's memory manager
--------------
It is a known issue that the current memory manager used by Go (up to version 1.1.2) never releases a memory back to the OS, after allocating it once. Thus, as long as a node is running you will notice decreases in "Heap Size", but never in "SysMem Used". Until this issue is fixed by Go developers, the only way to free the unused memory back to the system is by restarting the node.


Possible UTXO db inconsistency
--------------
It has not been observed for a long time already, but it might happen that sometimes when you kill a client node (instead of quitting it gracefully), the unspent outputs database gets corrupt. In that case, when you start your node the next time, it will malfunction (i.e. panic or do not process any new blocks).

To fix this issue you need to rebuild the unspent database. In order to do this, start the client with "-r" switch (`client -r`).

The UTXO rebuild operation might take around an hour and therefore it is worth to have a backup of some recent version of this database. Then you don't need to rebuild it all, but only the part that came after your backup. What you need to backup is the entire folder named "unspent3", in gocoin's data folder. After you had recovered a backup, do not use "-r" switch.

During the rescan operation it seems to require more system memory in peaks, so if you have like 4GB, your swap file might be getting a bit hot.


WebUI browser compatibility
--------------
The WebUI is being developed and tested with Chrome. As for other browsers some functions might not work.


Contribution
==============
You are welcome to contribute into the project by providing feedback and reporting issues.

Pull requests will not be merged in, because I want to stay the only Gocoin's copyright holder.


Support
==============
If you have a question here are some ways of contacting me:

 * Send an email to tonikt@assets-otc.com
 * Ask in this forum thread: https://bitcointalk.org/index.php?topic=199306.0
 * Open an issue on GitHub: https://github.com/piotrnar/gocoin/issues
 * Try reaching me on Freenet-IRC, looking for nick "tonikt" (sometimes "tonikt2" or "tonikt3").

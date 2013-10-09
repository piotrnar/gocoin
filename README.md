Gocoin
==============
Gocoin is a full bitcoin client solution (node + wallet) written in Go language (golang).

The official webpage fo the project:

* http://www.assets-otc.com/gocoin


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
Find the manual on Gocoin's web page:

 * http://www.assets-otc.com/gocoin/manual


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

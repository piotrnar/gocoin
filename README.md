**Gocoin** is a full **Bitcoin** solution written in Go language (golang).
The wallet combined with `balio` tool provide also a **Litecoin** solution.

The software's architecture is focused on maximum security and good performance.

The **client** (p2p node) is an application independent from the **wallet**.

The **wallet** is deterministic and password seeded.
As long as you remember the password, you do not need any backups of your wallet.

There is additional tool tool called **downloader**.
It can quickly sync (download) the blockchain state from the p2p network.
Use it for the intial blockchain download, or to sync your **client** after keeping it offline for a longer time.

In addition there is also a set of more and less useful tools.
They are all inside the `tools/` folder.
Each source file in that folder is a separate tool.


# Documentation
The official web page of the project is served at <a href="http://www.assets-otc.com/gocoin">www.assets-otc.com/gocoin</a>.

There you can find extended documentation, including <a href="http://www.assets-otc.com/gocoin/manual">User Manual</a>.


# Requirements

## Hardware

**client** and **downloader**

* At least 4GB of system memory.
* 64-bit architecture OS and Go compiler.
* File system where you store the database must support files larger than 4GB.


**wallet**

* Should work on any platform with a working Go (cross)compiler. For instance: it has been successfully tested on Raspberry Pi model A.
* For security reasons, make sure to use encrypted swap file.
* If you decide to store your password in a file, keep it on encrypted disc.


## Software

### Operating System
Having hardware requirements met, any target OS supported by your Go compiler will do.
Currently that can be at least one of the following:

* Windows
* Linux
* OS X
* Free BSD

### Additional software
Since no binaries are provided, in order to build Gocoin yourself, you will need the following tools installed in your system:

* **Go** (version 1.2 or higher) - http://golang.org/doc/install
* **Git** - http://git-scm.com/downloads
* **Mercurial** - http://mercurial.selenic.com/

If the tools mentioned above are all properly installed, you should be able to execute `go`, `git` and `hg` from your OS's command prompt without a need to specify a full path to the executables.


#### Optionally: gcc

It is recommended to have `gcc` complier installed in your system, to get advantage of performance improvements and memory usage optimizations.

For Windows install `mingw`, or rather `mingw64` since the client node needs 64-bit architecture.

 * http://mingw-w64.sourceforge.net/


# Getting sources

Use `go get` to fecth and install the source code files.
Note that source files get installed within your GOPATH folder.

## Dependencies

Two extra packages are needed, which are not included in the standard set of Go libraries.
You need to install them before building Gocoin.
In order to do this, simply execute the following commands:

	go get code.google.com/p/go.crypto/ripemd160
	go get code.google.com/p/snappy-go/snappy

## Gocoin
Use `go get` to fetch and install Gocoin sources for you:

	go get github.com/piotrnar/gocoin


# Building

## Compile client
Go to the `client/` folder and execute `go build` there.

### Not having gcc

Not having a compatible `gcc` installed in your system, trying to build the software, you will likely see an error like this:

	# github.com/piotrnar/gocoin/lib/qdb
	exec: "gcc": executable file not found in %PATH%

You can still compile the problematic package...

To work around the problem, copy file `lib/qdb/no_gcc/membind.go` one folder up (overwriting the original `lib/qdb/membind.go`).

## Compile wallet
Go to the `wallet/` folder and execute `go build` there.

## Compile downloader
Go to the `downloader/` folder and execute `go build` there.

## Compile all the tools
Go to the `tools/` folder and execute:

	go build btcversig.go
	go build compressdb.go
	go build fetchbal.go
	go build fetchtx.go
	go build importblocks.go
	go build type2determ.go


# Pull request
Although it is an open source project, I am sorry to inform you that I will not merge in any pull requests.
The reason is that I want to stay an explicit author of this software, to keep a full control its licensing.
If you are missing some functionality, just describe me your needs and I will see what I can do for you.
But if you want your specific code in, please fork and develop your own repo.

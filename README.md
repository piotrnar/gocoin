Gocoin is a full bitcoin client solution (node + wallet) written in Go language (golang).

The two basic components of the software are:

* **client** - a bitcoin node that must be connected to Internet
* **wallet** - a wallet app, that is designed to be used offline

In addition to the client there is an app called **downloader** that is able to download
the block chain at a decent speed.
The downloader can be used before you run the client for the first time, but also any time later -
e.g. if you had your node inactive for some time and want to quickly sync it up with the network.


# Webpage
The official webpage of the project:

* http://www.assets-otc.com/gocoin

On that webpage you can find all the information from this file, plus much much more (e.g. *User Manual*).



# Requirements

## Hardware

### Online node
At least 4GB of system memory is required, though 8GB is highly recommended.

You need to build it using 64 bit Go compiler and run it on 64 bit OS.

The file system where you store the database must support files larger than 4GB.

### Offline wallet
The wallet app has very little requirements and should work on any platform with a working Go compiler.

For security reasons, use an encrypted swap file and if you decide to store a password in the `.secret` file,
do it on an encrypted disc.

## Software
Since no binaries are provided, in order to build Gocoin youself, you will need the following tools installed in your system:

* **Go** - http://golang.org/doc/install
* **Git** - http://git-scm.com/downloads
* **Mercurial** - http://mercurial.selenic.com/

If they are all properly installed you should be able to execute `go`, `git` and `hg` from your OS's command prompt without a need to specify their full path.

Note: Git and Mercurial are needed only for the automatic `go get` command to work. You can replace `go get` with some manual steps and then you do  not need these two tools. Read more at Gocoin's webpage.


# Building

## Download sources
Two extra  packages are needed, that are not included in the default set of Go libraries.
You need to download them, before building Gocoin.

	go get code.google.com/p/go.crypto/ripemd160
	go get code.google.com/p/snappy-go/snappy

You can also use `go get` to fetch the gocoin sources from GitHub for you:

	go get github.com/piotrnar/gocoin

Make sure that the all sources are placed in a proper location within your GOPATH folder, before compiling them (`go get` should take care of this).

## Have gcc if you can
Is is recommended to have gcc complier installed in your system, to get advantage of performance improvements.

For Windows install mingw, or rather mingw64 since the client node needs 64-bit architecture.

### Not having gcc

Not having gcc, trying to build the client you will see such an error:

	# github.com/piotrnar/gocoin/qdb
	exec: "gcc": executable file not found in %PATH%

You can still compile the problematic package...

To fix the problem just copy file "qdb/no_gcc/membind.go" one folder up, overwriting the original "qdb/membind.go"

## Compile client
Go to the `client/` folder and execute `go build` there.

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
I am sorry to inform you that I will not merge in any pull requests.
The reason is that I want to stay the only author of this software and therefore the only holder of the copyrights.
If you are missing some functionality that you'd want in, just describe me what you need and I will see what I can do.
If you want your specific code in though, please fork and develop your own repo.

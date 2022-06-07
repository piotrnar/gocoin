** This functionality is outdated (incompatible with Taproot) **

---
Files in this folder allow to setup Gocoin node with so called bitcoin
consensus lib. It is - depending on the OS - either `libbitcoinconsensus.so` or
`libbitcoinconsensus-0.dll`, which get released along with Bitcoin Core client.

To get adventage of this functionality, simply place the consensus library
in the `gocoin/client` folder or within the system's `PATH`, so your OS can
find it (e.g. in `C:\WINDOWS` or `/lib/`).
Also copy all the `*.go` files from this foler to its parent folder and then
re-build your gocoin client.

Please note that you may need `gcc` compiler installed in your system before
you can build the client with the consensus lib support.

Having Gocoin built with the consensus lib support and the consensus lib placed
within your system's PATH, the node will cross-check each new transaction,
making sure that the gocoin's consensus checks return the same results as the
ones of the reeference client's (from Bitcoin Core).

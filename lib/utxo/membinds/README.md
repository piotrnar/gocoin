Files in this folder allow to disable garbage collector over UTXO records,
thus freeing the memory used by such records as soon as possible.

This may sometimes result in a lower memory usage, although it isn't always
the case and it may decrease the performance.

To acheive a similar goal, perhaps you prefer to decrease the value of
`Memory.GCPercTrshold` in `gocoin.conf` file instead.

If you want to use this functionality, simply copy all the `*.go` files from
this foler to its parent folder and then re-build your gocoin client.

Please note that you may need `gcc` compiler installed in your system before
you can build the client.

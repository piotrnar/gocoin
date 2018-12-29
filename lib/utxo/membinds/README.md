Files in this folder allow to disable garbage collector over UTXO records,
thus freeing the memory used by such records as soon as possible.

This used to result in a lower usage of the system memory, although these
days it seems to worsen both; performance and memory usage.

If you still want to use this functionality, simply copy all the `*.go` files
from this foler to its parent folder and then re-build your gocoin client.

Please note that you may need `gcc` compiler installed in your system before
you can build the client.

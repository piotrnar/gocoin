# memory

This package originated from "modernc.org/memory"
New address: https://gitlab.com/cznic/memory

It has been hugely refactored and in its current version is only meant
to be used internally by Gocoin project.

It is used by Gocoin client node to allocate UTXO records in memory.
The idea is to improve performance by disablig GC over UTXO records.

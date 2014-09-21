package sys

import "runtime"

// Increase the number of threads to optimize txs verification time,
// proportionaly among cores, but if you set it too high, the UI and
// network threads may be laggy while parsing blocks.
var UseThreads int = 4 * runtime.NumCPU()

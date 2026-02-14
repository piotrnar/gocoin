package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/piotrnar/gocoin/lib/utxo"
)

func print_help() {
	fmt.Println("Use this tool to compress or decompress UTXO database")
	fmt.Println("Possible arguments are:")
	fmt.Println("You have to specify at least one argument")
	fmt.Println("   * Path to UTXO.db file - to see basic info about it")
	fmt.Println("   * Path to dir (may be .) with UTXO db - (de)compress UTXO records")
	fmt.Println("   * -bench : to benchmark UTXO db")
	fmt.Println("   * -compr or -decompr : to request DB conversion")
	fmt.Println("   * -gc[<perc>] : to use native Go heap [with the given GC target percentage]")
	fmt.Println("   * -n[<num>] : to use concurrent threads [number of threads]")
	fmt.Println("   * -csv : to create the csv file with UTXO stats (for slot_optimizer)")
	fmt.Println("   * -ms<size> : use this max slot size for the csv file")
	os.Exit(1)
}

func main() {
	var dir = ""
	var benchmark bool

	if len(os.Args) < 2 {
		print_help()
	}

	for _, _arg := range os.Args[1:] {
		arg := strings.ToLower(_arg)
		if strings.HasPrefix(arg, "-h") {
			print_help()
		} else if strings.HasPrefix(arg, "-b") {
			if benchmark {
				println("ERROR: benchmark specified more than once")
				print_help()
			}
			benchmark = true
		} else if strings.HasPrefix(arg, "-com") {
			if compress {
				println("ERROR: compress specified more than once")
				print_help()
			}
			compress = true
		} else if strings.HasPrefix(arg, "-dec") {
			if decompress {
				println("ERROR: decompress specified more than once")
				print_help()
			}
			decompress = true
		} else if strings.HasPrefix(arg, "-gc") {
			if gc != -1 {
				println("ERROR: gc specified more than once")
				print_help()
			}
			if n, er := strconv.ParseUint(arg[3:], 10, 32); er == nil && n > 0 {
				gc = int(n)
			} else {
				gc = 30
				println("WARNING: Using default GC value")
			}
		} else if strings.HasPrefix(arg, "-n") {
			if ncpu != -1 {
				println("ERROR: n specified more than once")
				print_help()
			}
			if n, er := strconv.ParseUint(arg[2:], 10, 32); er == nil && n > 0 && n < 1e6 {
				ncpu = int(n)
			} else {
				ncpu = runtime.NumCPU()
				println("WARNING: Using default NumCPU value")
			}
		} else if strings.HasPrefix(arg, "-cs") {
			create_csv = true
		} else if strings.HasPrefix(arg, "-ms") {
			if n, er := strconv.ParseUint(arg[3:], 10, 32); er == nil && n >= 72 {
				max_size = int(n)
			} else {
				println("Incorrect -ms<size> value:", arg)
				os.Exit(1)
			}
		} else {
			if dir != "" {
				println("ERROR: db directory specified more than once")
				print_help()
			}
			dir = _arg
		}
	}

	if dir != "" {
		if fi, er := os.Stat(dir); er == nil && !fi.IsDir() {
			decode_utxo_header(dir)
			return
		}
	}

	if compress && decompress {
		println("ERROR: requested both; compress and decompress")
		print_help()
	}

	if dir != "" && !strings.HasSuffix(dir, string(os.PathSeparator)) {
		dir += string(os.PathSeparator)
	}

	if gc == -1 {
		fmt.Println("Using designated memory allocator")
		utxo.Memory_Malloc = Memory.Malloc
		utxo.Memory_Free = Memory.Free
	} else {
		fmt.Println("Using native Go heap with GC target of", gc)
		debug.SetGCPercent(gc)
	}

	if benchmark {
		utxo_benchmark(dir)
		return
	}

	do_compress(dir, compress, decompress, ncpu)
}

package main

import (
	"os"
	"fmt"
	"time"
	"sort"
	"bufio"
	"strings"
	"strconv"
	"sync/atomic"
)

func readline() string {
	li, _, _ := bufio.NewReader(os.Stdin).ReadLine()
	return string(li)
}


func show_connections() {
	open_connection_mutex.Lock()
	ss := make([]string, len(open_connection_list))
	i := 0
	for _, v := range open_connection_list {
		ss[i] = fmt.Sprintf("%6d  %15s", v.id, v.peerip)
		if !v.isconnected() {
			ss[i] += fmt.Sprint(" - Connecting...")
		} else {
			png := v.avg_ping()
			kbps := v.bps()/1e3
			v.Lock()
			ss[i] += fmt.Sprintf(" %20s %5dms %6.2fKB/s", time.Now().Sub(v.connected_at), png, kbps)
			if !v.last_blk_rcvd.IsZero() {
				ss[i] += fmt.Sprintf("  %20s %3d", time.Now().Sub(v.last_blk_rcvd), len(v.blockinprogress))
			}
			v.Unlock()
		}
		i++
	}
	open_connection_mutex.Unlock()
	sort.Strings(ss)
	for i = range ss {
		fmt.Printf("%5d) %s\n", i+1, ss[i])
	}
}


func save_peers() {
	f, _ := os.Create("ips.txt")
	fmt.Fprintf(f, "%d.%d.%d.%d\n", FirstIp[0], FirstIp[1], FirstIp[2], FirstIp[3])
	ccc := 1
	AddrMutex.Lock()
	for k, v := range AddrDatbase {
		if k!=FirstIp && v {
			fmt.Fprintf(f, "%d.%d.%d.%d\n", k[0], k[1], k[2], k[3])
			ccc++
		}
	}
	AddrMutex.Unlock()
	f.Close()
	println(ccc, "peers saved")
}

var iii uint32

func do_usif() {
	print("cmd> ")
	for {
		cmd := readline()
		go func(cmd string) {
			ll := strings.Split(cmd, " ")
			if len(ll)>0 {
				switch ll[0] {
					case "i":
						println("iii", atomic.LoadUint32(&iii))

					case "g":
						if GetRunPings() {
							SetRunPings(false)
							println("Goto download phase...")
							time.Sleep(3e8)
						} else {
							println("Already in download phase?")
						}

					case "a":
						AddrMutex.Lock()
						println(len(AddrDatbase), "addressess in the database")
						AddrMutex.Unlock()

					case "q":
						os.Exit(0)
						//SetRunPings(false)
						//SetDoBlocks(false)
						//exit = true

					case "n":
						show_connections()

					case "c":
						print_stats()

					case "s":
						save_peers()

					case "p":
						show_inprogress()

					case "d":
						if len(ll)>1 {
							n, e := strconv.ParseUint(ll[1], 10, 32)
							if e==nil {
								open_connection_mutex.Lock()
								for _, v := range open_connection_list {
									if v.id==uint32(n) {
										println("dropping peer id", n, "...")
										v.setbroken(true)
									}
								}
								open_connection_mutex.Unlock()
							}
						} else {
							if GetRunPings() {
								println("dropping longest ping")
								drop_longest_ping()
							} else {
								println("dropping slowest peers")
								drop_slowest_peers()
							}
						}

					case "mc":
						if len(ll)>1 {
							n, e := strconv.ParseUint(ll[1], 10, 32)
							if e == nil {
								atomic.StoreUint32(&MAX_CONNECTIONS, uint32(n))
								println("MAX_CONNECTIONS set to", n)
							}
						}

					default:
						println("unknown command:", ll[0])
				}
			}
			print("cmd> ")
		}(cmd)
	}
	println("do_usif terminated")
}

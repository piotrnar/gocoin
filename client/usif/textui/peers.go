package textui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/network/peersdb"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/lib/others/qdb"
)

func show_node_stats(par string) {
	ns := make(map[string]uint)
	peersdb.Lock()
	peersdb.PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
		pr := peersdb.NewPeer(v)
		if pr.NodeAgent != "" {
			ns[pr.NodeAgent]++
		}
		return 0
	})
	peersdb.Unlock()
	if len(ns) == 0 {
		fmt.Println("Nothing to see here")
		return
	}
	ss := make([]string, len(ns))
	var i int
	for k, v := range ns {
		ss[i] = fmt.Sprintf("%5d %s", v, k)
		i++
	}
	sort.Strings(ss)
	for _, s := range ss {
		fmt.Println(s)
	}
}

func show_addresses(par string) {
	pars := strings.Split(par, " ")
	if len(pars) < 1 || pars[0] == "" {
		bcnt, acnt := 0, 0
		peersdb.PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
			pr := peersdb.NewPeer(v)
			if pr.Banned != 0 {
				bcnt++
			}
			if pr.SeenAlive {
				acnt++
			}
			return 0
		})
		fmt.Println("Peers in DB:", peersdb.PeerDB.Count())
		fmt.Println("Peers seen alive:", acnt)
		fmt.Println("Peers banned:", bcnt)
		fmt.Println("QDB stats:", peersdb.PeerDB.GetStats())
		return
	}

	var only_ban, only_alive, show_help bool
	var ban_reason, only_agent string
	limit := peersdb.PeerDB.Count()
	check_next := true
	switch pars[0] {
	case "ali":
		only_alive = true
	case "ban":
		only_ban = true
	case "help":
		show_help = true
	case "str":
		only_agent = "/Gocoin"
	case "all":
		// do nothing
	case "purge":
		var torem []qdb.KeyType
		peersdb.Lock()
		peersdb.PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
			pr := peersdb.NewPeer(v)
			if !pr.SeenAlive {
				torem = append(torem, k)
			}
			return 0
		})
		fmt.Println("Purginig", len(torem), "records")
		for _, k := range torem {
			peersdb.PeerDB.Del(k)
		}
		peersdb.Unlock()
		show_addresses("")
		return

	case "defrag":
		peersdb.PeerDB.Defrag(true)
		return

	case "save":
		peersdb.PeerDB.Sync()
		return

	default:
		if u, e := strconv.ParseUint(pars[0], 10, 32); e != nil {
			fmt.Println("Incorrect number A of peers max count:", e.Error())
			show_help = true
		} else {
			limit = int(u)
			check_next = false
		}
	}
	if check_next {
		if len(pars) >= 2 {
			if u, e := strconv.ParseUint(pars[1], 10, 32); e != nil {
				if only_ban {
					ban_reason = pars[1]
					if len(pars) >= 3 {
						if u, e := strconv.ParseUint(pars[2], 10, 32); e == nil {
							limit = int(u)
						}
					}
				} else if only_agent != "" {
					only_agent = pars[1]
				} else {
					fmt.Println("Incorrect number B of peers max count:", e.Error())
					show_help = true
				}
			} else {
				limit = int(u)
			}
		}
	}

	if show_help {
		fmt.Println("Use 'peers all [max_cnt]' or 'peers [max_cnt]' to list all")
		fmt.Println("Use 'peers ali [max_cnt]' to list only those seen alive")
		fmt.Println("Use 'peers ban [ban_reason] [max_cnt]' to list the banned ones")
		fmt.Println("Use 'peers str [string]' to list only if agent contains string")
		fmt.Println("Use 'peers agents' to see node agent statistics")
		fmt.Println("Use 'peers purge' to remove all peers never seen alive")
		fmt.Println("Use 'peers defrag' to defragment DB")
		fmt.Println("Use 'peers save' to save DB now")
		return
	}
	prs := peersdb.GetRecentPeers(uint(limit), true, func(p *peersdb.PeerAddr) bool {
		if only_agent != "" && !strings.Contains(p.NodeAgent, only_agent) {
			return true
		}
		if only_alive && !p.SeenAlive {
			return true
		}
		if only_ban {
			if p.Banned == 0 || ban_reason != "" && p.BanReason != ban_reason {
				return true
			}
			p.Time, p.Banned = p.Banned, p.Time // to sort them by when they were banned
		}
		return false
	})
	for i, p := range prs {
		var sc string
		if only_ban {
			p.Time, p.Banned = p.Banned, p.Time // Revert the change
		}
		if network.ConnectionActive(p) {
			sc = "*"
		} else {
			sc = " "
		}

		fmt.Printf("%4d%s%s\n", i+1, sc, p.String())
	}
	fmt.Println("Note: currently connected peers marked with *")
}

func unban_peer(par string) {
	fmt.Print(usif.UnbanPeer(par))
}

func ban_peer(par string) {
	ad, er := peersdb.NewAddrFromString(par, false)
	if er != nil {
		fmt.Println(par, er.Error())
		return
	}
	ad.Ban("FromTUI")
}

func add_peer(par string) {
	ad, er := peersdb.NewAddrFromString(par, false)
	if er != nil {
		fmt.Println(par, er.Error())
		return
	}
	ad.Time = uint32(time.Now().Unix())
	ad.Save()
}

func del_peer(par string) {
	peersdb.Lock()
	defer peersdb.Unlock()

	ad, er := peersdb.NewAddrFromString(par, false)
	if er != nil {
		fmt.Println(par, er.Error())
		return
	}

	fmt.Print("Deleting ", ad.Ip(), " ... ")
	id := ad.UniqID()
	if peersdb.PeerDB.Get(qdb.KeyType(id)) != nil {
		peersdb.PeerDB.Del(qdb.KeyType(id))
		fmt.Println("OK")
	} else {
		fmt.Println("not found")
	}
}

func init() {
	newUi("ban", false, show_node_stats, "Ban a peer specified by IP")
	newUi("nodestat ns", false, show_node_stats, "Shows all known nodes version statistics")
	newUi("peeradd pa", false, add_peer, "Add a peer to the database, mark it as alive")
	newUi("peerdel pd", false, del_peer, "Delete peer from the DB")
	newUi("peers p", false, show_addresses, "Operation on pers database ('peers help' for help)")
	newUi("unban", false, unban_peer, "Unban a peer specified by IP[:port] (or 'unban all')")
}

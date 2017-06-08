package network

import (
	"fmt"
	"time"
	"bytes"
	"errors"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/client/common"
)

var IgnoreExternalIpFrom = []string{"/Snoopy:0.1/", "/libbitcoin:2.0.0/", "/Snoopy:0.2.1/", "/xbadprobe:1.0/"}

func (c *OneConnection) SendVersion() {
	b := bytes.NewBuffer([]byte{})

	binary.Write(b, binary.LittleEndian, uint32(common.Version))
	binary.Write(b, binary.LittleEndian, uint64(common.Services))
	binary.Write(b, binary.LittleEndian, uint64(time.Now().Unix()))

	b.Write(c.PeerAddr.NetAddr.Bytes())
	if ExternalAddrLen()>0 {
		b.Write(BestExternalAddr())
	} else {
		b.Write(bytes.Repeat([]byte{0}, 26))
	}

	b.Write(nonce[:])

	b.WriteByte(byte(len(common.CFG.UserAgent)))
	b.Write([]byte(common.CFG.UserAgent))

	binary.Write(b, binary.LittleEndian, uint32(common.Last.BlockHeight()))
	if !common.CFG.TXPool.Enabled {
		b.WriteByte(0)  // don't notify me about txs
	}

	c.SendRawMsg("version", b.Bytes())
}



func (c *OneConnection) HandleVersion(pl []byte) error {
	if len(pl) >= 80 /*Up to, includiong, the nonce */ {
		c.Mutex.Lock()
		c.Node.Version = binary.LittleEndian.Uint32(pl[0:4])
		if bytes.Equal(pl[72:80], nonce[:]) {
			c.Mutex.Unlock()
			return errors.New("Connecting to ourselves")
		}
		if c.Node.Version < MIN_PROTO_VERSION {
			c.Mutex.Unlock()
			return errors.New("Client version too low")
		}
		c.Node.Services = binary.LittleEndian.Uint64(pl[4:12])
		c.Node.Timestamp = binary.LittleEndian.Uint64(pl[12:20])
		c.Node.ReportedIp4 = binary.BigEndian.Uint32(pl[40:44])

		use_this_ip := sys.ValidIp4(pl[40:44])

		if len(pl) >= 86 {
			le, of := btc.VLen(pl[80:])
			of += 80
			c.Node.Agent = string(pl[of:of+le])
			of += le
			if len(pl) >= of+4 {
				c.Node.Height = binary.LittleEndian.Uint32(pl[of:of+4])
				c.X.GetBlocksDataNow = true
				of += 4
				if len(pl) > of && pl[of]==0 {
					c.Node.DoNotRelayTxs = true
				}
			}
		}
		c.Mutex.Unlock()

		if use_this_ip {
			if bytes.Equal(pl[40:44], c.PeerAddr.Ip4[:]) {
				fmt.Printf("* OWN IP from %s @ %s - %d\n> ", c.Node.Agent, c.PeerAddr.Ip(), c.ConnID)
				common.CountSafe("IgnoreExtIP-O")
				use_this_ip = false
			} else if len(pl) >= 86 && binary.BigEndian.Uint32(pl[66:70]) != 0 &&
				!bytes.Equal(pl[66:70], c.PeerAddr.Ip4[:]) {
				fmt.Printf("* BAD IP=%d.%d.%d.%d from %s @ %s - %d\n> ",
					pl[66], pl[67], pl[68], pl[69], c.Node.Agent, c.PeerAddr.Ip(), c.ConnID)
				common.CountSafe("IgnoreExtIP-B")
				use_this_ip = false
			}
		}

		if use_this_ip {
			ExternalIpMutex.Lock()
			_, use_this_ip = ExternalIp4[c.Node.ReportedIp4]
			if !use_this_ip { // New IP
				use_this_ip = true
				for x, v := range IgnoreExternalIpFrom {
					if c.Node.Agent==v {
						use_this_ip = false
						common.CountSafe(fmt.Sprint("IgnoreExtIP", x))
						break
					}
				}
				if use_this_ip {
					fmt.Printf("NEW EXT IP=%d.%d.%d.%d from %s @ %s - %d\n> ",
						pl[40], pl[41], pl[42], pl[43], c.Node.Agent, c.PeerAddr.Ip(), c.ConnID)
				}
			}
			if use_this_ip {
				ExternalIp4[c.Node.ReportedIp4] = [2]uint {ExternalIp4[c.Node.ReportedIp4][0]+1,
					uint(time.Now().Unix())}
			}
			ExternalIpMutex.Unlock()
		}

	} else {
		return errors.New("version message too short")
	}
	c.SendRawMsg("verack", []byte{})
	return nil
}

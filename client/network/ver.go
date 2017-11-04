package network

import (
	"os"
	"fmt"
	"time"
	"bytes"
	"errors"
	"strings"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/client/common"
)

var IgnoreExternalIpFrom = []string{}

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

	common.LockCfg()
	btc.WriteVlen(b, uint64(len(common.UserAgent)))
	b.Write([]byte(common.UserAgent))
	common.UnlockCfg()

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
			if strings.HasPrefix(c.Node.Agent, "/Gocoin:") {
				c.X.IsSpecial = true
			}
		}
		c.Mutex.Unlock()

		if use_this_ip {
			if bytes.Equal(pl[40:44], c.PeerAddr.Ip4[:]) {
				if common.FLAG.Log {
					ExternalIpMutex.Lock()
					f, _ := os.OpenFile("badip_log.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660);
					if f != nil {
						fmt.Fprintf(f, "%s: OWN IP from %s @ %s - %d\n",
							time.Now().Format("2006-01-02 15:04:05"),
							c.Node.Agent, c.PeerAddr.Ip(), c.ConnID)
						f.Close()
					}
					ExternalIpMutex.Unlock()
				}
				common.CountSafe("IgnoreExtIP-O")
				use_this_ip = false
			} else if len(pl) >= 86 && binary.BigEndian.Uint32(pl[66:70]) != 0 &&
				!bytes.Equal(pl[66:70], c.PeerAddr.Ip4[:]) {
				if common.FLAG.Log {
					ExternalIpMutex.Lock()
					f, _ := os.OpenFile("badip_log.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660);
					if f != nil {
						fmt.Fprintf(f, "%s: BAD IP=%d.%d.%d.%d from %s @ %s - %d\n",
							time.Now().Format("2006-01-02 15:04:05"),
							pl[66], pl[67], pl[68], pl[69], c.Node.Agent, c.PeerAddr.Ip(), c.ConnID)
						f.Close()
					}
					ExternalIpMutex.Unlock()
				}
				common.CountSafe("IgnoreExtIP-B")
				use_this_ip = false
			}
		}

		if use_this_ip {
			ExternalIpMutex.Lock()
			if _, known := ExternalIp4[c.Node.ReportedIp4]; !known { // New IP
				use_this_ip = true
				for x, v := range IgnoreExternalIpFrom {
					if c.Node.Agent==v {
						use_this_ip = false
						common.CountSafe(fmt.Sprint("IgnoreExtIP", x))
						break
					}
				}
				if use_this_ip && common.IsListenTCP() {
					fmt.Printf("New external IP %d.%d.%d.%d from ConnID=%d\n> ",
						pl[40], pl[41], pl[42], pl[43], c.ConnID)
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

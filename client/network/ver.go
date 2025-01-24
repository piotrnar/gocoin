package network

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/lib/secp256k1"
)

var IgnoreExternalIpFrom = []string{}

func (c *OneConnection) SendVersion() {
	b := bytes.NewBuffer([]byte{})

	binary.Write(b, binary.LittleEndian, uint32(common.Version))
	binary.Write(b, binary.LittleEndian, uint64(common.Services))
	binary.Write(b, binary.LittleEndian, uint64(time.Now().Unix()))

	b.Write(c.PeerAddr.NetAddr.Bytes())
	if ExternalAddrLen() > 0 {
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
	if !common.GetBool(&common.CFG.TXPool.Enabled) {
		b.WriteByte(0) // don't notify me about txs
	}

	c.SendRawMsg("version", b.Bytes())
}

func (c *OneConnection) IsGocoin() bool {
	return strings.HasPrefix(c.Node.Agent, "/Gocoin:")
}

func (c *OneConnection) HandleVersion(pl []byte) error {
	if len(pl) < 80 /*Up to, includiong, the nonce */ {
		return errors.New("MsgTooShort")
	}

	c.Mutex.Lock()
	c.Node.Version = binary.LittleEndian.Uint32(pl[0:4])
	c.Node.Services = binary.LittleEndian.Uint64(pl[4:12])
	c.PeerAddr.Services = c.Node.Services
	copy(c.Node.Nonce[:], pl[72:80])
	c.Node.Timestamp = binary.LittleEndian.Uint64(pl[12:20])
	c.Node.ReportedIp4 = binary.BigEndian.Uint32(pl[40:44])

	use_this_ip := sys.ValidIp4(pl[40:44])

	if len(pl) >= 82 {
		le, of := btc.VLen(pl[80:])
		if of == 0 || len(pl) < 80+le {
			c.Mutex.Unlock()
			return errors.New("MsgCorrupt")
		}
		of += 80
		c.Node.Agent = string(pl[of : of+le])
		of += le
		if len(pl) >= of+4 {
			c.Node.Height = binary.LittleEndian.Uint32(pl[of : of+4])
			c.X.GetBlocksDataNow = true
			of += 4
			if len(pl) > of && pl[of] == 0 {
				c.Node.DoNotRelayTxs = true
			}
		}
		c.X.IsGocoin = c.IsGocoin()
	}
	c.PeerAddr.NodeAgent = c.Node.Agent
	c.X.VersionReceived = true
	c.Mutex.Unlock()

	if !c.X.IsSpecial {
		FriendsAccess.Lock()
		for _, ua := range SpecialAgents {
			if strings.HasPrefix(c.Node.Agent, ua) {
				c.X.IsSpecial = true
				goto special
			}
		}
		for _, ip := range SpecialIPs {
			if c.PeerAddr.Ip4 == ip {
				c.X.IsSpecial = true
				goto special
			}
		}
	special:
		FriendsAccess.Unlock()
	}

	if !c.X.IsSpecial {
		if c.Node.Version < MIN_PROTO_VERSION {
			return errors.New("TooLow")
		}
		if (c.Node.Services & btc.SERVICE_SEGWIT) == 0 {
			return errors.New("NoSegwit")
		}
		if !c.HasNetworkService() {
			return errors.New("NoService")
		}
		if c.Node.Nonce == [8]byte{0, 0, 0, 0, 0, 0, 0, 0} {
			return errors.New("NullNonce")
		}
		if c.Node.Nonce == nonce {
			return errors.New("OurNonce")
		}
	}

	// check if we don't have this nonce yet
	Mutex_net.Lock()
	for _, v := range OpenCons {
		if v != c {
			v.Mutex.Lock()
			yes := v.X.VersionReceived && v.Node.Nonce == c.Node.Nonce
			v.Mutex.Unlock()
			if yes {
				Mutex_net.Unlock()
				return errors.New("SameNonce")
			}
		}
	}
	Mutex_net.Unlock()

	if use_this_ip {
		if bytes.Equal(pl[40:44], c.PeerAddr.Ip4[:]) {
			if common.FLAG.Log {
				ExternalIpMutex.Lock()
				f, _ := os.OpenFile("badip_log.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
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
				f, _ := os.OpenFile("badip_log.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
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
				if c.Node.Agent == v {
					use_this_ip = false
					if !common.NoCounters.Get() {
						common.CountSafe(fmt.Sprint("IgnoreExtIP-", x))
					}
					break
				}
			}
			if use_this_ip && common.IsListenTCP() && common.GetExternalIp() == "" {
				fmt.Printf("New external IP %d.%d.%d.%d from ConnID=%d (%s)\n> ",
					pl[40], pl[41], pl[42], pl[43], c.ConnID, c.Node.Agent)
			}
		}
		if use_this_ip {
			ExternalIp4[c.Node.ReportedIp4] = [2]uint{ExternalIp4[c.Node.ReportedIp4][0] + 1,
				uint(time.Now().Unix())}
		}
		ExternalIpMutex.Unlock()
	}

	c.SendRawMsg("verack", []byte{})
	return nil
}

// SendAuth sends auth messages (only used by other gocoin nodes).
func (c *OneConnection) SendAuth() {
	rnd := make([]byte, 32)
	copy(rnd, c.Node.Nonce[:])
	r, s, er := btc.EcdsaSign(common.SecretKey, rnd)
	if er != nil {
		println(er.Error())
		return
	}
	var sig secp256k1.Signature
	sig.R.Set(r)
	sig.S.Set(s)

	msg := bytes.NewBuffer(sig.Bytes())
	// add last block hash and last block height
	common.Last.Mutex.Lock()
	msg.Write(common.Last.Block.BlockHash.Hash[:])
	binary.Write(msg, binary.LittleEndian, uint32(common.Last.Block.Height))
	common.Last.Mutex.Unlock()
	c.SendRawMsg("auth", msg.Bytes())
}

// AuthRvcd processes auth messages (from other gocoin nodes).
func (c *OneConnection) AuthRvcd(pl []byte) {
	if c.X.AuthMsgGot > 0 {
		c.DoS("AuthMsgCnt") // Only allow one auth message per connection (DoS prevention)
		return
	}
	c.X.AuthMsgGot++

	c.X.Authorized = false

	var sig secp256k1.Signature
	var pkey secp256k1.XY
	var m secp256k1.Number
	var b32 [32]byte

	sig_len := sig.ParseBytes(pl)
	if sig_len < 0 {
		return
	}

	copy(b32[:8], nonce[:]) // the remaining bytes shall be zero'ed
	m.SetBytes(b32[:])

	FriendsAccess.Lock()
	for _, pub := range AuthPubkeys {
		if pkey.ParsePubkey(pub) && sig.Verify(&pkey, &m) {
			c.X.Authorized = true
			break
		}
	}
	FriendsAccess.Unlock()
	if !c.X.Authorized {
		return
	}

	// Authorized node - check for last block data fields
	if len(pl) >= sig_len+32+4 {
		bl_height := binary.LittleEndian.Uint32(pl[sig_len+32 : sig_len+36])
		common.Last.Mutex.Lock()
		c.X.ChainSynchronized = bl_height >= uint32(common.Last.Block.Height)
		common.Last.Mutex.Unlock()

		if c.X.ChainSynchronized {
			copy(b32[:], pl[sig_len:sig_len+32])
			common.LockCfg()
			common.ApplyLTB(btc.NewUint256(b32[:]), bl_height)
			common.UnlockCfg()
		}
	}
	var repl [1]byte // return whether (we think that) we are synchronized
	if common.GetBool(&common.BlockChainSynchronized) {
		repl[0] = 1
	}
	c.SendRawMsg("authack", repl[:])
}

func (c *OneConnection) HasNetworkService() bool {
	return (c.Node.Services & (btc.SERVICE_NETWORK | btc.SERVICE_NETWORK_LIMITED)) != 0
}

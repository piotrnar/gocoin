package network

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
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
	if !common.Get(&common.CFG.TXPool.Enabled) {
		b.WriteByte(0) // don't notify me about txs
	}

	c.SendRawMsg("version", b.Bytes(), false)
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
		// of==0 means the var_int was corrupt or its value did not fit in a
		// non-negative int. le<0 is a belt-and-braces guard (VLen no longer
		// returns negatives). The len check must be written so it cannot be
		// bypassed by a huge/negative le: compare the remaining bytes against le.
		if of == 0 || le < 0 || len(pl)-(80+of) < le {
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
			common.CountSafe("IgnoreExtIP-O")
			use_this_ip = false
		} else if len(pl) >= 86 && binary.BigEndian.Uint32(pl[66:70]) != 0 &&
			!bytes.Equal(pl[66:70], c.PeerAddr.Ip4[:]) {
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
					common.CountSafePar("IgnoreExtIP-", x)
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

	c.SendRawMsg("verack", []byte{}, false)
	return nil
}

const (
	authChallengeLen = 32
	authAckLen       = authChallengeLen + 33 + 1 + 32 + 4
)

// SendAuth sends our public key and a random challenge (only used by other gocoin nodes).
// The peer proves its identity by sending the challenge back inside "authack", which
// is encrypted with the key that only the owners of both private keys can calculate.
func (c *OneConnection) SendAuth() {
	c.authChallenge = make([]byte, authChallengeLen)
	rand.Read(c.authChallenge)
	msg := new(bytes.Buffer)
	msg.Write(common.PublicKeyBin)
	msg.Write(c.authChallenge)
	c.SendRawMsg("xauth", msg.Bytes(), false)
}

// AuthRvcd processes "xauth" message (from other gocoin nodes).
// Note that it does not authorize the peer yet - see AuthAckRcvd() for that.
func (c *OneConnection) AuthRvcd(pl []byte) {
	if c.X.AuthMsgGot {
		c.DoS("XAuthMsgCnt") // Only allow one auth message per connection (DoS prevention)
		return
	}
	c.X.AuthMsgGot = true

	if len(pl) < 33+authChallengeLen {
		c.DoS("XAuthMsgShort")
		return
	}

	var shared_secret [33]byte
	if !secp256k1.Multiply(pl[:33], common.SecretKey, shared_secret[:]) {
		return
	}
	var er error
	var dat aesData
	var aeskey [32]byte
	btc.ShaHash(shared_secret[:], aeskey[:])
	if dat.Block, er = aes.NewCipher(aeskey[:]); er != nil {
		fmt.Println("aes.NewCipher:", er.Error())
		return
	}
	if dat.AEAD, er = cipher.NewGCM(dat.Block); er != nil {
		fmt.Println("cipher.NewGCM:", er.Error())
		return
	}
	dat.nonceSize = dat.AEAD.NonceSize()
	c.aesData = &dat
	//fmt.Println(c.PeerAddr.Ip(), "- secure context established")

	FriendsAccess.Lock()
	for _, pub := range AuthPubkeys {
		if bytes.Equal(pub, pl[:33]) {
			c.authPubkey = pub
			break
		}
	}
	FriendsAccess.Unlock()
	if c.authPubkey == nil {
		return // not one of our friends
	}

	// Answer the challenge. Our public key is included, so the message cannot be
	// reflected back to us, pretending to be the peer's answer to our own challenge.
	msg := bytes.NewBuffer(make([]byte, 0, authAckLen))
	msg.Write(pl[33 : 33+authChallengeLen])
	msg.Write(common.PublicKeyBin)
	if common.BlockChainSynchronized.Load() {
		msg.WriteByte(1) // whether (we think that) we are synchronized
	} else {
		msg.WriteByte(0)
	}
	// add last block hash and last block height
	common.Last.Mutex.Lock()
	msg.Write(common.Last.Block.BlockHash.Hash[:])
	binary.Write(msg, binary.LittleEndian, uint32(common.Last.Block.Height))
	common.Last.Mutex.Unlock()
	c.SendRawMsg("authack", msg.Bytes(), true)
}

// AuthAckRcvd processes "authack" message, which is the peer's answer to our "xauth".
// The peer gets authorized only here: the message must be encrypted (so the sender
// knows the private key of the public key it sent us in its "xauth"), come from one
// of our friends and carry the challenge that we made for this very connection.
func (c *OneConnection) AuthAckRcvd(cmd *BCmsg) {
	pl := cmd.pl
	if !cmd.crypted || c.authPubkey == nil || c.authChallenge == nil || c.X.AuthAckGot {
		// Do not ban for it, as any peer can send it.
		c.Disconnect(false, "UnsignedAuthAck")
		return
	}
	if len(pl) < authAckLen || !bytes.Equal(pl[:authChallengeLen], c.authChallenge) ||
		!bytes.Equal(pl[authChallengeLen:authChallengeLen+33], c.authPubkey) {
		common.CountSafe("AuthAckInvalid")
		c.Disconnect(false, "InvalidAuthAck")
		return
	}
	of := authChallengeLen + 33
	synchronized := pl[of] != 0
	bl_hash := btc.NewUint256(pl[of+1 : of+33])
	bl_height := binary.LittleEndian.Uint32(pl[of+33 : of+37])

	c.Mutex.Lock()
	c.X.Authorized = true
	c.X.AuthAckGot = true
	c.X.ChainSynchronized = synchronized
	c.Mutex.Unlock()

	common.Last.Mutex.Lock()
	not_behind := bl_height >= uint32(common.Last.Block.Height)
	common.Last.Mutex.Unlock()
	if not_behind {
		common.LockCfg()
		common.ApplyLTB(bl_hash, bl_height)
		common.UnlockCfg()
	}

	if synchronized {
		c.GetMPNow() // No point in asking non-synched nodes for their mempool
	}
}

func (c *OneConnection) HasNetworkService() bool {
	return (c.Node.Services & (btc.SERVICE_NETWORK | btc.SERVICE_NETWORK_LIMITED)) != 0
}

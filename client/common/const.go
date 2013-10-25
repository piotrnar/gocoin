package common

import (
	"github.com/piotrnar/gocoin/btc"
)

const (
	ConfigFile = "gocoin.conf"

	Version = 70001
	UserAgent = "/Gocoin:"+btc.SourcesTag+"/"
	Services = uint64(0x00000001)

	MaxCachedBlocks = 600
)

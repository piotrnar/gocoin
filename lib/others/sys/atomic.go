package sys

import (
	"fmt"
	"sync/atomic"
)

type SyncBool struct {
	atomic.Bool
}

func (b *SyncBool) Get() bool {
	return b.Load()
}

func (b *SyncBool) Set() {
	b.Store(true)
}

func (b *SyncBool) Clr() {
	b.Store(false)
}

func (b *SyncBool) MarshalText() (text []byte, err error) {
	return []byte(fmt.Sprint(b.Get())), nil
}

type SyncInt struct {
	atomic.Int64
}

func (b *SyncInt) Get() int {
	return int(b.Load())
}

func (b *SyncInt) Store(val int) {
	b.Int64.Store(int64(val))
}

func (b *SyncInt) Add(val int) {
	b.Int64.Add(int64(val))
}

func (b *SyncInt) MarshalText() (text []byte, err error) {
	return []byte(fmt.Sprint(b.Get())), nil
}

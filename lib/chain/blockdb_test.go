package chain

import (
	"bytes"
	"testing"

	"github.com/piotrnar/gocoin/lib/btc"
)

func nopWalk(ch *Chain, hash, hdr []byte, height, blen, txs uint32) {}

// makeTestBlock returns a fake block (BlockDB does not look inside), unique for the given id.
func makeTestBlock(id byte, size int) *btc.Block {
	raw := make([]byte, size)
	for i := range raw {
		raw[i] = byte(i) ^ id
	}
	raw[80] = 1 // one transaction
	bl, er := btc.NewBlock(raw)
	if er != nil {
		panic(er.Error())
	}
	return bl
}

func openTestDB(t *testing.T, dir string, opts *BlockDBOpts) (db *BlockDB, heights map[[32]byte]uint32) {
	heights = make(map[[32]byte]uint32)
	db = NewBlockDBExt(dir, opts)
	db.LoadBlockIndex(nil, func(ch *Chain, hash, hdr []byte, height, blen, txs uint32) {
		var h [32]byte
		copy(h[:], hash)
		heights[h] = height
	})
	t.Cleanup(db.Close) // in case the test fails before closing it (closing it twice does no harm)
	return
}

func TestBlockDBStoreAndReload(t *testing.T) {
	for _, compress := range []bool{false, true} {
		dir := t.TempDir() + "/"
		opts := &BlockDBOpts{MaxCachedBlocks: 2, CompressOnDisk: compress}
		db, _ := openTestDB(t, dir, opts)

		var bls []*btc.Block
		for i := 0; i < 6; i++ {
			bl := makeTestBlock(byte(i+1), 200+50*i)
			if i == 1 {
				bl.Trusted.Set()
			}
			bls = append(bls, bl)
			db.BlockAdd(uint32(i+1), bl)
		}

		// before they are written, the blocks must be served from the cache
		for i, bl := range bls {
			dat, trusted, er := db.BlockGet(bl.Hash)
			if er != nil || !bytes.Equal(dat, bl.Raw) || trusted != (i == 1) {
				t.Fatal("unwritten block", i, er, trusted)
			}
		}
		db.Idle() // this writes them
		db.BlockTrusted(bls[3].Hash.Hash[:])

		// now most of them have to be read from the disk
		for i, bl := range bls {
			dat, trusted, er := db.BlockGet(bl.Hash)
			if er != nil || !bytes.Equal(dat, bl.Raw) || trusted != (i == 1 || i == 3) {
				t.Fatal("written block", i, er, trusted)
			}
			if le, er := db.BlockLength(bl.Hash, true); er != nil || int(le) != len(bl.Raw) {
				t.Fatal("block length", i, le, er)
			}
		}
		db.Close()

		db, heights := openTestDB(t, dir, opts)
		if len(heights) != len(bls) {
			t.Fatal("blocks after reload:", len(heights))
		}
		for i, bl := range bls {
			if heights[bl.Hash.Hash] != uint32(i+1) {
				t.Fatal("height of block", i)
			}
			dat, trusted, er := db.BlockGet(bl.Hash)
			if er != nil || !bytes.Equal(dat, bl.Raw) || trusted != (i == 1 || i == 3) {
				t.Fatal("reloaded block", i, er, trusted)
			}
		}

		// add one more, to make sure that it does not damage the existing ones
		bl := makeTestBlock(100, 300)
		bls = append(bls, bl)
		db.BlockAdd(7, bl)
		db.Close()

		db, heights = openTestDB(t, dir, opts)
		if len(heights) != len(bls) {
			t.Fatal("blocks after second reload:", len(heights))
		}
		for i, bl := range bls {
			if dat, _, er := db.BlockGet(bl.Hash); er != nil || !bytes.Equal(dat, bl.Raw) {
				t.Fatal("block after second reload", i, er)
			}
		}
		db.Close()
	}
}

// An invalid block's record stays in the index file, so the records that follow it must keep their positions.
func TestBlockDBReloadAfterInvalid(t *testing.T) {
	dir := t.TempDir() + "/"
	db, _ := openTestDB(t, dir, nil)
	var bls []*btc.Block
	for i := 0; i < 4; i++ {
		bl := makeTestBlock(byte(i+1), 150)
		bls = append(bls, bl)
		db.BlockAdd(uint32(i+1), bl)
	}
	db.Idle()
	db.BlockInvalid(bls[1].Hash.Hash[:])
	db.Close()

	db, heights := openTestDB(t, dir, nil)
	if len(heights) != 3 {
		t.Fatal("blocks after reload:", len(heights))
	}
	if _, ok := heights[bls[1].Hash.Hash]; ok {
		t.Fatal("the invalid block was loaded")
	}
	for i, bl := range bls {
		if i == 1 {
			continue
		}
		if rec := db.blockIndex[bl.Hash.BIdx()]; rec == nil || rec.ipos != int64(i)*136 {
			t.Fatal("wrong index position of block", i)
		}
	}
	if db.maxidxfilepos != 4*136 {
		t.Fatal("wrong end of the index:", db.maxidxfilepos)
	}

	// Setting a flag must go to the right record, and a new block must not overwrite any.
	db.BlockTrusted(bls[3].Hash.Hash[:])
	bl := makeTestBlock(50, 150)
	db.BlockAdd(5, bl)
	db.Close()

	db, heights = openTestDB(t, dir, nil)
	if len(heights) != 4 || heights[bls[3].Hash.Hash] != 4 || heights[bl.Hash.Hash] != 5 {
		t.Fatal("wrong blocks after second reload:", len(heights))
	}
	for i, b := range append(bls, bl) {
		if i == 1 {
			continue
		}
		dat, trusted, er := db.BlockGet(b.Hash)
		if er != nil || !bytes.Equal(dat, b.Raw) || trusted != (i == 3) {
			t.Fatal("block after second reload", i, er, trusted)
		}
	}
	db.Close()
}

// A block that was marked invalid before it got written shall never be stored.
func TestBlockDBInvalidBeforeWrite(t *testing.T) {
	dir := t.TempDir() + "/"
	db, _ := openTestDB(t, dir, nil)
	b1, b2 := makeTestBlock(1, 150), makeTestBlock(2, 150)
	db.BlockAdd(1, b1)
	db.BlockAdd(2, b2)
	db.BlockInvalid(b1.Hash.Hash[:])
	if _, _, er := db.BlockGet(b1.Hash); er == nil {
		t.Fatal("invalid block still there")
	}
	db.Close()

	db, heights := openTestDB(t, dir, nil)
	if len(heights) != 1 || heights[b2.Hash.Hash] != 2 || db.maxidxfilepos != 136 {
		t.Fatal("wrong content after reload:", len(heights), db.maxidxfilepos)
	}
	db.Close()
}

package leveldb

func putuint32(p []byte, val uint32) {
	p[0] = byte(val)
	p[1] = byte(val>>8)
	p[2] = byte(val>>16)
	p[3] = byte(val>>24)
}

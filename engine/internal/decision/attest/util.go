package attest

import (
	"encoding/binary"
	"hash"
)

// writeLP writes a length-prefixed byte slice into h. The length prefix is
// an 8-byte big-endian unsigned integer. This gives an unambiguous encoding
// so that concatenations of two fields cannot collide with a longer single
// field.
func writeLP(h hash.Hash, b []byte) {
	var lp [8]byte
	binary.BigEndian.PutUint64(lp[:], uint64(len(b)))
	h.Write(lp[:])
	h.Write(b)
}

// writeU64 writes a uint64 big-endian into h.
func writeU64(h hash.Hash, v uint64) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	h.Write(buf[:])
}

package writer

import (
	"hash"
	"math/bits"
	"unsafe"
)

type bmixer interface {
	bmix(p []byte) (tail []byte)
	Size() (n int)
	reset()
}

type digest struct {
	clen int
	tail []byte
	buf  [16]byte
	seed uint32
	bmixer
}

func (d *digest) BlockSize() int { return 1 }

func (d *digest) Write(p []byte) (n int, err error) {
	n = len(p)
	d.clen += n

	if len(d.tail) > 0 {
		// Stick back pending bytes.
		nfree := d.Size() - len(d.tail) // nfree ∈ [1, d.Size()-1].
		if nfree < len(p) {
			// One full block can be formed.
			block := append(d.tail, p[:nfree]...)
			p = p[nfree:]
			_ = d.bmix(block) // No tail.
		} else {
			// Tail's buf is large enough to prevent reallocs.
			p = append(d.tail, p...)
		}
	}

	d.tail = d.bmix(p)

	// Keep own copy of the 0 to Size()-1 pending bytes.
	nn := copy(d.buf[:], d.tail)
	d.tail = d.buf[:nn]

	return n, nil
}

func (d *digest) Reset() {
	d.clen = 0
	d.tail = nil
	d.bmixer.reset()
}

// Make sure interfaces are correctly implemented.
var (
	_ hash.Hash   = new(digest32)
	_ hash.Hash32 = new(digest32)
	_ bmixer      = new(digest32)
)

const (
	c1_32 uint32 = 0xcc9e2d51
	c2_32 uint32 = 0x1b873593
)

// digest32 represents a partial evaluation of a 32 bites hash.
type digest32 struct {
	digest
	h1 uint32 // Unfinalized running hash.
}

// NewWithSeed returns new 32-bit hasher
func Murmur3() hash.Hash32 {
	d := new(digest32)
	d.seed = 0
	d.bmixer = d
	d.Reset()
	return d
}

func (d *digest32) Size() int { return 4 }

func (d *digest32) reset() { d.h1 = d.seed }

func (d *digest32) Sum(b []byte) []byte {
	h := d.Sum32()
	return append(b, byte(h>>24), byte(h>>16), byte(h>>8), byte(h))
}

// Digest as many blocks as possible.
func (d *digest32) bmix(p []byte) (tail []byte) {
	h1 := d.h1

	nblocks := len(p) / 4
	for i := 0; i < nblocks; i++ {
		k1 := *(*uint32)(unsafe.Pointer(&p[i*4]))

		k1 *= c1_32
		k1 = bits.RotateLeft32(k1, 15)
		k1 *= c2_32

		h1 ^= k1
		h1 = bits.RotateLeft32(h1, 13)
		h1 = h1*4 + h1 + 0xe6546b64
	}
	d.h1 = h1
	return p[nblocks*d.Size():]
}

func (d *digest32) Sum32() (h1 uint32) {

	h1 = d.h1

	var k1 uint32
	switch len(d.tail) & 3 {
	case 3:
		k1 ^= uint32(d.tail[2]) << 16
		fallthrough
	case 2:
		k1 ^= uint32(d.tail[1]) << 8
		fallthrough
	case 1:
		k1 ^= uint32(d.tail[0])
		k1 *= c1_32
		k1 = bits.RotateLeft32(k1, 15)
		k1 *= c2_32
		h1 ^= k1
	}

	h1 ^= uint32(d.clen)

	h1 ^= h1 >> 16
	h1 *= 0x85ebca6b
	h1 ^= h1 >> 13
	h1 *= 0xc2b2ae35
	h1 ^= h1 >> 16

	return h1
}

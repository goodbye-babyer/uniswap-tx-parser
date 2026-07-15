package uniswaptxparser

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

const abiWord = 32

type decoder struct{ b []byte }

func (d decoder) word(off int) ([]byte, error) {
	if off < 0 || off > len(d.b)-abiWord {
		return nil, fmt.Errorf("%w: word at %d", ErrMalformedCalldata, off)
	}
	return d.b[off : off+abiWord], nil
}
func (d decoder) offset(off int) (int, error) {
	w, e := d.word(off)
	if e != nil {
		return 0, e
	}
	for _, x := range w[:24] {
		if x != 0 {
			return 0, fmt.Errorf("%w: offset overflow", ErrMalformedCalldata)
		}
	}
	n := binary.BigEndian.Uint64(w[24:])
	if n > uint64(len(d.b)) {
		return 0, fmt.Errorf("%w: offset out of bounds", ErrMalformedCalldata)
	}
	return int(n), nil
}
func (d decoder) uint(off int) (*big.Int, error) {
	w, e := d.word(off)
	if e != nil {
		return nil, e
	}
	return new(big.Int).SetBytes(w), nil
}
func (d decoder) uint64(off int) (uint64, error) {
	w, e := d.word(off)
	if e != nil {
		return 0, e
	}
	for _, x := range w[:24] {
		if x != 0 {
			return 0, fmt.Errorf("%w: uint64 overflow", ErrMalformedCalldata)
		}
	}
	return binary.BigEndian.Uint64(w[24:]), nil
}
func (d decoder) address(off int) (common.Address, error) {
	w, e := d.word(off)
	if e != nil {
		return common.Address{}, e
	}
	for _, x := range w[:12] {
		if x != 0 {
			return common.Address{}, fmt.Errorf("%w: dirty address padding", ErrMalformedCalldata)
		}
	}
	return common.BytesToAddress(w[12:]), nil
}
func (d decoder) boolean(off int) (bool, error) {
	n, e := d.uint64(off)
	if e != nil {
		return false, e
	}
	if n > 1 {
		return false, fmt.Errorf("%w: invalid bool", ErrMalformedCalldata)
	}
	return n == 1, nil
}
func (d decoder) signed24(off int) (int32, error) {
	w, e := d.word(off)
	if e != nil {
		return 0, e
	}
	neg := w[29]&0x80 != 0
	pad := byte(0)
	if neg {
		pad = 0xff
	}
	for _, x := range w[:29] {
		if x != pad {
			return 0, fmt.Errorf("%w: invalid int24 padding", ErrMalformedCalldata)
		}
	}
	u := uint32(w[29])<<16 | uint32(w[30])<<8 | uint32(w[31])
	if neg {
		u |= 0xff000000
	}
	return int32(u), nil
}
func (d decoder) bytesAt(head, base int) ([]byte, error) {
	rel, e := d.offset(head)
	if e != nil {
		return nil, e
	}
	return d.bytesBody(base + rel)
}
func (d decoder) bytesBody(start int) ([]byte, error) {
	n, e := d.offset(start)
	if e != nil {
		return nil, e
	}
	begin := start + 32
	if n < 0 || begin > len(d.b) || n > len(d.b)-begin {
		return nil, fmt.Errorf("%w: bytes out of bounds", ErrMalformedCalldata)
	}
	return cloneBytes(d.b[begin : begin+n]), nil
}
func (d decoder) addressArray(head, base int) ([]common.Address, error) {
	rel, e := d.offset(head)
	if e != nil {
		return nil, e
	}
	start := base + rel
	n, e := d.offset(start)
	if e != nil {
		return nil, e
	}
	if n > (len(d.b)-start-32)/32 {
		return nil, fmt.Errorf("%w: address array out of bounds", ErrMalformedCalldata)
	}
	out := make([]common.Address, n)
	for i := range out {
		out[i], e = d.address(start + 32 + i*32)
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func (d decoder) bytesArray(head, base int) ([][]byte, error) {
	rel, e := d.offset(head)
	if e != nil {
		return nil, e
	}
	start := base + rel
	n, e := d.offset(start)
	if e != nil {
		return nil, e
	}
	itemsBase := start + 32
	if n > (len(d.b)-itemsBase)/32 {
		return nil, fmt.Errorf("%w: bytes array out of bounds", ErrMalformedCalldata)
	}
	out := make([][]byte, n)
	for i := range out {
		out[i], e = d.bytesAt(itemsBase+i*32, itemsBase)
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}

func selectorFor(signature string) [4]byte {
	h := keccak([]byte(signature))
	var out [4]byte
	copy(out[:], h[:4])
	return out
}
func selectorOf(data []byte) ([4]byte, error) {
	var s [4]byte
	if len(data) < 4 {
		return s, fmt.Errorf("%w: missing selector", ErrMalformedCalldata)
	}
	copy(s[:], data[:4])
	return s, nil
}

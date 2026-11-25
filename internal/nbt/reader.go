// Package nbt reads named, big-endian Java Edition NBT data.
package nbt

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// MaxBytes bounds data before and, for compressed files, after decompression.
const MaxBytes = 32 << 20

var ErrInvalid = errors.New("invalid NBT")

// Read visits the fields of a named root compound. The visitor must read or
// skip each field payload before returning.
func Read(ctx context.Context, data []byte, field func(*Reader, byte, string)) error {
	if len(data) > MaxBytes {
		return ErrInvalid
	}
	r := Reader{ctx: ctx, data: data}
	if r.Byte() != 10 {
		return ErrInvalid
	}
	r.rawString()
	r.Compound(func(tag byte, key string) { field(&r, tag, key) })
	if r.err != nil {
		return r.err
	}
	if len(r.data) != 0 {
		return ErrInvalid
	}
	return nil
}

type Reader struct {
	data []byte
	ctx  context.Context
	err  error
}

func (r *Reader) Invalidate() {
	if r.err == nil {
		r.err = ErrInvalid
	}
}

func (r *Reader) take(n int) []byte {
	if n < 0 || n > len(r.data) {
		r.Invalidate()
		return nil
	}
	part := r.data[:n]
	r.data = r.data[n:]
	return part
}

func (r *Reader) Byte() byte {
	if b := r.take(1); b != nil {
		return b[0]
	}
	return 0
}

func (r *Reader) Count() int {
	b := r.take(4)
	if b == nil {
		return 0
	}
	n := binary.BigEndian.Uint32(b)
	if uint64(n) > uint64(len(r.data)) {
		r.Invalidate()
		return 0
	}
	return int(n)
}

func (r *Reader) rawString() []byte {
	b := r.take(2)
	if b == nil {
		return nil
	}
	return r.take(int(binary.BigEndian.Uint16(b)))
}

func (r *Reader) Compound(field func(byte, string)) {
	for r.err == nil {
		if err := r.ctx.Err(); err != nil {
			r.err = err
			return
		}
		tag := r.Byte()
		if tag == 0 {
			return
		}
		field(tag, string(r.rawString()))
	}
}

func (r *Reader) Err() error { return r.err }

func (r *Reader) Skip(tag byte) { r.skip(tag, 0) }

func (r *Reader) Int() int32 {
	b := r.take(4)
	if b == nil {
		return 0
	}
	return int32(binary.BigEndian.Uint32(b))
}

func (r *Reader) Long() int64 {
	b := r.take(8)
	if b == nil {
		return 0
	}
	return int64(binary.BigEndian.Uint64(b))
}

func (r *Reader) JavaString() (string, bool) {
	b := r.rawString()
	if r.err != nil {
		return "", false
	}
	return javaText(b)
}

func (r *Reader) skip(tag byte, depth int) {
	if depth > 64 {
		r.Invalidate()
		return
	}
	switch tag {
	case 1:
		r.take(1)
	case 2:
		r.take(2)
	case 3, 5:
		r.take(4)
	case 4, 6:
		r.take(8)
	case 8:
		r.rawString()
	case 7, 11, 12:
		width := 1
		if tag == 11 {
			width = 4
		} else if tag == 12 {
			width = 8
		}
		n := r.Count()
		if n > len(r.data)/width {
			r.Invalidate()
			return
		}
		r.take(n * width)
	case 9:
		elem, n := r.Byte(), r.Count()
		for range n {
			if r.err != nil {
				return
			}
			if err := r.ctx.Err(); err != nil {
				r.err = err
				return
			}
			r.skip(elem, depth+1)
		}
	case 10:
		r.Compound(func(k byte, _ string) { r.skip(k, depth+1) })
	default:
		r.Invalidate()
	}
}

// Java's writeUTF encodes NUL and UTF-16 surrogate pairs differently from UTF-8.
func javaText(b []byte) (string, bool) {
	if utf8.Valid(b) {
		return string(b), true
	}
	var out strings.Builder
	valid := true
	for len(b) > 0 {
		switch {
		case len(b) >= 2 && b[0] == 0xc0 && b[1] == 0x80:
			out.WriteByte(0)
			b = b[2:]
		case len(b) >= 6 && b[0] == 0xed && b[1]&0xf0 == 0xa0 && b[2]&0xc0 == 0x80 && b[3] == 0xed && b[4]&0xf0 == 0xb0 && b[5]&0xc0 == 0x80:
			hi := rune(0xd800) + rune(b[1]&15)*64 + rune(b[2]&63)
			lo := rune(0xdc00) + rune(b[4]&15)*64 + rune(b[5]&63)
			out.WriteRune(utf16.DecodeRune(hi, lo))
			b = b[6:]
		default:
			ch, n := utf8.DecodeRune(b)
			if ch == utf8.RuneError && n == 1 {
				valid = false
			}
			out.WriteRune(ch)
			b = b[n:]
		}
	}
	return out.String(), valid
}

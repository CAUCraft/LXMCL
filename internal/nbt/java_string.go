package nbt

import (
	"encoding/binary"
	"unicode/utf16"
	"unicode/utf8"
)

// AppendJavaString appends a length-prefixed Java modified UTF-8 string.
func AppendJavaString(dst []byte, value string) ([]byte, error) {
	const maxLength = 1<<16 - 1
	if len(value) > maxLength || !utf8.ValidString(value) {
		return dst, ErrInvalid
	}
	start := len(dst)
	dst = append(dst, 0, 0)
	for _, ch := range value {
		switch {
		case ch == 0:
			dst = append(dst, 0xc0, 0x80)
		case ch > 0xffff:
			hi, lo := utf16.EncodeRune(ch)
			for _, unit := range []rune{hi, lo} {
				dst = append(dst, byte(0xe0|unit>>12), byte(0x80|unit>>6&0x3f), byte(0x80|unit&0x3f))
			}
		default:
			dst = utf8.AppendRune(dst, ch)
		}
	}
	size := len(dst) - start - 2
	if size > maxLength {
		return dst[:start], ErrInvalid
	}
	binary.BigEndian.PutUint16(dst[start:], uint16(size))
	return dst, nil
}

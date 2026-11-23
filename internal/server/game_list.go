package server

import (
	"context"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

const maxGameListBytes = 32 << 20

// ReadGameList reads the visible favorites of a Java Edition instance, in file order.
// An absent game directory or servers.dat is reported as os.ErrNotExist.
func ReadGameList(ctx context.Context, gameDir string) ([]Server, error) {
	if !filepath.IsAbs(gameDir) {
		return nil, i18n.Error("instance.game_dir_required")
	}
	root, err := os.OpenRoot(gameDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat("servers.dat")
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, i18n.Error("server.game_list_invalid")
	}
	if info.Size() > maxGameListBytes {
		return nil, i18n.Error("server.game_list_large")
	}
	file, err := root.Open("servers.dat")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(transfer.Reader(ctx, file), maxGameListBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxGameListBytes {
		return nil, i18n.Error("server.game_list_large")
	}
	return decodeGameList(ctx, data)
}

// servers.dat uses an uncompressed, named, big-endian NBT compound.
func decodeGameList(ctx context.Context, data []byte) ([]Server, error) {
	r := gameNBT{data: data, ctx: ctx}
	if r.octet() != 10 {
		return nil, i18n.Error("server.game_list_invalid")
	}
	r.rawString() // root name
	var entries []Server
	found := false
	r.compound(func(kind byte, key string) {
		if key != "servers" {
			r.skip(kind, 0)
			return
		}
		if kind != 9 || found {
			r.invalid()
			return
		}
		found = true
		element, count := r.octet(), r.count()
		if count > 0 && element != 10 {
			r.invalid()
			return
		}
		if count > 65536 {
			r.err = i18n.Error("server.game_list_large")
			return
		}
		for range count {
			if r.err != nil {
				break
			}
			var s Server
			hidden := false
			r.compound(func(tag byte, name string) {
				switch {
				case tag == 8 && name == "name":
					s.Name = gameNBTText(r.rawString())
				case tag == 8 && name == "ip":
					s.Address = gameNBTText(r.rawString())
				case tag == 1 && name == "hidden":
					hidden = r.octet() != 0
				default:
					r.skip(tag, 0)
				}
			})
			if !hidden && r.err == nil {
				entries = append(entries, s)
			}
		}
	})
	if r.err != nil {
		return nil, r.err
	}
	if !found || len(r.data) != 0 {
		return nil, i18n.Error("server.game_list_invalid")
	}
	return entries, nil
}

type gameNBT struct {
	data []byte
	ctx  context.Context
	err  error
}

func (r *gameNBT) invalid() {
	if r.err == nil {
		r.err = i18n.Error("server.game_list_invalid")
	}
}

func (r *gameNBT) take(n int) []byte {
	if n < 0 || n > len(r.data) {
		r.invalid()
		return nil
	}
	part := r.data[:n]
	r.data = r.data[n:]
	return part
}

func (r *gameNBT) octet() byte {
	if b := r.take(1); b != nil {
		return b[0]
	}
	return 0
}

func (r *gameNBT) count() int {
	b := r.take(4)
	if b == nil {
		return 0
	}
	n := binary.BigEndian.Uint32(b)
	if uint64(n) > uint64(len(r.data)) {
		r.invalid()
		return 0
	}
	return int(n)
}

func (r *gameNBT) rawString() []byte {
	b := r.take(2)
	if b == nil {
		return nil
	}
	return r.take(int(binary.BigEndian.Uint16(b)))
}

func (r *gameNBT) compound(field func(byte, string)) {
	for r.err == nil {
		if err := r.ctx.Err(); err != nil {
			r.err = err
			return
		}
		tag := r.octet()
		if tag == 0 {
			return
		}
		field(tag, string(r.rawString()))
	}
}

func (r *gameNBT) skip(tag byte, depth int) {
	if depth > 64 {
		r.invalid()
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
		n := r.count()
		if n > len(r.data)/width {
			r.invalid()
			return
		}
		r.take(n * width)
	case 9:
		elem, n := r.octet(), r.count()
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
		r.compound(func(k byte, _ string) { r.skip(k, depth+1) })
	default:
		r.invalid()
	}
}

// Java's writeUTF encodes NUL and UTF-16 surrogate pairs differently from UTF-8.
func gameNBTText(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var out strings.Builder
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
			out.WriteRune(ch)
			b = b[n:]
		}
	}
	return out.String()
}

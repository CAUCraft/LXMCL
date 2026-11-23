package content

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

type SavedPackKind int

const (
	OtherPackID SavedPackKind = iota
	LocalPack
	BuiltinPack
	LoaderPackID
	MissingPack
)

type SavedPack struct {
	ID   string
	Kind SavedPackKind
	File *File // Matched inventory entry for LocalPack.
}

// SavedResourcePacks follows the order of the last resourcePacks setting.
// A nil slice denotes an absent setting; an explicitly saved [] is non-nil.
func SavedResourcePacks(ctx context.Context, i instance.Instance) ([]SavedPack, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(i.GameDir) {
		return nil, i18n.Error("instance.game_dir_required")
	}
	root, err := os.OpenRoot(i.GameDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	data, err := packEntry(ctx, root.FS(), "options.txt", 4<<20)
	if err != nil {
		return nil, err
	}
	var value []byte
	for line := range bytes.Lines(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))) {
		key, v, ok := bytes.Cut(line, []byte(":"))
		if string(bytes.TrimSpace(key)) == "resourcePacks" {
			if !ok {
				return nil, i18n.Error("resource.saved_packs_invalid")
			}
			value = v
		}
	}
	if value == nil {
		return nil, nil
	}
	var ids []string
	if !utf8.Valid(value) || json.Unmarshal(value, &ids) != nil || ids == nil {
		return nil, i18n.Error("resource.saved_packs_invalid")
	}
	result := make([]SavedPack, 0, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	files, err := Inventory(ctx, i, ResourcePacks)
	if err != nil {
		return nil, err
	}
	local := make(map[string]*File, len(files))
	for n := range files {
		f := &files[n]
		// A matching active filename takes precedence over a .disabled sibling.
		if _, ok := local[f.Name]; !ok || f.Enabled {
			local[f.Name] = f
		}
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if id == "" || strings.ContainsFunc(id, unicode.IsControl) {
			return nil, i18n.Error("resource.saved_packs_invalid")
		}
		p := SavedPack{ID: id}
		switch id {
		case "vanilla", "programmer_art", "programer_art", "high_contrast":
			p.Kind = BuiltinPack
		case "fabric", "Fabric Mods", "mod_resources":
			p.Kind = LoaderPackID
		default:
			name, prefixed := strings.CutPrefix(id, "file/")
			if f, ok := local[name]; ok {
				p.Kind, p.File = LocalPack, f
			} else if prefixed || ValidName(id, ".zip") {
				p.Kind = MissingPack
			}
		}
		result = append(result, p)
	}
	return result, nil
}

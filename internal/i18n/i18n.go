// Package i18n renders launcher backend messages from embedded locale catalogs.
package i18n

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

const defaultLocale = "zh-cn"

//go:embed locales/*.json
var bundled embed.FS

var (
	catalogs = loadCatalogs(bundled)
	current  = selectCatalog(os.Getenv("LXMCL_LOCALE"), catalogs)
)

type catalog struct {
	messages, fallback map[string]string
}

func loadCatalogs(files fs.FS) map[string]map[string]string {
	paths, err := fs.Glob(files, "locales/*.json")
	if err != nil {
		panic(err)
	}
	out := make(map[string]map[string]string, len(paths))
	for _, path := range paths {
		raw, err := fs.ReadFile(files, path)
		if err != nil {
			panic(err)
		}
		var messages map[string]string
		if err := json.Unmarshal(raw, &messages); err != nil {
			panic(fmt.Errorf("load locale %s: %w", path, err))
		}
		name := strings.TrimSuffix(strings.TrimPrefix(path, "locales/"), ".json")
		out[strings.ToLower(name)] = messages
	}
	if len(out[defaultLocale]) == 0 {
		panic("missing default locale catalog")
	}
	return out
}

func selectCatalog(locale string, available map[string]map[string]string) catalog {
	locale = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(locale), "_", "-"))
	for candidate := locale; candidate != ""; {
		if messages, ok := available[candidate]; ok {
			return catalog{messages: messages, fallback: available[defaultLocale]}
		}
		last := strings.LastIndexByte(candidate, '-')
		if last < 0 {
			break
		}
		candidate = candidate[:last]
	}
	return catalog{fallback: available[defaultLocale]}
}

func (c catalog) template(key string) string {
	if msg := c.messages[key]; msg != "" {
		return msg
	}
	if msg := c.fallback[key]; msg != "" {
		return msg
	}
	return "[" + key + "]"
}

func (c catalog) text(key string, args ...any) string {
	msg := c.template(key)
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}

func (c catalog) err(key string, args ...any) error {
	msg := c.template(key)
	if len(args) == 0 {
		return errors.New(msg)
	}
	return fmt.Errorf(msg, args...)
}

// Text formats a backend message in the process locale.
func Text(key string, args ...any) string { return current.text(key, args...) }

// Error formats a backend error. A %w placeholder preserves the wrapped cause.
func Error(key string, args ...any) error { return current.err(key, args...) }

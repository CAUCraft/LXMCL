package javaruntime

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// Requirement comes from verified Minecraft version metadata.
type Requirement struct {
	Component string
	Major     int
}

// Manager owns only managed Java installations. An explicit path is never replaced.
type Manager struct {
	Root   string
	Client *http.Client
	probe  func(context.Context, string) (Runtime, error)
}

type Progress func(done, total int)

// Resolve prefers user-selected and system Java, then installs the official runtime.
// Automatic installation currently supports Windows amd64. Other platforms keep
// local discovery and report a missing runtime without changing user configuration.
func (m Manager) Resolve(ctx context.Context, req Requirement, preferred string, report Progress) (Runtime, error) {
	found, err := m.Local(ctx, req, preferred)
	if err == nil || strings.TrimSpace(preferred) != "" || ctx.Err() != nil {
		return found, err
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" || req.Component == "" {
		return found, err
	}
	return m.Ensure(ctx, req, report)
}

// Ensure verifies a managed runtime or installs it atomically. It never launches a game.
func (m Manager) Ensure(ctx context.Context, req Requirement, report Progress) (Runtime, error) {
	if !componentID.MatchString(req.Component) || req.Major < 8 || req.Major > 100 || !filepath.IsAbs(m.Root) {
		return Runtime{}, i18n.Error("java.runtime_requirement_invalid")
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return Runtime{}, i18n.Error("java.runtime_platform_unsupported")
	}
	select {
	case installGate <- struct{}{}:
		defer func() { <-installGate }()
	case <-ctx.Done():
		return Runtime{}, ctx.Err()
	}
	root := filepath.Join(m.Root, "windows-x64", req.Component)
	if err := os.MkdirAll(root, 0700); err != nil {
		return Runtime{}, err
	}
	if found, err := m.managed(ctx, req); err == nil {
		return found, nil
	}
	if ctx.Err() != nil {
		return Runtime{}, ctx.Err()
	}
	return m.install(ctx, root, req, report)
}

var installGate = make(chan struct{}, 1)

func canceled(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// Local discovers system and verified managed Java without downloading anything.
func (m Manager) Local(ctx context.Context, req Requirement, preferred string) (Runtime, error) {
	found, err := Find(ctx, preferred, req.Major)
	if err == nil || strings.TrimSpace(preferred) != "" || ctx.Err() != nil {
		return found, err
	}
	if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" && componentID.MatchString(req.Component) && filepath.IsAbs(m.Root) {
		if found, cacheErr := m.managed(ctx, req); cacheErr == nil {
			return found, nil
		}
	}
	return found, canceled(ctx, err)
}

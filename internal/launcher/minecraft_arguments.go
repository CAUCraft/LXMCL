package launcher

import (
	"encoding/json"
	"regexp"
	"runtime"
	"slices"
	"strconv"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

var variable = regexp.MustCompile("\\$\\{([^}]+)\\}")

func allowed(rules []rule, features map[string]bool) bool {
	if len(rules) == 0 {
		return true
	}
	osName, arch := runtime.GOOS, runtime.GOARCH
	if osName == "darwin" {
		osName = "osx"
	}
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "386":
		arch = "x86"
	case "arm64":
		arch = "aarch64"
	}
	ok := false
	for _, r := range rules {
		if r.OS.Name != "" && r.OS.Name != osName || r.OS.Version != "" {
			continue
		}
		if r.OS.Arch != "" {
			archMatches, _ := regexp.MatchString("^(?:"+r.OS.Arch+")$", arch)
			if !archMatches {
				continue
			}
		}
		matches := true
		for feature, want := range r.Features {
			if features[feature] != want {
				matches = false
				break
			}
		}
		if matches {
			ok = r.Action == "allow"
		}
	}
	return ok
}

func gameArgs(raw []json.RawMessage, values map[string]string, features map[string]bool) ([]string, error) {
	var args []string
	for _, item := range raw {
		var s string
		if json.Unmarshal(item, &s) == nil {
			args = append(args, s)
			continue
		}
		var entry struct {
			Rules []rule
			Value json.RawMessage
		}
		if err := json.Unmarshal(item, &entry); err != nil {
			return nil, err
		}
		if !allowed(entry.Rules, features) {
			continue
		}
		if json.Unmarshal(entry.Value, &s) == nil {
			args = append(args, s)
		} else {
			var list []string
			if err := json.Unmarshal(entry.Value, &list); err != nil {
				return nil, err
			}
			args = append(args, list...)
		}
	}
	for i, s := range args {
		var unknown string
		args[i] = variable.ReplaceAllStringFunc(s, func(token string) string {
			value, ok := values[token[2:len(token)-1]]
			if !ok {
				unknown = token
			}
			return value
		})
		if unknown != "" {
			return nil, i18n.Error("launch.argument_unsupported", unknown)
		}
	}
	return args, nil
}

func clientArgs(m metadata, values map[string]string, opts LaunchOptions, server string) ([]string, error) {
	features := make(map[string]bool)
	if opts.Display == DisplayResolution {
		features["has_custom_resolution"] = true
		values["resolution_width"] = strconv.Itoa(opts.Width)
		values["resolution_height"] = strconv.Itoa(opts.Height)
	}
	if server != "" {
		address, err := ServerAddress(server)
		if err != nil {
			return nil, err
		}
		features["is_quick_play_multiplayer"] = true
		values["quickPlayMultiplayer"] = address
	}
	args, err := gameArgs(m.Arguments.Game, values, features)
	if err != nil {
		return nil, err
	}
	if opts.Display == DisplayFullscreen && !slices.Contains(args, "--fullscreen") {
		args = append(args, "--fullscreen")
	}
	if server != "" && !slices.Contains(args, "--quickPlayMultiplayer") {
		return nil, i18n.Error("launch.quick_play_unsupported")
	}
	return args, nil
}

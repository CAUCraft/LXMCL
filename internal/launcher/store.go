package launcher

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

type Settings struct {
	GameRoot string `json:"game_root"`
	JavaPath string `json:"java_path"`
	Player   string `json:"player"`
	Memory   int    `json:"memory_mib"`
}

type Instance struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Loader   string `json:"loader"`
	GameDir  string `json:"game_directory"`
	Revision uint64 `json:"revision"`
}

type State struct {
	Settings  Settings   `json:"settings"`
	Instances []Instance `json:"instances"`
	Selected  int        `json:"selected"`
	NextID    int        `json:"next_id"`
}

type Store struct{ Root string }

func DefaultStore() (Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return Store{}, err
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		dir = local
	}
	return Store{Root: filepath.Join(dir, "org.caucraft.lxmcl")}, nil
}

func (s Store) Load() (State, error) {
	state := State{
		Settings: Settings{GameRoot: filepath.Join(s.Root, "games"), Player: "Player", Memory: 4096},
		NextID:   1,
	}
	found, err := readJSON(filepath.Join(s.Root, "launcher.json"), &state)
	if found || err != nil {
		return state, err
	}
	// Import the last Rust release on first use. The original files stay in place.
	if _, err = readJSON(filepath.Join(s.Root, "launcher-settings.json"), &state.Settings); err != nil {
		return state, err
	}
	var old struct{ Instances []Instance }
	if _, err = readJSON(filepath.Join(s.Root, "instances.json"), &old); err != nil {
		return state, err
	}
	state.Instances = old.Instances
	for _, i := range state.Instances {
		state.NextID = max(state.NextID, i.ID+1)
	}
	if len(state.Instances) > 0 {
		state.Selected = state.Instances[0].ID
	}
	return state, nil
}

func readJSON(path string, dst any) (bool, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return true, fmt.Errorf("无法读取 %s：%w", path, err)
	}
	return true, nil
}

func (s Store) Save(state State) error {
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(s.Root, "launcher.json"), b)
}

func writeFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".lxmcl-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if err = errors.Join(err, f.Close()); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (s State) Clone() State {
	s.Instances = slices.Clone(s.Instances)
	return s
}

func (s State) Find(id int) (Instance, bool) {
	for _, i := range s.Instances {
		if i.ID == id {
			return i, true
		}
	}
	return Instance{}, false
}

func (s *State) Create(name, version string) error {
	name, err := s.checkName(0, name)
	if err != nil {
		return err
	}
	if !modernRelease(version) {
		return errors.New("请选择 1.20.5 或之后的原版正式版本")
	}
	if !filepath.IsAbs(s.Settings.GameRoot) {
		return errors.New("请先设置绝对游戏目录")
	}
	i := Instance{ID: s.NextID, Name: name, Version: version, Loader: "Vanilla", Revision: 1,
		GameDir: filepath.Join(s.Settings.GameRoot, fmt.Sprintf("instance-%d", s.NextID))}
	s.NextID++
	s.Instances = append(s.Instances, i)
	s.Selected = i.ID
	return nil
}

func (s *State) Rename(id int, revision uint64, name string) error {
	name, err := s.checkName(id, name)
	if err != nil {
		return err
	}
	for n := range s.Instances {
		i := &s.Instances[n]
		if i.ID == id && i.Revision == revision {
			if i.Name != name {
				i.Name = name
				i.Revision++
			}
			return nil
		}
	}
	return errors.New("实例已变更，请重新打开表单")
}

func (s *State) ChangeVersion(id int, revision uint64, version string) error {
	for n := range s.Instances {
		i := &s.Instances[n]
		if i.ID != id || i.Revision != revision {
			continue
		}
		next := *i
		next.Version = version
		if err := supported(next); err != nil {
			return err
		}
		if i.Version != version {
			i.Version = version
			i.Revision++
		}
		return nil
	}
	return errors.New("实例已变更，请重新打开表单")
}

func (s State) checkName(id int, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 48 {
		return "", errors.New("实例名称须为 1–48 个字符")
	}
	for _, i := range s.Instances {
		if i.ID != id && strings.EqualFold(i.Name, name) {
			return "", errors.New("已有同名实例")
		}
	}
	return name, nil
}

func (s *State) Remove(id int) {
	s.Instances = slices.DeleteFunc(s.Instances, func(i Instance) bool { return i.ID == id })
	if s.Selected == id {
		s.Selected = 0
		if len(s.Instances) != 0 {
			s.Selected = s.Instances[0].ID
		}
	}
}

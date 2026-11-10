package instance

type Settings struct {
	GameRoot string `json:"game_root"`
	JavaPath string `json:"java_path"`
	Player   string `json:"player"`
	Memory   int    `json:"memory_mib"`
}

type Instance struct {
	ID            int           `json:"id"`
	Name          string        `json:"name"`
	Version       string        `json:"version"`
	Loader        string        `json:"loader"`
	LoaderVersion string        `json:"loader_version,omitempty"`
	GameDir       string        `json:"game_directory"`
	Revision      uint64        `json:"revision"`
	Launch        LaunchOptions `json:"launch,omitempty,omitzero"`
}

package saves

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorldMetadata(t *testing.T) {
	game := t.TempDir()
	played := time.Date(2024, 3, 12, 8, 9, 10, 123000000, time.UTC)
	mtime := played.Add(24 * time.Hour)
	type writeField func(*bytes.Buffer, func(string))
	fixture := func(name string, millis int64, old bool, version writeField) []byte {
		var data bytes.Buffer
		writeString := func(s string) {
			data.Write(binary.BigEndian.AppendUint16(nil, uint16(len(s))))
			data.WriteString(s)
		}
		data.WriteByte(10)
		writeString("")
		data.WriteByte(10)
		writeString("Data")
		if !old {
			data.WriteByte(3)
			writeString("DataVersion")
			data.Write(binary.BigEndian.AppendUint32(nil, 3955))
		}
		if version != nil {
			version(&data, writeString)
		}
		if name != "" {
			data.WriteByte(8)
			writeString("LevelName")
			writeString(name)
		}
		data.WriteByte(4)
		writeString("LastPlayed")
		data.Write(binary.BigEndian.AppendUint64(nil, uint64(millis)))
		data.WriteByte(10)
		writeString("Player")
		data.WriteByte(8)
		writeString("LevelName")
		writeString("Not the world name")
		data.WriteByte(10)
		writeString("Version")
		data.WriteByte(8)
		writeString("Name")
		writeString("Not the world version")
		data.Write([]byte{0, 0, 0, 0})
		return data.Bytes()
	}
	version := func(tag byte, value string) writeField {
		return func(data *bytes.Buffer, writeString func(string)) {
			data.WriteByte(10)
			writeString("Version")
			data.WriteByte(3)
			writeString("Id")
			data.Write(binary.BigEndian.AppendUint32(nil, 3955))
			if tag != 0 {
				data.WriteByte(tag)
				writeString("Name")
				if tag == 8 {
					writeString(value)
				} else {
					data.Write(binary.BigEndian.AppendUint32(nil, 3955))
				}
			}
			data.WriteByte(0)
		}
	}
	wrongVersionType := func(data *bytes.Buffer, writeString func(string)) {
		data.WriteByte(8)
		writeString("Version")
		writeString("1.21.1")
	}
	truncated := fixture("半份数据", played.UnixMilli(), false, version(8, "1.21.8"))
	tests := []struct {
		folder  string
		nbt     []byte
		gzip    bool
		name    string
		time    time.Time
		version string
	}{
		{"A-旧存档", fixture("河谷\xed\xa0\xbc\xed\xbf\xb0", played.UnixMilli(), true, nil), true, "河谷🏰", played, ""},
		{"B-仅时间", fixture("", played.UnixMilli(), false, nil), true, "", played, ""},
		{"C-坏名称", fixture("\xff", played.UnixMilli(), false, nil), true, "", played, ""},
		{"D-仅名称", fixture("山谷", 0, false, nil), true, "山谷", time.Time{}, ""},
		{"E-坏时间", fixture("河谷", maxLastPlayedMillis+1, false, nil), true, "河谷", time.Time{}, ""},
		{"F-普通文件", []byte("unreadable metadata"), false, "", time.Time{}, ""},
		{"G-未完成", truncated[:len(truncated)-1], true, "", time.Time{}, ""},
		{"H-快照", fixture("河谷", played.UnixMilli(), false, version(8, "25w05a")), true, "河谷", played, "25w05a"},
		{"I-字段类型错误", fixture("河谷", played.UnixMilli(), false, version(3, "")), true, "河谷", played, ""},
		{"J-节点类型错误", fixture("河谷", played.UnixMilli(), false, wrongVersionType), true, "河谷", played, ""},
		{"K-字段编码错误", fixture("河谷", played.UnixMilli(), false, version(8, "\xff")), true, "河谷", played, ""},
		{"L-只有版本编号", fixture("河谷", played.UnixMilli(), false, version(0, "")), true, "河谷", played, ""},
		{"M-保留原文", fixture("河谷", played.UnixMilli(), false, version(8, " 1.21.8 ")), true, "河谷", played, " 1.21.8 "},
		{"N-空白版本名", fixture("河谷", played.UnixMilli(), false, version(8, "  ")), true, "河谷", played, ""},
	}
	for _, tc := range tests {
		folder := filepath.Join(game, "saves", tc.folder)
		if err := os.MkdirAll(folder, 0700); err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if tc.gzip {
			zipped := gzip.NewWriter(&buf)
			if _, err := zipped.Write(tc.nbt); err != nil {
				t.Fatal(err)
			}
			if err := zipped.Close(); err != nil {
				t.Fatal(err)
			}
		} else {
			buf.Write(tc.nbt)
		}
		level := filepath.Join(folder, "level.dat")
		if err := os.WriteFile(level, buf.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(level, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	worlds, err := Worlds(t.Context(), game)
	if err != nil || len(worlds) != len(tests) {
		t.Fatal(worlds, err)
	}
	for i, world := range worlds {
		tc := tests[i]
		if world.Folder() != tc.folder || world.LevelName() != tc.name || !world.LastPlayed().Equal(tc.time) {
			t.Errorf("%s: name %q, LastPlayed %v", world.Folder(), world.LevelName(), world.LastPlayed())
		}
		if world.SavedVersion() != tc.version {
			t.Errorf("%s: saved version %q, want %q", world.Folder(), world.SavedVersion(), tc.version)
		}
		if err := world.Check(game); err != nil {
			t.Error(world.Folder(), err)
		}
		if !world.level.ModTime().Equal(mtime) {
			t.Errorf("%s: file modification time %v", world.Folder(), world.level.ModTime())
		}
	}
}

func TestWorldGameplaySettings(t *testing.T) {
	field := func(tag byte, key string, payload []byte) []byte {
		data := binary.BigEndian.AppendUint16([]byte{tag}, uint16(len(key)))
		data = append(data, key...)
		return append(data, payload...)
	}
	mode := func(id uint32) []byte { return field(3, "GameType", binary.BigEndian.AppendUint32(nil, id)) }
	hardcore := func(value byte) []byte { return field(1, "hardcore", []byte{value}) }
	legacy := func(id byte) []byte { return field(1, "Difficulty", []byte{id}) }
	modern := func(id string) []byte {
		return field(8, "difficulty", append(binary.BigEndian.AppendUint16(nil, uint16(len(id))), id...))
	}
	settings := func(fields ...[]byte) []byte {
		return field(10, "difficulty_settings", append(bytes.Join(fields, nil), 0))
	}
	tests := []struct {
		name       string
		fields     [][]byte
		mode       GameMode
		hardcore   worldHardcore
		difficulty Difficulty
	}{
		{"a-survival", [][]byte{mode(0), hardcore(0), legacy(0)}, GameModeSurvival, hardcoreOff, DifficultyPeaceful},
		{"b-creative", [][]byte{mode(1), hardcore(1), legacy(1)}, GameModeCreative, hardcoreOn, DifficultyEasy},
		{"c-adventure", [][]byte{mode(2), legacy(2)}, GameModeAdventure, hardcoreUnknown, DifficultyNormal},
		{"d-spectator", [][]byte{mode(3), settings(hardcore(0), modern("normal")), hardcore(1), legacy(3)}, GameModeSpectator, hardcoreOff, DifficultyNormal},
		{"e-unknown-mode", [][]byte{mode(6), settings(hardcore(1), modern("hard"))}, GameModeUnknown, hardcoreOn, DifficultyHard},
		{"f-unknown-flag", [][]byte{mode(0), hardcore(2), legacy(255)}, GameModeSurvival, hardcoreUnknown, DifficultyUnknown},
		{"g-wrong-types", [][]byte{field(1, "GameType", []byte{1}), field(3, "hardcore", []byte{0, 0, 0, 1})}, GameModeUnknown, hardcoreUnknown, DifficultyUnknown},
		{"h-empty-settings", [][]byte{hardcore(1), legacy(0), settings()}, GameModeUnknown, hardcoreUnknown, DifficultyUnknown},
		{"i-player-only", nil, GameModeUnknown, hardcoreUnknown, DifficultyUnknown},
		{"j-modern-peaceful", [][]byte{settings(modern("peaceful"))}, GameModeUnknown, hardcoreUnknown, DifficultyPeaceful},
		{"k-modern-easy", [][]byte{settings(modern("easy"))}, GameModeUnknown, hardcoreUnknown, DifficultyEasy},
		{"l-modern-unrecognized", [][]byte{legacy(2), settings(hardcore(1), modern("Hard"))}, GameModeUnknown, hardcoreOn, DifficultyUnknown},
		{"m-modern-wrong-type", [][]byte{settings(hardcore(1), field(1, "difficulty", []byte{0}))}, GameModeUnknown, hardcoreOn, DifficultyUnknown},
		{"n-invalid-group", [][]byte{legacy(3), field(1, "difficulty_settings", []byte{1})}, GameModeUnknown, hardcoreUnknown, DifficultyUnknown},
		{"o-legacy-hard", [][]byte{legacy(3)}, GameModeUnknown, hardcoreUnknown, DifficultyHard},
		{"p-legacy-out-of-range", [][]byte{legacy(4)}, GameModeUnknown, hardcoreUnknown, DifficultyUnknown},
		{"q-legacy-wrong-type", [][]byte{field(3, "Difficulty", []byte{0, 0, 0, 0})}, GameModeUnknown, hardcoreUnknown, DifficultyUnknown},
	}
	game := t.TempDir()
	for _, tc := range tests {
		var fields []byte
		for _, value := range tc.fields {
			fields = append(fields, value...)
		}
		// Player game mode is separate from the world's default mode.
		player := field(3, "playerGameType", []byte{0, 0, 0, 1})
		fields = append(fields, field(10, "Player", append(append(player, legacy(3)...), 0))...)
		data := field(10, "", append(field(10, "Data", append(fields, 0)), 0))
		dir := filepath.Join(game, "saves", tc.name)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		var zipped bytes.Buffer
		writer := gzip.NewWriter(&zipped)
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "level.dat"), zipped.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	worlds, err := Worlds(t.Context(), game)
	if err != nil || len(worlds) != len(tests) {
		t.Fatal(worlds, err)
	}
	for i, world := range worlds {
		want := tests[i]
		enabled, known := world.Hardcore()
		if world.Folder() != want.name || world.DefaultGameMode() != want.mode ||
			enabled != (want.hardcore == hardcoreOn) || known != (want.hardcore != hardcoreUnknown) {
			t.Errorf("%s: mode %v, hardcore (enabled %v, known %v)", world.Folder(), world.DefaultGameMode(), enabled, known)
		}
		if got := world.Difficulty(); got != want.difficulty {
			t.Errorf("%s: difficulty %v, want %v", world.Folder(), got, want.difficulty)
		}
	}
}

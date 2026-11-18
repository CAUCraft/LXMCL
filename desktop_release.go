package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
)

//go:embed mygo.json
var packageMetadata []byte

var launcherVersion = func() string {
	var m struct{ Version string }
	if err := json.Unmarshal(packageMetadata, &m); err != nil {
		panic(err)
	}
	return m.Version
}()

type releaseView struct {
	checking            bool
	message, tag, notes string
}

func releaseParts(v string) ([]uint64, error) {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("无法识别版本号 %q", v)
	}
	numbers := make([]uint64, 3)
	for n, p := range parts {
		u, err := strconv.ParseUint(p, 10, 64)
		if err != nil || strconv.FormatUint(u, 10) != p {
			return nil, fmt.Errorf("无法识别版本号 %q", v)
		}
		numbers[n] = u
	}
	return numbers, nil
}
func latestRelease(ctx context.Context, client *http.Client) (releaseView, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/CAUCraft/LXMCL/releases/latest", nil)
	if err != nil {
		return releaseView{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "CAUCraft/LXMCL")
	res, err := client.Do(req)
	if err != nil {
		return releaseView{}, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return releaseView{message: "暂未发布正式版"}, nil
	}
	if res.StatusCode != http.StatusOK {
		return releaseView{}, fmt.Errorf("GitHub 返回 HTTP %d", res.StatusCode)
	}
	var release struct {
		Tag               string `json:"tag_name"`
		Body              string
		Draft, Prerelease bool
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil {
		return releaseView{}, err
	}
	if len(b) > 1<<20 {
		return releaseView{}, fmt.Errorf("发布说明过长")
	}
	if err = json.Unmarshal(b, &release); err != nil {
		return releaseView{}, err
	}
	version, ok := strings.CutPrefix(release.Tag, "v")
	if !ok || release.Draft || release.Prerelease {
		return releaseView{}, fmt.Errorf("无法识别正式版标签")
	}
	published, err := releaseParts(version)
	if err != nil {
		return releaseView{}, err
	}
	local, err := releaseParts(launcherVersion)
	if err != nil {
		return releaseView{}, err
	}
	if slices.Compare(published, local) <= 0 {
		return releaseView{message: "当前已是最新正式版或更新的开发版本"}, nil
	}
	return releaseView{message: "可更新至 " + release.Tag, tag: release.Tag, notes: strings.TrimSpace(release.Body)}, nil
}
func (a *desktop) releaseControls(c *ui.Context) {
	settingsRow(c, "启动器版本", func() {
		muted(c, "v"+launcherVersion).Grow(1)
		if button(c, "检查更新", action.Default, a.release.checking || a.ctx == nil) {
			a.release = releaseView{checking: true, message: "正在查询 GitHub Releases…"}
			go func() {
				ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
				defer cancel()
				r, err := latestRelease(ctx, http.DefaultClient)
				a.post(func() {
					if err != nil {
						r.message = "检查失败：" + err.Error()
					}
					a.release = r
				})
			}()
		}
	})
	if a.release.message != "" {
		muted(c, a.release.message).FontSize(13)
	}
	if a.release.tag != "" {
		if a.release.notes != "" {
			display.Text(c, a.release.notes, display.TextProps{Rows: 3, FontSize: 12}).Selectable()
		}
		if button(c, "查看发布说明并下载", action.Primary, false) {
			c.OpenURL("https://github.com/CAUCraft/LXMCL/releases/tag/" + a.release.tag)
		}
	}
}

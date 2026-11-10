package launcher

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

const (
	neoCDN  = "https://neoforged.forgecdn.net/releases/"
	neoBMCL = "https://bmclapi2.bangbang93.com/maven/"
	neoQLU  = "https://mirrors.qlu.edu.cn/bmclapi/"
)

var neoTransport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	t.TLSHandshakeTimeout = 5 * time.Second
	t.ResponseHeaderTimeout = 8 * time.Second
	return t
}()

func neoMavenPath(src string) (string, bool) {
	path, ok := strings.CutPrefix(src, neoMaven)
	if !ok {
		path, ok = strings.CutPrefix(src, neoCDN)
	}
	return path, ok && safeModpackPath(path) && !strings.ContainsAny(path, "%?#")
}

func neoRouteURL(u *url.URL, path string) bool {
	if u == nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawQuery != "" ||
		u.Fragment != "" || u.RawPath != "" {
		return false
	}
	switch u.Hostname() {
	case "maven.neoforged.net", "neoforged.forgecdn.net":
		return u.Path == "/releases/"+path
	case "bmclapi2.bangbang93.com":
		return u.Path == "/maven/"+path
	case "mirrors.cernet.edu.cn", "mirrors.qlu.edu.cn":
		return u.Path == "/bmclapi/"+path
	default:
		return false
	}
}

// Only repository assets for one exact Maven path can follow this route.
// In particular, the checksum sidecar never comes from a general file mirror.
func (d *downloads) fetch(ctx context.Context, src string, consume func(context.Context, *http.Response) error) error {
	path, ok := neoMavenPath(src)
	if !ok {
		return retry(ctx, func() error {
			resp, err := d.get(ctx, src)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			return consume(ctx, resp)
		})
	}
	checksum := strings.HasSuffix(path, ".sha1")
	client := *d.http
	if d.http == officialHTTP {
		client.Transport = neoTransport
	}
	allowed := func(u *url.URL) bool {
		return neoRouteURL(u, path) && (!checksum ||
			(u.Hostname() == "maven.neoforged.net" || u.Hostname() == "neoforged.forgecdn.net"))
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !allowed(req.URL) {
			return errors.New("NeoForge 下载重定向超出允许的镜像")
		}
		return nil
	}
	routes := []string{neoMaven, neoCDN}
	if !checksum {
		routes = append(routes, neoBMCL, neoQLU)
	}
	d.mu.Lock()
	failed := d.neoOriginDown
	d.mu.Unlock()
	if failed {
		// Once the origin fails, later libraries do not each wait for it first.
		routes = append(slices.Clone(routes[1:]), routes[0])
	}
	var failures []error
	for _, base := range routes {
		if err := ctx.Err(); err != nil {
			return err
		}
		timeout := 90 * time.Second
		if checksum {
			timeout = 9 * time.Second
		} else if base == neoMaven {
			timeout = 30 * time.Second
		}
		attempt, stop := context.WithTimeout(ctx, timeout)
		resp, err := d.getFrom(attempt, base+path, &client, allowed)
		if err == nil {
			err = consume(attempt, resp)
			resp.Body.Close()
		}
		stop()
		if err == nil {
			if base != neoMaven || (resp.Request != nil && resp.Request.URL.Hostname() != "maven.neoforged.net") {
				d.mu.Lock()
				d.neoMirrorUsed = true
				d.mu.Unlock()
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var status *responseError
		var netErr net.Error
		if base == neoMaven && (errors.As(err, &netErr) || errors.Is(err, context.DeadlineExceeded) ||
			(errors.As(err, &status) && (status.status == 429 || status.status >= 500))) {
			d.mu.Lock()
			d.neoOriginDown = true
			d.mu.Unlock()
		}
		failures = append(failures, fmt.Errorf("%s：%w", strings.TrimSuffix(base, "/"), err))
	}
	return fmt.Errorf("NeoForge 官方仓库和镜像均未提供可校验的文件：%w", errors.Join(failures...))
}

func (d *downloads) mirrorStage(stage string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.neoMirrorUsed {
		return stage + " · 镜像"
	}
	return stage
}

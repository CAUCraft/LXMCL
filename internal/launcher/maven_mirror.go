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
	neoCDN      = "https://neoforged.forgecdn.net/releases/"
	njuMaven    = "https://mirror.nju.edu.cn/bmclapi/"
	bmclMaven   = "https://bmclapi2.bangbang93.com/maven/"
	qluMaven    = "https://mirrors.qlu.edu.cn/bmclapi/"
	fabricMaven = "https://maven.fabricmc.net/"
)

var mavenTransport = func() *http.Transport {
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

func mavenRouteURL(u *url.URL, path, origin string, neo bool) bool {
	if u == nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawQuery != "" ||
		u.Fragment != "" || u.RawPath != "" {
		return false
	}
	if u.String() == origin+path {
		return true
	}
	switch u.Hostname() {
	case "maven.neoforged.net", "neoforged.forgecdn.net":
		return neo && u.Path == "/releases/"+path
	case "bmclapi2.bangbang93.com":
		return u.Path == "/maven/"+path
	case "mirror.nju.edu.cn", "mirrors.cernet.edu.cn", "mirrors.qlu.edu.cn":
		return u.Path == "/bmclapi/"+path
	default:
		return false
	}
}

// Only repository assets for one exact Maven path can follow this route.
// In particular, the checksum sidecar never comes from a general file mirror.
func (d *downloads) fetch(ctx context.Context, src string, consume func(context.Context, *http.Response) error) error {
	path, neo := neoMavenPath(src)
	origin := neoMaven
	if !neo {
		origin = ""
		for _, base := range []string{fabricMaven, forgeMaven} {
			name, ok := strings.CutPrefix(src, base)
			artifactPath := strings.TrimSuffix(name, ".sha1")
			if ok && safeModpackPath(name) && !strings.ContainsAny(name, "%?#") &&
				(strings.HasSuffix(artifactPath, ".jar") || strings.HasSuffix(artifactPath, ".zip")) {
				path, origin = name, base
				break
			}
		}
	}
	if origin == "" {
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
		client.Transport = mavenTransport
	}
	allowed := func(u *url.URL) bool {
		if !mavenRouteURL(u, path, origin, neo) {
			return false
		}
		return !checksum || u.String() == origin+path || (neo && u.String() == neoCDN+path)
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !allowed(req.URL) {
			return errors.New("Maven 下载重定向超出允许的镜像")
		}
		return nil
	}
	routes := []string{origin}
	if neo {
		routes = append(routes, neoCDN)
	}
	if !checksum {
		if !neo {
			routes = append(routes, njuMaven)
		}
		routes = append(routes, bmclMaven, qluMaven)
	}
	d.mu.Lock()
	failed := d.mavenOriginDown[origin]
	d.mu.Unlock()
	if failed {
		// Once the origin fails, later libraries do not each wait for it first.
		routes = append(slices.Clone(routes[1:]), routes[0])
	}
	var failures []error
	firstPass := len(routes)
	for index := 0; index < len(routes); index++ {
		base := routes[index]
		if err := ctx.Err(); err != nil {
			return err
		}
		timeout := 90 * time.Second
		if checksum {
			timeout = 9 * time.Second
		} else if base == origin {
			timeout = 20 * time.Second
		}
		attempt, stop := context.WithTimeout(ctx, timeout)
		resp, err := d.getFrom(attempt, base+path, &client, allowed)
		if err == nil {
			err = consume(attempt, resp)
			resp.Body.Close()
		}
		stop()
		if err == nil {
			if base != origin || (resp.Request != nil && resp.Request.URL.String() != origin+path) {
				d.mu.Lock()
				d.mavenMirrorUsed = true
				d.mu.Unlock()
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var status *responseError
		var netErr net.Error
		if base == origin && (errors.As(err, &netErr) || errors.Is(err, context.DeadlineExceeded) ||
			(errors.As(err, &status) && (status.status == 429 || status.status >= 500))) {
			d.mu.Lock()
			if d.mavenOriginDown == nil {
				d.mavenOriginDown = make(map[string]bool)
			}
			d.mavenOriginDown[origin] = true
			d.mu.Unlock()
		}
		failures = append(failures, fmt.Errorf("%s：%w", strings.TrimSuffix(base, "/"), err))
		// Try the next trusted checksum source before retrying a temporary failure.
		if checksum && index < firstPass && retryable(err) {
			routes = append(routes, base)
		}
	}
	if checksum {
		return fmt.Errorf("无法从可信来源读取 Maven 校验值：%w", errors.Join(failures...))
	}
	return fmt.Errorf("Maven 仓库和镜像均未提供可校验的文件：%w", errors.Join(failures...))
}

func (d *downloads) mirrorStage(stage string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.mavenMirrorUsed {
		return stage + " · 镜像"
	}
	return stage
}

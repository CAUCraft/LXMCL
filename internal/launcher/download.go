package launcher

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxMetadata = 8 << 20
	// Verified metadata and small libraries may omit the expected size.
	unknownSize = -1
)

var officialHTTP = &http.Client{
	Timeout: 2 * time.Minute,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !officialURL(req.URL) {
			return errors.New("下载重定向超出官方来源")
		}
		return nil
	},
}

type downloads struct {
	http     *http.Client
	mu       sync.Mutex
	cooldown map[string]time.Time
}

func officialURL(u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && u.Port() == "" &&
		slices.Contains([]string{"piston-meta.mojang.com", "piston-data.mojang.com",
			"libraries.minecraft.net", "resources.download.minecraft.net", "launcher.mojang.com",
			"launchermeta.mojang.com", "meta.fabricmc.net", "maven.fabricmc.net",
			"api.modrinth.com", "cdn.modrinth.com"}, u.Hostname())
}

type responseError struct {
	status int
	src    string
	delay  time.Duration
}

func (e *responseError) Error() string {
	if e.status == http.StatusTooManyRequests {
		return "官方下载服务繁忙（HTTP 429），请稍后重试"
	}
	return fmt.Sprintf("下载返回 HTTP %d：%s", e.status, e.src)
}

func retryAfter(value string, now time.Time) time.Duration {
	if n, err := strconv.Atoi(value); err == nil && n >= 0 {
		// Any value beyond 30s is enough to defer to an explicit user retry.
		return time.Duration(min(n, 31)) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(0, date.Sub(now))
	}
	return time.Second
}

func (d *downloads) get(ctx context.Context, src string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, err
	}
	if !officialURL(req.URL) {
		return nil, fmt.Errorf("下载来源不受支持：%s", src)
	}
	req.Header.Set("User-Agent", "CAUCraft/LXMCL (https://github.com/CAUCraft/LXMCL)")
	host := req.URL.Hostname()
	for {
		d.mu.Lock()
		delay := time.Until(d.cooldown[host])
		d.mu.Unlock()
		if delay <= 0 {
			break
		}
		if err := waitContext(ctx, delay); err != nil {
			return nil, err
		}
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}
	resp.Body.Close()
	failure := &responseError{status: resp.StatusCode, src: src}
	if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode == http.StatusServiceUnavailable && resp.Header.Get("Retry-After") != "") {
		now := time.Now()
		failure.delay = retryAfter(resp.Header.Get("Retry-After"), now)
		if failure.delay <= 30*time.Second {
			until := now.Add(failure.delay)
			d.mu.Lock()
			if until.After(d.cooldown[host]) {
				if d.cooldown == nil {
					d.cooldown = make(map[string]time.Time)
				}
				d.cooldown[host] = until
			}
			d.mu.Unlock()
		}
	}
	return nil, failure
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func retryable(err error) bool {
	var response *responseError
	if errors.As(err, &response) {
		return response.delay <= 30*time.Second && (response.status == http.StatusRequestTimeout ||
			response.status == http.StatusTooManyRequests || response.status >= 500 && response.status <= 599)
	}
	var operation *net.OpError
	var timeout net.Error
	return errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, errDownloadMismatch) ||
		errors.As(err, &operation) || (errors.As(err, &timeout) && timeout.Timeout())
}

func retry(ctx context.Context, fn func() error) error {
	var err error
	for attempt := range 3 {
		if err := ctx.Err(); err != nil {
			return err
		}
		err = fn()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil || attempt == 2 || !retryable(err) {
			return err
		}
		if err := waitContext(ctx, time.Duration(1<<attempt)*150*time.Millisecond); err != nil {
			return err
		}
	}
	return err
}

func (a artifact) checksum() ([]byte, error) {
	sum, err := hex.DecodeString(a.SHA)
	if err != nil || len(sum) != sha1.Size {
		return nil, errors.New("缺少有效 SHA-1")
	}
	if a.Size < unknownSize || a.Size == math.MaxInt64 {
		return nil, errors.New("文件大小无效")
	}
	return sum, nil
}

type cancelReader struct {
	ctx context.Context
	r   io.Reader
}

func (r cancelReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func checkFile(ctx context.Context, file string, a artifact, digest []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	st, err := os.Stat(file)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !st.Mode().IsRegular() {
		return false, fmt.Errorf("%s 不是普通文件", file)
	}
	if a.Size == unknownSize && st.Size() > maxMetadata || a.Size != unknownSize && a.Size != st.Size() {
		return false, nil
	}
	f, err := os.Open(file)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha1.New()
	n, err := io.Copy(h, cancelReader{ctx, f})
	return err == nil && (a.Size == unknownSize && n <= maxMetadata || a.Size == n) && bytes.Equal(h.Sum(nil), digest), err
}

var errDownloadMismatch = errors.New("下载文件校验失败")

func (d *downloads) file(ctx context.Context, file string, a artifact) error {
	digest, err := a.checksum()
	if err != nil {
		return fmt.Errorf("%s：%w", filepath.Base(file), err)
	}
	ok, err := checkFile(ctx, file, a, digest)
	if ok || err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		return err
	}
	return retry(ctx, func() error {
		resp, err := d.get(ctx, a.URL)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		f, err := os.CreateTemp(filepath.Dir(file), ".download-*")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		h := sha1.New()
		limit := a.Size + 1
		if a.Size == unknownSize {
			limit = maxMetadata + 1
		}
		n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(cancelReader{ctx, resp.Body}, limit))
		if err = errors.Join(err, f.Close()); err != nil {
			return err
		}
		if n >= limit || (a.Size != unknownSize && n != a.Size) || !bytes.Equal(h.Sum(nil), digest) {
			return fmt.Errorf("%s：%w", filepath.Base(file), errDownloadMismatch)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return os.Rename(f.Name(), file)
	})
}

type fileDownload struct {
	path string
	file artifact
}

func batch(ctx context.Context, files []fileDownload, work func(context.Context, fileDownload) error, progress func(int)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	var next atomic.Int32
	var first error
	var once sync.Once
	var progressMu sync.Mutex
	done := 0
	for range min(12, len(files)) {
		wg.Go(func() {
			for ctx.Err() == nil {
				index := int(next.Add(1)) - 1
				if index >= len(files) {
					return
				}
				if err := work(ctx, files[index]); err != nil {
					once.Do(func() { first = err; cancel() })
					return
				}
				if progress != nil {
					progressMu.Lock()
					done++
					progress(done)
					progressMu.Unlock()
				}
			}
		})
	}
	wg.Wait()
	if first != nil {
		return first
	}
	return ctx.Err()
}

func (d *downloads) batch(ctx context.Context, stage string, files []fileDownload, report func(Progress)) error {
	report(Progress{Stage: stage, Total: len(files)})
	return batch(ctx, files, func(ctx context.Context, file fileDownload) error {
		return d.file(ctx, file.path, file.file)
	}, func(done int) {
		report(Progress{Stage: stage, Done: done, Total: len(files)})
	})
}

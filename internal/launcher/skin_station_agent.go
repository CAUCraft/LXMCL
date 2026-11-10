package launcher

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

// Pinned to the official v1.2.8 release asset, not the provider's download.
const (
	stationAgentName = "authlib-injector-1.2.8.jar"
	stationAgentURL  = "https://github.com/yushijinhun/authlib-injector/releases/download/v1.2.8/" + stationAgentName
	stationAgentSHA  = "9c7f4343e6c82034958ffb48c14a2cb0c85928be7283103ce17da00c6d5a7b10"
	stationAgentSize = 349681
)

var agentHTTP = &http.Client{
	Timeout: 60 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 || req.URL.Scheme != "https" || req.URL.User != nil || req.URL.Port() != "" ||
			(req.URL.Hostname() != "release-assets.githubusercontent.com" && req.URL.Hostname() != "objects.githubusercontent.com") {
			return i18n.Error("station.agent_redirect_denied")
		}
		return nil
	},
}

func verifiedAgent(data []byte) bool {
	return len(data) == stationAgentSize && fmt.Sprintf("%x", sha256.Sum256(data)) == stationAgentSHA
}

func stationAgent(ctx context.Context, cache string, client *http.Client) (string, error) {
	path, err := filepath.Abs(filepath.Join(cache, "authlib-injector", stationAgentName))
	if err != nil {
		return "", err
	}
	if strings.ContainsRune(path, '=') {
		return "", i18n.Error("station.agent_cache_path_invalid")
	}
	if file, err := os.Open(path); err == nil {
		data, readErr := io.ReadAll(io.LimitReader(file, stationAgentSize+1))
		file.Close()
		if readErr == nil && verifiedAgent(data) {
			return path, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stationAgentURL, nil)
	if err != nil {
		return "", err
	}
	res, err := client.Do(req)
	if err != nil {
		return "", i18n.Error("station.agent_download_failed", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", i18n.Error("station.agent_http_status", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, stationAgentSize+1))
	if err != nil {
		return "", err
	}
	if !verifiedAgent(data) {
		return "", i18n.Error("station.agent_checksum_failed")
	}
	return path, storage.WriteFile(path, data)
}

func (s StationSession) launchArgs(ctx context.Context, cache string) ([]string, error) {
	root, err := stationRoot(s.Profile.API)
	if err != nil {
		return nil, err
	}
	_, _, metadata, err := (authClient{authHTTP}).stationMetadata(ctx, root, false)
	if err != nil {
		return nil, err
	}
	path, err := stationAgent(ctx, cache, agentHTTP)
	if err != nil {
		return nil, err
	}
	args := []string{"-javaagent:" + path + "=" + root.String()}
	// Keep unusually large provider metadata off the Windows command line.
	if len(metadata) <= 4096 {
		args = append(args, "-Dauthlibinjector.yggdrasil.prefetched="+base64.StdEncoding.EncodeToString(metadata))
	}
	return args, nil
}

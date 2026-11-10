package account

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

type StationTextures struct {
	Skin, Cape image.Image
	Slim       bool
}

func (p StationProfile) Textures(ctx context.Context) (StationTextures, error) {
	return (Client{}).StationTextures(ctx, p)
}

func (c Client) StationTextures(ctx context.Context, p StationProfile) (StationTextures, error) {
	root, err := stationRoot(p.API)
	if err != nil || !validStationRole(p.StationRole) {
		return StationTextures{}, i18n.Error("station.role_invalid")
	}
	_, meta, _, err := c.stationMetadata(ctx, root, false)
	if err != nil {
		return StationTextures{}, err
	}
	var profile struct {
		ID         string
		Properties []struct{ Name, Value string }
	}
	if err := c.request(ctx, i18n.Text("station.role_service"), root.String()+"sessionserver/session/minecraft/profile/"+p.ID,
		http.MethodGet, "", nil, "", &profile); err != nil {
		return StationTextures{}, err
	}
	if !strings.EqualFold(profile.ID, p.ID) {
		return StationTextures{}, i18n.Error("station.role_mismatch")
	}
	for _, prop := range profile.Properties {
		if prop.Name != "textures" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(prop.Value)
		var payload struct {
			ProfileID string
			Textures  map[string]struct {
				URL      string
				Metadata map[string]string
			}
		}
		if err != nil || json.Unmarshal(raw, &payload) != nil ||
			(payload.ProfileID != "" && !strings.EqualFold(payload.ProfileID, p.ID)) {
			return StationTextures{}, i18n.Error("station.texture_data_invalid")
		}
		skin, cape := payload.Textures["SKIN"], payload.Textures["CAPE"]
		result := StationTextures{Slim: skin.Metadata["model"] == "slim"}
		result.Skin, err = c.stationTexture(ctx, root, meta.SkinDomains, skin.URL, true)
		var capeErr error
		result.Cape, capeErr = c.stationTexture(ctx, root, meta.SkinDomains, cape.URL, false)
		return result, errors.Join(err, capeErr)
	}
	return StationTextures{}, nil
}

func (c Client) stationTexture(ctx context.Context, root *url.URL, domains []string, raw string, skin bool) (image.Image, error) {
	if raw == "" {
		return nil, nil
	}
	u, err := stationURL(raw)
	if err != nil {
		return nil, i18n.Error("station.texture_https_required")
	}
	host := strings.ToLower(u.Hostname())
	allowed := strings.EqualFold(u.Host, root.Host)
	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimPrefix(domain, "."))
		if u.Port() == "" && domain != "" && (host == domain || strings.HasSuffix(host, "."+domain)) {
			allowed = true
		}
	}
	if !allowed {
		return nil, i18n.Error("station.texture_domain_invalid")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	res, err := c.httpClient().Do(req)
	if err != nil {
		return nil, i18n.Error("station.texture_download_failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, i18n.Error("station.texture_http_status", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxTextureBytes+1))
	if err != nil {
		return nil, err
	}
	return decodeTexture(data, skin)
}

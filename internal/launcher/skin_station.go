package launcher

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
)

var ErrSkinStationLogin = errors.New("请重新登录皮肤站")

type StationRole struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type StationProfile struct {
	Key  string `json:"key"`
	API  string `json:"api"`
	Site string `json:"site"`
	StationRole
}

func (p StationProfile) Homepage() string {
	root, rootErr := stationRoot(p.API)
	site, siteErr := stationURL(p.Site)
	if rootErr != nil || siteErr != nil || !strings.EqualFold(root.Host, site.Host) {
		return ""
	}
	return site.String()
}

type StationSession struct {
	Profile              StationProfile
	access, client, user string
}

type stationAuth struct {
	AccessToken       string
	ClientToken       string
	SelectedProfile   StationRole
	AvailableProfiles []StationRole
	User              struct{ ID string }
}

type stationMetadata struct {
	Meta struct {
		Links struct{ Homepage string }
	}
	SkinDomains []string
}

type StationLogin struct {
	api, site, client, loginKey string
	auth                        stationAuth
}

func stationURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.Fragment != "" || u.Opaque != "" {
		return nil, errors.New("请输入有效的 HTTPS 皮肤站地址")
	}
	return u, nil
}

func stationRoot(raw string) (*url.URL, error) {
	u, err := stationURL(raw)
	if err != nil || u.RawQuery != "" || u.ForceQuery {
		return nil, errors.New("皮肤站 API 地址须为不含查询参数的 HTTPS 地址")
	}
	u.Host = strings.ToLower(u.Host)
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
		u.RawPath = ""
	}
	return u, nil
}

func (c authClient) stationMetadata(ctx context.Context, u *url.URL, discover bool) (*url.URL, stationMetadata, []byte, error) {
	var meta stationMetadata
	for hop := 0; hop < 4; hop++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, meta, nil, err
		}
		req.Header.Set("Accept", "application/json")
		res, err := c.http.Do(req)
		if err != nil {
			return nil, meta, nil, fmt.Errorf("无法连接皮肤站：%w", err)
		}
		if loc := res.Header.Get("X-Authlib-Injector-API-Location"); discover && loc != "" {
			res.Body.Close()
			ref, err := url.Parse(loc)
			if err != nil {
				return nil, meta, nil, errors.New("皮肤站返回了无效的 API 地址")
			}
			next, err := stationRoot(u.ResolveReference(ref).String())
			if err != nil || next.Host != u.Host {
				return nil, meta, nil, errors.New("请直接填写皮肤站提供的 HTTPS Yggdrasil API 地址")
			}
			if next.String() != u.String() {
				u = next
				continue
			}
			// Some deployments advertise the API address even at the API root.
			discover = false
			continue
		}
		if res.StatusCode != http.StatusOK {
			res.Body.Close()
			return nil, meta, nil, fmt.Errorf("皮肤站 API 返回 HTTP %d", res.StatusCode)
		}
		const limit = 64 << 10
		data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
		res.Body.Close()
		if err != nil || len(data) > limit || !bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) ||
			json.Unmarshal(data, &meta) != nil {
			return nil, meta, nil, errors.New("未找到 Yggdrasil API，请填写皮肤站的 API 地址")
		}
		return u, meta, data, nil
	}
	return nil, meta, nil, errors.New("皮肤站 API 地址循环，请填写完整的 API 地址")
}

func BeginStationLogin(ctx context.Context, endpoint, login, password string) (StationLogin, error) {
	return (authClient{authHTTP}).beginStationLogin(ctx, endpoint, login, password)
}

func (c authClient) beginStationLogin(ctx context.Context, endpoint, login, password string) (StationLogin, error) {
	u, err := stationRoot(endpoint)
	if err != nil {
		return StationLogin{}, err
	}
	login = strings.TrimSpace(login)
	if login == "" || password == "" {
		return StationLogin{}, errors.New("请输入皮肤站账号和密码")
	}
	root, meta, _, err := c.stationMetadata(ctx, u, true)
	if err != nil {
		return StationLogin{}, err
	}
	site := (&url.URL{Scheme: root.Scheme, Host: root.Host}).String()
	if home, err := stationURL(meta.Meta.Links.Homepage); err == nil && home.Host == root.Host {
		site = home.String()
	}
	var client [16]byte
	if _, err := rand.Read(client[:]); err != nil {
		return StationLogin{}, err
	}
	p := StationLogin{api: root.String(), site: site, client: hex.EncodeToString(client[:]), loginKey: stationHash(strings.ToLower(login))}
	body := map[string]any{"username": login, "password": password, "clientToken": p.client, "requestUser": true,
		"agent": map[string]any{"name": "Minecraft", "version": 1}}
	if err := c.post(ctx, "皮肤站登录", p.api+"authserver/authenticate", body, &p.auth); err != nil {
		var failure *authResponseError
		if errors.As(err, &failure) && (failure.status == 401 || failure.status == 403) {
			return StationLogin{}, errors.New("皮肤站拒绝登录，请检查账号和密码")
		}
		return StationLogin{}, err
	}
	if !p.auth.valid(p.client) {
		return StationLogin{}, errors.New("皮肤站返回了无效的登录凭据")
	}
	if len(p.Roles()) == 0 {
		return StationLogin{}, errors.New("此账号没有可用角色，请先在皮肤站创建角色")
	}
	return p, nil
}

func (a stationAuth) valid(client string) bool {
	return client != "" && a.ClientToken == client && a.AccessToken != "" && len(a.AccessToken) <= 4096 &&
		!strings.ContainsAny(a.AccessToken, "\r\n\x00")
}

func validStationRole(r StationRole) bool {
	return profileID.MatchString(r.ID) && playerID.MatchString(r.Name)
}

func (p StationLogin) Roles() []StationRole {
	if p.auth.SelectedProfile.ID != "" {
		if validStationRole(p.auth.SelectedProfile) {
			return []StationRole{p.auth.SelectedProfile}
		}
		return nil
	}
	var roles []StationRole
	for _, role := range p.auth.AvailableProfiles {
		if validStationRole(role) && !slices.ContainsFunc(roles, func(r StationRole) bool { return strings.EqualFold(r.ID, role.ID) }) {
			roles = append(roles, role)
		}
	}
	return roles
}

func (p StationLogin) Bind(ctx context.Context, id string) (StationSession, error) {
	return (authClient{authHTTP}).bindStationRole(ctx, p, id)
}

func (c authClient) bindStationRole(ctx context.Context, p StationLogin, id string) (StationSession, error) {
	roles := p.Roles()
	i := slices.IndexFunc(roles, func(r StationRole) bool { return strings.EqualFold(r.ID, id) })
	if i < 0 {
		return StationSession{}, errors.New("请选择皮肤站提供的角色")
	}
	reply := p.auth
	if reply.SelectedProfile.ID == "" {
		var err error
		reply, err = c.refreshStation(ctx, p.api, p.auth.AccessToken, p.client, &roles[i])
		if err != nil {
			return StationSession{}, err
		}
	}
	if !reply.valid(p.client) || !validStationRole(reply.SelectedProfile) || !strings.EqualFold(reply.SelectedProfile.ID, id) ||
		(p.auth.User.ID != "" && reply.User.ID != "" && p.auth.User.ID != reply.User.ID) {
		return StationSession{}, errors.New("皮肤站返回了不同的角色或账号")
	}
	user := p.auth.User.ID
	if user == "" {
		user = reply.User.ID
	}
	userKey := "login:" + p.loginKey
	if user != "" {
		userKey = "id:" + stationHash(user)
	}
	role := reply.SelectedProfile
	role.ID = strings.ToLower(role.ID)
	profile := StationProfile{API: p.api, Site: p.site, StationRole: role, Key: stationKey(p.api, userKey, role.ID)}
	return StationSession{Profile: profile, access: reply.AccessToken, client: p.client, user: userKey}, nil
}

func stationHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func stationKey(api, user, role string) string {
	return stationHash(api + "\x00" + user + "\x00" + role)
}

func (s StationSession) validFor(p StationProfile) bool {
	root, err := stationRoot(p.API)
	return err == nil && root.String() == p.API && validStationRole(p.StationRole) &&
		s.Profile.Key == p.Key && s.Profile.API == p.API && s.Profile.ID == p.ID &&
		p.Key == stationKey(p.API, s.user, p.ID) && s.access != "" && s.client != ""
}

func (c authClient) refreshStation(ctx context.Context, api, token, client string, role *StationRole) (stationAuth, error) {
	var reply stationAuth
	body := map[string]any{"accessToken": token, "clientToken": client, "requestUser": true}
	if role != nil {
		body["selectedProfile"] = role
	}
	err := c.post(ctx, "皮肤站会话", api+"authserver/refresh", body, &reply)
	return reply, err
}

func (s StationSession) Verify(ctx context.Context) (StationSession, error) {
	return (authClient{authHTTP}).verifyStation(ctx, s)
}

func (c authClient) verifyStation(ctx context.Context, s StationSession) (StationSession, error) {
	if !s.validFor(s.Profile) {
		return s, ErrSkinStationLogin
	}
	body, _ := json.Marshal(map[string]string{"accessToken": s.access, "clientToken": s.client})
	err := c.request(ctx, "皮肤站会话", s.Profile.API+"authserver/validate", http.MethodPost, "application/json", bytes.NewReader(body), "", nil)
	if err == nil {
		return s, nil
	}
	var failure *authResponseError
	if !errors.As(err, &failure) || (failure.status != 401 && failure.status != 403) {
		return s, err
	}
	reply, err := c.refreshStation(ctx, s.Profile.API, s.access, s.client, nil)
	if err != nil {
		if errors.As(err, &failure) && (failure.status == 401 || failure.status == 403) {
			return s, ErrSkinStationLogin
		}
		return s, err
	}
	if !reply.valid(s.client) || !validStationRole(reply.SelectedProfile) ||
		!strings.EqualFold(reply.SelectedProfile.ID, s.Profile.ID) ||
		(reply.User.ID != "" && strings.HasPrefix(s.user, "id:") && s.user != "id:"+stationHash(reply.User.ID)) {
		return s, ErrSkinStationLogin
	}
	s.access, s.Profile.Name = reply.AccessToken, reply.SelectedProfile.Name
	return s, nil
}

type StationTextures struct {
	Skin, Cape image.Image
	Slim       bool
}

func (p StationProfile) Textures(ctx context.Context) (StationTextures, error) {
	return (authClient{authHTTP}).stationTextures(ctx, p)
}

func (c authClient) stationTextures(ctx context.Context, p StationProfile) (StationTextures, error) {
	root, err := stationRoot(p.API)
	if err != nil || !validStationRole(p.StationRole) {
		return StationTextures{}, errors.New("皮肤站角色无效")
	}
	_, meta, _, err := c.stationMetadata(ctx, root, false)
	if err != nil {
		return StationTextures{}, err
	}
	var profile struct {
		ID         string
		Properties []struct{ Name, Value string }
	}
	if err := c.request(ctx, "皮肤站角色", root.String()+"sessionserver/session/minecraft/profile/"+p.ID,
		http.MethodGet, "", nil, "", &profile); err != nil {
		return StationTextures{}, err
	}
	if !strings.EqualFold(profile.ID, p.ID) {
		return StationTextures{}, errors.New("皮肤站返回了不同的角色")
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
			return StationTextures{}, errors.New("皮肤站贴图资料无效")
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

func (c authClient) stationTexture(ctx context.Context, root *url.URL, domains []string, raw string, skin bool) (image.Image, error) {
	if raw == "" {
		return nil, nil
	}
	u, err := stationURL(raw)
	if err != nil {
		return nil, errors.New("皮肤站的贴图须使用 HTTPS")
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
		return nil, errors.New("皮肤站的贴图地址不在其声明的域名内")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New("无法下载皮肤站贴图")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("皮肤站贴图返回 HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxTextureBytes+1))
	if err != nil {
		return nil, err
	}
	return decodeTexture(data, skin)
}

func stationVaultPath(root, key string) (string, error) {
	if b, err := hex.DecodeString(key); err != nil || len(b) != sha256.Size {
		return "", ErrSkinStationLogin
	}
	return filepath.Join(root, "skin-stations", strings.ToLower(key)+".dat"), nil
}

package launcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Public OAuth client ID registered for LXMCL. Desktop clients have no secret.
const microsoftClientID = "aa279040-8561-40d9-bde5-db472524df43"

const (
	microsoftOAuth    = "https://login.microsoftonline.com/consumers/oauth2/v2.0/"
	microsoftScope    = "XboxLive.signin offline_access"
	xboxUserURL       = "https://user.auth.xboxlive.com/user/authenticate"
	xboxXSTSURL       = "https://xsts.auth.xboxlive.com/xsts/authorize"
	minecraftServices = "https://api.minecraftservices.com"
)

var (
	ErrMicrosoftRequired = errors.New("需要重新登录 Microsoft")
	ErrMicrosoftExpired  = errors.New("Microsoft 登录代码已过期，请重新获取")
	profileID            = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
	authHTTP             = &http.Client{
		Timeout:       20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
)

type MicrosoftProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type MicrosoftAccount struct {
	Profile      MicrosoftProfile `json:"profile"`
	refreshToken string
}

type MinecraftSession struct {
	Profile           MicrosoftProfile
	accessToken, xuid string
	expiresAt         time.Time
}

func (s MinecraftSession) ValidFor(id string) bool {
	return s.Profile.ID != "" && s.Profile.ID == id && s.accessToken != "" && time.Until(s.expiresAt) > 90*time.Second
}

type MicrosoftDeviceCode struct {
	Code            string
	VerificationURL string
	ExpiresAt       time.Time
	deviceCode      string
	interval        time.Duration
}

type authClient struct{ http *http.Client }

type authResponseError struct {
	service string
	status  int
	code    string
	xbox    int64
}

func (e *authResponseError) Error() string {
	return fmt.Sprintf("%s 返回 HTTP %d", e.service, e.status)
}

func (c authClient) request(ctx context.Context, service, endpoint, method, contentType string, body io.Reader, bearer string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CAUCraft/LXMCL (https://github.com/CAUCraft/LXMCL)")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if req.URL.Hostname() == "user.auth.xboxlive.com" || req.URL.Hostname() == "xsts.auth.xboxlive.com" {
		req.Header.Set("x-xbl-contract-version", "1")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("无法连接 %s：%w", service, err)
	}
	defer res.Body.Close()
	const limit = 2 << 20
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return fmt.Errorf("无法读取 %s 响应：%w", service, err)
	}
	if len(data) > limit {
		return fmt.Errorf("%s 响应过大", service)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var failure struct {
			Error string `json:"error"`
			XErr  int64  `json:"XErr"`
		}
		_ = json.Unmarshal(data, &failure)
		return &authResponseError{service: service, status: res.StatusCode, code: failure.Error, xbox: failure.XErr}
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s 返回无法解析的认证响应", service)
	}
	return nil
}

func (c authClient) form(ctx context.Context, endpoint string, args url.Values, out any) error {
	args.Set("client_id", microsoftClientID)
	return c.request(ctx, "Microsoft", microsoftOAuth+endpoint, http.MethodPost,
		"application/x-www-form-urlencoded", strings.NewReader(args.Encode()), "", out)
}

func (c authClient) post(ctx context.Context, service, endpoint string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return c.request(ctx, service, endpoint, http.MethodPost, "application/json", bytes.NewReader(raw), "", out)
}

func BeginMicrosoftLogin(ctx context.Context) (MicrosoftDeviceCode, error) {
	return (authClient{authHTTP}).begin(ctx)
}

func (c authClient) begin(ctx context.Context) (MicrosoftDeviceCode, error) {
	var reply struct {
		Code         string `json:"user_code"`
		DeviceCode   string `json:"device_code"`
		Verification string `json:"verification_uri"`
		Expires      int    `json:"expires_in"`
		Interval     int    `json:"interval"`
	}
	if err := c.form(ctx, "devicecode", url.Values{"scope": {microsoftScope}}, &reply); err != nil {
		return MicrosoftDeviceCode{}, microsoftTokenError(err)
	}
	u, err := url.Parse(reply.Verification)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" ||
		(u.Host != "www.microsoft.com" && u.Host != "microsoft.com" && u.Host != "login.microsoftonline.com") ||
		reply.Code == "" || reply.DeviceCode == "" || reply.Expires <= 0 || reply.Expires > 1800 {
		return MicrosoftDeviceCode{}, errors.New("Microsoft 返回了无效的设备授权")
	}
	interval := reply.Interval
	if interval <= 0 {
		interval = 5
	}
	return MicrosoftDeviceCode{reply.Code, u.String(), time.Now().Add(time.Duration(reply.Expires) * time.Second),
		reply.DeviceCode, time.Duration(interval) * time.Second}, nil
}

type microsoftTokens struct {
	Access  string `json:"access_token"`
	Refresh string `json:"refresh_token"`
}

func CompleteMicrosoftLogin(ctx context.Context, code MicrosoftDeviceCode) (MicrosoftAccount, MinecraftSession, error) {
	return (authClient{authHTTP}).complete(ctx, code)
}

func (c authClient) complete(ctx context.Context, code MicrosoftDeviceCode) (MicrosoftAccount, MinecraftSession, error) {
	poll, cancel := context.WithDeadline(ctx, code.ExpiresAt)
	defer cancel()
	delay := code.interval
	for {
		if err := waitContext(poll, delay); err != nil {
			if ctx.Err() != nil {
				return MicrosoftAccount{}, MinecraftSession{}, ctx.Err()
			}
			return MicrosoftAccount{}, MinecraftSession{}, ErrMicrosoftExpired
		}
		var tokens microsoftTokens
		err := c.form(poll, "token", url.Values{
			"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "device_code": {code.deviceCode},
		}, &tokens)
		if err == nil {
			return c.minecraftSession(ctx, tokens)
		}
		if ctx.Err() != nil {
			return MicrosoftAccount{}, MinecraftSession{}, ctx.Err()
		}
		if poll.Err() != nil {
			return MicrosoftAccount{}, MinecraftSession{}, ErrMicrosoftExpired
		}
		var failure *authResponseError
		if errors.As(err, &failure) {
			switch failure.code {
			case "authorization_pending":
				continue
			case "slow_down":
				delay += 5 * time.Second
				continue
			}
		}
		return MicrosoftAccount{}, MinecraftSession{}, microsoftTokenError(err)
	}
}

func RefreshMicrosoftLogin(ctx context.Context, account MicrosoftAccount) (MicrosoftAccount, MinecraftSession, error) {
	return (authClient{authHTTP}).refresh(ctx, account)
}

func (c authClient) refresh(ctx context.Context, account MicrosoftAccount) (MicrosoftAccount, MinecraftSession, error) {
	if account.refreshToken == "" || !profileID.MatchString(account.Profile.ID) {
		return account, MinecraftSession{}, ErrMicrosoftRequired
	}
	var tokens microsoftTokens
	err := c.form(ctx, "token", url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {account.refreshToken}, "scope": {microsoftScope},
	}, &tokens)
	if err != nil {
		return account, MinecraftSession{}, microsoftTokenError(err)
	}
	if tokens.Refresh == "" {
		tokens.Refresh = account.refreshToken
	}
	next, session, err := c.minecraftSession(ctx, tokens)
	if err == nil && next.Profile.ID != account.Profile.ID {
		return account, MinecraftSession{}, errors.New("Microsoft 返回了不同的 Minecraft 档案，请重新登录")
	}
	return next, session, err
}

func microsoftTokenError(err error) error {
	var failure *authResponseError
	if errors.As(err, &failure) {
		switch failure.code {
		case "authorization_declined", "access_denied":
			return errors.New("Microsoft 授权已拒绝")
		case "expired_token":
			return ErrMicrosoftExpired
		case "invalid_grant", "interaction_required":
			return ErrMicrosoftRequired
		case "invalid_client", "unauthorized_client":
			return errors.New("Microsoft 应用未启用此登录方式")
		}
	}
	return err
}

type xboxToken struct {
	Token         string
	DisplayClaims struct{ XUI []struct{ UHS, XID string } }
}

func (c authClient) minecraftSession(ctx context.Context, tokens microsoftTokens) (MicrosoftAccount, MinecraftSession, error) {
	var account MicrosoftAccount
	var session MinecraftSession
	if tokens.Access == "" || tokens.Refresh == "" {
		return account, session, errors.New("Microsoft 未返回可刷新的登录凭据")
	}
	var user, xsts xboxToken
	userRequest := map[string]any{
		"Properties":   map[string]string{"AuthMethod": "RPS", "SiteName": "user.auth.xboxlive.com", "RpsTicket": "d=" + tokens.Access},
		"RelyingParty": "http://auth.xboxlive.com", "TokenType": "JWT",
	}
	if err := c.post(ctx, "Xbox Live", xboxUserURL, userRequest, &user); err != nil {
		return account, session, err
	}
	if user.Token == "" {
		return account, session, errors.New("Xbox Live 未返回用户凭据")
	}
	xstsRequest := map[string]any{
		"Properties":   map[string]any{"SandboxId": "RETAIL", "UserTokens": []string{user.Token}},
		"RelyingParty": "rp://api.minecraftservices.com/", "TokenType": "JWT",
	}
	if err := c.post(ctx, "Xbox XSTS", xboxXSTSURL, xstsRequest, &xsts); err != nil {
		var failure *authResponseError
		if errors.As(err, &failure) && failure.xbox != 0 {
			err = fmt.Errorf("Xbox 未授权（%d），请检查 Xbox 档案、地区或家庭设置", failure.xbox)
		}
		return account, session, err
	}
	if xsts.Token == "" || len(xsts.DisplayClaims.XUI) == 0 || xsts.DisplayClaims.XUI[0].UHS == "" {
		return account, session, errors.New("Xbox 未返回 Minecraft 所需的凭据")
	}
	claims := xsts.DisplayClaims.XUI[0]
	var game struct {
		Token   string `json:"access_token"`
		Expires int64  `json:"expires_in"`
	}
	body := map[string]string{"identityToken": "XBL3.0 x=" + claims.UHS + ";" + xsts.Token}
	if err := c.post(ctx, "Minecraft 登录", minecraftServices+"/authentication/login_with_xbox", body, &game); err != nil {
		var failure *authResponseError
		if errors.As(err, &failure) && failure.status == http.StatusForbidden {
			err = errors.New("Minecraft 拒绝此应用授权（HTTP 403），请核对应用的 Minecraft Services 访问权限")
		}
		return account, session, err
	}
	if game.Token == "" || game.Expires <= 90 || game.Expires > 7*86400 {
		return account, session, errors.New("Minecraft 返回了无效的访问凭据")
	}
	var entitlement struct{ Items []struct{ Name string } }
	if err := c.request(ctx, "Minecraft 授权", minecraftServices+"/entitlements/mcstore", http.MethodGet, "", nil, game.Token, &entitlement); err != nil {
		return account, session, err
	}
	owned := false
	for _, item := range entitlement.Items {
		switch item.Name {
		case "game_minecraft", "product_minecraft", "game_minecraft_java", "product_minecraft_java":
			owned = true
		}
	}
	if !owned {
		return account, session, errors.New("此账号没有 Minecraft Java 版使用权益")
	}
	var profile MicrosoftProfile
	if err := c.request(ctx, "Minecraft Java 档案", minecraftServices+"/minecraft/profile", http.MethodGet, "", nil, game.Token, &profile); err != nil {
		var failure *authResponseError
		if errors.As(err, &failure) && failure.status == http.StatusNotFound {
			err = errors.New("此账号还没有 Java 版档案，请先在 Minecraft 官网或官方启动器创建")
		}
		return account, session, err
	}
	if !profileID.MatchString(profile.ID) || !playerID.MatchString(profile.Name) {
		return account, session, errors.New("Minecraft 返回了无效的 Java 版档案")
	}
	profile.ID = strings.ToLower(profile.ID)
	account = MicrosoftAccount{Profile: profile, refreshToken: tokens.Refresh}
	session = MinecraftSession{Profile: profile, accessToken: game.Token, xuid: claims.XID, expiresAt: time.Now().Add(time.Duration(game.Expires) * time.Second)}
	return account, session, nil
}

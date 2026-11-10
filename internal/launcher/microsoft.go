package launcher

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

const microsoftClientID = "aa279040-8561-40d9-bde5-db472524df43"

const (
	microsoftOAuth    = "https://login.microsoftonline.com/consumers/oauth2/v2.0/"
	microsoftScope    = "XboxLive.signin offline_access"
	xboxUserURL       = "https://user.auth.xboxlive.com/user/authenticate"
	xboxXSTSURL       = "https://xsts.auth.xboxlive.com/xsts/authorize"
	minecraftServices = "https://api.minecraftservices.com"
)

var (
	ErrMicrosoftRequired = i18n.Error("microsoft.login_required")
	ErrMicrosoftExpired  = i18n.Error("microsoft.code_expired")
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
		return MicrosoftDeviceCode{}, i18n.Error("microsoft.device_authorization_invalid")
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
		return account, MinecraftSession{}, i18n.Error("microsoft.profile_changed")
	}
	return next, session, err
}

func microsoftTokenError(err error) error {
	var failure *authResponseError
	if errors.As(err, &failure) {
		switch failure.code {
		case "authorization_declined", "access_denied":
			return i18n.Error("microsoft.authorization_denied")
		case "expired_token":
			return ErrMicrosoftExpired
		case "invalid_grant", "interaction_required":
			return ErrMicrosoftRequired
		case "invalid_client", "unauthorized_client":
			return i18n.Error("microsoft.login_method_disabled")
		}
	}
	return err
}

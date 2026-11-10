package launcher

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

type xboxToken struct {
	Token         string
	DisplayClaims struct{ XUI []struct{ UHS, XID string } }
}

func (c authClient) minecraftSession(ctx context.Context, tokens microsoftTokens) (MicrosoftAccount, MinecraftSession, error) {
	var account MicrosoftAccount
	var session MinecraftSession
	if tokens.Access == "" || tokens.Refresh == "" {
		return account, session, i18n.Error("microsoft.refresh_token_missing")
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
		return account, session, i18n.Error("xbox.user_token_missing")
	}
	xstsRequest := map[string]any{
		"Properties":   map[string]any{"SandboxId": "RETAIL", "UserTokens": []string{user.Token}},
		"RelyingParty": "rp://api.minecraftservices.com/", "TokenType": "JWT",
	}
	if err := c.post(ctx, "Xbox XSTS", xboxXSTSURL, xstsRequest, &xsts); err != nil {
		var failure *authResponseError
		if errors.As(err, &failure) && failure.xbox != 0 {
			err = i18n.Error("xbox.authorization_denied", failure.xbox)
		}
		return account, session, err
	}
	if xsts.Token == "" || len(xsts.DisplayClaims.XUI) == 0 || xsts.DisplayClaims.XUI[0].UHS == "" {
		return account, session, i18n.Error("xbox.minecraft_token_missing")
	}
	claims := xsts.DisplayClaims.XUI[0]
	var game struct {
		Token   string `json:"access_token"`
		Expires int64  `json:"expires_in"`
	}
	body := map[string]string{"identityToken": "XBL3.0 x=" + claims.UHS + ";" + xsts.Token}
	if err := c.post(ctx, i18n.Text("microsoft.minecraft_login_service"), minecraftServices+"/authentication/login_with_xbox", body, &game); err != nil {
		var failure *authResponseError
		if errors.As(err, &failure) && failure.status == http.StatusForbidden {
			err = i18n.Error("microsoft.minecraft_access_denied")
		}
		return account, session, err
	}
	if game.Token == "" || game.Expires <= 90 || game.Expires > 7*86400 {
		return account, session, i18n.Error("microsoft.minecraft_token_invalid")
	}
	var entitlement struct{ Items []struct{ Name string } }
	if err := c.request(ctx, i18n.Text("microsoft.minecraft_entitlements_service"), minecraftServices+"/entitlements/mcstore", http.MethodGet, "", nil, game.Token, &entitlement); err != nil {
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
		return account, session, i18n.Error("microsoft.minecraft_license_missing")
	}
	var profile MicrosoftProfile
	if err := c.request(ctx, i18n.Text("microsoft.minecraft_profile_service"), minecraftServices+"/minecraft/profile", http.MethodGet, "", nil, game.Token, &profile); err != nil {
		var failure *authResponseError
		if errors.As(err, &failure) && failure.status == http.StatusNotFound {
			err = i18n.Error("microsoft.minecraft_profile_missing")
		}
		return account, session, err
	}
	if !profileID.MatchString(profile.ID) || !instance.ValidPlayerName(profile.Name) {
		return account, session, i18n.Error("microsoft.minecraft_profile_invalid")
	}
	profile.ID = strings.ToLower(profile.ID)
	account = MicrosoftAccount{Profile: profile, refreshToken: tokens.Refresh}
	session = MinecraftSession{Profile: profile, accessToken: game.Token, xuid: claims.XID, expiresAt: time.Now().Add(time.Duration(game.Expires) * time.Second)}
	return account, session, nil
}

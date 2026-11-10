package distribution

import (
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

// AllowedURL accepts file origins supported by the launcher, without credentials.
func AllowedURL(u *url.URL) bool {
	return u != nil && u.Scheme == "https" && u.User == nil && u.Port() == "" &&
		(slices.Contains([]string{"cdn.modrinth.com", "github.com", "raw.githubusercontent.com", "gitlab.com",
			"release-assets.githubusercontent.com", "objects.githubusercontent.com"}, u.Hostname()) || CurseCDN(u))
}

// CurseCDN identifies the hosts permitted to serve CurseForge file bytes.
func CurseCDN(u *url.URL) bool {
	return u != nil && u.Scheme == "https" && u.User == nil && u.Port() == "" &&
		slices.Contains([]string{"edge.forgecdn.net", "mediafilez.forgecdn.net", "media.forgecdn.net", "mediafiles.forgecdn.net"}, u.Hostname())
}

var defaultHTTP = &http.Client{
	Timeout: 5 * time.Minute,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 6 || !AllowedURL(req.URL) || (CurseCDN(via[0].URL) && !CurseCDN(req.URL)) {
			return i18n.Error("distribution.redirect_denied")
		}
		return nil
	},
}

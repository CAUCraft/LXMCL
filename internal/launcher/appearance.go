package launcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

const (
	profileEndpoint = minecraftServices + "/minecraft/profile"
	maxTextureBytes = 1 << 20
)

var capeID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type ProfileTexture struct {
	ID, State, URL, Alias, Variant string
}

type Appearance struct {
	Skin  ProfileTexture
	Capes []ProfileTexture
}

func (a Appearance) ActiveCape() ProfileTexture {
	for _, cape := range a.Capes {
		if cape.State == "ACTIVE" {
			return cape
		}
	}
	return ProfileTexture{}
}

func (s MinecraftSession) Appearance(ctx context.Context) (Appearance, error) {
	return (authClient{authHTTP}).appearance(ctx, s)
}

func (c authClient) appearance(ctx context.Context, s MinecraftSession) (Appearance, error) {
	var result Appearance
	var profile struct {
		ID           string
		Skins, Capes []ProfileTexture
	}
	err := c.appearanceRequest(ctx, s, "", http.MethodGet, "", nil, &profile)
	if err != nil {
		return result, err
	}
	if !strings.EqualFold(profile.ID, s.Profile.ID) {
		return result, i18n.Error("appearance.profile_mismatch")
	}
	for _, skin := range profile.Skins {
		if skin.State == "ACTIVE" {
			result.Skin = skin
			break
		}
	}
	for _, cape := range profile.Capes {
		if capeID.MatchString(cape.ID) {
			result.Capes = append(result.Capes, cape)
		}
	}
	return result, nil
}

func (s MinecraftSession) UploadSkin(ctx context.Context, data []byte, variant string) error {
	return (authClient{authHTTP}).uploadSkin(ctx, s, data, variant)
}

func (c authClient) uploadSkin(ctx context.Context, s MinecraftSession, data []byte, variant string) error {
	if variant != "classic" && variant != "slim" {
		return i18n.Error("appearance.skin_model_invalid")
	}
	img, err := decodeTexture(data, true)
	if err != nil {
		return err
	}
	if variant == "slim" && img.Bounds().Dy() == 32 {
		return i18n.Error("appearance.skin_model_classic_only")
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.WriteField("variant", variant); err != nil {
		return err
	}
	part, err := form.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="file"; filename="skin.png"`},
		"Content-Type":        {"image/png"},
	})
	if err != nil {
		return err
	}
	if _, err := part.Write(data); err != nil {
		return err
	}
	if err := form.Close(); err != nil {
		return err
	}
	return c.appearanceRequest(ctx, s, "/skins", http.MethodPost, form.FormDataContentType(), &body, nil)
}

func (s MinecraftSession) SetCape(ctx context.Context, id string) error {
	return (authClient{authHTTP}).setCape(ctx, s, id)
}

func (c authClient) setCape(ctx context.Context, s MinecraftSession, id string) error {
	const endpoint = "/capes/active"
	if id == "" {
		return c.appearanceRequest(ctx, s, endpoint, http.MethodDelete, "", nil, nil)
	}
	if !capeID.MatchString(id) {
		return i18n.Error("appearance.cape_id_invalid")
	}
	body, _ := json.Marshal(map[string]string{"capeId": id})
	return c.appearanceRequest(ctx, s, endpoint, http.MethodPut, "application/json", bytes.NewReader(body), nil)
}

func (c authClient) appearanceRequest(ctx context.Context, s MinecraftSession, path, method, contentType string, body io.Reader, out any) error {
	if !s.ValidFor(s.Profile.ID) {
		return ErrMicrosoftRequired
	}
	err := c.request(ctx, i18n.Text("appearance.service"), profileEndpoint+path, method, contentType, body, s.accessToken, out)
	var failure *authResponseError
	if errors.As(err, &failure) && failure.status == http.StatusUnauthorized {
		return ErrMicrosoftRequired
	}
	return err
}

// FetchProfileTexture only reads public assets on the official texture CDN.
// Old profiles may advertise HTTP URLs; use HTTPS for the same texture.
func FetchProfileTexture(ctx context.Context, raw string, skin bool) (image.Image, error) {
	return (authClient{authHTTP}).texture(ctx, raw, skin)
}

func (c authClient) texture(ctx context.Context, raw string, skin bool) (image.Image, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") ||
		u.Host != "textures.minecraft.net" || u.User != nil || u.RawQuery != "" ||
		u.Fragment != "" || !strings.HasPrefix(u.Path, "/texture/") {
		return nil, i18n.Error("appearance.texture_url_invalid")
	}
	u.Scheme = "https"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, i18n.Error("appearance.texture_http_status", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxTextureBytes+1))
	if err != nil {
		return nil, err
	}
	return decodeTexture(data, skin)
}

func ReadSkinFile(path string) ([]byte, image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxTextureBytes+1))
	if err != nil {
		return nil, nil, err
	}
	img, err := decodeTexture(data, true)
	if err != nil {
		return nil, nil, err
	}
	return data, img, nil
}

func decodeTexture(data []byte, skin bool) (image.Image, error) {
	if len(data) > maxTextureBytes {
		return nil, i18n.Error("appearance.png_too_large")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, i18n.Error("appearance.png_invalid")
	}
	if skin && (cfg.Width != 64 || (cfg.Height != 64 && cfg.Height != 32)) {
		return nil, i18n.Error("appearance.skin_dimensions_invalid")
	}
	if !skin && (cfg.Width < 22 || cfg.Height < 17 || cfg.Width > 1024 || cfg.Height > 1024) {
		return nil, i18n.Error("appearance.cape_dimensions_invalid")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, i18n.Error("appearance.png_incomplete")
	}
	return img, nil
}

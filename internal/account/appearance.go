package account

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"regexp"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/i18n"
)

const (
	profileEndpoint = minecraftServices + "/minecraft/profile"
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
	return (Client{}).Appearance(ctx, s)
}

func (c Client) Appearance(ctx context.Context, s MinecraftSession) (Appearance, error) {
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
	return (Client{}).UploadSkin(ctx, s, data, variant)
}

func (c Client) UploadSkin(ctx context.Context, s MinecraftSession, data []byte, variant string) error {
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
	return (Client{}).SetCape(ctx, s, id)
}

func (c Client) SetCape(ctx context.Context, s MinecraftSession, id string) error {
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

func (c Client) appearanceRequest(ctx context.Context, s MinecraftSession, path, method, contentType string, body io.Reader, out any) error {
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

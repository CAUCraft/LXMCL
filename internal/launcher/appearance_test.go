package launcher

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"slices"
	"testing"
	"time"
)

func textureFixture(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMinecraftAppearanceRequests(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	const cape = "01234567-89ab-cdef-0123-456789abcdef"
	const texture = "https://textures.minecraft.net/texture/abc"
	s := MinecraftSession{Profile: MicrosoftProfile{ID: id}, accessToken: "game-fixture", expiresAt: time.Now().Add(time.Hour)}
	pngBytes := textureFixture(t, 64, 64)
	var requests []string
	client := authClient{&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.minecraftservices.com" || r.Header.Get("Authorization") != "Bearer game-fixture" {
			t.Fatalf("unexpected Minecraft profile request: %s", r.URL)
		}
		request := r.Method + " " + r.URL.Path
		requests = append(requests, request)
		switch request {
		case "GET /minecraft/profile":
			return fixtureResponse(200, []byte(`{"id":"`+id+`","skins":[{"state":"INACTIVE"},{"state":"ACTIVE","url":"`+texture+`","variant":"SLIM"}],
				"capes":[{"id":"`+cape+`","state":"ACTIVE","url":"`+texture+`","alias":"Test Cape"},{"id":"bad-id"}]}`)), nil
		case "POST /minecraft/profile/skins":
			if err := r.ParseMultipartForm(1 << 20); err != nil || r.FormValue("variant") != "slim" {
				t.Fatalf("invalid skin form: %v", err)
			}
			f, meta, err := r.FormFile("file")
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			body, err := io.ReadAll(f)
			if err != nil || meta.Header.Get("Content-Type") != "image/png" || !bytes.Equal(body, pngBytes) {
				t.Fatal("skin upload did not contain the selected PNG")
			}
		case "PUT /minecraft/profile/capes/active":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["capeId"] != cape {
				t.Fatalf("incorrect cape request: %v", err)
			}
		case "DELETE /minecraft/profile/capes/active":
		default:
			t.Fatalf("unexpected request: %s", request)
		}
		return fixtureResponse(204, nil), nil
	})}}
	data, err := client.appearance(t.Context(), s)
	if err != nil || data.Skin.URL != texture || data.Skin.Variant != "SLIM" || len(data.Capes) != 1 || data.ActiveCape().ID != cape {
		t.Fatalf("unexpected appearance: %+v, %v", data, err)
	}
	if err := client.uploadSkin(t.Context(), s, pngBytes, "slim"); err != nil {
		t.Fatal(err)
	}
	if err := client.setCape(t.Context(), s, cape); err != nil {
		t.Fatal(err)
	}
	if err := client.setCape(t.Context(), s, ""); err != nil {
		t.Fatal(err)
	}
	if err := client.uploadSkin(t.Context(), s, textureFixture(t, 64, 32), "slim"); err == nil {
		t.Fatal("accepted the slim model for a legacy skin")
	}
	if err := client.uploadSkin(t.Context(), s, textureFixture(t, 128, 128), "classic"); err == nil {
		t.Fatal("accepted an unsupported skin size")
	}
	if err := client.setCape(t.Context(), s, "../"); err == nil {
		t.Fatal("accepted an invalid cape ID")
	}
	if _, err := client.appearance(t.Context(), MinecraftSession{}); !errors.Is(err, ErrMicrosoftRequired) {
		t.Fatalf("accepted an unsigned session: %v", err)
	}
	if !slices.Equal(requests, []string{"GET /minecraft/profile", "POST /minecraft/profile/skins",
		"PUT /minecraft/profile/capes/active", "DELETE /minecraft/profile/capes/active"}) {
		t.Fatalf("unexpected appearance requests: %v", requests)
	}
}

func TestProfileTextureUsesOfficialCDNWithoutCredentials(t *testing.T) {
	calls := 0
	texture := textureFixture(t, 64, 64)
	client := authClient{&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Scheme != "https" || r.URL.Host != "textures.minecraft.net" || r.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected texture request: %s", r.URL)
		}
		return fixtureResponse(200, texture), nil
	})}}
	if img, err := client.texture(t.Context(), "http://textures.minecraft.net/texture/abc", true); err != nil || img.Bounds().Dx() != 64 {
		t.Fatalf("official texture was not decoded: %v", err)
	}
	for _, url := range []string{"https://example.com/texture/abc", "https://textures.minecraft.net:443/texture/abc", "https://textures.minecraft.net/profile"} {
		if _, err := client.texture(t.Context(), url, true); err == nil {
			t.Fatalf("accepted unexpected texture address %s", url)
		}
	}
	if calls != 1 {
		t.Fatalf("unexpected number of texture requests: %d", calls)
	}
}

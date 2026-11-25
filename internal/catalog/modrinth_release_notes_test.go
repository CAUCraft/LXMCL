package catalog

import (
	"net/http"
	"testing"
)

func TestModrinthVersionReleaseNotes(t *testing.T) {
	for _, tc := range []struct {
		name, body, text string
		known, reject    bool
	}{
		{"text", `{"id":"next","project_id":"atlas","changelog":"改进地图缩放"}`, "改进地图缩放", true, false},
		{"empty", `{"id":"next","project_id":"atlas","changelog":""}`, "", true, false},
		{"null", `{"id":"next","project_id":"atlas","changelog":null}`, "", true, false},
		{"omitted", `{"id":"next","project_id":"atlas"}`, "", false, false},
		{"wrong project", `{"id":"next","project_id":"other","changelog":"other"}`, "", false, true},
		{"wrong ID", `{"id":"old","project_id":"atlas","changelog":"old"}`, "", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet || req.URL.String() != modrinthAPI+"version/next" {
					t.Fatal(req.Method, req.URL)
				}
				return fixtureResponse(http.StatusOK, []byte(tc.body)), nil
			})}
			version, err := c.ModrinthVersion(t.Context(), "atlas", "next")
			text, known := version.ChangelogText()
			if (err != nil) != tc.reject || known != tc.known || text != tc.text {
				t.Fatalf("got %q, known %t, error %v", text, known, err)
			}
		})
	}
}

package catalog

import (
	"net/http"
	"strings"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func TestModrinthSearchPaginatesAndUsesCorrectCategory(t *testing.T) {
	var got []string
	client := Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		q := req.URL.Query()
		if req.URL.Path != "/v2/search" || q.Get("query") != "test" || q.Get("index") != "updated" ||
			q.Get("limit") != "20" || q.Get("offset") != "40" || !strings.Contains(q.Get("facets"), "versions:1.21.1") {
			t.Fatalf("unexpected search request: %s", req.URL)
		}
		got = append(got, q.Get("facets"))
		return fixtureResponse(200, []byte(`{"total_hits":54,"hits":[{"project_id":"valid","title":"Demo","description":"Pack"},{"project_id":"../bad","title":"Invalid"}]}`)), nil
	})}
	for _, tc := range []struct {
		kind          content.Kind
		loader, facet string
	}{
		{content.Mods, instance.LoaderFabric, "categories:fabric"},
		{content.Mods, instance.LoaderForge, "categories:forge"},
		{content.Mods, instance.LoaderNeoForge, "categories:neoforge"},
		{content.Mods, instance.LoaderQuilt, "categories:quilt"},
		{content.ShaderPacks, instance.LoaderFabric, "project_type:shader"},
	} {
		result, err := client.SearchModrinth(t.Context(), instance.Instance{Version: "1.21.1", Loader: tc.loader}, tc.kind, " test ", "updated", 2)
		if err != nil || result.Total != 54 || len(result.Hits) != 1 || result.Hits[0].Description != "Pack" {
			t.Fatalf("unexpected result: %+v / %v", result, err)
		}
		facets := got[len(got)-1]
		if !strings.Contains(facets, tc.facet) || tc.kind == content.ShaderPacks && strings.Contains(facets, "categories:") {
			t.Fatalf("incompatible search facets: %s", facets)
		}
	}
}

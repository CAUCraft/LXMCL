package launcher

import (
	"net/http"
	"testing"
)

func TestOpenResourceDependencyUsesMetadataAndRespectsPinnedVersion(t *testing.T) {
	i := Instance{ID: 1, Version: "1.21.1", Loader: LoaderNeoForge, LoaderVersion: "21.1.265", GameDir: t.TempDir()}
	d := &downloads{http: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v2/project/library":
			return fixtureResponse(200, []byte(`{"id":"library","title":"Library","project_type":"mod"}`)), nil
		case "/v2/project/library/version":
			q := req.URL.Query()
			if q.Get("loaders") != `["neoforge"]` || q.Get("game_versions") != `["1.21.1"]` {
				t.Fatalf("incorrect dependency filters: %s", req.URL)
			}
			return fixtureResponse(200, []byte(`[
				{"id":"newer","project_id":"library","loaders":["neoforge"],"game_versions":["1.21.1"],"files":[{"filename":"newer.jar"}]},
				{"id":"older","project_id":"library","loaders":["forge"],"game_versions":["1.21.1"],"files":[{"filename":"older.jar"}]}
			]`)), nil
		case "/v2/version/pinned":
			return fixtureResponse(200, []byte(`{"id":"pinned","project_id":"library","loaders":["neoforge"],"game_versions":["1.21.1"],"files":[{"filename":"pinned.jar"}]}`)), nil
		default:
			t.Fatalf("dependency browsing requested an unexpected endpoint: %s", req.URL)
			return nil, nil
		}
	})}}
	linked, err := openResourceDependency(t.Context(), ModDependency{ProjectID: "library"}, i, ResourceFilter{}, d)
	if err != nil || linked.Project.Title != "Library" || len(linked.Versions) != 1 || linked.Versions[0].ID != "newer" {
		t.Fatalf("unexpected project dependency: %+v %v", linked, err)
	}
	linked, err = openResourceDependency(t.Context(), ModDependency{VersionID: "pinned"}, i, ResourceFilter{}, d)
	if err != nil || linked.Project.ID != "library" || len(linked.Versions) != 1 || linked.Versions[0].ID != "pinned" {
		t.Fatalf("author's pinned dependency was not retained: %+v %v", linked, err)
	}
	i.Loader, i.LoaderVersion = LoaderForge, "52.1.21"
	if linked, err = openResourceDependency(t.Context(), ModDependency{VersionID: "pinned"}, i, ResourceFilter{}, d); err == nil || len(linked.Versions) != 0 {
		t.Fatalf("incompatible pinned dependency was offered for installation: %+v %v", linked, err)
	}
}

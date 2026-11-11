package catalog

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func TestOpenResourceDependencyUsesMetadataAndRespectsPinnedVersion(t *testing.T) {
	i := instance.Instance{ID: 1, Version: "1.21.1", Loader: instance.LoaderNeoForge, LoaderVersion: "21.1.265", GameDir: t.TempDir()}
	client := Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v2/project/library":
			return fixtureResponse(200, []byte(`{"id":"library","title":"Library","project_type":"mod"}`)), nil
		case "/v2/project/library/version":
			q := req.URL.Query()
			if q.Get("loaders") != `["neoforge"]` || q.Get("game_versions") != `["1.21.1"]` {
				t.Fatalf("incorrect dependency filters: %s", req.URL)
			}
			newer, older := versionFixture("newer"), versionFixture("older")
			older.Loaders = []string{"forge"}
			return jsonResponse(t, []PackVersion{newer, older}), nil
		case "/v2/version/pinned":
			return jsonResponse(t, versionFixture("pinned")), nil
		default:
			t.Fatalf("dependency browsing requested an unexpected endpoint: %s", req.URL)
			return nil, nil
		}
	})}
	linked, err := client.OpenResourceDependency(t.Context(), ModDependency{ProjectID: "library"}, i, ResourceFilter{})
	if err != nil || linked.Project.Title != "Library" || len(linked.Versions) != 1 || linked.Versions[0].ID != "newer" {
		t.Fatalf("unexpected project dependency: %+v %v", linked, err)
	}
	linked, err = client.OpenResourceDependency(t.Context(), ModDependency{VersionID: "pinned"}, i, ResourceFilter{})
	if err != nil || linked.Project.ID != "library" || len(linked.Versions) != 1 || linked.Versions[0].ID != "pinned" {
		t.Fatalf("author's pinned dependency was not retained: %+v %v", linked, err)
	}
	i.Loader, i.LoaderVersion = instance.LoaderForge, "52.1.21"
	if linked, err = client.OpenResourceDependency(t.Context(), ModDependency{VersionID: "pinned"}, i, ResourceFilter{}); err == nil || len(linked.Versions) != 0 {
		t.Fatalf("incompatible pinned dependency was offered for installation: %+v %v", linked, err)
	}
}

func TestDependencyMetadataUsesDetailIDsAndKeepsPinnedTarget(t *testing.T) {
	pinned := versionFixture("pinned")
	calls := 0
	c := Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		switch req.URL.Path {
		case "/v2/projects":
			var ids []string
			if err := json.Unmarshal([]byte(req.URL.Query().Get("ids")), &ids); err != nil || !slices.Equal(ids, []string{"library"}) {
				t.Fatalf("batch lookup did not deduplicate: %v, %v", ids, err)
			}
			return fixtureResponse(200, []byte(`[{"id":"library","title":"Library","project_type":"mod"},{"id":"unsolicited","title":"Ignore"}]`)), nil
		case "/v2/version/pinned":
			return jsonResponse(t, pinned), nil
		case "/v2/project/library":
			return fixtureResponse(200, []byte(`{"id":"library","title":"Library","project_type":"mod"}`)), nil
		default:
			t.Fatalf("dependency browsing requested file bytes or wrong metadata: %s", req.URL)
			return nil, nil
		}
	})}
	projects, err := c.ResourceDependencyProjects(t.Context(), []ModDependency{{ProjectID: "library"}, {ProjectID: "library"}, {ProjectID: "../invalid"}})
	if err != nil || len(projects) != 1 || projects["library"].ID != "library" {
		t.Fatalf("detail IDs were lost: %+v, %v", projects, err)
	}
	// An author's exact version takes precedence over browsing filters outside an instance.
	result, err := c.OpenResourceDependency(t.Context(), ModDependency{ProjectID: "library", VersionID: "pinned"}, instance.Instance{}, ResourceFilter{Game: "1.20.1", Loader: "fabric"})
	if err != nil || len(result.Versions) != 1 || result.Versions[0].ID != "pinned" {
		t.Fatalf("pinned version changed: %+v, %v", result, err)
	}
	pinned.ProjectID = "wrong-project"
	before := calls
	if _, err := c.OpenResourceDependency(t.Context(), ModDependency{ProjectID: "library", VersionID: "pinned"}, instance.Instance{}, ResourceFilter{}); err == nil || calls != before+1 {
		t.Fatalf("mismatched pinned project followed: %d calls, %v", calls-before, err)
	}
	pinned.ProjectID = "library"
	pinned.Files[0].Hashes.SHA512 = "missing"
	if result, err := c.OpenResourceDependency(t.Context(), ModDependency{VersionID: "pinned"}, instance.Instance{}, ResourceFilter{}); err == nil || len(result.Versions) != 0 {
		t.Fatalf("unverifiable pinned file offered: %+v, %v", result, err)
	}
}

func TestQuiltDependencyFallbackPreservesCompatibility(t *testing.T) {
	for _, bundleAvailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "fabric-api", true: "quilt-bundle"}[bundleAvailable], func(t *testing.T) {
			c := Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/v2/project/" + fabricAPIProject, "/v2/project/" + quiltedAPIProject:
					id := fabricAPIProject
					if req.URL.Path == "/v2/project/"+quiltedAPIProject {
						id = quiltedAPIProject
					}
					return jsonResponse(t, map[string]string{"id": id, "title": "API", "project_type": "mod"}), nil
				case "/v2/project/" + fabricAPIProject + "/version":
					if req.URL.Query().Get("loaders") == `["quilt"]` {
						return jsonResponse(t, []PackVersion{}), nil
					}
					v := versionFixture("fabric")
					v.ProjectID, v.Loaders = fabricAPIProject, []string{"fabric"}
					return jsonResponse(t, []PackVersion{v}), nil
				case "/v2/project/" + quiltedAPIProject + "/version":
					if !bundleAvailable {
						return jsonResponse(t, []PackVersion{}), nil
					}
					v := versionFixture("quilt")
					v.ProjectID, v.Loaders = quiltedAPIProject, []string{"quilt"}
					return jsonResponse(t, []PackVersion{v}), nil
				default:
					t.Fatalf("unexpected request: %s", req.URL)
					return nil, nil
				}
			})}
			i := instance.Instance{ID: 1, Version: "1.21.1", Loader: instance.LoaderQuilt, LoaderVersion: "0.30.2", GameDir: t.TempDir()}
			got, err := c.OpenResourceDependency(t.Context(), ModDependency{ProjectID: fabricAPIProject}, i, ResourceFilter{})
			want := fabricAPIProject
			if bundleAvailable {
				want = quiltedAPIProject
			}
			if err != nil || got.Project.ID != want || len(got.Versions) != 1 || !got.Versions[0].Compatible(i, content.Mods) {
				t.Fatalf("Quilt fallback = %+v, %v", got, err)
			}
		})
	}
}

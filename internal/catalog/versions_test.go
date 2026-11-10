package catalog

import (
	"net/http"
	"slices"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/content"
)

func TestResourceVersionsRequireVerifiableSelectedFile(t *testing.T) {
	valid := versionFixture("valid")
	versions := []PackVersion{valid}
	for _, mutate := range []func(*PackVersion){
		func(v *PackVersion) { v.Files[0].URL = "https://example.com/file.jar" },
		func(v *PackVersion) { v.Files[0].Hashes.SHA512 = "missing" },
		func(v *PackVersion) { v.Files[0].Size = 0 },
		func(v *PackVersion) { v.Files[0].Filename = "../escape.jar" },
		func(v *PackVersion) { v.ProjectID = "different" },
		func(v *PackVersion) { v.GameVersions = []string{"1.20.1"} },
		func(v *PackVersion) {
			v.Files[0].Hashes.SHA512 = "missing"
			v.Files = append(v.Files, valid.Files[0])
			v.Files[1].Primary = false
		},
	} {
		v := versionFixture("invalid")
		mutate(&v)
		versions = append(versions, v)
	}
	c := Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v2/project/library/version" {
			t.Fatalf("unexpected query: %s", req.URL)
		}
		return jsonResponse(t, versions), nil
	})}
	got, err := c.ResourceCatalogVersions(t.Context(), "library", ResourceFilter{Game: "1.21.1", Loader: "neoforge"}, content.Mods)
	if err != nil || len(got) != 1 || got[0].ID != "valid" {
		t.Fatalf("unverifiable version offered: %+v, %v", got, err)
	}
	if _, _, err := valid.DownloadAsset(content.Kind("unknown")); err == nil {
		t.Fatal("unknown resource kind accepted")
	}
}

func TestModpackReleaseKeepsDownloadPolicyPrivate(t *testing.T) {
	for _, restricted := range []bool{false, true} {
		t.Run(map[bool]string{true: "restricted", false: "unsupported-loader"}[restricted], func(t *testing.T) {
			file := versionFixture("pack").Files[0]
			file.Filename = "pack.mrpack"
			c := Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				if restricted {
					return fixtureResponse(200, []byte(`{"data":[{"id":2,"modId":1,"gameId":432,"fileName":"pack.zip","isAvailable":false,"fileLength":4,"downloadUrl":"https://edge.forgecdn.net/pack.zip","hashes":[{"algo":1,"value":"971c419dd609331343dee105fffd0f4608dc0bf2"}]}]}`)), nil
				}
				v := versionFixture("pack")
				v.Loaders = []string{"unsupported"}
				v.Files = []PackFile{file}
				return jsonResponse(t, []PackVersion{v}), nil
			})}
			project := ModpackProject{ID: "library", Source: ModrinthModpacks}
			if restricted {
				project = ModpackProject{ID: "1", Source: CurseForgeModpacks}
			}
			releases, _, err := c.ListModpackReleases(t.Context(), project, "api-key", "", 0)
			if err != nil || len(releases) != 1 || releases[0].Unavailable == "" {
				t.Fatalf("release = %+v, %v", releases, err)
			}
			r := releases[0]
			r.Unavailable = ""
			a, ok := r.DownloadAsset()
			if ok == restricted {
				t.Fatalf("download policy followed display text: %v", ok)
			}
			if ok {
				want := slices.Clone(a.URLs)
				a.URLs[0] = "https://invalid.example"
				again, _ := r.DownloadAsset()
				if !slices.Equal(again.URLs, want) {
					t.Fatal("caller modified release sources")
				}
			}
		})
	}
}

func TestProjectDistributionRestrictionSurvivesVersionQuery(t *testing.T) {
	c := Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v1/mods/search":
			return fixtureResponse(200, []byte(`{"data":[{"id":1,"gameId":432,"classId":4471,"name":"Restricted pack","isAvailable":true,"allowModDistribution":false}]}`)), nil
		case "/v1/mods/1/files":
			return fixtureResponse(200, []byte(`{"data":[{"id":2,"modId":1,"gameId":432,"fileName":"pack.zip","isAvailable":true,"fileLength":4,"downloadUrl":"https://edge.forgecdn.net/pack.zip","hashes":[{"algo":1,"value":"971c419dd609331343dee105fffd0f4608dc0bf2"}]}]}`)), nil
		default:
			t.Fatalf("unexpected request: %s", req.URL)
			return nil, nil
		}
	})}
	result, err := c.SearchModpacks(t.Context(), CurseForgeModpacks, "key", "", "", 0)
	if err != nil || len(result.Projects) != 1 {
		t.Fatalf("projects = %+v, %v", result, err)
	}
	releases, _, err := c.ListModpackReleases(t.Context(), result.Projects[0], "key", "", 0)
	if err != nil || len(releases) != 1 {
		t.Fatalf("releases = %+v, %v", releases, err)
	}
	r := releases[0]
	r.Unavailable = ""
	if _, ok := r.DownloadAsset(); ok || r.Downloadable() {
		t.Fatal("project restriction bypassed through a listed file")
	}
}

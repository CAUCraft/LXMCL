package catalog

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func (c Client) ModrinthVersions(ctx context.Context, project string, i instance.Instance, kind content.Kind) ([]PackVersion, error) {
	if !modrinthID.MatchString(project) || !instance.SupportedRelease(i.Version) {
		return nil, i18n.Error("modrinth.project_or_game_invalid")
	}
	if _, _, err := kind.Format(i); err != nil {
		return nil, err
	}
	return c.ResourceCatalogVersions(ctx, project, instanceResourceFilter(i, kind), kind)
}

func (c Client) ResourceCatalogVersions(ctx context.Context, project string, f ResourceFilter, kind content.Kind) ([]PackVersion, error) {
	if !modrinthID.MatchString(project) || !f.valid(kind) {
		return nil, i18n.Error("modrinth.project_or_filter_invalid")
	}
	params := url.Values{"include_changelog": {"false"}}
	if f.Game != "" {
		versions, _ := json.Marshal([]string{f.Game})
		params.Set("game_versions", string(versions))
	}
	if f.Loader != "" {
		loaders, _ := json.Marshal([]string{f.Loader})
		params.Set("loaders", string(loaders))
	}
	var result []PackVersion
	if err := c.json(ctx, modrinthAPI+"project/"+project+"/version?"+params.Encode(), "", nil, &result); err != nil {
		return nil, err
	}
	return slices.DeleteFunc(result, func(v PackVersion) bool {
		_, _, err := v.DownloadAsset(kind)
		return v.ProjectID != project || !modrinthID.MatchString(v.ID) || !f.matches(v) || err != nil
	}), nil
}

func (c Client) ModrinthVersion(ctx context.Context, project, id string) (PackVersion, error) {
	if !modrinthID.MatchString(project) {
		return PackVersion{}, i18n.Error("modrinth.project_invalid")
	}
	return c.modrinthVersion(ctx, project, id)
}

func (c Client) modrinthVersion(ctx context.Context, project, id string) (PackVersion, error) {
	if (project != "" && !modrinthID.MatchString(project)) || !modrinthID.MatchString(id) {
		return PackVersion{}, i18n.Error("modrinth.project_or_version_invalid")
	}
	var v PackVersion
	if err := c.json(ctx, modrinthAPI+"version/"+id, "", nil, &v); err != nil {
		return PackVersion{}, err
	}
	if v.ID != id || !modrinthID.MatchString(v.ProjectID) || project != "" && v.ProjectID != project {
		return PackVersion{}, i18n.Error("modrinth.version_project_mismatch")
	}
	return v, nil
}

package catalog

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func (v PackVersion) RequiredDependencies() []ModDependency {
	var result []ModDependency
	seen := make(map[[3]string]bool)
	for _, dep := range v.Dependencies {
		if dep.Type != "required" {
			continue
		}
		key := [3]string{dep.ProjectID, dep.VersionID, dep.Filename}
		if dep.ProjectID != "" || dep.VersionID != "" {
			key[2] = ""
		}
		if !seen[key] {
			seen[key] = true
			result = append(result, dep)
		}
	}
	return result
}

func (d ModDependency) Linkable() bool {
	return (d.ProjectID != "" || d.VersionID != "") &&
		(d.ProjectID == "" || modrinthID.MatchString(d.ProjectID)) &&
		(d.VersionID == "" || modrinthID.MatchString(d.VersionID))
}

type modrinthProjectDetails struct {
	ID   string `json:"id"`
	Type string `json:"project_type"`
	PackProject
}

func (p modrinthProjectDetails) kind() content.Kind {
	for _, kind := range []content.Kind{content.Mods, content.ResourcePacks, content.ShaderPacks} {
		if p.Type == modrinthType(kind) {
			return kind
		}
	}
	return ""
}

func (p modrinthProjectDetails) project() PackProject {
	p.PackProject.ID = p.ID
	return p.PackProject
}

// ResourceDependencyProjects looks up names and icons only. It never downloads a resource file.
func (c Client) ResourceDependencyProjects(ctx context.Context, deps []ModDependency) (map[string]PackProject, error) {
	var ids []string
	seen := make(map[string]bool)
	for _, dep := range deps {
		if dep.Linkable() && dep.ProjectID != "" && !seen[dep.ProjectID] {
			ids = append(ids, dep.ProjectID)
			seen[dep.ProjectID] = true
			if len(ids) == 64 {
				break
			}
		}
	}
	projects := make(map[string]PackProject)
	if len(ids) == 0 {
		return projects, nil
	}
	raw, _ := json.Marshal(ids)
	params := url.Values{"ids": {string(raw)}}
	var entries []modrinthProjectDetails
	if err := c.json(ctx, modrinthAPI+"projects?"+params.Encode(), "", nil, &entries); err != nil {
		return nil, err
	}
	for _, p := range entries {
		if seen[p.ID] && strings.TrimSpace(p.Title) != "" {
			projects[p.ID] = p.project()
		}
	}
	return projects, nil
}

type DependencyCatalog struct {
	Project  PackProject
	Kind     content.Kind
	Versions []PackVersion
}

func (c Client) OpenResourceDependency(ctx context.Context, dep ModDependency, target instance.Instance, filter ResourceFilter) (DependencyCatalog, error) {
	var result DependencyCatalog
	if !dep.Linkable() {
		return result, i18n.Error("modrinth.dependency_link_missing")
	}
	projectID := dep.ProjectID
	var pinned PackVersion
	if dep.VersionID != "" {
		v, err := c.modrinthVersion(ctx, projectID, dep.VersionID)
		if err != nil {
			return result, err
		}
		pinned, projectID = v, v.ProjectID
	}
	getProject := func(id string) (modrinthProjectDetails, error) {
		var p modrinthProjectDetails
		err := c.json(ctx, modrinthAPI+"project/"+id, "", nil, &p)
		if err == nil && (p.ID != id || p.Title == "" || p.kind() == "") {
			err = i18n.Error("modrinth.dependency_kind_unsupported")
		}
		return p, err
	}
	p, err := getProject(projectID)
	if err != nil {
		return result, err
	}
	result.Project, result.Kind = p.project(), p.kind()
	if target.ID != 0 {
		if !result.Kind.Supports(target) {
			return result, i18n.Error("modrinth.dependency_instance_unsupported")
		}
		filter = instanceResourceFilter(target, result.Kind)
	} else if result.Kind != content.Mods {
		filter.Loader = ""
	}
	if pinned.ID != "" {
		if _, _, err := pinned.DownloadAsset(result.Kind); err != nil || target.ID != 0 && !pinned.Compatible(target, result.Kind) {
			return result, i18n.Error("modrinth.dependency_pinned_unavailable")
		}
		result.Versions = []PackVersion{pinned}
		return result, nil
	}
	result.Versions, err = c.ResourceCatalogVersions(ctx, projectID, filter, result.Kind)
	if err != nil || len(result.Versions) != 0 || result.Kind != content.Mods || filter.Loader != "quilt" || projectID != fabricAPIProject {
		return result, err
	}
	// An unpinned Fabric API reference can be fulfilled by Quilt's API bundle.
	versions, err := c.ResourceCatalogVersions(ctx, quiltedAPIProject, filter, content.Mods)
	if err != nil {
		return result, err
	}
	if len(versions) != 0 {
		p, err = getProject(quiltedAPIProject)
		if err != nil {
			return result, err
		}
		result.Project, result.Versions = p.project(), versions
		return result, nil
	}
	filter.Loader = "fabric"
	result.Versions, err = c.ResourceCatalogVersions(ctx, projectID, filter, content.Mods)
	return result, err
}

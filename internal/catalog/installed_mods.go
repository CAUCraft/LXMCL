package catalog

import (
	"context"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

type ModMatch struct {
	Title   string
	Current PackVersion
	Update  PackVersion
}

func (c Client) MatchInstalledMods(ctx context.Context, target instance.Instance, hashes []string) (map[string]ModMatch, error) {
	result := make(map[string]ModMatch)
	for batch := range slices.Chunk(hashes, 64) {
		query := map[string]any{"hashes": batch, "algorithm": "sha512"}
		var known, latest map[string]PackVersion
		if err := c.json(ctx, modrinthAPI+"version_files", "", query, &known); err != nil {
			return nil, err
		}
		if len(known) == 0 {
			continue
		}
		query["loaders"] = []string{modrinthModLoader(target)}
		query["game_versions"] = []string{target.Version}
		if err := c.json(ctx, modrinthAPI+"version_files/update", "", query, &latest); err != nil {
			return nil, err
		}
		var projects []ModDependency
		for _, hash := range batch {
			cur := known[hash]
			if !ValidModrinthID(cur.ID) || !ValidModrinthID(cur.ProjectID) || !slices.ContainsFunc(cur.Files, func(f PackFile) bool { return strings.EqualFold(f.Hashes.SHA512, hash) }) {
				continue
			}
			match := ModMatch{Current: cur}
			projects = append(projects, ModDependency{ProjectID: cur.ProjectID})
			next := latest[hash]
			if next.ProjectID == cur.ProjectID && ValidModrinthID(next.ID) && next.ID != cur.ID && next.Published.After(cur.Published) && next.Compatible(target, content.Mods) {
				if _, _, err := next.DownloadAsset(content.Mods); err == nil {
					match.Update = next
				}
			}
			result[hash] = match
		}
		titles, err := c.ResourceDependencyProjects(ctx, projects)
		if err != nil {
			return nil, err
		}
		for _, hash := range batch {
			if m, ok := result[hash]; ok {
				m.Title = titles[m.Current.ProjectID].Title
				result[hash] = m
			}
		}
	}
	return result, nil
}

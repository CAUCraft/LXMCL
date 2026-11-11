package catalog

import (
	"context"
	"path"
	"slices"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/distribution"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/modpack"
	"github.com/CAUCraft/LXMCL/internal/storage"
)

func (cf curseForge) Resolve(ctx context.Context, refs []modpack.Reference) ([]modpack.Download, error) {
	var projectIDs, fileIDs []int
	projects, files := make(map[int]curseProject), make(map[int]curseFile)
	for _, ref := range refs {
		if ref.ProjectID <= 0 || ref.FileID <= 0 {
			return nil, i18n.Error("curseforge.manifest_reference_invalid")
		}
		projectIDs = append(projectIDs, ref.ProjectID)
		fileIDs = append(fileIDs, ref.FileID)
	}
	for ids := range slices.Chunk(uniquePackIDs(projectIDs), 50) {
		var data cursePage[curseProject]
		if err := cf.call(ctx, "mods", map[string][]int{"modIds": ids}, &data); err != nil {
			return nil, err
		}
		for _, p := range data.Data {
			projects[p.ID] = p
		}
	}
	for ids := range slices.Chunk(uniquePackIDs(fileIDs), 50) {
		var data cursePage[curseFile]
		if err := cf.call(ctx, "mods/files", map[string][]int{"fileIds": ids}, &data); err != nil {
			return nil, err
		}
		for _, f := range data.Data {
			files[f.ID] = f
		}
	}
	var result []modpack.Download
	for _, ref := range refs {
		p, f := projects[ref.ProjectID], files[ref.FileID]
		name := p.Name
		if name == "" {
			name = i18n.Text("curseforge.project_label", ref.ProjectID)
		}
		if p.GameID != curseGameID || f.GameID != curseGameID || f.ModID != ref.ProjectID || !p.IsAvailable || !f.IsAvailable ||
			f.IsEarlyAccessContent || f.IsServerPack || f.DownloadURL == "" || (p.AllowModDistribution != nil && !*p.AllowModDistribution) {
			return nil, i18n.Error("curseforge.dependency_unavailable", name, ref.FileID)
		}
		dir := map[int]string{6: "mods", 12: "resourcepacks", 6552: "shaderpacks"}[p.ClassID]
		a := f.asset()
		if dir == "" || !storage.ValidPortablePath(f.FileName) || strings.Contains(f.FileName, "/") || !a.Valid(distribution.MaxFileSize) {
			return nil, i18n.Error("curseforge.dependency_invalid", name, ref.FileID)
		}
		result = append(result, modpack.Download{Path: path.Join(dir, f.FileName), Asset: a})
	}
	return result, nil
}

func uniquePackIDs(ids []int) []int {
	slices.Sort(ids)
	return slices.Compact(ids)
}

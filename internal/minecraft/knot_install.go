package minecraft

import (
	"context"
	"encoding/json"
	"net/url"
	"path/filepath"

	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
	"github.com/CAUCraft/LXMCL/internal/storage"
	"github.com/CAUCraft/LXMCL/internal/transfer"
)

func (g Runtime) installKnot(ctx context.Context, i instance.Instance, d *downloads, report func(Progress)) error {
	loader := knotName(i)
	if _, _, err := g.verifyKnot(ctx, i); err == nil {
		return nil
	}
	report(Progress{Stage: i18n.Text("progress.fetch_loader", loader)})
	var remote struct {
		ID           string
		InheritsFrom string
		MainClass    string
		Arguments    loaderArguments
		Libraries    []struct {
			Name, URL, SHA1 string
			Size            int64
		}
	}
	meta, prefix, main := knotSource(i)
	source := meta + url.PathEscape(i.Version) + "/" + url.PathEscape(i.LoaderVersion) + "/profile/json"
	if err := downloadJSON(ctx, d, source, &remote); err != nil {
		return err
	}
	if remote.InheritsFrom != i.Version || remote.ID != prefix+i.LoaderVersion+"-"+i.Version ||
		remote.MainClass != main || len(remote.Libraries) == 0 {
		return i18n.Error("loader.profile_mismatch", loader)
	}
	p := knotRuntime{GameVersion: i.Version, LoaderVersion: i.LoaderVersion, MainClass: remote.MainClass, Arguments: remote.Arguments}
	var checksums []int
	for _, lib := range remote.Libraries {
		path, err := knotLibraryPath(lib.Name)
		if err != nil {
			return err
		}
		if lib.URL != fabricMaven && lib.URL != "https://libraries.minecraft.net/" &&
			!(i.Quilt() && lib.URL == "https://maven.quiltmc.org/repository/release/") {
			return i18n.Error("loader.library_source_invalid", loader, lib.URL)
		}
		a := artifact{Path: path, URL: lib.URL + path, SHA: lib.SHA1, Size: lib.Size}
		if a.Size == 0 {
			a.Size = unknownSize
		}
		if a.SHA == "" {
			checksums = append(checksums, len(p.Libraries))
		} else if _, err := a.Checksum(); err != nil {
			return err
		}
		p.Libraries = append(p.Libraries, a)
	}
	if err := transfer.Batch(ctx, checksums, func(ctx context.Context, index int) error {
		lib := &p.Libraries[index]
		sha, err := d.fetchSHA1(ctx, lib.URL)
		if err != nil {
			return i18n.Error("loader.library_checksum_failed", filepath.Base(lib.Path), err)
		}
		lib.SHA = sha
		_, err = lib.Checksum()
		return err
	}, nil); err != nil {
		return err
	}
	files, _, err := g.knotFiles(i, p.Libraries)
	if err != nil {
		return err
	}
	if err := d.batch(ctx, i18n.Text("progress.download_and_verify", loader), files, report); err != nil {
		return err
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return storage.WriteFile(filepath.Join(g.knotDir(i), "verified.json"), data)
}

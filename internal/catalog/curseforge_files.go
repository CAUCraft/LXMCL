package catalog

import (
	"net/url"

	"github.com/CAUCraft/LXMCL/internal/distribution"
)

func (f curseFile) asset() distribution.Asset {
	a := distribution.Asset{URLs: []string{f.DownloadURL}, Size: f.FileLength}
	u, err := url.Parse(f.DownloadURL)
	if err != nil || !distribution.CurseCDN(u) || f.FileLength <= 0 {
		return distribution.Asset{}
	}
	for _, algo := range []int{1, 2} {
		for _, h := range f.Hashes {
			if h.Algo != algo {
				continue
			}
			a.Digest, a.Algorithm = h.Value, "sha1"
			if algo == 2 {
				a.Algorithm = "md5"
			}
			if a.Valid(distribution.MaxFileSize) {
				return a
			}
		}
	}
	return distribution.Asset{}
}

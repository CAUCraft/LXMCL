package catalog

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/CAUCraft/LXMCL/internal/content"
	"github.com/CAUCraft/LXMCL/internal/i18n"
	"github.com/CAUCraft/LXMCL/internal/instance"
)

func (c Client) SearchModrinth(ctx context.Context, i instance.Instance, kind content.Kind, query, order string, page int) (PackSearch, error) {
	if !instance.SupportedRelease(i.Version) {
		return PackSearch{}, i18n.Error("version.select_supported")
	}
	if _, _, err := kind.Format(i); err != nil {
		return PackSearch{}, err
	}
	return c.SearchResourceCatalog(ctx, instanceResourceFilter(i, kind), kind, query, order, page)
}

func (c Client) SearchResourceCatalog(ctx context.Context, f ResourceFilter, kind content.Kind, query, order string, page int) (PackSearch, error) {
	projectType := modrinthType(kind)
	if !f.valid(kind) || page < 0 || page > 500 || len(query) > 512 {
		return PackSearch{}, i18n.Error("modrinth.search_invalid")
	}
	if order != "downloads" && order != "updated" {
		order = "relevance"
	}
	filter := [][]string{{"project_type:" + projectType}}
	if f.Game != "" {
		filter = append(filter, []string{"versions:" + f.Game})
	}
	if f.Loader != "" {
		filter = append(filter, []string{"categories:" + f.Loader})
	}
	facets, _ := json.Marshal(filter)
	params := url.Values{"query": {strings.TrimSpace(query)}, "facets": {string(facets)}, "index": {order},
		"limit": {strconv.Itoa(PackPageSize)}, "offset": {strconv.Itoa(PackPageSize * page)}}
	var result PackSearch
	if err := c.json(ctx, modrinthAPI+"search?"+params.Encode(), "", nil, &result); err != nil {
		return PackSearch{}, err
	}
	result.Hits = slices.DeleteFunc(result.Hits, func(p PackProject) bool {
		return !modrinthID.MatchString(p.ID) || p.Title == ""
	})
	return result, nil
}

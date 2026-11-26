package main

import (
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/field"
)

const (
	screenshotModifiedToday = "today"
	screenshotModifiedWeek  = "week"
	screenshotOldestFirst   = "oldest"
)

var screenshotDateOptions = []field.SelectOption{
	{Value: "", Label: "全部日期"},
	{Value: screenshotModifiedToday, Label: "今天"},
	{Value: screenshotModifiedWeek, Label: "最近 7 天"},
}

var screenshotOrderOptions = []field.SelectOption{
	{Value: "", Label: "最近在前"},
	{Value: screenshotOldestFirst, Label: "最早在前"},
}

type screenshotFilterKey struct {
	query, period string
	oldestFirst   bool
	day           time.Time
	location      *time.Location
}

// Use a civil date key so a day with a clock change is still one date.
func screenshotDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func (p *screenshotGallery) visibleRows() []display.ListItem {
	return p.visibleRowsAt(time.Now().Local())
}

func (p *screenshotGallery) visibleRowsAt(now time.Time) []display.ListItem {
	filter := screenshotFilterKey{
		query:       strings.ToLower(strings.TrimSpace(p.query)),
		period:      p.dateFilter,
		oldestFirst: p.order == screenshotOldestFirst,
	}
	useMatches := filter.query != "" || filter.period != "" || filter.oldestFirst
	if filter.period != "" {
		filter.day, filter.location = screenshotDay(now), now.Location()
	}
	if filter != p.appliedFilter {
		p.appliedFilter, p.matches = filter, nil
		start := filter.day
		if filter.period == screenshotModifiedWeek {
			start = start.AddDate(0, 0, -6)
		}
		if useMatches {
			for _, row := range p.rows {
				if filter.query != "" && !strings.Contains(strings.ToLower(row.Key), filter.query) {
					continue
				}
				if filter.period != "" {
					modified := p.entries[row.Key].Modified
					day := screenshotDay(modified.In(filter.location))
					if modified.IsZero() || day.Before(start) || day.After(filter.day) {
						continue
					}
				}
				p.matches = append(p.matches, row)
			}
		}
		if filter.oldestFirst {
			// The original list already orders equal timestamps by filename.
			slices.SortStableFunc(p.matches, func(a, b display.ListItem) int {
				return p.entries[a.Key].Modified.Compare(p.entries[b.Key].Modified)
			})
		}
	}
	if !useMatches {
		return p.rows
	}
	return p.matches
}

func screenshotIsVisible(rows []display.ListItem, name string) bool {
	return name != "" && slices.ContainsFunc(rows, func(row display.ListItem) bool { return row.Key == name })
}

func (a *desktop) currentScreenshotRows(c *ui.Context, p *screenshotGallery) []display.ListItem {
	now := time.Now().Local()
	rows := p.visibleRowsAt(now)
	if p.dateFilter != "" {
		y, m, d := now.Date()
		next := time.Date(y, m, d+1, 0, 0, 0, 0, now.Location())
		if !next.After(now) {
			// Some time zones skip midnight on a clock change.
			next = now.Add(time.Minute)
		}
		c.After(next.Sub(now))
	}
	if p.selectionFilter != p.appliedFilter {
		p.selectionFilter = p.appliedFilter
		p.restrictBatchSelection(rows)
		p.scroll = ui.ScrollState{}
		if p.selected != "" && !screenshotIsVisible(rows, p.selected) {
			a.selectScreenshot(p, "")
		}
		c.Invalidate()
	}
	return rows
}

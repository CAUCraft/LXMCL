package main

import (
	"slices"
	"testing"
	"time"

	"github.com/CAUCraft/LXMCL/internal/screenshots"
	"github.com/lobehub/lobe-mygo/pkg/display"
)

func TestScreenshotModifiedDateFilter(t *testing.T) {
	local, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 3, 10, 1, 0, 0, 0, local)
	midnight := time.Date(2026, 3, 10, 0, 0, 0, 0, local)
	first := midnight.AddDate(0, 0, -6)
	p := &screenshotGallery{entries: make(map[string]screenshots.Entry)}
	for _, e := range []screenshots.Entry{
		{Name: "River-future.png", Modified: midnight.AddDate(0, 0, 1)},
		{Name: "River-today.png", Modified: midnight},
		{Name: "River-today-z.png", Modified: midnight},
		{Name: "Lake-yesterday.png", Modified: midnight.Add(-time.Second)},
		{Name: "River-oldest.png", Modified: first},
		{Name: "River-too-old.png", Modified: first.Add(-time.Second)},
		{Name: "River-unknown.png"},
	} {
		p.rows = append(p.rows, display.ListItem{Key: e.Name})
		p.entries[e.Name] = e
	}
	check := func(at time.Time, want ...string) {
		t.Helper()
		var got []string
		for _, row := range p.visibleRowsAt(at) {
			got = append(got, row.Key)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("visible screenshots = %v, want %v", got, want)
		}
	}
	p.dateFilter = screenshotModifiedToday
	check(now, "River-today.png", "River-today-z.png")
	p.dateFilter = screenshotModifiedWeek
	check(now, "River-today.png", "River-today-z.png", "Lake-yesterday.png", "River-oldest.png")
	p.query = " rIvEr "
	check(now, "River-today.png", "River-today-z.png", "River-oldest.png")
	p.order = screenshotOldestFirst
	check(now, "River-oldest.png", "River-today.png", "River-today-z.png")
	check(midnight.AddDate(0, 0, 1), "River-today.png", "River-today-z.png", "River-future.png")
	p.order = ""
	check(now, "River-today.png", "River-today-z.png", "River-oldest.png")
	p.dateFilter = ""
	check(now, "River-future.png", "River-today.png", "River-today-z.png", "River-oldest.png", "River-too-old.png", "River-unknown.png")
}

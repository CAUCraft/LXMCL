package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/field"
)

func (a *desktop) taskHistoryInstanceLabel(id int) string {
	if i, ok := a.state.Find(id); ok {
		return fmt.Sprintf("#%d · %s", id, i.Name)
	}
	return fmt.Sprintf("#%d · 已移除实例", id)
}

func (a *desktop) taskHistoryInstancePicker(c *ui.Context) {
	options := []field.SelectOption{{Value: "0", Label: "全部实例"}}
	seen := make(map[int]bool)
	for _, row := range a.history {
		id := row.InstanceID
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		options = append(options, field.SelectOption{Value: strconv.Itoa(id), Label: a.taskHistoryInstanceLabel(id)})
	}
	if len(seen) == 0 && a.taskInstanceFilter == 0 {
		return
	}
	fallback := ""
	if a.taskInstanceFilter != 0 && !seen[a.taskInstanceFilter] {
		fallback = a.taskHistoryInstanceLabel(a.taskInstanceFilter)
	}
	selected := strconv.Itoa(a.taskInstanceFilter)
	catalogPicker(c, &selected, options, "筛选历史实例", fallback, false).Width(208).Shrink(1)
	a.taskInstanceFilter, _ = strconv.Atoi(selected)
}

func (a *desktop) matchesTaskHistoryQuery(row taskRecord) bool {
	query := strings.ToLower(strings.TrimSpace(a.taskHistoryQuery))
	return query == "" || strings.Contains(strings.ToLower(row.Name), query) ||
		strings.Contains(strings.ToLower(row.Action), query)
}

func (a *desktop) matchesTaskHistory(row taskRecord) bool {
	return (a.taskInstanceFilter == 0 || row.InstanceID == a.taskInstanceFilter) &&
		(a.taskFilter != 1 || failedOrCanceled(row)) && a.matchesTaskHistoryQuery(row)
}

func (a *desktop) visibleTaskHistory() []taskRecord {
	if a.taskInstanceFilter == 0 && a.taskFilter == 0 && strings.TrimSpace(a.taskHistoryQuery) == "" {
		return a.history
	}
	var visible []taskRecord
	for _, row := range a.history {
		if a.matchesTaskHistory(row) {
			visible = append(visible, row)
		}
	}
	return visible
}

package main

import (
	"github.com/CAUCraft/LXMCL/internal/tasks"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/field"
)

// Filter only actions with stable recorded values. Other action strings can
// still be found through the existing history search.
var taskHistoryActionOptions = []field.SelectOption{
	{Label: "全部操作"},
	{Value: tasks.Launch, Label: tasks.Launch},
	{Value: tasks.Verify, Label: tasks.Verify},
	{Value: tasks.Backup, Label: tasks.Backup},
	{Value: tasks.PrismExportAction, Label: tasks.PrismExportAction},
	{Value: tasks.ModUpdateAction, Label: tasks.ModUpdateAction},
}

func (a *desktop) matchesTaskHistoryAction(row taskRecord) bool {
	return a.taskActionFilter == "" || row.Action == a.taskActionFilter
}

func (a *desktop) taskHistoryActionPicker(c *ui.Context) {
	catalogPicker(c, &a.taskActionFilter, taskHistoryActionOptions, "筛选历史操作", "", false).Width(148).Shrink(0)
}

package tasks

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestModUpdateReportHistory(t *testing.T) {
	root := t.TempDir()
	legacy := (Record{Action: ModUpdateAction, Name: "同名实例", InstanceID: 1, Status: Failed, Detail: "旧摘要"}).Submitted()
	if err := Save(root, []Record{legacy}); err != nil {
		t.Fatal(err)
	}
	record := (Record{Action: ModUpdateAction, Name: legacy.Name, InstanceID: 2, Revision: 7, Status: Canceled,
		ModUpdates: SummarizeModUpdates([]ModUpdateItem{
			{File: "地图.jar.disabled", From: "1.0", To: "2.0+fabric", Status: Completed},
			{File: "/local/private.jar", From: "https://user:secret@example.invalid/", To: "C:\\private", Status: Failed},
		})}).Submitted()
	var journal Journal
	snapshot, err := journal.Record(root, record)
	if err != nil {
		t.Fatal(err)
	}
	record.ModUpdates.Items[0].File = "caller changed"
	snapshot[0].ModUpdates.Items[0].File = "snapshot changed"
	fresh := journal.Snapshot()
	reopened, err := Load(root)
	if err != nil || len(reopened) != 2 || len(fresh) != 2 {
		t.Fatal(reopened, err)
	}
	for _, rows := range [][]Record{fresh, reopened} {
		if rows[0].InstanceID != 2 || rows[0].Revision != 7 || rows[0].ModUpdates.Items[0].File != "地图.jar.disabled" ||
			rows[0].ModUpdates.Items[0].To != "2.0+fabric" || rows[0].ModUpdates.Items[1] != (ModUpdateItem{Status: Failed}) ||
			rows[1].Key() != legacy.Key() || rows[1].Detail != legacy.Detail || len(rows[1].ModUpdates.Items) != 0 {
			t.Fatal(rows)
		}
	}

	// JSON escaping can make a portable filename larger than its utf-8 byte count.
	many := make([]ModUpdateItem, 1000)
	for i := range many {
		many[i] = ModUpdateItem{File: strings.Repeat("&", 240) + ".jar", From: "1.0", To: "2.0", Status: Completed}
	}
	report := SummarizeModUpdates(many)
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil || len(b) > maxModUpdateReportBytes || len(report.Items) == 0 ||
		report.Omitted == 0 || len(report.Items)+report.Omitted != len(many) {
		t.Fatal(len(b), len(report.Items), report.Omitted, err)
	}
}

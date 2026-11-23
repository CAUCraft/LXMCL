package main

import (
	"context"
	"testing"
)

func TestTaskBackupPreviewIsReadOnly(t *testing.T) {
	a, tt := testDesktop(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // The real archive reader is covered by the managed backup tests.
	a.ctx = ctx
	row := taskRecord{
		ID: "saved-backup", InstanceID: 7, Name: "旧实例", SaveArchive: "世界.zip",
		Action: taskBackup, Status: taskCompleted,
	}
	a.history, a.browsedTask = []taskRecord{row}, row.Key()
	a.openTasks()
	tt.Frame()
	if err := tt.Click("预览备份…"); err != nil {
		t.Fatal(err)
	}
	p := a.restore
	if p == nil || !p.readOnly || p.target.ID != row.InstanceID || p.filename != row.SaveArchive {
		t.Fatal("preview lost the recorded archive or instance", p)
	}
	p.loading, p.worlds = false, []string{"南岛", "北岛"}
	tt.Frame()
	if !tt.HasText("原实例 · 旧实例 · #7") || !tt.HasText("南岛") || !tt.HasText("北岛") {
		t.Fatal(tt.Texts())
	}
	for _, label := range []string{"恢复世界", "恢复全部世界", "重新选择 ZIP…"} {
		if _, visible := tt.Access(label); visible {
			t.Fatal("write or retarget action in read-only preview", label)
		}
	}
	if a.startRestore(p) || a.busy {
		t.Fatal("read-only preview started a restore")
	}
	if err := tt.Click("关闭"); err != nil {
		t.Fatal(err)
	}
	if a.restore != nil || a.browsedTask != row.Key() || a.state.Selected != 1 {
		t.Fatal("closing preview changed the task or the home instance")
	}
}

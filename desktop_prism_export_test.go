package main

import (
	"testing"
	"time"
)

func TestPrismExportScreenshotOption(t *testing.T) {
	a, tt := testDesktop(t)
	tt.SetSize(820, 530)
	click := func(labels ...string) {
		t.Helper()
		for _, label := range labels {
			if err := tt.Click(label); err != nil {
				t.Fatal(label, err, tt.Texts())
			}
			tt.Frame()
		}
	}
	const option = "游戏截图（PNG）"
	open := func() {
		t.Helper()
		click("更多实例操作", "导出实例 ZIP…")
		if a.form.prismExport == nil || a.form.prismExport.screenshots || a.form.target.ID != 2 || a.state.Selected != 1 {
			t.Fatal("export did not start unchecked on the inspected instance")
		}
	}
	click("实例", "建筑测试")
	open()
	tt.Advance(300 * time.Millisecond)
	capture(t, tt, "prism-export-screenshots-default-light")
	click(option)
	tt.SetDark(true)
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	if input, ok := tt.Access(option); !ok || !input.Checked || !a.form.prismExport.screenshots {
		t.Fatal("export checkbox did not retain the explicit opt-in")
	}
	if button, ok := tt.Access("选择目标文件…"); !ok || button.Disabled || button.Bounds.Y+button.Bounds.H > 530 {
		t.Fatal("export submit is not accessible at 820×530", button)
	}
	capture(t, tt, "prism-export-screenshots-selected-dark")
	click("取消")
	open()
}

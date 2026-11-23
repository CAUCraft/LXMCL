package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const commandUsage = "用法：LXMCL [--data-dir 目录] [--launch 实例ID]\n在「实例 → 概览」查看实例 ID。"

type commandLine struct {
	dataDir  string
	launchID int
}

func parseCommandLine(args []string) (commandLine, error) {
	var cmd commandLine
	flags := flag.NewFlagSet("LXMCL", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&cmd.dataDir, "data-dir", "", "使用独立的数据目录")
	var idErr error
	flags.Func("launch", "按实例 ID 启动", func(s string) error {
		id, err := strconv.ParseUint(s, 10, strconv.IntSize-1)
		switch {
		case err != nil || id == 0:
			idErr = errors.New("实例 ID 必须是正十进制整数")
		case cmd.launchID != 0:
			idErr = errors.New("一次只能指定一个启动实例")
		default:
			cmd.launchID = int(id)
		}
		return idErr
	})
	err := flags.Parse(args)
	switch {
	case idErr != nil:
		return commandLine{}, idErr
	case errors.Is(err, flag.ErrHelp):
		return commandLine{}, err
	case err != nil || flags.NArg() != 0:
		return commandLine{}, errors.New("命令行参数无效，请检查选项及其值")
	}
	return cmd, nil
}

func profilePath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func (a *desktop) secondInstance(args []string, _ string) {
	if a.window != nil {
		a.window.Restore()
		a.window.Focus()
		defer a.window.Invalidate()
	}
	cmd, err := parseCommandLine(args)
	if err != nil {
		a.message("无法处理启动命令", err)
		return
	}
	if cmd.launchID != 0 {
		a.launchByID(cmd.launchID)
	}
}

func (a *desktop) launchByID(id int) {
	if !a.ready {
		switch {
		case a.loadErr != "":
			a.message("无法启动实例", errors.New("请重新读取本机配置后再次运行启动命令"))
		case a.pendingLaunchID != 0:
			a.message("已有启动请求，请稍后重试", nil)
		default:
			a.pendingLaunchID = id
		}
		return
	}
	target, ok := a.state.Find(id)
	switch {
	case !ok:
		a.message("无法启动实例", fmt.Errorf("找不到实例 ID %d，请在实例概览核对 ID", id))
	case a.busy || a.running != nil:
		a.message("无法启动实例", errors.New("请在当前任务或游戏结束后重试"))
	default:
		a.play(target, false)
	}
}

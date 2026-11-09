package main

import (
	"context"
	_ "embed"
	"flag"
	"log"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/CAUCraft/LXMCL/internal/launcher"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

//go:embed ui/assets/launcher-logo.png
var logoPNG []byte

//go:embed ui/assets/daylight-river.png
var homePNG []byte

func main() {
	dataDir := flag.String("data-dir", "", "use a separate launcher data directory")
	flag.Parse()
	store, err := launcher.DefaultStore()
	if *dataDir != "" {
		store.Root, err = filepath.Abs(*dataDir)
	}
	if err != nil {
		log.Fatal(err)
	}
	logo, err := ui.DecodeImage(logoPNG)
	if err != nil {
		log.Fatal(err)
	}
	home, err := ui.DecodeImage(homePNG)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &desktop{
		store: store, game: launcher.Game{Cache: filepath.Join(store.Root, "cache")},
		ctx: ctx, logo: logo, hero: home, installed: make(map[int]bool),
	}
	mygo.App.SetName("LXMCL")
	mygo.App.WhenReady(func() {
		opts := mygo.WindowOptions{
			Title: "LXMCL", Width: 1140, Height: 720, MinWidth: 820, MinHeight: 530,
			Content: ui.View(a.view), BackgroundColor: "light-dark(#f5f5f5, #161616)",
		}
		if runtime.GOOS == "windows" {
			opts.TitleBarStyle = mygo.TitleBarHidden
			opts.TitleBarHeight = 54
		}
		a.window = mygo.NewWindow(opts)
		a.window.OnClosed(cancel)
		a.load()
	})
	if err := mygo.App.Run(); err != nil {
		log.Print(err)
	}
	cancel()
	// Accepted configuration writes finish even when the user closes the window.
	a.writes.Wait()
}

type editor struct {
	open    bool
	mode    string
	target  launcher.Instance
	name    string
	version string
	err     string
	focus   bool
}

type desktop struct {
	window      *mygo.Window
	ctx         context.Context
	store       launcher.Store
	game        launcher.Game
	logo, hero  ui.ImageSource
	writes      sync.WaitGroup
	state       launcher.State
	draft       launcher.Settings
	memory      string
	ready       bool
	loadErr     string
	notice      string
	noticeError bool

	page, settingsPage, libraryPage int
	inspected                       int
	query                           string
	scroll                          ui.ScrollState
	form                            editor
	installed                       map[int]bool
	releases                        []launcher.Version
	catalogBusy                     bool
	catalogErr                      string
	javaInfo                        string

	busy       bool
	cancelTask context.CancelFunc
	task       launcher.Progress
	taskName   string
	running    *launcher.Running
	runningID  int
	lastLog    string
}

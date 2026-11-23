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
	launcherlog "github.com/CAUCraft/LXMCL/internal/log"
	"github.com/CAUCraft/LXMCL/internal/playtime"
	"github.com/CAUCraft/LXMCL/internal/tasks"
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
	if *dataDir == "" && mygo.IsDev() {
		store.Root = filepath.Join(store.Root, "dev")
	}
	mygo.App.SetName("LXMCL")
	// An isolated development directory may have its own window. In the normal
	// data directory, keep a single writer for configuration and the game cache.
	if *dataDir == "" && !mygo.App.RequestSingleInstanceLock() {
		return
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
	mygo.App.OnSecondInstance(func([]string, string) {
		if a.window != nil {
			a.window.Restore()
			a.window.Focus()
		}
	})
	mygo.App.WhenReady(func() {
		opts := mygo.WindowOptions{
			Title: "LXMCL", Width: 1140, Height: 720, MinWidth: 820, MinHeight: 530,
			Content: ui.View(a.view), BackgroundColor: "light-dark(#f5f5f5, #161616)",
		}
		if runtime.GOOS == "windows" {
			opts.TitleBarStyle = mygo.TitleBarHidden
			opts.TitleBarHeight = titleBarHeight
		}
		a.window = mygo.NewWindow(opts)
		a.window.OnClosed(cancel)
		a.load()
	})
	runErr := mygo.App.Run()
	cancel()
	// Drain accepted writes and canceled downloads; never wait for an open game.
	a.writes.Wait()
	if runErr != nil {
		log.Fatal(runErr)
	}
}

type editor struct {
	open        bool
	mode        string
	target      launcher.Instance
	name        string
	version     string
	err         string
	focus       bool
	copyGroup   string
	copyContent bool
	loaders     *loaderSelection
	files       launcher.PortableFiles

	note, originalNote string
	originalGroup      string
	groupMove          *groupMoveDraft
	icon               *instanceIconDraft
	prism              *prismDraft
	worlds             *worldBrowser
}

type desktop struct {
	journal          tasks.Journal
	playtimes        map[int]playtime.Entry
	playtimeErr      error
	instanceIcons    map[int]ui.ImageSource
	historyReadErr   string
	historyReloading bool
	historyRetried   bool

	window           *mygo.Window
	ctx              context.Context
	store            launcher.Store
	game             launcher.Game
	logo, hero       ui.ImageSource
	writes           sync.WaitGroup
	state            launcher.State
	draft            launcher.Settings
	memory           string
	ready, loading   bool
	loadErr          string
	notice           string
	noticeError      bool
	noticeJava       bool
	noticeTaskKey    string
	noticeAccount    bool
	noticeHistory    bool
	noticeJavaTarget launcher.Instance
	microsoftAccount launcher.MicrosoftAccount
	minecraftSession launcher.MinecraftSession
	microsoft        *microsoftLoginView
	appearance       *appearanceView
	browsedAccount   string
	newOfflinePlayer string
	accountReturn    int
	stationLogin     *stationLoginView
	stationSessions  map[string]launcher.StationSession
	stationPreviews  map[string]*stationPreview
	browsedServer    string
	serverSource     int
	gameServers      *gameServerView

	page, settingsPage, downloadPage int
	inspected                        int
	detailPage                       int
	launchDrafts                     map[int]*instanceLaunchDraft
	screenshots                      *screenshotGallery
	content                          contentView
	contentRequest                   uint64
	contentCancel                    context.CancelFunc
	install                          installView
	modpacks                         modpackView
	packs                            map[launcher.ContentKind]*packBrowser
	catalogIcons                     map[string]catalogIcon
	catalogIconSlots                 chan struct{}
	downloadFolder                   string
	serverForm                       *serverForm
	serverStatus                     map[string]*serverCheck
	serverInstanceID                 int
	query                            string
	focusSearch                      bool
	instanceFilter                   int
	libraryGroup                     *string
	listMenuID                       int
	taskFilter                       int
	browsedTask                      string
	scroll                           ui.ScrollState
	form                             editor
	installed                        map[int]bool
	releases                         []launcher.Version
	queue                            resourceQueue
	release                          releaseView
	logDiagnosis                     launcherlog.Diagnosis
	catalogBusy                      bool
	catalogErr                       string
	javaInfo                         string
	javaChecking, javaError          bool

	busy          bool
	cancelTask    context.CancelFunc
	taskCanceling bool
	task          launcher.Progress
	taskName      string
	running       *launcher.Running
	runningID     int
	lastLog       string
	history       []taskRecord
	logOpen       bool
	logLibrary    *instanceLogView
	restore       *restoreView
	backups       *backupView
	logText       string
	logPath       string
	logRequest    uint64
	logCancel     context.CancelFunc
	logReading    bool
	logExporting  bool
	logExported   bool
	logExportErr  string
	logError      string
	logQuery      string
	logFilter     int
	logScroll     ui.ScrollState
	licensesOpen  bool
	licenseFile   string
	pickerBusy    bool
}

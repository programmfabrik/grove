//go:build desktop

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// The desktop front door: the same dashboard, in a window of its own, built
// with `-tags desktop`. The default build has none of this in it — no cgo, no
// webview — so `go install` still produces the plain command.
//
// The window loads http://127.0.0.1:PORT rather than being served through the
// webview's own asset scheme. That is the whole design: the page then sits on
// an ordinary HTTP origin and everything it already does keeps working —
// range requests, so <video> in a diff can seek; a secure context, so the
// clipboard is there; caching, so a reload is not a re-download. Wails' asset
// server would have put it on a custom scheme and put all three in doubt.
//
// It also means the page and the window share no JavaScript bridge, which is
// less of a loss than it sounds: the page already talks to Go over HTTP, and
// everything native — the folder dialog, the menu — is driven from this side.

// desktopPort is where the window looks first. It walks a small range rather
// than taking whatever port the OS offers, because the page's remembered pane
// widths, folds and theme live in localStorage — which is keyed by ORIGIN, so
// a port that moved every launch would hand back an empty dashboard each time.
const (
	desktopPort  = 7433
	desktopPorts = 10
)

func init() { distKind = "app" }

func run(d *grove, addr string, explicit, _ bool) error {
	rememberOrRecallDir(d)

	ln, err := listenDesktop(addr, explicit)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: d.routes(loopbackListener(ln))}
	go srv.Serve(ln)
	url := dashboardURL(ln)

	// "Grove" with a capital: the command you type is `grove`, and the
	// application the window belongs to is a proper noun like every other one
	// in the menu bar. The name reaches the About and Quit items through
	// Options.Name, and the menu bar itself through the executable's name,
	// which is why the Makefile builds bin/Grove and not bin/grove-app.
	app := application.New(application.Options{
		Name:        "Grove",
		Description: aboutText(),
	})
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Grove",
		Width:     1400,
		Height:    900,
		MinWidth:  700,
		MinHeight: 420,
		URL:       url,
	})
	app.Menu.SetApplicationMenu(desktopMenu(app, d, win, url))
	// the settings window has a Choose… button, and only this front door has a
	// dialog to put behind it
	pickFolder = func() { chooseFolder(app, d, win, url) }
	pickApp = func(kind string) (string, string, bool) { return chooseApp(app, kind) }

	// Nothing to show means a first run, or a remembered directory that has
	// since gone. Either way an empty window explains nothing, so ask — once
	// the application is up, since a dialog needs one.
	if len(d.reposList(context.Background())) == 0 {
		app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
			chooseFolder(app, d, win, url)
		})
	}
	return app.Run()
}

// listenDesktop keeps the origin stable across launches — see desktopPort. An
// explicit -addr still wins: somebody who named a port meant it.
func listenDesktop(addr string, explicit bool) (net.Listener, error) {
	if explicit {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("cannot listen on %s: %w", addr, err)
		}
		return ln, nil
	}
	for p := desktopPort; p < desktopPort+desktopPorts; p++ {
		if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p)); err == nil {
			return ln, nil
		}
	}
	// every port in the range is taken, which is stranger than it is fatal
	return net.Listen("tcp", "127.0.0.1:0")
}

func desktopMenu(app *application.App, d *grove, win application.Window, url string) *application.Menu {
	menu := application.NewMenu()

	// The application menu is built by hand rather than taken from AppMenu,
	// for one item: Settings belongs under the application's own name with
	// cmd-comma on it, which is where every Mac user looks for it and where
	// no amount of buttons in a web toolbar will make them look instead.
	// Wails has no role for it, so the rest of the menu is reproduced around it.
	if runtime.GOOS == "darwin" {
		appMenu := menu.AddSubmenu("Grove")
		// not the About role: that opens the system panel, which can say what
		// is in Info.plist and nothing else — no git, no platform. This is the
		// same native panel with something worth reading in it.
		appMenu.Add("About Grove").OnClick(func(*application.Context) {
			app.Menu.ShowAbout()
		})
		appMenu.AddSeparator()
		appMenu.Add("Settings…").SetAccelerator("cmd+,").OnClick(func(*application.Context) {
			openSettings(app, url)
		})
		appMenu.AddSeparator()
		appMenu.AddRole(application.ServicesMenu)
		appMenu.AddSeparator()
		appMenu.AddRole(application.Hide)
		appMenu.AddRole(application.HideOthers)
		appMenu.AddRole(application.UnHide)
		appMenu.AddSeparator()
		appMenu.AddRole(application.Quit)
	}

	file := menu.AddSubmenu("File")
	file.Add("Open Folder…").SetAccelerator("cmdorctrl+o").OnClick(func(*application.Context) {
		chooseFolder(app, d, win, url)
	})
	file.Add("Refresh").SetAccelerator("cmdorctrl+r").OnClick(func(*application.Context) {
		d.dropCaches() // the panes poll, so they pick it up on their own
	})
	file.AddSeparator()
	file.Add("Reload Window").SetAccelerator("cmdorctrl+shift+r").OnClick(func(*application.Context) {
		win.Reload()
	})
	// everywhere without an application menu, Settings goes here
	if runtime.GOOS != "darwin" {
		file.AddSeparator()
		file.Add("Settings…").SetAccelerator("ctrl+,").OnClick(func(*application.Context) {
			openSettings(app, url)
		})
	}

	menu.AddRole(application.EditMenu) // grove copies shas and paths
	menu.AddRole(application.ViewMenu)
	menu.AddRole(application.WindowMenu)
	return menu
}

// rememberOrRecallDir settles which directory the window opens on. A command
// is started in the directory it is meant to show; an application is started
// from an icon, in `/`, where there is nothing to show and never will be. So
// the working directory wins when it actually holds repositories — which is
// what a launch from a terminal means — and otherwise the last directory this
// app was pointed at does. An explicit -dir beats both and is not recorded,
// since it was a one-off instruction rather than a change of mind.
func rememberOrRecallDir(d *grove) {
	if isSet("dir") {
		return
	}
	if len(d.reposList(context.Background())) == 0 {
		if last := loadSettings().Dir; last != "" {
			// an error here means the remembered directory is gone too, and
			// the window opens empty with Open Folder… waiting
			d.setDir(last)
		}
		return
	}
	remember(d.dir())
}

// aboutText is what About says: what this grove is, and what it is running on.
// The version of git in particular belongs here rather than in Settings — it
// is a fact about the machine, not something anybody can change, and a
// settings screen full of read-only facts is how a settings screen stops being
// read at all.
//
// The panel is a narrow column of proportional text, which rules out the
// obvious layout: padding labels with spaces lines nothing up when every
// character is a different width, and a path on the end of a longer line wraps
// through the middle of itself. So each fact is a short line of its own, and
// the path — the longest thing here and the one worth reading exactly — gets
// the whole of one.
func aboutText() string {
	lines := []string{
		"Every checkout, every worktree, and what each of them changed.",
		"",
		"Version " + version + " · " + runtime.GOOS + "/" + runtime.GOARCH,
	}
	if v, err := git("", "--version"); err == nil {
		lines = append(lines, "git "+strings.TrimPrefix(v, "git version "))
	}
	lines = append(lines, gitExe)
	return strings.Join(lines, "\n")
}

// openSettings shows what grove is standing on, in a window of its own.
//
// A settings panel inside the dashboard is a panel; a separate window with the
// application's name on it, opened by cmd-comma, is where somebody looks for
// one. The contents are still the page — there is no native form toolkit here
// and pretending otherwise would mean two settings screens to keep in step —
// but the window, the menu item and the shortcut are the operating system's.
func openSettings(app *application.App, url string) {
	if w, ok := app.Window.GetByName("settings"); ok {
		w.Show().Focus()
		return
	}
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:          "settings",
		Title:         "Grove Settings",
		Width:         640,
		Height:        680,
		MinWidth:      420,
		MinHeight:     360,
		DisableResize: false,
		URL:           url + "?view=settings",
	})
}

// chooseApp lets somebody point at any application on the disk, which is the
// only honest answer to "the list should be what I actually have": no
// catalogue is ever complete, and a bundle nobody guessed still opens.
func chooseApp(app *application.App, kind string) (string, string, bool) {
	path, err := app.Dialog.OpenFile().
		CanChooseDirectories(false).
		CanChooseFiles(true).
		TreatsFilePackagesAsDirectories(false). // an .app is picked, not opened
		SetTitle("Choose a "+kind).
		SetDirectory("/Applications").
		AddFilter("Applications", "*.app").
		PromptForSingleSelection()
	if err != nil || path == "" {
		return "", "", false
	}
	return strings.TrimSuffix(filepath.Base(path), ".app"), path, true
}

// chooseFolder is what makes the window an application rather than a page:
// without it the directory can only come from the shell that started grove,
// and an app is not started from a shell.
func chooseFolder(app *application.App, d *grove, win application.Window, url string) {
	// Where the dialog opens matters more than it looks. On a first run the
	// working directory is "/" — an icon has no shell to have been started in
	// — and dropping somebody at the root of the disk to go looking is no help
	// at all.
	start := d.dir()
	if start == "" || start == string(filepath.Separator) || !hasRepos(start) {
		start = likelyStartDir()
	}
	dir, err := app.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		SetTitle("Choose a directory of repositories").
		SetMessage("Grove shows every checkout and worktree in one directory.").
		SetDirectory(start).
		PromptForSingleSelection()
	if err != nil || dir == "" {
		return // cancelled
	}
	if err := d.setDir(normPath(dir)); err != nil {
		app.Dialog.Error().SetMessage(err.Error()).Show()
		return
	}
	// setDir has already written the directory and the recents; nothing else
	// to save, and nothing else to accidentally overwrite
	// SetURL rather than Reload: the fragment names a repo, a worktree and a
	// file of the directory just left, and none of them is there any more
	win.SetURL(url)
}

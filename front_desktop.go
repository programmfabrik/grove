//go:build desktop

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/wailsapp/wails/v3/pkg/application"
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
		Description: "Every checkout, every worktree, and what each of them changed.\n\nVersion " + version,
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
	menu.AddRole(application.AppMenu)

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
	settings{Dir: d.dir()}.save()
}

// chooseFolder is what makes the window an application rather than a page:
// without it the directory can only come from the shell that started grove,
// and an app is not started from a shell.
func chooseFolder(app *application.App, d *grove, win application.Window, url string) {
	dir, err := app.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		SetTitle("Choose a directory of repositories").
		SetDirectory(d.dir()).
		PromptForSingleSelection()
	if err != nil || dir == "" {
		return // cancelled
	}
	if err := d.setDir(normPath(dir)); err != nil {
		app.Dialog.Error().SetMessage(err.Error()).Show()
		return
	}
	settings{Dir: d.dir()}.save() // so the next launch from the icon lands here
	// SetURL rather than Reload: the fragment names a repo, a worktree and a
	// file of the directory just left, and none of them is there any more
	win.SetURL(url)
}

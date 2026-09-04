package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The three programs grove hands things to: a browser for a link, a terminal
// for a checkout, an editor for a file.
//
// It does not bring its own. Somebody who reads diffs all day already has an
// editor they mean, and a dashboard that opened a different one would be
// wrong in a way no setting could excuse — so the ones on this machine are
// found, listed, and one of each is chosen.
//
// Editors are opened through the command line tool inside their bundle rather
// than through the app, because that is the difference between "open this
// file" and "open this file at line 412". None of VS Code, Cursor, Zed or
// Sublime puts that tool on the PATH by default, and all four ship it inside
// the .app, so that is where it is looked for.

// known is what grove knows how to open things with. App is the macOS bundle
// name, CLI the tool inside it, Bin a name on the PATH, and Line how that tool
// wants to be told which line.
type known struct {
	ID, Name, Kind string
	App            string
	CLI            string // relative to the bundle
	Bin            string
	Line           string // goto | colon | plus | none
}

// The catalogue is not the list of what you have — that is whatever is on the
// disk, and Choose an application… reaches all of it. This is what grove knows
// something EXTRA about: where the command line tool hides inside the bundle,
// and how to tell it a line number. An app that is not here still works; it
// just opens the file rather than the line.
var catalogue = []known{
	// browsers — a link, no line to worry about
	{ID: "safari", Name: "Safari", Kind: "browser", App: "Safari"},
	{ID: "safari-tp", Name: "Safari Technology Preview", Kind: "browser", App: "Safari Technology Preview"},
	{ID: "chrome", Name: "Google Chrome", Kind: "browser", App: "Google Chrome"},
	{ID: "chrome-canary", Name: "Google Chrome Canary", Kind: "browser", App: "Google Chrome Canary"},
	{ID: "chromium", Name: "Chromium", Kind: "browser", App: "Chromium"},
	{ID: "firefox", Name: "Firefox", Kind: "browser", App: "Firefox"},
	{ID: "firefox-dev", Name: "Firefox Developer Edition", Kind: "browser", App: "Firefox Developer Edition"},
	{ID: "arc", Name: "Arc", Kind: "browser", App: "Arc"},
	{ID: "brave", Name: "Brave Browser", Kind: "browser", App: "Brave Browser"},
	{ID: "edge", Name: "Microsoft Edge", Kind: "browser", App: "Microsoft Edge"},
	{ID: "vivaldi", Name: "Vivaldi", Kind: "browser", App: "Vivaldi"},
	{ID: "opera", Name: "Opera", Kind: "browser", App: "Opera"},
	{ID: "orion", Name: "Orion", Kind: "browser", App: "Orion"},
	{ID: "zen", Name: "Zen", Kind: "browser", App: "Zen"},
	{ID: "librewolf", Name: "LibreWolf", Kind: "browser", App: "LibreWolf"},

	// terminals — a directory to start in. Nothing in a bundle says "I am a
	// terminal", so unlike the other two this list is the only way grove
	// recognises one by itself; anything missing is still reachable through
	// Choose an application….
	{ID: "warp", Name: "Warp", Kind: "terminal", App: "Warp"},
	{ID: "ghostty", Name: "Ghostty", Kind: "terminal", App: "Ghostty"},
	{ID: "iterm", Name: "iTerm", Kind: "terminal", App: "iTerm"},
	{ID: "kitty", Name: "kitty", Kind: "terminal", App: "kitty"},
	{ID: "alacritty", Name: "Alacritty", Kind: "terminal", App: "Alacritty"},
	{ID: "wezterm", Name: "WezTerm", Kind: "terminal", App: "WezTerm"},
	{ID: "rio", Name: "Rio", Kind: "terminal", App: "Rio"},
	{ID: "hyper", Name: "Hyper", Kind: "terminal", App: "Hyper"},
	{ID: "tabby", Name: "Tabby", Kind: "terminal", App: "Tabby"},
	{ID: "terminal", Name: "Terminal", Kind: "terminal", App: "Terminal"},

	// editors — a file, and the line it is on
	{ID: "vscode", Name: "Visual Studio Code", Kind: "editor", App: "Visual Studio Code",
		CLI: "Contents/Resources/app/bin/code", Bin: "code", Line: "goto"},
	{ID: "cursor", Name: "Cursor", Kind: "editor", App: "Cursor",
		CLI: "Contents/Resources/app/bin/cursor", Bin: "cursor", Line: "goto"},
	{ID: "windsurf", Name: "Windsurf", Kind: "editor", App: "Windsurf",
		CLI: "Contents/Resources/app/bin/windsurf", Bin: "windsurf", Line: "goto"},
	{ID: "zed", Name: "Zed", Kind: "editor", App: "Zed",
		CLI: "Contents/MacOS/cli", Bin: "zed", Line: "colon"},
	{ID: "sublime", Name: "Sublime Text", Kind: "editor", App: "Sublime Text",
		CLI: "Contents/SharedSupport/bin/subl", Bin: "subl", Line: "colon"},
	{ID: "goland", Name: "GoLand", Kind: "editor", App: "GoLand",
		CLI: "Contents/MacOS/goland", Bin: "goland", Line: "goland"},
	{ID: "idea", Name: "IntelliJ IDEA", Kind: "editor", App: "IntelliJ IDEA",
		CLI: "Contents/MacOS/idea", Bin: "idea", Line: "goland"},
	{ID: "vscodium", Name: "VSCodium", Kind: "editor", App: "VSCodium",
		CLI: "Contents/Resources/app/bin/codium", Bin: "codium", Line: "goto"},
	{ID: "webstorm", Name: "WebStorm", Kind: "editor", App: "WebStorm",
		CLI: "Contents/MacOS/webstorm", Bin: "webstorm", Line: "goland"},
	{ID: "pycharm", Name: "PyCharm", Kind: "editor", App: "PyCharm",
		CLI: "Contents/MacOS/pycharm", Bin: "pycharm", Line: "goland"},
	{ID: "rubymine", Name: "RubyMine", Kind: "editor", App: "RubyMine",
		CLI: "Contents/MacOS/rubymine", Bin: "rubymine", Line: "goland"},
	{ID: "textmate", Name: "TextMate", Kind: "editor", App: "TextMate",
		CLI: "Contents/Resources/mate", Bin: "mate", Line: "textmate"},
	{ID: "nova", Name: "Nova", Kind: "editor", App: "Nova", Bin: "nova", Line: "colon"},
	{ID: "typora", Name: "Typora", Kind: "editor", App: "Typora"},
	{ID: "xcode", Name: "Xcode", Kind: "editor", App: "Xcode"},
	{ID: "textedit", Name: "TextEdit", Kind: "editor", App: "TextEdit"},
	{ID: "bbedit", Name: "BBEdit", Kind: "editor", App: "BBEdit", Bin: "bbedit", Line: "plus"},
	{ID: "helix", Name: "Helix", Kind: "editor", Bin: "hx", Line: "colon"},
	{ID: "nvim", Name: "Neovim", Kind: "editor", Bin: "nvim", Line: "plus"},
	{ID: "vim", Name: "Vim", Kind: "editor", Bin: "vim", Line: "plus"},
	{ID: "emacs", Name: "Emacs", Kind: "editor", Bin: "emacs", Line: "plus"},
	{ID: "micro", Name: "micro", Kind: "editor", Bin: "micro", Line: "plus"},
}

// Program is one that is actually here.
type Program struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	// How says which way it will be run: "cli" can be told a line number and
	// "app" cannot, which is worth saying out loud next to an editor.
	How  string `json:"how"`
	Path string `json:"path"`
	Line bool   `json:"line"` // can open at a line
}

var (
	foundOnce sync.Mutex
	foundAt   time.Time
	foundList []Program
)

// findPrograms looks once a minute rather than on every settings render:
// stat-ing thirty bundles is cheap and doing it per keystroke is not.
func findPrograms() []Program {
	foundOnce.Lock()
	defer foundOnce.Unlock()
	if time.Since(foundAt) < time.Minute && foundList != nil {
		return foundList
	}
	var out []Program
	for _, k := range catalogue {
		if p, ok := k.find(); ok {
			out = append(out, p)
		}
	}
	foundList, foundAt = out, time.Now()
	return out
}

func (k known) find() (Program, bool) {
	p := Program{ID: k.ID, Name: k.Name, Kind: k.Kind}
	// the tool inside the bundle first: it is the one that knows about lines
	if runtime.GOOS == "darwin" && k.App != "" {
		for _, dir := range appDirs() {
			bundle := filepath.Join(dir, k.App+".app")
			if _, err := os.Stat(bundle); err != nil {
				continue
			}
			if k.CLI != "" {
				cli := filepath.Join(bundle, k.CLI)
				if st, err := os.Stat(cli); err == nil && st.Mode()&0o111 != 0 {
					p.How, p.Path, p.Line = "cli", cli, k.Line != "" && k.Line != "none"
					return p, true
				}
			}
			p.How, p.Path = "app", bundle
			return p, true
		}
	}
	if k.Bin != "" {
		if bin, err := lookPath(k.Bin); err == nil {
			p.How, p.Path, p.Line = "cli", bin, k.Line != "" && k.Line != "none"
			return p, true
		}
	}
	return Program{}, false
}

func appDirs() []string {
	dirs := []string{"/Applications", "/System/Applications", "/System/Applications/Utilities"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "Applications"))
	}
	return dirs
}

// programByID resolves what was chosen. A plain id is one grove knows about;
// "path:/Applications/Whatever.app" is one somebody pointed at themselves,
// which is how the choice reaches every program on the disk rather than only
// the ones this file has heard of.
func programByID(id string) (known, Program, bool) {
	if id == "" {
		return known{}, Program{}, false
	}
	if path, ok := strings.CutPrefix(id, "path:"); ok {
		if _, err := os.Stat(path); err != nil {
			return known{}, Program{}, false
		}
		name := strings.TrimSuffix(filepath.Base(path), ".app")
		// a bundle grove happens to know is still opened the way it knows how,
		// so choosing VS Code by hand does not lose the line numbers
		for _, k := range catalogue {
			if k.App != "" && k.App == name {
				if p, found := k.find(); found {
					return k, p, true
				}
			}
		}
		return known{}, Program{ID: id, Name: name, How: "app", Path: path}, true
	}
	for _, p := range findPrograms() {
		if p.ID == id {
			for _, k := range catalogue {
				if k.ID == id {
					return k, p, true
				}
			}
		}
	}
	return known{}, Program{}, false
}

// command is how to run one against a target — a URL, a directory or a file.
func (k known) command(p Program, target string, line int) *exec.Cmd {
	if p.How == "app" {
		return exec.Command("open", "-a", p.Path, target)
	}
	switch k.Line {
	case "goto": // code -g file:line
		if line > 0 {
			return exec.Command(p.Path, "-g", target+":"+strconv.Itoa(line))
		}
	case "colon": // zed, subl: file:line
		if line > 0 {
			return exec.Command(p.Path, target+":"+strconv.Itoa(line))
		}
	case "plus": // vim +line file
		if line > 0 {
			return exec.Command(p.Path, "+"+strconv.Itoa(line), target)
		}
	case "goland": // idea --line n file
		if line > 0 {
			return exec.Command(p.Path, "--line", strconv.Itoa(line), target)
		}
	case "textmate": // mate -l n file
		if line > 0 {
			return exec.Command(p.Path, "-l", strconv.Itoa(line), target)
		}
	}
	return exec.Command(p.Path, target)
}

// programOff is the choice that is not a program: no button, nothing opened.
// A terminal and an editor are offers rather than parts of the dashboard, and
// somebody who does not want grove reaching for either should be able to say
// so and have the buttons go away rather than sit there declined.
const programOff = "off"

// launch opens one thing with the program chosen for its kind, or with
// whatever the system would have used when nothing is chosen.
func launch(kind, target string, line int) error {
	s := loadSettings()
	id := map[string]string{"browser": s.Browser, "terminal": s.Terminal, "editor": s.Editor}[kind]
	if id == programOff {
		article := "a "
		if strings.ContainsRune("aeiou", rune(kind[0])) {
			article = "an "
		}
		return fmt.Errorf("grove is set not to open %s%s", article, kind)
	}
	if k, p, ok := programByID(id); ok {
		cmd := k.command(p, target, line)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("could not start %s: %w", p.Name, err)
		}
		go cmd.Wait() // do not leave it a zombie; nobody is waiting on the result
		return nil
	}
	return systemOpen(kind, target)
}

// systemOpen is what happens with nothing chosen: whatever this machine would
// do with the thing on its own.
func systemOpen(kind, target string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", target).Start()
	case "windows":
		if kind == "browser" {
			return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
		}
		return exec.Command("cmd", "/c", "start", "", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}

// handlePrograms lists what is here, so the settings can offer it.
func (d *grove) handlePrograms(w http.ResponseWriter, r *http.Request) {
	s := loadSettings()
	found := findPrograms()
	// a program somebody pointed at themselves is in the list too, or the
	// select would have nothing to show as chosen
	for kind, id := range map[string]string{"browser": s.Browser, "terminal": s.Terminal, "editor": s.Editor} {
		if !strings.HasPrefix(id, "path:") {
			continue
		}
		if _, p, ok := programByID(id); ok {
			p.ID, p.Kind = id, kind
			found = append(found, p)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"programs": found,
		"chosen":   map[string]string{"browser": s.Browser, "terminal": s.Terminal, "editor": s.Editor},
	})
}

// handleLaunch opens a checkout in a terminal, or a file in an editor.
//
// The path has to be one grove already knows about. It is not a place to name
// any file on the machine and have it opened: a page on another site cannot
// reach this, but a mistake here would still be grove running a program on
// something nobody asked about.
func (d *grove) handleLaunch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind string `json:"kind"` // terminal | editor
		Name string `json:"name"` // the checkout
		Repo string `json:"repo"` // which repository under it
		File string `json:"file"` // optional, relative to that repository
		Line int    `json:"line"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Kind != "terminal" && req.Kind != "editor" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("nothing opens a %q", req.Kind))
		return
	}
	c, ok := d.checkout(req.Name)
	if !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("no checkout named %q", req.Name))
		return
	}
	root := c.Path
	if req.Repo != "" {
		root = ""
		for _, repo := range diffRepos(c) {
			if repo.Name == req.Repo {
				root = repo.Path
			}
		}
		if root == "" {
			writeErr(w, http.StatusNotFound, fmt.Errorf("%s has no %s checkout", c.Name, req.Repo))
			return
		}
	}
	target := root
	if req.File != "" {
		if !safeRepoPath(req.File) {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("path must stay inside the checkout"))
			return
		}
		target = filepath.Join(root, filepath.Clean(req.File))
	}
	if err := launch(req.Kind, target, req.Line); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"opened": target})
}

// pickFolder is set by the desktop front door, because only it has a native
// dialog to offer. In a browser there is nothing to open, and the settings
// window says so rather than showing a button that does nothing.
var pickFolder func()

// pickApp is set by the desktop front door for the same reason pickFolder is.
// It hands back the chosen bundle so the settings can store it.
var pickApp func(kind string) (name, path string, ok bool)

// handleChooseProgram opens the application picker, so the choice is not
// limited to the programs this file has heard of. Every .app on the disk is
// reachable; grove uses what it knows about the ones it recognises and opens
// the rest plainly.
func (d *grove) handleChooseProgram(w http.ResponseWriter, r *http.Request) {
	var req struct{ Kind string }
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	switch req.Kind {
	case "browser", "terminal", "editor":
	default:
		writeErr(w, http.StatusBadRequest, fmt.Errorf("nothing opens a %q", req.Kind))
		return
	}
	if pickApp == nil {
		writeErr(w, http.StatusNotImplemented,
			fmt.Errorf("only the app has a file dialog — in a browser, choose from the list"))
		return
	}
	name, path, ok := pickApp(req.Kind)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"cancelled": true})
		return
	}
	s := loadSettings()
	id := "path:" + path
	switch req.Kind {
	case "browser":
		s.Browser = id
	case "terminal":
		s.Terminal = id
	case "editor":
		s.Editor = id
	}
	if err := s.save(); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "name": name, "path": path})
}

// handleChooseFolder asks the window to put the folder dialog up. It answers
// straight away: the dialog is somebody else's to close.
func (d *grove) handleChooseFolder(w http.ResponseWriter, r *http.Request) {
	if pickFolder == nil {
		writeErr(w, http.StatusNotImplemented,
			fmt.Errorf("only the app has a folder dialog; start grove in the directory you want, or pass -dir"))
		return
	}
	go pickFolder()
	writeJSON(w, http.StatusOK, map[string]any{"asked": true})
}

// handleUseFolder points grove at one it has been pointed at before.
func (d *grove) handleUseFolder(w http.ResponseWriter, r *http.Request) {
	var req struct{ Dir string }
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	// only somewhere it has already been: this is the recents list, not a way
	// to name any directory on the machine
	found := false
	for _, r := range loadSettings().Recent {
		if samePath(r, req.Dir) {
			found = true
		}
	}
	if !found {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("that is not one of the recent directories"))
		return
	}
	if err := d.setDir(normPath(req.Dir)); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dir": d.dir()})
}

// handleLoginItem reads and writes whether grove starts with the machine.
func (d *grove) handleLoginItem(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"on": loginItemOn(), "possible": bundlePath() != ""})
		return
	}
	var req struct{ On bool }
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := loginItemSet(req.On); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"on": loginItemOn()})
}

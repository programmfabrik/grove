package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

// A second window on the same dashboard, opened at a different place in it.
//
// The view already lives in the URL fragment — that is what makes a deep link
// work — so a window on a repository, a worktree or a scope is nothing more
// than the same page with a different fragment. All this side has to do is
// make the window, which is the one thing a page cannot do for itself: the
// webview ignores window.open, so the request comes back here.

// openWindow is set by the desktop front door, because only it has windows.
// The browser has its own and never asks.
var openWindow func(frag, title string)

// viewKeys are the fragment keys grove knows. The fragment ends up in a URL a
// window is told to load, so it is rebuilt from these rather than passed
// through — whatever a page sends, what comes out is a fragment naming a view.
var viewKeys = []string{"repo", "wt", "sub", "scope", "file"}

const maxViewValue = 512 // a path is the longest of these, and none is a file's contents

func safeFragment(s string) (string, error) {
	q, err := url.ParseQuery(s)
	if err != nil {
		return "", fmt.Errorf("not a view: %w", err)
	}
	out := url.Values{}
	for _, k := range viewKeys {
		v := q.Get(k)
		if v == "" {
			continue
		}
		if len(v) > maxViewValue {
			return "", fmt.Errorf("%s is too long to be a view", k)
		}
		if strings.ContainsFunc(v, func(r rune) bool { return unicode.IsControl(r) }) {
			return "", fmt.Errorf("%s has control characters in it", k)
		}
		out.Set(k, v)
	}
	return out.Encode(), nil
}

// safeTitle is what the title bar says. It is text somebody's branch or commit
// subject wrote, so it gets the same treatment: one line, and not a long one.
func safeTitle(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if len([]rune(s)) > 80 {
		s = string([]rune(s)[:79]) + "…"
	}
	if s == "" {
		return "Grove"
	}
	return s
}

func (d *grove) handleWindow(w http.ResponseWriter, r *http.Request) {
	var req struct{ Frag, Title string }
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if openWindow == nil {
		writeErr(w, http.StatusNotImplemented,
			fmt.Errorf("only the app has windows of its own; a browser opens its own"))
		return
	}
	frag, err := safeFragment(req.Frag)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	openWindow(frag, safeTitle(req.Title))
	writeJSON(w, http.StatusOK, map[string]any{"opened": true})
}

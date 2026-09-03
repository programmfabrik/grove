package main

import (
	"os/exec"
	"strings"
	"testing"
)

// An editor is worth choosing over the system default for one reason: it can
// be told which line. Each way of saying that is a different flag, and getting
// it wrong opens the file at the top, silently.
func TestEditorCommandsCarryTheLine(t *testing.T) {
	for _, c := range []struct {
		name string
		k    known
		p    Program
		want []string
	}{
		{"vs code", known{Line: "goto"}, Program{How: "cli", Path: "/code"}, []string{"/code", "-g", "/f.go:12"}},
		{"zed", known{Line: "colon"}, Program{How: "cli", Path: "/zed"}, []string{"/zed", "/f.go:12"}},
		{"vim", known{Line: "plus"}, Program{How: "cli", Path: "/vim"}, []string{"/vim", "+12", "/f.go"}},
		{"goland", known{Line: "goland"}, Program{How: "cli", Path: "/idea"}, []string{"/idea", "--line", "12", "/f.go"}},
		// an app has no way to be told, so it is opened plainly rather than
		// handed a filename with a colon in it that it would fail to find
		{"an app bundle", known{Line: "goto"}, Program{How: "app", Path: "/A.app"}, []string{"open", "-a", "/A.app", "/f.go"}},
	} {
		got := c.k.command(c.p, "/f.go", 12)
		if strings.Join(args(got), " ") != strings.Join(c.want, " ") {
			t.Errorf("%s: %v, want %v", c.name, args(got), c.want)
		}
	}
	// no line to go to: none of the flags appear
	got := known{Line: "goto"}.command(Program{How: "cli", Path: "/code"}, "/dir", 0)
	if strings.Join(args(got), " ") != "/code /dir" {
		t.Errorf("without a line: %v", args(got))
	}
}

func args(c *exec.Cmd) []string { return c.Args }

// Every program in the catalogue has to be openable one way or another, or it
// is an entry that can be chosen and then does nothing.
func TestEveryKnownProgramCanBeOpened(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range catalogue {
		if k.App == "" && k.Bin == "" {
			t.Errorf("%s has neither an app nor a binary to look for", k.ID)
		}
		if k.CLI != "" && k.Line == "" {
			t.Errorf("%s ships a command line tool and no way to tell it a line", k.ID)
		}
		switch k.Kind {
		case "browser", "terminal", "editor":
		default:
			t.Errorf("%s is a %q, which nothing opens", k.ID, k.Kind)
		}
		if seen[k.ID] {
			t.Errorf("%s is in the catalogue twice", k.ID)
		}
		seen[k.ID] = true
	}
}

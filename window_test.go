package main

import "testing"

// The fragment ends up in a URL a window is told to load. Whatever a page
// sends, what comes out has to be a fragment naming a view and nothing else.
func TestSafeFragmentRebuildsFromTheKeysGroveKnows(t *testing.T) {
	long := make([]byte, maxViewValue+1)
	for i := range long {
		long[i] = 'a'
	}
	cases := []struct {
		name, in, want string
		bad            bool
	}{
		{name: "a view", in: "repo=grove&wt=grove2&scope=base", want: "repo=grove&scope=base&wt=grove2"},
		{name: "keys grove does not know are dropped",
			in: "repo=grove&view=settings&cmd=rm", want: "repo=grove"},
		{name: "order is grove's, not the caller's",
			in: "file=cmd/main.go&repo=grove", want: "file=cmd%2Fmain.go&repo=grove"},
		{name: "empty stays empty", in: "", want: ""},
		{name: "a value cannot break out of the fragment",
			in: "repo=" + urlish("a#b?c&d=e"), want: "repo=a%23b%3Fc%26d%3De"},
		{name: "control characters are not a view name", in: "repo=a\nb", bad: true},
		{name: "and neither is a novel", in: "file=" + string(long), bad: true},
		{name: "nor something that is not a query at all", in: "%zz", bad: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := safeFragment(c.in)
			if c.bad {
				if err == nil {
					t.Fatalf("safeFragment(%q) = %q, want an error", c.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("safeFragment(%q): %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("safeFragment(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// urlish is the test writing what a browser would have sent.
func urlish(s string) string {
	out := ""
	for _, b := range []byte(s) {
		switch b {
		case '#', '?', '&', '=', '%', '+', ' ':
			out += "%" + string("0123456789ABCDEF"[b>>4]) + string("0123456789ABCDEF"[b&0xf])
		default:
			out += string(b)
		}
	}
	return out
}

// The title bar says what a branch or a commit subject wrote, so it is one
// line, and not a long one.
func TestSafeTitle(t *testing.T) {
	long := ""
	for i := 0; i < 200; i++ {
		long += "x"
	}
	for _, c := range []struct{ in, want string }{
		{"ai-provider-77999", "ai-provider-77999"},
		{"  spaced  ", "spaced"},
		{"two\nlines", "two lines"},
		{"", "Grove"},
		{"\t\n ", "Grove"},
	} {
		if got := safeTitle(c.in); got != c.want {
			t.Errorf("safeTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := safeTitle(long); len([]rune(got)) != 80 {
		t.Errorf("safeTitle(200 chars) is %d runes, want 80", len([]rune(got)))
	}
}

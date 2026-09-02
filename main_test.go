package main

import (
	"net"
	"strings"
	"testing"
)

// The default address is allowed to move and an explicit one is not, because
// the second is a promise about where the dashboard will be.
func TestListenFallback(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	busy := taken.Addr().String()

	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fallback := free.Addr().String()
	free.Close() // free it again: we only wanted a port nothing else holds

	ln, err := listen(busy, fallback, false)
	if err != nil {
		t.Fatalf("default address did not fall back: %v", err)
	}
	if ln.Addr().String() != fallback {
		t.Errorf("fell back to %s, want %s", ln.Addr(), fallback)
	}
	ln.Close()

	if ln, err := listen(busy, fallback, true); err == nil {
		ln.Close()
		t.Error("an explicit -addr moved to another port instead of failing")
	}

	// nowhere to go: the first failure is what the user needs to read
	if _, err := listen(busy, busy, false); err == nil {
		t.Error("listen succeeded with both addresses taken")
	}
}

func TestDashboardURL(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	// loopback reads as localhost, and the port comes off the listener rather
	// than off what was asked for — it may have moved
	if got, want := dashboardURL(ln), "http://localhost:"+port; got != want {
		t.Errorf("dashboardURL = %q, want %q", got, want)
	}
	if !strings.HasPrefix(dashboardURL(ln), "http://localhost") {
		t.Error("a loopback bind should read as localhost")
	}
}

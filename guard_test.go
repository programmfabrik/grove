package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func guarded() http.Handler {
	return newGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}), true)
}

// do sends one request and returns the response, carrying any cookie given.
func do(h http.Handler, method, target string, headers map[string]string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader("{}"))
	r.Host = "127.0.0.1:7433"
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func sessionOf(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	return nil
}

// A page on any site could POST to /api/revert: `Content-Type: text/plain`
// makes it a "simple" request, so no preflight is asked for, and the browser
// sends it happily while refusing to show the attacker the answer — which is
// all that is needed when the effect is a deletion.
func TestCrossSitePostIsRefused(t *testing.T) {
	h := guarded()
	for _, c := range []struct {
		name    string
		headers map[string]string
	}{
		{"an origin somewhere else", map[string]string{"Origin": "https://evil.example", "Content-Type": "application/json"}},
		{"the browser says cross-site", map[string]string{"Sec-Fetch-Site": "cross-site", "Content-Type": "application/json"}},
		{"the browser says same-site, not same-origin", map[string]string{"Sec-Fetch-Site": "same-site", "Content-Type": "application/json"}},
		{"a sandboxed frame, origin null", map[string]string{"Origin": "null", "Content-Type": "application/json"}},
	} {
		if got := do(h, "POST", "/api/revert", c.headers, nil).Code; got != http.StatusForbidden {
			t.Errorf("%s: got %d, want 403", c.name, got)
		}
	}
}

// DNS rebinding: a name the attacker controls that answers 127.0.0.1 makes
// their page same-origin with the dashboard. The Host header is the only thing
// that tells the two apart.
func TestRebindingHostIsRefused(t *testing.T) {
	h := guarded()
	for _, host := range []string{"evil.example", "evil.example:7433", "grove.attacker.test:7433"} {
		r := httptest.NewRequest("GET", "/api/repos", nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Errorf("Host %q: got %d, want 403", host, w.Code)
		}
	}
	for _, host := range []string{"localhost:7433", "127.0.0.1:7433", "[::1]:7433", "127.0.0.1", "LOCALHOST:80"} {
		r := httptest.NewRequest("GET", "/api/repos", nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("Host %q: got %d, want 200", host, w.Code)
		}
	}
}

// A write needs the secret, and reading the page is how you are given it.
func TestWritesNeedTheSession(t *testing.T) {
	h := guarded()
	json := map[string]string{"Content-Type": "application/json"}

	if got := do(h, "POST", "/api/revert", json, nil).Code; got != http.StatusForbidden {
		t.Errorf("write without a session: got %d, want 403", got)
	}
	page := do(h, "GET", "/", nil, nil)
	c := sessionOf(page)
	if c == nil {
		t.Fatal("loading the dashboard handed out no session")
	}
	if c.SameSite != http.SameSiteStrictMode || !c.HttpOnly {
		t.Errorf("session cookie is %+v, want SameSite=Strict and HttpOnly", c)
	}
	if got := do(h, "POST", "/api/revert", json, c).Code; got != http.StatusOK {
		t.Errorf("write with the session: got %d, want 200", got)
	}
	// somebody else's guess is not the secret
	if got := do(h, "POST", "/api/revert", json, &http.Cookie{Name: sessionCookie, Value: "guessed"}).Code; got != http.StatusForbidden {
		t.Errorf("write with a wrong token: got %d, want 403", got)
	}
}

// Demanding JSON is not pedantry: it is not a content type a form can send, so
// it forces any cross-origin attempt through a preflight that will not pass.
func TestWritesMustBeJSON(t *testing.T) {
	h := guarded()
	c := sessionOf(do(h, "GET", "/", nil, nil))
	for _, ct := range []string{"text/plain", "application/x-www-form-urlencoded", "multipart/form-data", ""} {
		got := do(h, "POST", "/api/revert", map[string]string{"Content-Type": ct}, c).Code
		if got != http.StatusUnsupportedMediaType {
			t.Errorf("Content-Type %q: got %d, want 415", ct, got)
		}
	}
	if got := do(h, "POST", "/api/revert", map[string]string{"Content-Type": "application/json; charset=utf-8"}, c).Code; got != http.StatusOK {
		t.Errorf("a charset on the content type: got %d, want 200", got)
	}
}

// `npm run dev` serves the page from vite and proxies /api here, so the page's
// origin is loopback on another port. Forging that means already running a
// program on this machine, which needs no help from grove.
func TestTheDevServerStillWorks(t *testing.T) {
	h := guarded()
	dev := map[string]string{"Origin": "http://localhost:5173", "Sec-Fetch-Site": "same-origin", "Content-Type": "application/json"}
	page := do(h, "GET", "/api/repos", dev, nil)
	if page.Code != http.StatusOK {
		t.Fatalf("dev read: got %d, want 200", page.Code)
	}
	// the cookie is handed out on any response, not only on the page, because
	// in development grove never serves the page at all
	c := sessionOf(page)
	if c == nil {
		t.Fatal("an API response handed out no session; the dev server can never write")
	}
	if got := do(h, "POST", "/api/revert", dev, c).Code; got != http.StatusOK {
		t.Errorf("dev write: got %d, want 200", got)
	}
}

// `-addr :80` binds the wildcard on purpose so other machines can reach the
// dashboard. Then the Host and Origin of a legitimate request are whatever
// address the browser used, and enforcing loopback would refuse the very thing
// that flag exists to allow. Everything else still holds.
func TestWildcardBindKeepsTheRestOfTheGuard(t *testing.T) {
	h := newGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}), false)

	r := httptest.NewRequest("GET", "/api/repos", nil)
	r.Host = "192.168.1.5:80"
	r.Header.Set("Origin", "http://192.168.1.5")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("a deliberate wildcard bind was refused its own address: %d", w.Code)
	}
	// still not a free-for-all: cross-site and session-less writes stay out
	if got := do(h, "POST", "/api/revert", map[string]string{"Sec-Fetch-Site": "cross-site", "Content-Type": "application/json"}, nil).Code; got != http.StatusForbidden {
		t.Errorf("cross-site write on a wildcard bind: got %d, want 403", got)
	}
	if got := do(h, "POST", "/api/revert", map[string]string{"Content-Type": "application/json"}, nil).Code; got != http.StatusForbidden {
		t.Errorf("session-less write on a wildcard bind: got %d, want 403", got)
	}
}

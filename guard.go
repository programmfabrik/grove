package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Binding the loopback interface keeps the dashboard off the network. It does
// not keep it to this browser: every page in every tab can make requests to
// 127.0.0.1, and /api/revert deletes files. Two attacks follow from that, and
// both worked before this file existed.
//
// A page on any site could POST to /api/revert. Cross-origin JSON would have
// been stopped by the CORS preflight, but `Content-Type: text/plain` makes the
// request "simple", so no preflight is asked for; the browser refuses to show
// the attacker the RESPONSE and sends the request anyway, which is all that is
// needed when the effect is a deletion.
//
// And DNS rebinding: a name the attacker controls, answering first with their
// address and then with 127.0.0.1, makes their page same-origin with the
// dashboard and lets it read every repository on the machine. The Host header
// is the only thing that tells the two apart, and grove was serving any.
//
// So, in order: the Host must be a loopback name, a browser must not say the
// request came from another site, and anything that writes must carry a
// per-launch secret that only this origin can have been given.
//
// None of this is defence against another program running as you. It cannot
// be: that program can read the repositories directly, and grove's endpoints
// are the long way round. What is defended is the browser, which runs code
// from strangers all day long.

const sessionCookie = "grove_session"

type guard struct {
	token string
	next  http.Handler
	// loopbackOnly is whether grove is bound where only this machine can reach
	// it, which is the default. `-addr :80` binds the wildcard on purpose, and
	// then the Host and Origin of a legitimate request are whatever address
	// the browser used — so those two checks are dropped rather than refusing
	// the very thing that flag exists to allow. The rest still holds.
	loopbackOnly bool
}

func newGuard(next http.Handler, loopbackOnly bool) http.Handler {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// a dashboard with no usable secret must not fall back to none
		panic("grove: no randomness for a session token: " + err.Error())
	}
	return &guard{token: hex.EncodeToString(b), next: next, loopbackOnly: loopbackOnly}
}

// loopbackListener says whether the address grove is bound to is one only this
// machine can reach.
func loopbackListener(ln net.Listener) bool {
	a, ok := ln.Addr().(*net.TCPAddr)
	return ok && a.IP != nil && a.IP.IsLoopback()
}

func (g *guard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Whose name did they arrive under? Anything but a loopback name means
	//    the request found its way here through DNS, which loopback cannot do.
	if g.loopbackOnly && !loopbackHost(r.Host) {
		http.Error(w, "grove answers on the loopback interface only", http.StatusForbidden)
		return
	}
	// 2. What does the browser say about where it came from? Both headers are
	//    set by the browser and cannot be forged by the page.
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		http.Error(w, "cross-site requests are refused", http.StatusForbidden)
		return
	}
	// A loopback origin on another port is the vite dev server, which proxies
	// /api here while serving the page itself. Forging one means already
	// running a program on this machine, which needs no help from grove.
	if origin := r.Header.Get("Origin"); g.loopbackOnly && origin != "" && !loopbackOrigin(origin) {
		http.Error(w, "cross-origin requests are refused", http.StatusForbidden)
		return
	}
	// 3. Writes carry the secret. SameSite=Strict means no cross-site context
	//    is ever handed it, so this holds even in a browser too old to send
	//    the headers above.
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if !g.hasSession(r) {
			http.Error(w, "no session: load the dashboard first", http.StatusForbidden)
			return
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			// JSON is not a content type a form can send, so demanding it
			// forces any cross-origin attempt through a preflight
			http.Error(w, "writes must be application/json", http.StatusUnsupportedMediaType)
			return
		}
	}
	g.grant(w, r)
	g.next.ServeHTTP(w, r)
}

// grant hands this origin the secret if it has not got it. Every response
// carries it rather than only the page, because in development the browser
// talks to vite and only vite talks to grove — the page itself is never served
// from here, and a cookie set on it would never be set at all.
func (g *guard) grant(w http.ResponseWriter, r *http.Request) {
	if g.hasSession(r) {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    g.token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		// no Secure: this is http on loopback, which browsers already treat as
		// a secure context, and Secure would stop the cookie being kept at all
	})
}

func (g *guard) hasSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(g.token)) == 1
}

// loopbackHost says whether a Host header names this machine. A name that is
// not loopback reached us through DNS, which is the rebinding case.
func loopbackHost(host string) bool {
	h := host
	if only, _, err := net.SplitHostPort(host); err == nil {
		h = only
	}
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func loopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false // "null", and anything else that is not an origin
	}
	return loopbackHost(u.Host)
}

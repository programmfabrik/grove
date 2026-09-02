//go:build !desktop

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// The default front door: serve the dashboard and say where it is. This is
// what `go install github.com/programmfabrik/grove@latest` builds — pure Go,
// no cgo, no webview, nothing to install first.

func init() { distKind = "cli" }

func run(d *grove, addr string, explicit, open bool) error {
	// One scan before binding a port: an empty directory is worth reporting
	// now rather than as an empty first page.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	repos := d.reposList(ctx)
	if len(repos) == 0 {
		return fmt.Errorf("no git repository in %s — pass -dir", d.dir())
	}

	ln, err := listen(addr, fallbackAddr, explicit)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w\n  something else holds the port. Try -addr %s", addr, err, fallbackAddr)
	}
	url := dashboardURL(ln)
	fmt.Printf("grove: %s  —  %d repositories in %s\n", url, len(repos), d.dir())
	if open {
		go openBrowser(url)
	}

	srv := &http.Server{Handler: d.routes()}
	go func() {
		<-ctx.Done()
		sc, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(sc)
	}()
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

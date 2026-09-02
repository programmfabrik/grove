# grove — a local dashboard over a directory of git repositories.
#
# ui/dist is committed so that `go install github.com/programmfabrik/grove@latest`
# works without node; `make ui` rebuilds it only when the UI sources changed.

SHA256   := shasum -a 256
UI_DIR   := ui
UI_STAMP := $(UI_DIR)/.build-stamp
UI_HASH   = find $(UI_DIR)/src $(UI_DIR)/index.html $(UI_DIR)/package-lock.json \
	$(UI_DIR)/vite.config.ts $(UI_DIR)/tsconfig.json -type f | sort | xargs cat | $(SHA256) | cut -d" " -f1
ADDR     ?=

.PHONY: ui build app bundle app-windows run test clean help

ui: ## rebuild ui/dist if the UI sources changed (no-op when current)
	@want=$$($(UI_HASH)); have=$$(cat $(UI_STAMP) 2>/dev/null || true); \
	if [ -f $(UI_DIR)/dist/index.html ] && [ "$$want" = "$$have" ]; then exit 0; fi; \
	( cd $(UI_DIR) && npm ci --no-audit --no-fund && npm run build ) && $(UI_HASH) > $(UI_STAMP)

build: ui ## build bin/grove, the command
	go build -o bin/grove .

app: ui ## the same dashboard in a window of its own (bin/Grove.app on macOS)
	go build -tags desktop -o bin/Grove .
	@[ "$$(uname)" = "Darwin" ] && $(MAKE) --no-print-directory bundle || true

# A bare executable is not an application on macOS. Launched from a shell it
# dies with that shell, it has no icon, and the menu bar names it after the
# file. The bundle is what LaunchServices needs to treat it as a program:
# `open bin/Grove.app` and it outlives the terminal it came from.
#
# Grove.icns is committed rather than converted here, the same bargain ui/dist
# strikes: one less tool to have installed to build the thing.
bundle: ## wrap bin/Grove into bin/Grove.app (macOS)
	rm -rf bin/Grove.app
	mkdir -p bin/Grove.app/Contents/MacOS bin/Grove.app/Contents/Resources
	cp packaging/macos/Info.plist bin/Grove.app/Contents/Info.plist
	cp packaging/macos/Grove.icns bin/Grove.app/Contents/Resources/Grove.icns
	cp bin/Grove bin/Grove.app/Contents/MacOS/Grove

app-windows: ui ## cross-build the Windows app from here (Wails v3 needs no cgo there)
	GOOS=windows GOARCH=amd64 go build -tags desktop -ldflags "-H=windowsgui" -o bin/Grove.exe .

run: build ## build and start it (ADDR=127.0.0.1:8000 to pick the address)
	./bin/grove $(if $(ADDR),-addr $(ADDR))

test: ## go vet and the tests (they need git on the path)
	go vet ./... && go test ./...

clean: ## remove the binaries and the UI build stamp (ui/dist stays: it is committed)
	rm -rf bin $(UI_STAMP)

help: ## this list
	@grep -E '^[a-z]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-8s %s\n", $$1, $$2}'

# grove — a local dashboard over a directory of git repositories.
#
# ui/dist is committed so that `go install github.com/programmfabrik/grove@latest`
# works without node; `make ui` rebuilds it only when the UI sources changed.

SHA256   := shasum -a 256
UI_DIR   := ui
UI_STAMP := $(UI_DIR)/.build-stamp
UI_HASH   = find $(UI_DIR)/src $(UI_DIR)/index.html $(UI_DIR)/package-lock.json \
	$(UI_DIR)/vite.config.ts $(UI_DIR)/tsconfig.json -type f | sort | xargs cat | $(SHA256) | cut -d" " -f1
ADDR     ?= :80

.PHONY: ui build run clean help

ui: ## rebuild ui/dist if the UI sources changed (no-op when current)
	@want=$$($(UI_HASH)); have=$$(cat $(UI_STAMP) 2>/dev/null || true); \
	if [ -f $(UI_DIR)/dist/index.html ] && [ "$$want" = "$$have" ]; then exit 0; fi; \
	( cd $(UI_DIR) && npm ci --no-audit --no-fund && npm run build ) && $(UI_HASH) > $(UI_STAMP)

build: ui ## build bin/grove
	go build -o bin/grove .

run: build ## build and start on http://localhost (ADDR=:8000 to move it)
	./bin/grove -addr $(ADDR)

clean: ## remove the binary and the UI build stamp (ui/dist stays: it is committed)
	rm -rf bin $(UI_STAMP)

help: ## this list
	@grep -E '^[a-z]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-8s %s\n", $$1, $$2}'

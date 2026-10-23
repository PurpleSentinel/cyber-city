# CyberCity-3D build entry point. CI runs the same targets (AGENTS.md "Commands").
.DEFAULT_GOAL := help
SHELL := /bin/bash

GO            ?= go
NPM           ?= npm
GOBIN         := $(shell $(GO) env GOPATH)/bin
GOLANGCI_LINT ?= $(shell command -v golangci-lint 2>/dev/null || echo $(GOBIN)/golangci-lint)
BIN           := bin/cybercity
WEB           := web
DEMO_PCAP     := testdata/pcap/demo.pcap
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS       := -s -w -X github.com/PurpleSentinel/cyber-city/internal/buildinfo.Version=$(VERSION)

# Runs $(2) when path $(1) exists; otherwise says which task adds it. Keeps every
# target callable while the tree is still being built out (docs/TASKS.md).
define when_exists
	@if [ -e "$(1)" ]; then $(2); else echo "skip: $(1) not present yet ($(3))"; fi
endef

.PHONY: help
help: ## List targets
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: setup
setup: ## Download Go modules, install web deps and Playwright Chromium
	$(GO) mod download
	$(call when_exists,$(WEB)/package.json,cd $(WEB) && $(NPM) ci && npx playwright install --with-deps chromium,T0.2)

.PHONY: fixtures
fixtures: ## Regenerate testdata/ with fixturegen
	$(call when_exists,tools/fixturegen,$(GO) run ./tools/fixturegen -out testdata,T0.7)

.PHONY: oui
oui: ## Regenerate the OUI vendor table (needs network)
	$(call when_exists,tools/ouigen,$(GO) generate ./internal/enrich/...,T0.6)

.PHONY: web
web: ## Build the frontend into web/dist
	$(call when_exists,$(WEB)/package.json,cd $(WEB) && $(NPM) run build,T0.2)

.PHONY: build
build: web ## Build bin/cybercity with the embedded frontend
	$(call when_exists,cmd/cybercity,$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/cybercity,T0.2)

.PHONY: test
test: ## Go tests (race) and Vitest
	$(GO) test -race ./...
	$(call when_exists,$(WEB)/package.json,cd $(WEB) && $(NPM) run test,T0.2)

.PHONY: lint
lint: ## golangci-lint, tsc, eslint
	$(GO) vet ./...
	$(GOLANGCI_LINT) run ./...
	$(call when_exists,$(WEB)/package.json,cd $(WEB) && $(NPM) run typecheck && $(NPM) run lint,T0.2)

.PHONY: fixtures-check
fixtures-check: ## Fail if regenerating fixtures changes testdata/
	$(call when_exists,tools/fixturegen,$(MAKE) fixtures && git diff --exit-code -- testdata,T0.7)

.PHONY: e2e
e2e: build ## Playwright end-to-end tests
	$(call when_exists,$(WEB)/e2e,cd $(WEB) && npx playwright test,T3.12)

.PHONY: check
check: lint test fixtures-check ## Everything CI runs except e2e: run before every push

.PHONY: dev
dev: ## Backend replaying demo.pcap plus the Vite dev server
	$(call when_exists,$(DEMO_PCAP),$(MAKE) build && ( $(BIN) --read $(DEMO_PCAP) --loop --allow-origin http://localhost:5173 & cd $(WEB) && $(NPM) run dev ; kill %1 ),T1.3)

.PHONY: demo
demo: build ## Run bin/cybercity on the demo capture
	$(call when_exists,$(DEMO_PCAP),$(BIN) --read $(DEMO_PCAP) --loop,T0.7)

.PHONY: setcap
setcap: ## Grant capture capabilities to bin/cybercity (live capture without root)
	sudo setcap cap_net_raw,cap_net_admin+eip $(BIN)

.PHONY: clean
clean: ## Remove build output
	rm -rf bin $(WEB)/dist/assets $(WEB)/dist/index.html

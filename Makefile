.PHONY: help web build test lint e2e docker run dev-upstream demo clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
PKG := github.com/aurimasniekis/go-pwr-tracker/internal/version
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(shell git rev-parse HEAD 2>/dev/null)

help: ## Show targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-14s %s\n", $$1, $$2}'

web: ## Build the frontend into web/dist
	cd web && npm ci && npm run build

build: web ## Build bin/pwr-tracker with the UI embedded
	CGO_ENABLED=0 go build -trimpath -tags embedui -ldflags "$(LDFLAGS)" -o bin/pwr-tracker ./cmd/pwr-tracker

test: ## Run Go and frontend unit tests
	go test -race ./...
	cd web && npm test

lint: ## Run formatters/linters in check mode
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	go vet ./...
	cd web && npm run format:check && npm run lint && npm run typecheck

e2e: web ## Run the Playwright smoke test
	cd web && npx playwright test

docker: ## Build the container image
	docker build --build-arg VERSION=$(VERSION) -t pwr-tracker:$(VERSION) .

dev-upstream: ## Run the deterministic fake upstream on :18081
	go run ./cmd/fakeupstream -loop

run: ## Run against the fake upstream with a local database
	mkdir -p tmp
	DATABASE_PATH=tmp/dev.db UPSTREAM_URL=http://127.0.0.1:18081/Pwr/lastSong/ POLL_INTERVAL=5s \
	  go run ./cmd/pwr-tracker

demo: web ## Seed tmp/demo.db with synthetic history and serve it (collector off)
	mkdir -p tmp
	test -f tmp/demo.db || go run ./cmd/pwr-tracker demo-seed -db tmp/demo.db -days 60
	DATABASE_PATH=tmp/demo.db COLLECTOR_ENABLED=false go run -tags embedui ./cmd/pwr-tracker

clean: ## Remove build outputs and local databases
	rm -rf bin dist tmp web/dist

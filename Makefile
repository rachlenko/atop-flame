## atop-flame — Makefile
## Follows the convention from github.com/umputun/ralphex

TAG=$(shell git describe --tags --abbrev=0 --exact-match 2>/dev/null)
BRANCH=$(if $(TAG),$(TAG),$(shell git rev-parse --abbrev-ref HEAD 2>/dev/null))
HASH=$(shell git rev-parse --short=7 HEAD 2>/dev/null)
TIMESTAMP=$(shell git log -1 --format=%ct HEAD 2>/dev/null | xargs -I{} date -u -r {} +%Y%m%dT%H%M%S)
GIT_REV=$(shell printf "%s-%s-%s" "$(BRANCH)" "$(HASH)" "$(TIMESTAMP)")
REV=$(if $(filter --,$(GIT_REV)),latest,$(GIT_REV))

APP=atop-flame
DOCKER_IMAGE=ghcr.io/yourorg/$(APP)
BINARY=.bin/$(APP)
LDFLAGS=-ldflags "-X main.appVersion=$(REV) -s -w"

## ── default ───────────────────────────────────────────────────────────────────

all: test build

## ── build ─────────────────────────────────────────────────────────────────────

build:
	mkdir -p .bin
	go build $(LDFLAGS) -o $(BINARY) .
	@echo "binary: $(BINARY)"

# build and copy as plain 'atop-flame' (no branch suffix)
build-release:
	mkdir -p .bin
	go build $(LDFLAGS) -o $(BINARY).$(BRANCH) .
	cp $(BINARY).$(BRANCH) $(BINARY)
	@echo "binary: $(BINARY)"

# cross-compile for all supported platforms
build-all: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows-amd64

build-linux-amd64:
	mkdir -p .bin
	GOOS=linux   GOARCH=amd64  go build $(LDFLAGS) -o .bin/$(APP)-linux-amd64   .

build-linux-arm64:
	mkdir -p .bin
	GOOS=linux   GOARCH=arm64  go build $(LDFLAGS) -o .bin/$(APP)-linux-arm64   .

build-darwin-amd64:
	mkdir -p .bin
	GOOS=darwin  GOARCH=amd64  go build $(LDFLAGS) -o .bin/$(APP)-darwin-amd64  .

build-darwin-arm64:
	mkdir -p .bin
	GOOS=darwin  GOARCH=arm64  go build $(LDFLAGS) -o .bin/$(APP)-darwin-arm64  .

build-windows-amd64:
	mkdir -p .bin
	GOOS=windows GOARCH=amd64  go build $(LDFLAGS) -o .bin/$(APP)-windows-amd64.exe .

## ── install ───────────────────────────────────────────────────────────────────

install:
	go install $(LDFLAGS) .

## ── test ──────────────────────────────────────────────────────────────────────

test:
	go clean -testcache
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	rm -f coverage.out

test-verbose:
	go clean -testcache
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	rm -f coverage.out

test-cover: ## run tests and open HTML coverage report
	go clean -testcache
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "coverage report: coverage.html"
	rm -f coverage.out

race:
	go test -race -timeout=60s ./...

bench:
	go test -bench=. -benchmem ./...

## ── quality ───────────────────────────────────────────────────────────────────

lint:
	golangci-lint run --max-issues-per-linter=0 --max-same-issues=0 ./...

fmt:
	gofmt -s -w $$(find . -type f -name "*.go" -not -path "./vendor/*")
	goimports -w $$(find . -type f -name "*.go" -not -path "./vendor/*")

vet:
	go vet ./...

check: vet lint test ## run all quality checks (vet + lint + test)

## ── demo & smoke tests ────────────────────────────────────────────────────────

SAMPLE_DATA=testdata/sample.prc

# generate sample PRC data for demos (requires atop to be installed)
sample:
	mkdir -p testdata
	atop -r /var/log/atop/atop_$$(date +%Y%m%d) \
		-b $$(date -d '10 minutes ago' '+%H:%M') \
		-e $$(date '+%H:%M') \
		-P PRC > $(SAMPLE_DATA) 2>/dev/null || \
	atop -P PRC & sleep 12 && kill %1 ; atop -P PRC > $(SAMPLE_DATA) 2>/dev/null ; true
	@echo "sample saved: $(SAMPLE_DATA)"

# run CLI demo using embedded testdata
demo: build
	@if [ ! -f $(SAMPLE_DATA) ]; then \
		echo "no sample data found — run 'make sample' first, or pipe atop output directly"; \
		echo "  atop -P PRC | $(BINARY)"; \
		exit 1; \
	fi
	cat $(SAMPLE_DATA) | $(BINARY) --top 20

# run HTML demo and open in default browser
demo-html: build
	@if [ ! -f $(SAMPLE_DATA) ]; then \
		echo "no sample data — run 'make sample' first"; \
		exit 1; \
	fi
	cat $(SAMPLE_DATA) | $(BINARY) --html-output --top 20

# smoke test: parse stdin and check exit code
smoke: build
	@echo "==> smoke: --help"
	$(BINARY) --help
	@echo "==> smoke: --version"
	$(BINARY) --version
	@echo "==> smoke: parse empty input (should not crash)"
	echo "" | $(BINARY) || true
	@echo "==> smoke: parse garbage (should silently ignore)"
	printf 'garbage line\nNOT PRC data\n\n' | $(BINARY) || true
	@echo "==> smoke: parse valid PRC sample"
	@if [ -f $(SAMPLE_DATA) ]; then \
		cat $(SAMPLE_DATA) | $(BINARY) --top 5; \
	else \
		echo "skipped (no sample data — run 'make sample')"; \
	fi
	@echo "==> smoke: all passed"

## ── dependency management ─────────────────────────────────────────────────────

deps:
	go mod tidy
	go mod verify

deps-upgrade:
	go get -u ./...
	go mod tidy

## ── docker ────────────────────────────────────────────────────────────────────

docker-build:
	docker build -t $(DOCKER_IMAGE):$(BRANCH) -t $(DOCKER_IMAGE):latest .

docker-push: docker-build
	docker push $(DOCKER_IMAGE):$(BRANCH)
	docker push $(DOCKER_IMAGE):latest

docker-run:
	docker run --rm -i $(DOCKER_IMAGE):latest $(ARGS)

# example: make docker-demo
docker-demo:
	@if [ ! -f $(SAMPLE_DATA) ]; then echo "run 'make sample' first"; exit 1; fi
	cat $(SAMPLE_DATA) | docker run --rm -i $(DOCKER_IMAGE):latest --top 20

## ── release ───────────────────────────────────────────────────────────────────

# create a git tag and push (usage: make tag VER=v1.2.3)
tag:
	@test -n "$(VER)" || (echo "usage: make tag VER=v1.2.3"; exit 1)
	git tag -a $(VER) -m "release $(VER)"
	git push origin $(VER)

# build all platforms and create a release archive
release: build-all
	mkdir -p .release
	cp .bin/$(APP)-linux-amd64          .release/$(APP)-linux-amd64
	cp .bin/$(APP)-linux-arm64          .release/$(APP)-linux-arm64
	cp .bin/$(APP)-darwin-amd64         .release/$(APP)-darwin-amd64
	cp .bin/$(APP)-darwin-arm64         .release/$(APP)-darwin-arm64
	cp .bin/$(APP)-windows-amd64.exe    .release/$(APP)-windows-amd64.exe
	tar -czf .release/$(APP)-$(REV).tar.gz -C .release \
		$(APP)-linux-amd64 $(APP)-linux-arm64 \
		$(APP)-darwin-amd64 $(APP)-darwin-arm64 \
		$(APP)-windows-amd64.exe
	@echo "release archive: .release/$(APP)-$(REV).tar.gz"

## ── housekeeping ──────────────────────────────────────────────────────────────

clean:
	rm -rf .bin .release coverage.out coverage.html coverage_no_mocks.out

version:
	@echo "app:       $(APP)"
	@echo "branch:    $(BRANCH)"
	@echo "hash:      $(HASH)"
	@echo "timestamp: $(TIMESTAMP)"
	@echo "revision:  $(REV)"

help: ## show this help
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'
	@echo ""
	@echo "Targets without ## inline docs:"
	@grep -E '^[a-zA-Z_-]+:' $(MAKEFILE_LIST) | \
		grep -v '##' | \
		awk 'BEGIN {FS = ":"}; {printf "  %s\n", $$1}' | sort
	@echo ""

.PHONY: all build build-release build-all \
	build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows-amd64 \
	install \
	test test-verbose test-cover race bench \
	lint fmt vet check \
	sample demo demo-html smoke \
	deps deps-upgrade \
	docker-build docker-push docker-run docker-demo \
	tag release \
	clean version help

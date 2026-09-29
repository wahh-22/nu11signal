# nu11signal build tooling. Signing is configured through the environment
# (see README.md): NU11SIGNAL_BUNDLE_ID, NU11SIGNAL_TEAM_ID, NU11SIGNAL_PROFILE,
# NU11SIGNAL_SIGN_IDENTITY. Defaults live in helper/build.sh.

export NU11SIGNAL_BUNDLE_ID NU11SIGNAL_TEAM_ID NU11SIGNAL_PROFILE NU11SIGNAL_SIGN_IDENTITY NU11SIGNAL_HELPER

GO     ?= go
BIN    := bin/nu11signal
PKG    := ./cmd/nu11signal

.PHONY: all build helper go demo test test-scripts vet fmt-check release release-dry-run cask check-version clean clean-dist

all: build

## build: signed helper bundle (build/Nu11SignalHelper.app) + Go binary (bin/nu11signal)
build: helper go

## helper: build, bundle, and sign the MusicKit helper
helper:
	./helper/build.sh

## go: build only the Go binary
go:
	$(GO) build -o $(BIN) $(PKG)

## demo: build only the Go binary and run it against the simulated player
demo: go
	./$(BIN) --demo

## test: Go tests with the race detector, the Swift protocol tests, then the script tests
test:
	$(GO) test -race ./...
	cd helper && swift test
	./scripts/test/run.sh

## test-scripts: hermetic tests for scripts/release.sh and scripts/bump-cask.sh (stubbed tools, temp dirs)
test-scripts:
	./scripts/test/run.sh

## vet: go vet
vet:
	$(GO) vet ./...

## fmt-check: fail when any Go file is not gofmt-formatted
fmt-check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

## release: signed, notarized universal archive in dist/vVERSION/ (VERSION=x.y.z required; FORCE=1 replaces an existing dist/vVERSION)
release: check-version
	./scripts/release.sh $(if $(filter 1,$(FORCE)),--force) $(VERSION)

## release-dry-run: assemble the release layout ad hoc in build/release-dry-run/vVERSION, without notarizing
release-dry-run: check-version
	./scripts/release.sh --dry-run $(VERSION)

## cask: render the Homebrew cask for VERSION (sha256 from dist/vVERSION/) into the tap checkout and audit it (PUSH=1 commits and pushes, or pushes an earlier unpushed bump)
cask: check-version
	./scripts/bump-cask.sh $(VERSION) $(if $(filter 1,$(PUSH)),--push)

check-version:
	@if [ -z "$(VERSION)" ]; then echo "usage: make $(MAKECMDGOALS) VERSION=x.y.z"; exit 2; fi

## clean: remove build outputs (bin/, build/); release archives in dist/ are kept
clean:
	rm -rf bin build

## clean-dist: remove release archives (dist/)
clean-dist:
	rm -rf dist

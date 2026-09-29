# soul-king build tooling. Signing is configured through the environment
# (see README.md): SOULKING_BUNDLE_ID, SOULKING_TEAM_ID, SOULKING_PROFILE,
# SOULKING_SIGN_IDENTITY. Defaults live in helper/build.sh.

export SOULKING_BUNDLE_ID SOULKING_TEAM_ID SOULKING_PROFILE SOULKING_SIGN_IDENTITY SOULKING_HELPER

GO     ?= go
BIN    := bin/soul-king
PKG    := ./cmd/soul-king

.PHONY: all build helper go demo test vet fmt-check release release-dry-run check-version clean

all: build

## build: signed helper bundle (build/SoulKingHelper.app) + Go binary (bin/soul-king)
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

## test: Go tests with the race detector, then the Swift protocol tests
test:
	$(GO) test -race ./...
	cd helper && swift test

## vet: go vet
vet:
	$(GO) vet ./...

## fmt-check: fail when any Go file is not gofmt-formatted
fmt-check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

## release: signed, notarized universal archive in dist/ (VERSION=x.y.z required)
release: check-version
	./scripts/release.sh $(VERSION)

## release-dry-run: build and assemble the release layout ad hoc, without notarizing
release-dry-run: check-version
	./scripts/release.sh --dry-run $(VERSION)

check-version:
	@if [ -z "$(VERSION)" ]; then echo "usage: make $(MAKECMDGOALS) VERSION=x.y.z"; exit 2; fi

## clean: remove build outputs
clean:
	rm -rf bin build dist

GO     := GO15VENDOREXPERIMENT=1 go
GINKGO := ginkgo
PROMU  := $(GOPATH)/bin/promu
pkgs   = $(shell $(GO) list ./... | grep -v /vendor/)

PREFIX                  ?= $(shell pwd)
BIN_DIR                 ?= $(shell pwd)
TARBALLS_DIR            ?= $(shell pwd)/.tarballs

all: format build test

deps:
	@$(GO) get github.com/onsi/ginkgo/ginkgo
	@$(GO) get github.com/onsi/gomega

format:
	@echo ">> formatting code"
	@$(GO) fmt $(pkgs)

style:
	@echo ">> checking code style"
	@! gofmt -d $(shell find . -path ./vendor -prune -o -name '*.go' -print) | grep '^'

vet:
	@echo ">> vetting code"
	@$(GO) vet $(pkgs)

test: deps
	@echo ">> running tests"
	@$(GINKGO) -r -race .

promu:
	@GOOS=$(shell uname -s | tr A-Z a-z) \
		GOARCH=$(subst x86_64,amd64,$(patsubst i%86,386,$(shell uname -m))) \
		$(GO) get -u github.com/prometheus/promu

build: promu
	@echo ">> building binaries"
	@$(PROMU) build --prefix $(PREFIX)

crossbuild: promu
	@echo ">> building cross-platform binaries"
	@$(PROMU) crossbuild

tarball: build
	@echo ">> building release tarball"
	@$(PROMU) tarball --prefix $(PREFIX) $(BIN_DIR)

tarballs: crossbuild
	@echo ">> building release tarballs"
	@$(PROMU) crossbuild tarballs

release: promu
	@echo ">> uploading tarballs to the Github release"
	@$(PROMU) release ${TARBALLS_DIR}

.PHONY: all deps format style vet test promu build crossbuild tarball tarballs release

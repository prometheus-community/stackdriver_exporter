DEFAULT: build

PROJECT_DIR          := $(shell pwd)
GO                   ?= go
GOFMT                ?= $(GO)fmt
STACKDRIVER_EXPORTER := $(PROJECT_DIR)/bin/stackdriver_exporter
BUILD_TIME           := $(shell date -u +%FT%T%z)
GIT_LATEST_COMMIT_ID := $(shell git rev-parse HEAD)
GO_VER               := $(shell go version | awk '{print $$3}')
TAGVER               ?= unspecified
LDFLAGS              =-ldflags "-extldflags '-static' -w -s -X main.applicationBuildTime=$(BUILD_TIME) -X main.applicationGitCommitID=$(GIT_LATEST_COMMIT_ID) -X main.applicationGoVersion=$(GO_VER) -X main.applicationGoArch=$(GOARCH)"
BUILD_SUBDIR         ?= .build
CGO_ENABLED          ?= 0
IMAGE                ?= "gcr.io/chronosphere-dev/stackdriver-exporter-external"
GOARCH               ?= $($(GO) env GOARCH)
GOOS                 ?= $($(GO) env GOOS)

GIT_REVISION         := $(shell git rev-parse --short=8 HEAD)
BUILD_DATE           := $(shell date -u  +"%Y-%m-%dT%H:%M:%SZ") # Use RFC-3339 date format
DOCKER_LABELS        := $(addprefix --label org.opencontainers.image.,\
	revision=$(GIT_REVISION) \
	created=$(BUILD_DATE))

.PHONY: build
build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) $(GO) build -mod=mod $(LDFLAGS) -o $(BUILD_SUBDIR)/stackdriver_exporter -a -v .

.PHONY: docker-local
docker-local:
	docker buildx build --platform=linux/amd64 -f ./deploy/Dockerfile -t $(IMAGE):$(GIT_LATEST_COMMIT_ID) $(DOCKER_LABELS) .

.PHONY: docker
docker:
	docker buildx build --platform=linux/amd64 -f ./deploy/Dockerfile -t $(IMAGE):$(GIT_LATEST_COMMIT_ID) $(DOCKER_LABELS) --push .


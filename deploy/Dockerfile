FROM --platform=$BUILDPLATFORM gcr.io/chronosphere-dev/golang-builder:1.21.1 AS builder

WORKDIR /build

ENV GOPRIVATE github.com/chronosphereio

COPY go.mod go.sum ./

RUN --mount=type=ssh \
    --mount=type=cache,target=/go/pkg \
    go mod download

RUN --mount=target=. \
    --mount=type=cache,target=/go/pkg \
    BUILD_SUBDIR=/tmp/bin GOOS=linux GOARCH=amd64 CGO_ENABLED=0 make build

FROM gcr.io/chronosphere-global-infra/service-base:89e5dd5b

COPY --from=builder /tmp/bin/stackdriver_exporter /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/stackdriver_exporter"]
EXPOSE     9255
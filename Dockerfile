ARG ARCH="amd64"
ARG OS="linux"
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:latest
LABEL maintainer="The Prometheus Authors <prometheus-developers@googlegroups.com>"

ARG ARCH="amd64"
ARG OS="linux"
COPY .build/${OS}-${ARCH}/stackdriver_exporter /bin/stackdriver_exporter
COPY LICENSE /LICENSE

USER       nobody
ENTRYPOINT ["/bin/stackdriver_exporter"]
EXPOSE     9255

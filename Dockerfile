FROM        quay.io/prometheus/busybox:latest
LABEL maintainer="The Prometheus Authors <prometheus-developers@googlegroups.com>"

COPY stackdriver_exporter /bin/stackdriver_exporter

ENTRYPOINT ["/bin/stackdriver_exporter"]
EXPOSE     9255

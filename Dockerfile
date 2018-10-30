###############################################################################
# compile stage
FROM golang:1.11

RUN mkdir -vp /go/src/github.com/frodenas/stackdriver_exporter/
COPY . /go/src/github.com/frodenas/stackdriver_exporter
# RUN go get -d github.com/frodenas/stackdriver_exporter

RUN CGO_ENABLED=0 GOOS=linux go install github.com/frodenas/stackdriver_exporter

###############################################################################
# binary stage
FROM alpine

COPY --from=0 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=0 /go/bin/stackdriver_exporter       /root/stackdriver_exporter

ENTRYPOINT ["/root/stackdriver_exporter"]
EXPOSE     9255

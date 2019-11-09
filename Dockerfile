FROM golang:1.13.3-alpine as builder

WORKDIR /go/src/github.com/aokumasan/nifcloud_nas_exporter
RUN apk add --no-cache make git
ADD . .
RUN make build

FROM alpine:3.10.3

COPY --from=builder /go/src/github.com/aokumasan/nifcloud_nas_exporter/bin/nifcloud_nas_exporter /bin/nifcloud_nas_exporter
ENTRYPOINT [ "/bin/nifcloud_nas_exporter" ]
EXPOSE 9123
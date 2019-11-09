PKG=github.com/aokumasan/nifcloud_nas_exporter
IMAGE?=aokumasan/nifcloud-nas-exporter
VERSION=v0.0.1
GIT_COMMIT?=$(shell git rev-parse HEAD)
GIT_BRANCH?=$(shell git symbolic-ref --short HEAD)
BUILD_DATE?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
USER?=$(shell id -n -u)
HOSTNAME?=$(shell hostname)
LDFLAGS?="-X github.com/prometheus/common/version.Version=${VERSION} \
-X github.com/prometheus/common/version.Revision=${GIT_COMMIT}  \
-X github.com/prometheus/common/version.Branch=${GIT_BRANCH}  \
-X github.com/prometheus/common/version.BuildUser=${USER}@${HOSTNAME} \
-X github.com/prometheus/common/version.BuildDate=${BUILD_DATE} -s -w"

build:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -ldflags ${LDFLAGS} -o bin/nifcloud_nas_exporter main.go

image:
	docker build -t $(IMAGE):$(VERSION) .

push:
	docker push $(IMAGE):$(VERSION)
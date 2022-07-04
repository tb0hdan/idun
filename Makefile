BUILD = $(shell git rev-parse HEAD)
BDATE = $(shell date -u '+%Y-%m-%d_%I:%M:%S%p_UTC')
GO_VERSION = $(shell go version|awk '{print $$3}')
VERSION = $(shell cat ./VERSION)
COMPOSE_TIMEOUT = 300

all: buildx

buildx:
	@docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 -t tb0hdan/idun:latest --push .
	@docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 -t tb0hdan/idun:v$(VERSION) --push .

build:
	@docker build -t tb0hdan/idun .

idun:
	@go build -o $@ ./cmd/*.go

idun-docker:
	@go build -a -trimpath -tags netgo -installsuffix netgo -v -x -ldflags "-s -w -X main.Build=$(BUILD) -X main.BuildDate=$(BDATE) -X main.GoVersion=$(GO_VERSION) -X main.Version=$(VERSION)" -o /$@ ./cmd/*.go
	@strip -S -x /idun-docker

docker-run:
	@docker run --env FREYA=$$FREYA --rm -it tb0hdan/idun

tag:
	@git tag -a v$(VERSION) -m v$(VERSION)
	@git push --tags

dockertag:
	@docker tag tb0hdan/idun tb0hdan/idun:v$(VERSION)
	@docker tag tb0hdan/idun tb0hdan/idun:latest
	@docker push tb0hdan/idun:v$(VERSION)
	@docker push tb0hdan/idun:latest

start:
	@COMPOSE_HTTP_TIMEOUT=$(COMPOSE_TIMEOUT) ./start.sh

stop:
	@COMPOSE_HTTP_TIMEOUT=$(COMPOSE_TIMEOUT) docker-compose rm -f -s

restart: stop start

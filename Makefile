BUILD = $(shell git rev-parse HEAD)
BDATE = $(shell date -u '+%Y-%m-%d_%I:%M:%S%p_UTC')
GO_VERSION = $(shell go version|awk '{print $$3}')
VERSION = $(shell cat ./VERSION)

all: build

build:
	@docker build -t tb0hdan/idun .

build-local:
	@go build -o idun *.go

idun:
	@go build -a -trimpath -tags netgo -installsuffix netgo -v -x -ldflags "-s -w  -X main.Build=$(BUILD) -X main.BuildDate=$(BDATE) -X main.GoVersion=$(GO_VERSION) -X main.Version=$(VERSION)" -o /idun *.go
	@strip -S -x /idun

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
	@COMPOSE_HTTP_TIMEOUT=120 ./start.sh

stop:
	@COMPOSE_HTTP_TIMEOUT=120 docker-compose rm -f -s

restart: stop start

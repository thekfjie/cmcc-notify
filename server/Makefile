SHELL := /bin/sh

GO ?= go
NPM ?= npm
OUT ?= ../dist/cmcc-notify
VERSION ?= dev

.PHONY: test vet check build docker web-install web-check web-build

test:
	GOWORK=off $(GO) test ./...

vet:
	GOWORK=off $(GO) vet ./...

check: test vet

web-install:
	cd web && $(NPM) ci

web-check:
	cd web && $(NPM) run typecheck

web-build:
	cd web && $(NPM) run build

build: check
	@mkdir -p ../dist
	GOWORK=off $(GO) build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(OUT) ./cmd/cmcc-notify

docker:
	docker build -f Dockerfile -t cmcc-notify/server:dev ..

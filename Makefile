SHELL := /bin/sh

GO ?= go
PLUGIN_DIR := gotify-plugin
SERVER_DIR := server

.PHONY: test test-integrations check test-independent check-boundaries build-server build-plugin fmt vet

test: test-integrations
	$(GO) test ./cmcc/... ./gotify-plugin/... ./server/...

test-integrations:
	python3 -m unittest discover -s integrations/codex -p 'test_*.py'

test-independent:
	cd cmcc && GOWORK=off $(GO) test ./...
	cd gotify-plugin && GOWORK=off $(GO) test ./...
	cd server && GOWORK=off $(GO) test ./...

check: test-integrations test-independent check-boundaries vet
	cd gotify-plugin && $(MAKE) check-gotify-mod

check-boundaries:
	@plugin_module="$$(cd gotify-plugin && GOWORK=off $(GO) list -m -f '{{.Path}}')"; \
	server_module="$$(cd server && GOWORK=off $(GO) list -m -f '{{.Path}}')"; \
	! (cd cmcc && GOWORK=off $(GO) list -deps -f '{{with .Module}}{{.Path}}{{end}}' ./... | \
		grep -Fxe "$$plugin_module" -e "$$server_module")
	@server_module="$$(cd server && GOWORK=off $(GO) list -m -f '{{.Path}}')"; \
	! (cd gotify-plugin && GOWORK=off $(GO) list -m -f '{{.Path}}' all | grep -Fx "$$server_module")

fmt:
	gofmt -w cmcc gotify-plugin server

vet:
	$(GO) vet ./cmcc/... ./gotify-plugin/... ./server/...

build-server:
	$(GO) build -trimpath -o bin/cmcc-notify ./server/cmd/cmcc-notify

build-plugin:
	$(MAKE) -C $(PLUGIN_DIR) build

SHELL := /bin/sh

GO ?= go
CURL ?= curl
GOTIFY_VERSION ?= v3.1.1
GO_VERSION ?= 1.26.0
PLUGIN_NAME ?= cmcc-notify-gotify
PLUGIN_VERSION ?= dev
BUILD_DIR ?= build
DIST_DIR ?= ../dist
ARM64_BUILD_IMAGE ?= docker.io/library/golang:$(GO_VERSION)-bookworm
AMD64_BUILD_IMAGE ?= docker.io/gotify/build:$(GO_VERSION)-linux-amd64
GOTIFY_MOD := $(BUILD_DIR)/gotify-server.mod

.PHONY: test vet check check-gotify-mod sync-gotify-mod build build-local build-linux-amd64 build-linux-arm64

test:
	GOWORK=off $(GO) test ./...

vet:
	GOWORK=off $(GO) vet ./...

check: test vet check-gotify-mod

$(GOTIFY_MOD):
	@mkdir -p $(BUILD_DIR)
	$(CURL) --fail --silent --show-error --location \
		-o $(GOTIFY_MOD) \
		https://raw.githubusercontent.com/gotify/server/$(GOTIFY_VERSION)/go.mod

# gotify/plugin-template uses gomod-cap to ensure shared dependencies never
# exceed the target server's versions. check-gotify-mod is read-only.
check-gotify-mod: $(GOTIFY_MOD)
	GOWORK=off $(GO) run github.com/gotify/plugin-api/cmd/gomod-cap@v1.0.0 \
		-check -from $(GOTIFY_MOD) -to go.mod

sync-gotify-mod: $(GOTIFY_MOD)
	GOWORK=off $(GO) run github.com/gotify/plugin-api/cmd/gomod-cap@v1.0.0 \
		-from $(GOTIFY_MOD) -to go.mod
	GOWORK=off $(GO) mod tidy

build-local: check
	@mkdir -p $(DIST_DIR)
	GOWORK=off $(GO) build -mod=readonly -a -installsuffix cgo -buildmode=plugin \
		-ldflags "-X main.version=$(PLUGIN_VERSION)" \
		-o $(DIST_DIR)/$(PLUGIN_NAME)-linux-$$(go env GOARCH)-for-gotify-$(GOTIFY_VERSION).so .

build-linux-amd64: check-gotify-mod
	@mkdir -p $(DIST_DIR)
	docker run --rm --platform linux/amd64 \
		-v "$$(pwd)/..:/src" -w /src/gotify-plugin \
		$(AMD64_BUILD_IMAGE) \
		env GOWORK=off go build -mod=readonly -a -installsuffix cgo -buildmode=plugin \
		-ldflags "-X main.version=$(PLUGIN_VERSION)" \
		-o /src/gotify-plugin/$(DIST_DIR)/$(PLUGIN_NAME)-linux-amd64-for-gotify-$(GOTIFY_VERSION).so .

# As of 2026-09-16, gotify/build:1.26.0-linux-arm64 resolves to an amd64
# manifest. Use the exact Go version from the Gotify 3.1.1 binary instead.
build-linux-arm64: check-gotify-mod
	@mkdir -p $(DIST_DIR)
	docker run --rm --platform linux/arm64 \
		-v "$$(pwd)/..:/src" -w /src/gotify-plugin \
		$(ARM64_BUILD_IMAGE) \
		env GOWORK=off go build -mod=readonly -a -installsuffix cgo -buildmode=plugin \
		-ldflags "-X main.version=$(PLUGIN_VERSION)" \
		-o /src/gotify-plugin/$(DIST_DIR)/$(PLUGIN_NAME)-linux-arm64-for-gotify-$(GOTIFY_VERSION).so .

build:
	@case "$$(go env GOARCH)" in \
		arm64) $(MAKE) build-linux-arm64 ;; \
		amd64) $(MAKE) build-linux-amd64 ;; \
		*) echo "unsupported plugin build architecture: $$(go env GOARCH)" >&2; exit 1 ;; \
	esac

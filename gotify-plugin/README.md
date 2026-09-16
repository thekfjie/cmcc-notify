# Gotify Plugin

This directory is an independent Gotify v1 Go Plugin project. Gotify creates
one plugin instance per user; each instance owns its CMCC accounts, routes and
deduplication state.

## Build and test

Run the module without the repository workspace:

```bash
GOWORK=off go test ./...
make check
```

Build artifacts are tied to both the target architecture and exact Gotify
version:

```bash
make build GOTIFY_VERSION=v3.1.1
```

The current build target is Gotify 3.1.1. Re-run `make check-gotify-mod` and
build against the matching Gotify/Go toolchain before changing that target.

## Configuration

Configure the plugin in Gotify's plugin page. A minimal configuration is:

```yaml
enabled: true
gotify_url: http://127.0.0.1:80
client_token: C_replace_with_a_dedicated_gotify_client_token
include_title: true
title_separator: "\\n\\n"
accounts:
  - name: primary
    api_key: ak_replace_with_cmcc_key
    enabled: true
    default_to: "13800138000"
routes:
  - applications: [1, 2]
    accounts: [primary]
    minimum_priority: 0
```

The `client_token` is required because the Gotify Plugin API does not expose a
receive-all-messages callback. The plugin connects to the user's `/stream`
endpoint with this dedicated token. Do not reuse an application token.

The CMCC API key is validated for the documented `ak_`/`app_` prefixes and is
never written to logs or the display page in full. Gotify's plugin config is
stored in its database; operators should protect the database and backups.

## Gotify plugin page

The plugin intentionally uses Gotify's native plugin detail page instead of
embedding the Standalone Server WebUI. Gotify renders the YAML configuration
editor through the plugin `Configurer` capability and renders a Markdown
status summary through `Displayer`. The summary includes the Gotify stream,
redacted CMCC accounts, routing rules, forwarding counters and direct endpoint
paths. Custom handlers remain under Gotify's standard plugin-token route.

Account names in the YAML are routing identifiers. If an account is renamed,
update the matching values in `routes[].accounts` in the same edit. The
Standalone Server can migrate these references automatically because it owns
both records; Gotify intentionally leaves YAML editing and validation inside
its native plugin page.

## Direct endpoint

Gotify registers the plugin's custom route under the plugin token. The plugin
adds:

```text
POST /plugin/<id>/custom/<plugin-token>/send
GET  /plugin/<id>/custom/<plugin-token>/status
```

Text request:

```json
{"account":"primary","to":"13800138000","text":"hello"}
```

CMCC rich-media request:

```json
{"account":"primary","to":"13800138000","media":{"type":"IMAGE","url":"https://example.invalid/a.jpg","caption":"photo"}}
```

If `to` is omitted, the account's `default_to` is used. Gotify stream media
extras may also provide `to` or `phone`; otherwise forwarding uses the account
default. Recipient-addressed PNG image and ZIP file delivery was verified on
2026-09-16. The plugin is a native Gotify product and does not require
OpenClaw.

The returned `accepted` field means the gateway accepted the WebSocket write;
`acknowledged` remains false until the CMCC protocol defines a reliable text
acknowledgement.

## Community listing

After the repository URL and first release are final, `gotify/contrib` can
link directly to this directory:

```text
https://github.com/thekfjie/cmcc-notify/tree/main/gotify-plugin
```

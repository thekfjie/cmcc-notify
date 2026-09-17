# CMCC Notify Gotify Plugin

[简体中文](README.md) | [English](README.en.md)

This independent Gotify v1 Go Plugin forwards Gotify messages to China Mobile
New Message. It uses Gotify's native configuration editor and plugin detail
page; it does not embed the Standalone Server WebUI.

## Build and test

```bash
GOWORK=off go test ./...
make check
make build GOTIFY_VERSION=v3.1.1
```

Go Plugin artifacts must match the target Gotify version, Go toolchain and CPU
architecture. The current build target is Gotify 3.1.1.

## Configuration

```yaml
enabled: true
gotify_url: http://127.0.0.1:80
client_token: C_replace_with_a_dedicated_gotify_client_token
include_title: true
title_separator: "\n\n"

accounts:
  - name: primary
    note: "Operations on-call device"
    api_key: ak_replace_with_cmcc_key
    enabled: true

routes:
  - applications: [1, 2]
    accounts: [primary]
    minimum_priority: 0
```

`client_token` must be a dedicated Gotify client token; the plugin consumes
messages from the user's `/stream`. Each `accounts` entry is a CMCC channel
with a Channel API Key, name, optional note and enabled state. Messages are
sent to the user bound to that key.

Routes match Gotify Application IDs and forward to one or more named CMCC
channels. `minimum_priority` is a local Gotify filter only; CMCC does not render
Gotify priority as a message style. Without routes, every message is forwarded
to all enabled channels.

## Gotify plugin page

The plugin implements Gotify's `Configurer`, `Displayer`, `Storager` and
`Webhooker` interfaces. Gotify's standard plugin page displays stream and
channel status, redacted keys, routing rules, counters, recent errors and the
custom endpoint paths.

When renaming a channel, update matching `routes[].accounts` entries in the
same configuration change.

## Direct endpoint

```text
POST /plugin/<id>/custom/<plugin-token>/send
GET  /plugin/<id>/custom/<plugin-token>/status
```

Text request:

```json
{"account":"primary","text":"backup completed"}
```

Remote-media request:

```json
{
  "account": "primary",
  "media": {
    "type": "IMAGE",
    "url": "https://example.com/screenshot.png",
    "caption": "monitoring screenshot"
  }
}
```

The plugin does not embed `caption` in the media frame. It sends a plain-text
message first, followed by a media message without content. When `caption` is
empty, the Gotify message body is used as companion text. This avoids losing
the description on clients that do not display media-frame content.

The remote URL must be accessible to the CMCC gateway. Use the Standalone
Server `/v1/send/media` endpoint when local-file upload is required.

## Rendering

Real-device testing on September 17, 2026 showed plain-text rendering with
preserved line breaks, automatic URL detection, Unicode and emoji support.
Markdown, HTML, tables and code fences are displayed as ordinary strings.
Media companion text is sent as a separate plain-text message first.

HTTP `202 Accepted` or `accepted: true` only means the request was written to
the CMCC gateway; it is not a delivery or read receipt.

## Installation

Place the `.so` artifact matching the Gotify version and architecture in the
Gotify plugin directory, then restart Gotify. Back up the Gotify database and
plugin configuration before production upgrades.

## Community listing

`gotify/contrib` can link directly to:

```text
https://github.com/thekfjie/cmcc-notify/tree/main/gotify-plugin
```

## License

MIT

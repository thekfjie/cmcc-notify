# CMCC Notify

CMCC Notify is an open-source integration for China Mobile New Message. One
repository ships two products on top of one shared CMCC protocol library.

> 中文：本项目在同一个 monorepo 中维护 Gotify 插件和独立推送服务，二者
> 共享 `cmcc/` 协议实现，避免重复维护。

## Choose a product

| You are… | Use | Directory |
| --- | --- | --- |
| Already using Gotify | **Gotify Plugin** — forward selected Gotify messages to CMCC | [`gotify-plugin/`](gotify-plugin/) |
| Looking for a standalone deployment | **Standalone Server** — application tokens, notification REST API, management WebUI and Docker deployment | [`server/`](server/) |

Standalone 的开通、CMCC 通道、本地号码组、应用级通知 API 以及单目标/本地扇出文字与多媒体发送，请直接查看
[`server/README.md`](server/README.md)。相同说明也内置在 WebUI 的“API 文档 → 使用指南”中。

Both products use [`cmcc/`](cmcc/), which owns only CMCC authentication,
WebSocket transport, text and rich-media frames, uploads, protocol errors and
connection management. It contains no Gotify or standalone-server policy.

## Repository layout

```text
cmcc-notify/
├── cmcc/                  shared CMCC Go SDK (independent module)
├── gotify-plugin/         complete Gotify Plugin project (independent module)
├── server/                complete Standalone Server (independent module)
├── integrations/          optional examples built on the public REST API
├── deployments/           deployment examples for both products
├── docs/                  architecture and audited protocol notes
├── .github/workflows/     product-specific test and release pipelines
└── go.work                local multi-module development only
```

The three directories have separate `go.mod` files. Server-only dependencies
do not enter the Gotify Plugin module graph. `go.work` is a developer
convenience and is not required by released binaries.

## Optional integrations

Optional adapters live under [`integrations/`](integrations/). They are not
additional server products and are never required to use the REST API. The
Codex example provides a user-customizable lifecycle-hook adapter and a generic
one-shot sender while keeping `POST /v1/notify` as the only integration
contract. Users may instead call that endpoint directly from their own scripts.

See [`integrations/codex/`](integrations/codex/) for the optional Codex example.

## Development

```bash
make test
make check
make build-server
make build-plugin
```

Each product can also be checked from its own directory:

```bash
cd gotify-plugin && make test
cd ../server && GOWORK=off go test ./...
```

## Versioning and releases

The products are versioned independently:

- `cmcc/vX.Y.Z` — shared Go SDK module;
- `gotify-plugin/vX.Y.Z` — Gotify Plugin binaries;
- `server/vX.Y.Z` — Standalone Server binaries and multi-architecture GHCR
  images.

Gotify Plugin artifacts include the target Gotify version and architecture in
their filename. Standalone Server and Gotify Plugin binary releases also
publish adjacent `.sha256` files. A Go Plugin binary is not assumed compatible
with arbitrary Gotify builds.

## Security

Never commit CMCC API keys, Gotify client tokens, Standalone administrator
tokens or application tokens.
Examples use environment-variable references. See [`SECURITY.md`](SECURITY.md)
for reporting vulnerabilities and credential-handling guidance.

## Protocol status

China Mobile's public channel guide is
[`channel-guide.md`](https://5gvas01.cmicmaap.com/aifile/public/file/channel-guide.md).
It states that the China Mobile New Message channel supports text and
rich-media messages. The initial wire-format implementation was also informed
by a static audit of a CMCC-distributed channel reference implementation on
2026-09-16. CMCC Notify does not depend on or deploy OpenClaw: its two products
are the Gotify Plugin and the Standalone Server.

On 2026-09-16, recipient-addressed rich-media delivery was verified end to end
with both a PNG image and a ZIP file. The successful frame includes `to`,
`mediaType`, `mediaUrl`, file metadata and a message ID; the gateway returned
`media_processed`, and both payloads reached the target China Mobile New
Message client. Text and rich media therefore share the same explicit
recipient-routing model in CMCC Notify. The protocol still does not expose a
reliable delivery/read receipt, so the API reports `accepted` separately from
`acknowledged`.

See [`docs/cmcc-protocol.md`](docs/cmcc-protocol.md) for the known wire format
and limitations.

## Concepts and local fan-out

CMCC Notify uses a **CMCC channel** to mean one configured Channel API Key and
uses `to` as the observed recipient-routing field. A **local number group** is
only a stored list of `to` targets: sending to it produces N one-to-one frames
through the selected channel. It is not a native CMCC group, broadcast API or
chat room.

The exact Chinese terminology, evidence levels and current/future data models
are documented in [`docs/concepts.md`](docs/concepts.md).

## Source repository

The canonical repository is `github.com/thekfjie/cmcc-notify`. The three Go
modules use paths below that namespace and remain independently buildable.

## License

MIT

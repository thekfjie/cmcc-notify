# CMCC Notify

[简体中文](README.md) | [English](README.en.md)

CMCC Notify is an open-source notification gateway for China Mobile New
Message. This monorepo ships two products backed by one shared Go SDK:

| Use case | Product | Documentation |
| --- | --- | --- |
| Already using Gotify | Gotify Plugin: forward Gotify messages to CMCC | [gotify-plugin/](gotify-plugin/) |
| Need a standalone gateway | Standalone Server: WebUI, REST API, application tokens and Docker | [server/](server/) |

## Features

- Multiple named CMCC channels, each with a Channel API Key and optional note;
- Text messages, image delivery and file upload delivery;
- Notification groups that fan out to multiple CMCC channels;
- Scoped application tokens for monitoring, scripts, NAS, CI/CD and agents;
- WebSocket authentication, heartbeat, reconnection, uploads and structured errors;
- AMD64/ARM64 binaries, container images and independent product releases.

A CMCC channel represents one Channel API Key, and messages are delivered to
the user bound to that key. A notification group stores channel names and returns a per-channel
result for each fan-out request.

## Obtain a Channel API Key

1. [Enable the phone-level New Message switch](https://mp.weixin.qq.com/s/sWdK7mbKnLOdZmXXOMjAYA) to turn on the underlying device capability.
2. [Activate the New Message service](https://rcs.10086.cn/i/#/?RPwXWk9k0yk) with the China Mobile number that should receive notifications.
3. Open the New Message ClawBot service account, choose **Bind/Unbind → Authorize now**, and complete authentication to obtain the dedicated API Key.

The Standalone WebUI accepts either a bare `ak_...` value or the complete
ClawBot authorization message. Both frontend and backend retain only the
extracted API Key.

## Repository layout

```text
cmcc-notify/
├── cmcc/                  shared CMCC Go SDK (independent module)
├── gotify-plugin/         Gotify Plugin (independent module)
├── server/                Standalone Server (independent module)
├── integrations/          optional integration examples
├── deployments/           deployment examples
├── docs/                  architecture and protocol notes
├── .github/workflows/     product-specific CI and releases
└── go.work                local multi-module development
```

All three modules build independently with `GOWORK=off`. Server-only
dependencies do not enter the Gotify Plugin dependency graph.

## Getting started

- Standalone Server: [中文](server/README.md) · [English](server/README.en.md)
- Gotify Plugin: [中文](gotify-plugin/README.md) · [English](gotify-plugin/README.en.md)
- Go SDK: [cmcc/README.md](cmcc/README.md)
- Architecture: [docs/architecture.md](docs/architecture.md)
- Protocol notes: [docs/cmcc-protocol.md](docs/cmcc-protocol.md)

## Development

```bash
make test
make check
make build-server
make build-plugin
```

Each module can also be tested independently:

```bash
cd cmcc && GOWORK=off go test ./...
cd ../gotify-plugin && GOWORK=off go test ./...
cd ../server && GOWORK=off go test ./...
```

## Versioning

- `cmcc/vX.Y.Z` — shared Go SDK;
- `gotify-plugin/vX.Y.Z` — Gotify Plugin artifacts;
- `server/vX.Y.Z` — Standalone binaries and multi-architecture images.

Go Plugin artifacts are tied to the target Gotify version, Go toolchain and
architecture and should not be assumed compatible across versions.

## Security

Never commit CMCC API keys, Gotify client tokens, Standalone administrator
tokens or application tokens. See [SECURITY.md](SECURITY.md).

## License

MIT

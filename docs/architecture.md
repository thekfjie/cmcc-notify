# Architecture

```text
cmcc-notify/
├── cmcc/                         shared CMCC SDK
├── gotify-plugin/                independent Gotify Go Plugin
├── server/                       independent standalone HTTP service
├── deployments/                  product-specific deployment material
├── docs/                         protocol and architecture notes
└── .github/workflows/            test and release workflows
```

## Shared boundary

`cmcc/` owns only the CMCC protocol: WebSocket authentication, connection
state, heartbeat, reconnect loop, text/media frames, and multipart upload. It
does not know about Gotify users or HTTP API authentication.

## Gotify product

`gotify-plugin/` is compiled as a Go Plugin and loaded by Gotify 3.x. Each
Gotify user gets an isolated plugin instance. The instance connects to that
user's Gotify `/stream` using a dedicated client token, filters by application
and priority, then forwards to configured CMCC channels (stored under the
compatibility configuration key `accounts`). Its custom webhook is
for direct sends and is protected by Gotify's plugin token path. Configuration
and status stay inside Gotify's native plugin detail page through the official
Configurer and Displayer capabilities; the plugin does not embed the
Standalone Server's separate React control panel.

## Standalone product

`server/` exposes health, authenticated management/test endpoints and an
application-scoped `POST /v1/notify` production endpoint. Each notification
application binds an independent token to one CMCC channel and a fixed `to`
target or local number group. A local number group is expanded by the server
into N one-to-one gateway frames; it is not a native CMCC broadcast. Tokens
are generated with cryptographic randomness,
shown once, persisted only as SHA-256 hashes, independently rate-limited and
revocable without changing the administrator credential or CMCC API key.

The responsive React WebUI is compiled to static assets and embedded in the
same Go binary, so standalone deployments do not need a separate frontend
service. It is primarily a management and connectivity-test console. The
browser uses the configured administrator bearer token and receives only
redacted account fields. CMCC API keys and the administrator token are loaded
from environment variables or secret files referenced by YAML, keeping
credentials out of examples and version control.

The current release deliberately has no generic Recipient entity. Groups store
target strings rather than recipient IDs, and every send selects exactly one
CMCC channel. See [`concepts.md`](concepts.md) for the terminology boundary and
the possible future multi-channel recipient model.

## Compatibility

The checked-in deployment targets Gotify 3.1.1 on Linux ARM64. The plugin must
be built with Gotify's matching Go build image and dependency graph; a plugin
binary is not treated as universally compatible across arbitrary Gotify builds.

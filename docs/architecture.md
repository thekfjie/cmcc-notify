# Architecture

CMCC Notify is a three-module Go monorepo with one shared protocol layer and
two independently released products.

```text
                         ┌─────────────────────┐
Gotify messages ────────>│ Gotify Plugin       │
                         └──────────┬──────────┘
                                    │
Applications / WebUI ───>┌──────────▼──────────┐
                         │ Standalone Server   │
                         └──────────┬──────────┘
                                    │
                         ┌──────────▼──────────┐
                         │ cmcc/ Go SDK        │
                         └──────────┬──────────┘
                                    │
                         China Mobile gateway
```

## Module boundaries

### `cmcc/`

Owns WebSocket authentication, connection state, heartbeat, reconnection,
outbound text/media frames, multipart upload and protocol errors. It contains
no Gotify, HTTP management or application-routing policy.

### `gotify-plugin/`

Runs inside Gotify as a native Go Plugin. Each plugin instance consumes the
user's Gotify stream through a dedicated client token, applies Application ID
and priority filters, and forwards matching messages to one or more configured
CMCC channels. Configuration and status use Gotify's standard plugin page.

### `server/`

Runs as an independent HTTP service. It embeds the React WebUI and provides:

- administrator-authenticated status and management endpoints;
- direct text and media test endpoints;
- scoped notification applications and one-time application tokens;
- notification groups composed of multiple CMCC channels;
- per-application in-process rate limiting.

## Standalone data relationships

```text
CMCC channel
├── name
├── optional note
├── Channel API Key
└── connection settings

Notification group ──> CMCC channel names[]

Notification application
├── token hash
├── one CMCC channel OR one notification group
└── per-minute request limit
```

Normal sends omit a recipient value and rely on the Channel API Key's bound
user. A notification-group request is expanded into one independent send per
channel. This fan-out is implemented by CMCC Notify and returns per-channel
results.

## Credential boundaries

- The administrator token protects the WebUI and management/test API.
- Application tokens protect `POST /v1/notify` and are stored only as hashes.
- CMCC Channel API Keys authenticate gateway connections and uploads.
- Gotify client tokens are used only by the plugin to consume `/stream`.

These credentials are not interchangeable. Runtime secret files or
environment variables are preferred over inline YAML.

## Frontend packaging

The Standalone React application is compiled into `server/internal/httpapi/web`
and embedded in the Go binary. A deployment therefore needs only one server
process and no separate frontend service.

## Release boundaries

- `cmcc/vX.Y.Z` releases the SDK module;
- `gotify-plugin/vX.Y.Z` releases version-specific plugin artifacts;
- `server/vX.Y.Z` releases Standalone binaries and container images.

The root `go.work` is used only for local development. CI verifies each module
with `GOWORK=off`.

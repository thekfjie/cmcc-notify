# CMCC Notify Standalone Server

[简体中文](README.md) | [English](README.en.md)

Standalone Server is a self-hosted CMCC notification gateway for monitoring,
NAS systems, scripts, CI/CD, agents and application services. It provides a
management WebUI, REST API, scoped application tokens, multiple CMCC channels,
notification groups and Docker deployment.

## Obtain a Channel API Key

1. [Enable the phone-level New Message switch](https://mp.weixin.qq.com/s/sWdK7mbKnLOdZmXXOMjAYA), which controls the underlying device capability.
2. [Activate the New Message service](https://rcs.10086.cn/i/#/?RPwXWk9k0yk) for the China Mobile number that should receive notifications.
3. Open the New Message ClawBot service account, choose **Bind/Unbind → Authorize now**, and complete authentication to obtain the dedicated API Key.

When adding or editing a channel, the WebUI accepts either a bare `ak_...`
value or the complete ClawBot authorization message. The browser extracts the
key immediately and the server normalizes it again before persistence; the
surrounding message is not written to state or logs. Configuration, environment
and secret-file inputs use the same extraction behavior.

## Model

- **CMCC channel** — one named Channel API Key configuration with an optional
  note. Messages are delivered to the user bound to the key.
- **Notification group** — a set of CMCC channels. A group request sends once
  through every channel and returns per-channel results.
- **Notification application** — a scoped caller identity with its own token,
  fixed channel or group, and per-minute request limit.

## Quick deployment

```bash
cd deployments/standalone
cp config.example.yaml config.yaml
mkdir -p secrets data
printf '%s' '<ADMIN_TOKEN>' > secrets/auth_token
printf '%s' '<CMCC_CHANNEL_API_KEY>' > secrets/cmcc_api_key
chmod 600 secrets/*
docker compose up -d --build
```

The example binds to `127.0.0.1:8080`. Use an HTTPS reverse proxy before
publishing it and restrict access to management endpoints.

## Configuration

```yaml
listen: ":8080"
auth_token_file: /run/secrets/auth_token
state_file: /var/lib/cmcc-notify/state.yaml

accounts:
  - name: primary
    note: "Operations on-call device"
    enabled: true
    api_key_file: /run/secrets/cmcc_api_key

groups:
  - name: on-call
    channels: [primary]

applications: []
```

`accounts` is retained as the configuration/API field name; the WebUI calls
these records CMCC channels. With `state_file` enabled, channels, groups and
applications can be managed without restarting the service. Complete API keys
are never returned to the browser.

## Application API

Create an application in **Account Status → Notification Applications** and
bind it to one channel or group. The full `cn_app_...` token is shown once;
only its SHA-256 hash is stored.

```bash
curl -X POST http://localhost:8080/v1/notify \
  -H "Authorization: Bearer <APPLICATION_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"title":"Service alert","message":"Disk usage is above 90%"}'
```

The body accepts `title` plus `message`, or a single `text` field. Callers
cannot override the channel or group selected by the application.

## Management and test API

Management endpoints use the administrator token. A single-channel text send:

```bash
curl -X POST http://localhost:8080/v1/send \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"account":"primary","text":"backup completed"}'
```

Notification-group send:

```bash
curl -X POST http://localhost:8080/v1/send \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"group":"on-call","text":"service unavailable"}'
```

Create a group with:

```json
{"name":"on-call","channels":["primary","backup"]}
```

## Media

Local files are uploaded with the destination channel key before the media
message is sent:

```bash
curl -X POST http://localhost:8080/v1/send/media \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -F "account=primary" \
  -F "type=IMAGE" \
  -F "caption=monitoring screenshot" \
  -F "file=@./screenshot.png"
```

`caption` is optional companion text. Real-device clients did not reliably
display text embedded in a media frame, so CMCC Notify sends the caption as a
plain-text message first and then sends a media message without content. With
an empty caption, only the media message is sent.

Group uploads are performed separately for every channel. Supported request
types are `AUTO`, `IMAGE`, `AUDIO`, `VIDEO` and `FILE`, with a 200 MiB limit.
Remote media URLs are also accepted by `/v1/send`, but the upload flow is the
recommended path.

## Rendering and responses

Real-device testing on September 17, 2026 showed plain-text rendering with
preserved line breaks, automatic URL detection, Unicode and emoji support.
Markdown, HTML, tables and fenced code are displayed as ordinary text. Images
and files were verified through the upload-then-send flow. Media companion
text is sent as a separate plain-text message before the media. A CMCC Notify
0.4.0 real-device check confirmed the same order: the text appeared first,
followed by a separate multimedia notification and its client web entry.

HTTP `202 Accepted` means the request was written to the CMCC gateway. A group
with partial failures returns `207 Multi-Status`. Neither status is a delivery
or read receipt.

## Build

```bash
cd server
GOWORK=off go test ./...
GOWORK=off go vet ./...
make web-build
make build VERSION=dev
```

```bash
docker build -f server/Dockerfile -t cmcc-notify/server:dev .
```

## Security

Use secret files or environment variables, never place tokens in URLs or
logs, create a separate application for each caller, configure appropriate
rate limits and use HTTPS plus edge access controls for public deployments.

## License

MIT

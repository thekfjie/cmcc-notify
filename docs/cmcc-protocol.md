# CMCC protocol notes

This document records the protocol behavior implemented by the shared SDK and
verified by the project. The China Mobile channel guide is available at
[`channel-guide.md`](https://5gvas01.cmicmaap.com/aifile/public/file/channel-guide.md).

## Endpoints and authentication

- WebSocket: `wss://5gvas01.cmicmaap.com/gtw-ai/openclaw/ws/msg`
- Upload base: `https://5gvas01.cmicmaap.com/gtw-ai/openclaw/api`
- Handshake header: `X-API-Key: <key>`
- Initial frame: `{"type":"auth","apiKey":"<key>","version":"2.0"}`
- Success frame: `{"type":"auth_ok"}`

The SDK sends periodic `ping` frames, monitors `pong`, and reconnects with
backoff after connection loss.

## Text send

The normal product path omits `to`:

```json
{
  "type": "send",
  "apiKey": "ak_...",
  "content": "hello",
  "messageId": "msg_..."
}
```

Live testing on September 17, 2026 confirmed that an authenticated key can
deliver text to its bound user without a `to` field. The SDK still accepts an
optional explicit `To` value for protocol-level compatibility, but the
Standalone Server and Gotify Plugin do not require or expose a phone-number
target.

## Media upload and send

Files are uploaded with `multipart/form-data` to `/upload`:

- `file`: the binary payload;
- `apiKey`: the Channel API Key.

Success code `10200` returns the media URL in `data`. The client-side maximum
upload size is 200 MiB.

The returned URL is then used in a media frame:

```json
{
  "type": "send",
  "apiKey": "ak_...",
  "mediaType": "IMAGE",
  "mediaUrl": "https://...",
  "mediaFileName": "photo.jpg",
  "mediaMimeType": "image/jpeg",
  "mediaSize": 12345,
  "messageId": "msg_..."
}
```

Supported media discriminators are `IMAGE`, `TEXT`, `AUDIO`, `VIDEO` and
`FILE`. Real-device tests verified an uploaded image and an uploaded file.
Externally hosted media URLs may be accepted by the gateway but are less
predictable than the upload-then-send flow.

Real-device tests did not reliably display the `content` value of a media
frame. The Standalone Server and Gotify Plugin therefore implement companion
text as two ordered sends: a plain-text frame first, then a media frame with no
`content` field. This is product behavior built on top of the protocol, not a
native atomic text-and-media message.

The ordered behavior was verified again on a real device on September 17,
2026 with CMCC Notify 0.4.0: the companion text appeared first, followed by a
separate multimedia notification containing the client-provided web link.
This confirms the split-send approach preserves both the description and the
media entry in the observed CMCC client.

## Text rendering

Real-device tests on September 17, 2026 established the following client
behavior:

- plain text and line breaks are preserved;
- URLs are detected by the client;
- Unicode and emoji are displayed;
- Markdown, HTML, tables and fenced code are not rendered as formatting.

Applications should therefore compose concise plain-text notifications and use
ordinary URLs when additional detail is needed.

## Notification groups

No native group frame is used by this project. A Standalone notification group
contains multiple configured Channel API Keys. CMCC Notify sends one frame per
channel and returns `channel_fanout` results. For local media uploads, each
channel uploads with its own key before sending.

## Acknowledgement semantics

Local WebSocket write success and HTTP `202 Accepted` mean the gateway accepted
the request. `media_processed` indicates media processing by the gateway. The
project does not treat either result as a reliable terminal delivery or read
receipt.

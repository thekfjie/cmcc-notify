# CMCC protocol notes

These notes are derived from a static audit of the CMCC-distributed
`openclaw-cmcc-newmsg-channel-1.0.0.tgz` channel reference package downloaded
on 2026-09-16. The package is protocol evidence only: CMCC Notify neither
depends on nor deploys OpenClaw. The installer was not executed and no
credential was embedded in the audit.

The public China Mobile installation guide is
[`channel-guide.md`](https://5gvas01.cmicmaap.com/aifile/public/file/channel-guide.md).
It describes the China Mobile New Message Channel as supporting text and
rich-media messages. Recipient-addressed rich-media delivery was subsequently
verified against the live gateway and a real China Mobile New Message client
on 2026-09-16. The public guide does not document a recipient-array API,
native broadcast, `groupId`, delivery receipts, or the authorization scope of
arbitrary `to` values.

## Connection

- WebSocket endpoint: `wss://5gvas01.cmicmaap.com/gtw-ai/openclaw/ws/msg`
- Upload base URL: `https://5gvas01.cmicmaap.com/gtw-ai/openclaw/api`
- Protocol version: `2.0`
- Handshake header: `X-API-Key: <key>`
- First frame: `{"type":"auth","apiKey":"<key>","version":"2.0"}`
- Authentication result: `{"type":"auth_ok"}` or `{"type":"auth_failed",...}`
- Heartbeat: client sends `{"type":"ping"}` every 15 seconds and expects
  `{"type":"pong"}` within 10 seconds.

## Outbound frames

Text:

```json
{"type":"send","apiKey":"ak_...","to":"13800138000","content":"hello","messageId":"msg_..."}
```

Rich media:

```json
{
  "type":"send",
  "apiKey":"ak_...",
  "to":"13800138000",
  "mediaType":"IMAGE",
  "content":"caption",
  "mediaUrl":"https://...",
  "messageId":"msg_..."
}
```

Supported media discriminators are `IMAGE`, `TEXT`, `AUDIO`, `VIDEO` and
`FILE`. The distributed reference implementation omitted `to` from its media
builder, but live testing established that proactive rich-media delivery uses
the same explicit `to` recipient field as text. A PNG image and an 8.6 MiB ZIP
file both produced `media_processed` and were received by the target terminal.

## Upload

The package posts multipart form data to `/upload` with:

- `file`: binary upload, filename preserved;
- `apiKey`: the CMCC API key.

The expected JSON response is a `DataResult`; code `10200` is success and the
remote media URL is returned in `data`. The reference implementation uses a
200 MiB client-side limit and a 60-second upload timeout.

## Recipient routing and local fan-out

The observed and tested outbound schema uses a single `to` value per `send`
frame. CMCC Notify therefore treats `to` as active routing data, not descriptive
phone metadata. Whether a particular target is authorized remains a CMCC
gateway decision; this project must not be described as an unrestricted SMS
gateway.

There is currently no verified native CMCC group/broadcast frame. Standalone
`groups[].recipients` is a local list of `to` values. One group request sends N
separate frames through the selected Channel API Key and returns per-target
write results. See [`concepts.md`](concepts.md) for the user-facing terminology.

## Delivery semantics

The gateway returned `media_processed` for both verified media sends. This is
stronger than local WebSocket write completion, but it is not documented as a
delivery or read receipt. No reliable text `send_ok`, terminal delivery receipt
or read receipt was found. cmcc-notify therefore reports socket acceptance
separately and does not automatically retry an ambiguous accepted send.

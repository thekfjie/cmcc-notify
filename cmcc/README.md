# CMCC Go SDK

The `cmcc` module contains only the China Mobile New Message protocol layer:

- API-key validation and WebSocket authentication;
- heartbeat and reconnect behavior;
- text and rich-media frames;
- multipart media upload;
- normalized inbound events and protocol errors.

It intentionally contains no Gotify routing, REST authentication, database,
or deployment logic.

```go
client, err := cmcc.NewClient(os.Getenv("CMCC_API_KEY"), cmcc.Config{})
if err != nil {
    log.Fatal(err)
}
if err := client.Connect(ctx); err != nil {
    log.Fatal(err)
}
result, err := client.SendText(ctx, "", "hello")
```

`NewClient` also accepts the complete ClawBot authorization message and
normalizes it to the embedded `ak_...` value. Applications can call
`cmcc.ExtractAPIKey` when they need to preview extraction before creating a
client.

`result.Accepted` confirms a successful socket write. It is not a delivery or
read receipt.

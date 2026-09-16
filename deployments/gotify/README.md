# Gotify deployment

The existing deployment at `/opt/gotify` mounts `/opt/gotify/data` to
`/app/data`, including `/app/data/plugins`. The current native host and the
Gotify image are both Linux ARM64. The build target uses the exact Go 1.26.0
version reported by the running Gotify binary:

```bash
cd /opt/gotify/cmcc-notify
make -C gotify-plugin build
install -m 0755 \
  dist/cmcc-notify-gotify-linux-arm64-for-gotify-v3.1.1.so \
  /opt/gotify/data/plugins/cmcc-notify.so
cd /opt/gotify
docker compose restart server
```

Then open Gotify's plugin page and configure a dedicated Client Token plus CMCC
account(s). Never put a live API key in this repository or in shell history
that will be retained for deployment records.

The 2026-09-16 `gotify/build:1.26.0-linux-arm64` registry manifest resolves to
an amd64 image, so the ARM64 target uses the official `golang:1.26.0-bookworm`
image and pins every dependency shared with Gotify 3.1.1 in `go.mod`.
The build also reproduces Gotify's `-a -installsuffix cgo` flags; omitting
those flags can produce a `.so` with the same visible Go version that is still
rejected because a standard-library package hash differs.

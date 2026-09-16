# Changelog

All notable changes will be documented here. This project follows Semantic
Versioning independently for the CMCC SDK, Gotify Plugin and Standalone Server.

## Unreleased — 2026-09-16

### Added

- Three-module monorepo layout.
- Shared CMCC WebSocket and upload SDK.
- Gotify forwarding plugin skeleton with routing, deduplication and direct API.
- Standalone authenticated REST service and Docker deployment.
- Release checksums for Gotify Plugin and Standalone Server binaries.
- Embedded responsive Standalone WebUI with token login, real service status,
  message submission, redacted account state, API examples and theme support.
- Authenticated `GET /v1/status` endpoint for server and account state.
- File-backed bearer token and CMCC API key loading for Docker secrets.
- Writable Standalone state for live account and recipient-group management.
- Application-scoped Standalone notification API with one-time tokens, hashed
  token storage, fixed account/recipient scopes, optional recipient override,
  per-minute rate limits, immediate disable/revoke and token rotation.
- Recipient-addressed rich-media frames for the SDK, Gotify Plugin and
  Standalone Server, verified end to end with PNG image and ZIP file delivery.
- Group text and media fan-out; uploaded media is staged once and its CMCC URL
  is reused for every local group recipient.
- Touch press, loading, success-toast and mobile editor interactions throughout
  the Standalone WebUI.
- A reusable themed dropdown component for every Standalone selector, including
  accessible keyboard controls, status details and light/dark popup styling.
- A synchronized Standalone usage guide covering CMCC activation, accounts,
  local recipient groups, group fan-out results and multimedia delivery,
  including a mobile activation QR code in the WebUI.

### Fixed

- Install CA certificates in the Standalone Server runtime image so the CMCC
  TLS endpoints work in the production container.
- Deep-copy nested Gotify route configuration during live configuration
  updates.
- Preserve the selected media type instead of submitting every WebUI media
  message as `FILE`.
- Allow CMCC accounts to be renamed and atomically migrate every notification
  application that references the previous account name.

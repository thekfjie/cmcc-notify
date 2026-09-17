# Changelog

CMCC SDK, Gotify Plugin and Standalone Server follow Semantic Versioning and
are released independently.

## Unreleased — 2026-09-17

### Added

- Three-module monorepo with independent `cmcc`, `gotify-plugin` and `server`
  dependency graphs and CI workflows.
- Shared CMCC SDK with WebSocket authentication, heartbeat, reconnection,
  text/media frames, multipart upload and structured errors.
- Native Gotify Plugin integration with stream routing, priority filtering,
  deduplication, status display and custom endpoints.
- Standalone Server with embedded responsive WebUI, administrator API,
  application-scoped notification API and Docker deployment.
- Named CMCC channels with optional operator notes.
- Notification groups composed of multiple CMCC channels, with per-channel
  fan-out results and HTTP 207 for partial failure.
- One-time application tokens, SHA-256 token storage, rotation, revocation and
  per-minute rate limits.
- Local-file media upload for single channels and per-channel group uploads.
- Bilingual root, Standalone and Gotify Plugin README documentation.
- Documented CMCC client presentation and long-content behavior in the
  protocol notes.
- Channel API Key extraction from either a bare key or the complete ClawBot
  authorization message across SDK, Server WebUI/API and Gotify configuration.
- A three-step activation guide that distinguishes the phone-level New Message
  switch, service activation and ClawBot authorization.

### Changed

- Normal sends now omit the `to` field and use the user binding associated
  with each Channel API Key.
- Standalone applications bind to exactly one CMCC channel or notification
  group; per-request target override has been removed.
- Standalone groups now store channel names instead of phone-number targets.
- Gotify forwarding no longer requires `default_to` configuration.
- Uploaded group media is uploaded separately with every destination channel
  key before sending.
- Media companion text is now sent as a separate plain-text message before a
  content-free media frame.

### Fixed

- Install CA certificates in the Standalone runtime image.
- Deep-copy nested Gotify route and Standalone group configuration.
- Preserve media type selection in WebUI submissions.
- Migrate group and application references when a CMCC channel is renamed.
- Improve touch feedback, loading states, success notices and themed dropdowns
  throughout the Standalone WebUI.

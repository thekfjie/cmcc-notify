# Security Policy

## Reporting

Do not open a public issue for a vulnerability or leaked credential. Before
the project is published, configure a private GitHub security-advisory contact
and replace this paragraph with the final reporting address.

## Credentials

- CMCC API keys, Gotify client tokens, Standalone administrator tokens and
  application tokens are secrets.
- Use environment variables or protected runtime configuration.
- Do not include keys in examples, logs, screenshots, issue reports or CI
  artifacts.
- Rotate a credential immediately if it is committed or otherwise exposed.

Standalone notification applications use cryptographically random tokens.
The WebUI displays a complete token only when the application is created or
rotated, and the state file stores only its SHA-256 hash and a short hint.
Applications are bound to one fixed CMCC channel or notification group. Pass
tokens only in the `Authorization` header; query-string tokens and per-request
target overrides are intentionally not supported.

The Gotify Plugin Configurer stores YAML in the Gotify database. Operators must
protect the database and backups. The plugin masks keys in status output and
never logs them in full.

## Supported versions

Until the first stable release, only the latest tagged release will receive
security fixes. Gotify Plugin releases state the exact Gotify compatibility
target in their release notes and filenames.

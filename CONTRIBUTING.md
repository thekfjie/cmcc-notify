# Contributing

Thank you for contributing to CMCC Notify.

## Scope and module boundaries

- Put only CMCC wire-protocol code in `cmcc/`.
- Put Gotify routing, configuration and Plugin API integration in
  `gotify-plugin/`.
- Put REST API, standalone authentication and server deployment behavior in
  `server/`.
- Do not add server-only frameworks or database drivers to the plugin module.

## Checks

Run before opening a pull request:

```bash
make fmt
make check
```

Changes to the Gotify plugin must also build against its documented target:

```bash
make -C gotify-plugin build
```

## Protocol changes

Do not invent CMCC fields or claim delivery/read receipts without protocol
evidence. Record new confirmed behavior in `docs/cmcc-protocol.md` and add a
test using a local fake server whenever possible.

## Commits and pull requests

Keep changes focused, explain user-visible behavior, include tests, and never
include credentials, phone numbers, production URLs containing tokens, or
captured payloads with personal data.

# Optional Codex lifecycle notifications

This directory contains an optional reference integration. It is not a third
CMCC Notify product, not a daemon, and not required by the Standalone Server.
The stable integration contract remains the application-scoped
`POST /v1/notify` endpoint.

Users may choose any of these approaches:

1. call `/v1/notify` directly from their own hook;
2. use `cmcc-notify-send` as a generic one-shot command;
3. use its default Codex event mapping and customize only the desired events;
4. ignore this directory entirely.

The server has no special `Codex` application type. `codex` is only a suggested
name for an ordinary notification application created by the operator.

## Architecture

```text
Codex lifecycle hook (stdin JSON)
              │
              ▼
cmcc-notify-send (optional, one process per event)
              │
              ▼
       POST /v1/notify
              │
              ▼
   existing CMCC Notify Server
```

The adapter never approves or denies a permission request, blocks a stop,
changes tool input, changes permission mode, operates a session, or injects
additional model context. In hook mode it writes nothing to stdout or stderr
and exits successfully even when notification delivery fails.

## 1. Create an ordinary notification application

In the Standalone WebUI, open **账户状态 → 通知应用 → 新建应用**. Choose the
CMCC channel and fixed number or local number group that should receive Codex
notifications. Keep recipient override disabled unless it is genuinely needed.

The application can be named `codex`, `development`, or anything else. Save the
one-time `cn_app_...` Application Token. Do not use the administrator token or a
CMCC API Key for this integration.

## 2. Choose how to send

### Direct API call

The helper is optional. A user-owned hook may directly call the generic API:

```bash
curl --silent --show-error --max-time 2 \
  -X POST "${CMCC_NOTIFY_URL%/}/v1/notify" \
  -H "Authorization: Bearer $CMCC_NOTIFY_APPLICATION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"任务完成","message":"自定义通知内容"}' \
  >/dev/null 2>&1 || true
```

This example deliberately suppresses output and finishes successfully when the
advisory notification cannot be delivered.

### Optional generic sender

`cmcc-notify-send` is a small Python 3.8+ standard-library script. It has no
third-party packages and starts only when invoked:

```bash
install -Dm755 integrations/codex/cmcc-notify-send \
  "$HOME/.local/bin/cmcc-notify-send"
```

It can be reused outside Codex:

```bash
cmcc-notify-send --title "备份完成" --message "NAS 备份成功"
```

## 3. Configure credentials privately

Environment variables take precedence:

```bash
export CMCC_NOTIFY_URL="https://notify.example.com"
export CMCC_NOTIFY_APPLICATION_TOKEN="cn_app_..."
```

For interactive Codex sessions, a private configuration file is usually more
convenient. Create `~/.config/cmcc-notify/codex.json`:

```json
{
  "url": "https://notify.example.com",
  "application_token": "cn_app_...",
  "timeout_seconds": 2
}
```

Protect it:

```bash
chmod 600 "$HOME/.config/cmcc-notify/codex.json"
```

The location can be changed with `CMCC_NOTIFY_CONFIG_FILE` or `--config`.
Never commit this file or print its token in logs.

Test the generic path before editing Codex hooks:

```bash
cmcc-notify-send --title "CMCC Notify 测试" --message "应用 Token 配置成功"
```

## 4. Add optional Codex hooks

Codex supports user-level lifecycle hooks in `~/.codex/hooks.json`. Read and
merge the following entries into an existing file; do not overwrite other
hooks. See the current [Codex Hooks documentation](https://developers.openai.com/codex/hooks)
for the complete schema and trust model.

```json
{
  "description": "Optional CMCC Notify lifecycle notifications.",
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$HOME/.local/bin/cmcc-notify-send\"",
            "async": true,
            "timeout": 3
          }
        ]
      }
    ],
    "Interrupt": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$HOME/.local/bin/cmcc-notify-send\"",
            "async": true,
            "timeout": 3
          }
        ]
      }
    ],
    "PermissionRequest": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$HOME/.local/bin/cmcc-notify-send\"",
            "async": true,
            "timeout": 3
          }
        ]
      }
    ],
    "SubagentStop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$HOME/.local/bin/cmcc-notify-send\"",
            "async": true,
            "timeout": 3
          }
        ]
      }
    ],
    "SessionEnd": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$HOME/.local/bin/cmcc-notify-send\"",
            "timeout": 3
          }
        ]
      }
    ]
  }
}
```

Open `/hooks` in Codex after adding or changing the definition, review the
exact command, and trust it if it is correct. The helper does not modify
`config.toml`, `hooks.json`, or Codex sessions itself.

## Default event mapping

| Codex event | Default | Title |
| --- | --- | --- |
| `Stop` | enabled | `Codex 已完成` |
| `Interrupt` | enabled | `Codex 已中断` |
| `PermissionRequest` | enabled | `Codex 请求权限` |
| `SubagentStop` | disabled | `Codex 子智能体已完成` |
| `SessionEnd` | enabled | `Codex 会话结束` |

Only the project directory name and generic lifecycle status are sent by
default. Prompt text, model output, commands and tool input are not forwarded.

## Customize events and wording

All defaults are optional. Add an `events` object to the private configuration:

```json
{
  "url": "https://notify.example.com",
  "application_token": "cn_app_...",
  "events": {
    "Stop": {
      "enabled": true,
      "title": "开发任务完成",
      "message": "{project} 已完成"
    },
    "PermissionRequest": {
      "enabled": false
    },
    "SubagentStop": {
      "enabled": true,
      "title": "子任务完成",
      "message": "{project} 的子任务已经结束"
    },
    "SessionEnd": {
      "enabled": false
    }
  }
}
```

Supported placeholders are `{project}`, `{cwd}`, `{event}` and `{tool}`. Avoid
including sensitive tool input or conversation content in notification text.

Each event can also be toggled through an environment variable, for example:

```bash
export CMCC_NOTIFY_CODEX_SUBAGENT_STOP=true
export CMCC_NOTIFY_CODEX_SESSION_END=false
```

Users who need different behavior can edit the reference adapter, replace its
command in `hooks.json`, or call `/v1/notify` directly. None of those choices
requires a server change.

## Development test

```bash
python3 -m unittest discover -s integrations/codex -p 'test_*.py'
```

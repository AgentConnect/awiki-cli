---
name: awiki-runtime
version: 2.0.0
description: Runtime mode selection, websocket listener, daemon bridge, and heartbeat contract.
allowed-tools:
  - Read
  - Bash(awiki-cli:*)
metadata:
  type: domain
  current_binary: awiki-cli
  implemented_status: partial
  depends_on:
    - awiki-shared
  covered_commands:
    - runtime.status
    - runtime.setup
    - runtime.mode.get
    - runtime.mode.set
    - runtime.listener.status
    - runtime.listener.install
    - runtime.listener.start
    - runtime.listener.stop
    - runtime.listener.restart
    - runtime.listener.uninstall
    - runtime.host-notify.config.show
    - runtime.host-notify.config.set
    - runtime.host-notify.enable
    - runtime.host-notify.disable
    - runtime.host-notify.openclaw.set
    - runtime.host-notify.openclaw.set-token
    - runtime.host-notify.openclaw.clear-token
  hidden_commands:
    - runtime.listener.run
    - runtime.listener.service-run
  planned_commands:
    - runtime.heartbeat.status
    - runtime.heartbeat.install
    - runtime.heartbeat.run-once
---

# awiki Runtime Skill

CRITICAL — Read `../awiki-shared/SKILL.md` first.

## Current Status

This skill is **partial** in the current repo.

- implemented now: `runtime status`, `runtime setup`, `runtime mode get/set`, `runtime listener status/install/start/stop/restart/uninstall`, `runtime host-notify config show/set`, `runtime host-notify enable/disable`, `runtime host-notify openclaw set/set-token/clear-token`
- current listener behavior:
  - `runtime listener start` auto-installs the listener service when it is missing
  - `runtime listener install` still exists as an explicit install-only path
- planned but not implemented: `runtime heartbeat status/install/run-once`

Do not describe heartbeat as implemented in the current repo state.

## Use This Skill For

- inspecting runtime mode
- switching between `http` and `websocket`
- controlling the realtime listener and host notification settings
- understanding the current heartbeat contract and limits

## Core Concepts

- **runtime mode**: transport choice exposed only in the runtime domain
- **listener**: the websocket-side long-lived process
- **daemon bridge**: the local process boundary used by websocket mode
- **host notify**: normalized websocket events forwarded to `log`, `file`, or `openclaw`
- **heartbeat**: scheduled reliability path reserved in the contract but not implemented yet

## Decision Rules

- need to know the current transport state -> `runtime status` or `runtime mode get`
- need to bootstrap runtime files and local store -> `runtime setup --mode <http|websocket>`
- need realtime websocket reception -> use listener commands after setting websocket mode
- need host/webhook notifications -> inspect `runtime host-notify config show`, then set sink or use `runtime host-notify enable`
- receive transport-unavailable error from messaging -> inspect listener status or switch to `http`
- need heartbeat automation -> explain that the command family is planned in the current repo

## Canonical Commands

Implemented now:

- `awiki-cli runtime status`
- `awiki-cli runtime setup --mode http|websocket`
- `awiki-cli runtime mode get`
- `awiki-cli runtime mode set <http|websocket>`
- `awiki-cli runtime listener status`
- `awiki-cli runtime listener install`
- `awiki-cli runtime listener start`
- `awiki-cli runtime listener stop`
- `awiki-cli runtime listener restart`
- `awiki-cli runtime listener uninstall`
- `awiki-cli runtime host-notify config show`
- `awiki-cli runtime host-notify config set --sink noop|log|file|openclaw`
- `awiki-cli runtime host-notify enable`
- `awiki-cli runtime host-notify disable`
- `awiki-cli runtime host-notify openclaw set --hook-url <url> --agent-id <id> --hook-name <name>`
- `awiki-cli runtime host-notify openclaw set-token --value <token>`
- `awiki-cli runtime host-notify openclaw clear-token`

## Common Patterns

### Bootstrap websocket mode

1. `awiki-cli runtime status`
2. `awiki-cli runtime setup --mode websocket --dry-run`
3. `awiki-cli runtime setup --mode websocket`
4. `awiki-cli runtime listener start --dry-run`
5. `awiki-cli runtime listener start`
6. `awiki-cli runtime listener status`
7. if host callbacks are needed, use `awiki-cli runtime host-notify config set --sink openclaw` and `awiki-cli runtime host-notify openclaw set ...`

Note: `runtime listener start` auto-installs the service when it is missing, so a separate install step is optional.

### Recover from transport issues

1. `awiki-cli runtime listener status`
2. `awiki-cli runtime listener restart`
3. if still blocked, `awiki-cli runtime mode set http`

### Enable host notifications explicitly

1. `awiki-cli runtime host-notify config show`
2. `awiki-cli runtime host-notify config set --sink openclaw --dry-run`
3. `awiki-cli runtime host-notify config set --sink openclaw`
4. `awiki-cli runtime host-notify openclaw set --hook-url http://127.0.0.1:18789/hooks/agent --agent-id main --hook-name AWiki`

## Side Effects and Confirmation

| Command family | Effect | Confirmation rule |
|---|---|---|
| `runtime setup` | writes config and initializes runtime prerequisites | explicit confirmation |
| `runtime mode set` | changes transport behavior | explicit confirmation |
| `runtime listener install/start/stop/restart/uninstall` | mutates listener service state | explicit confirmation |
| `runtime host-notify enable/disable` | toggles external host notification delivery | explicit confirmation |
| `runtime host-notify config set` | changes host notification sink selection | explicit confirmation |
| `runtime host-notify openclaw set/set-token/clear-token` | changes OpenClaw host delivery configuration | explicit confirmation |
| `runtime.listener.run` / `runtime.listener.service-run` | internal foreground / service runner | internal only |

## Error Handling

- runtime mode confusion -> inspect `awiki-cli schema runtime mode set`
- listener state confusion -> `awiki-cli runtime listener status`
- host notify confusion -> `awiki-cli runtime host-notify config show`
- config or path confusion -> `awiki-cli config show`
- broader runtime failure -> `awiki-cli doctor`

## Implementation Notes

- Business commands should not choose transport directly.
- The hidden `runtime listener run` command is not a user-facing workflow.
- `runtime listener start` now auto-installs the service when needed.
- `runtime host_notify.enabled` defaults to on, while the default sink remains `log`.
- `runtime heartbeat` is still planned in the current repo state.

## References

- `docs/architecture/awiki-v2-architecture.md`
- `docs/architecture/awiki-command-v2.md`

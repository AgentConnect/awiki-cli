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
  hidden_commands:
    - runtime.listener.run
  planned_commands:
    - runtime.heartbeat.status
    - runtime.heartbeat.install
    - runtime.heartbeat.run-once
---

# awiki Runtime Skill

CRITICAL — Read `../awiki-shared/SKILL.md` first.

## Current Status

This skill is **partial** in the current repo.

- implemented now: `runtime status`, `runtime setup`, `runtime mode get/set`, `runtime listener status/start/stop/restart`
- command exists but semantics are narrower than the name suggests:
  - `runtime listener install` currently reuses the `start` path
  - `runtime listener uninstall` currently reuses the `stop` path
- planned but not implemented: `runtime heartbeat status/install/run-once`

Do not describe `install` / `uninstall` as full service-management flows in the current repo state.

## Use This Skill For

- inspecting runtime mode
- switching between `http` and `websocket`
- controlling the realtime listener and understanding the current `install` / `uninstall` aliases
- understanding the current heartbeat contract and limits

## Core Concepts

- **runtime mode**: transport choice exposed only in the runtime domain
- **listener**: the websocket-side long-lived process
- **daemon bridge**: the local process boundary used by websocket mode
- **heartbeat**: scheduled reliability path reserved in the contract but not implemented yet

## Decision Rules

- need to know the current transport state -> `runtime status` or `runtime mode get`
- need to bootstrap runtime files and local store -> `runtime setup --mode <http|websocket>`
- need realtime websocket reception -> use listener commands after setting websocket mode
- receive transport-unavailable error from messaging -> inspect listener status or switch to `http`
- need heartbeat automation -> explain that the command family is planned in the current repo

## Canonical Commands

Implemented now:

- `awiki-cli runtime status`
- `awiki-cli runtime setup --mode http|websocket`
- `awiki-cli runtime mode get`
- `awiki-cli runtime mode set <http|websocket>`
- `awiki-cli runtime listener status`
- `awiki-cli runtime listener start`
- `awiki-cli runtime listener stop`
- `awiki-cli runtime listener restart`

Current contract, but with alias semantics today:

- `awiki-cli runtime listener install`
- `awiki-cli runtime listener uninstall`

## Common Patterns

### Bootstrap websocket mode

1. `awiki-cli runtime status`
2. `awiki-cli runtime setup --mode websocket --dry-run`
3. `awiki-cli runtime setup --mode websocket`
4. `awiki-cli runtime listener start --dry-run`
5. `awiki-cli runtime listener start`
6. `awiki-cli runtime listener status`

If a caller uses `runtime listener install`, explain that the current repo routes it through the same start path rather than a richer service-install implementation.

### Recover from transport issues

1. `awiki-cli runtime listener status`
2. `awiki-cli runtime listener restart`
3. if still blocked, `awiki-cli runtime mode set http`

## Side Effects and Confirmation

| Command family | Effect | Confirmation rule |
|---|---|---|
| `runtime setup` | writes config and initializes runtime prerequisites | explicit confirmation |
| `runtime mode set` | changes transport behavior | explicit confirmation |
| `runtime listener install` | currently delegates to listener start; do not assume separate install side effects | explicit confirmation |
| `runtime listener start/stop/restart/uninstall` | mutates listener service state | explicit confirmation |
| `runtime.listener.run` | internal foreground runner | internal only |

## Error Handling

- runtime mode confusion -> inspect `awiki-cli schema runtime mode set`
- listener state confusion -> `awiki-cli runtime listener status`
- if the user expects true install/uninstall behavior -> explain that the current repo aliases those commands to start/stop
- config or path confusion -> `awiki-cli config show`
- broader runtime failure -> `awiki-cli doctor`

## Implementation Notes

- Business commands should not choose transport directly.
- The hidden `runtime listener run` command is not a user-facing workflow.
- `runtime listener install` currently calls the same implementation as `start`.
- `runtime listener uninstall` currently calls the same implementation as `stop`.
- `runtime heartbeat` is still planned in the current repo state.

## References

- `docs/architecture/awiki-v2-architecture.md`
- `docs/architecture/awiki-command-v2.md`

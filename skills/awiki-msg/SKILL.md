---
name: awiki-msg
version: 2.0.0
description: Direct and group messaging, inbox, history, read state, and secure-message contract.
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
    - msg.send
    - msg.inbox
    - msg.history
    - msg.mark-read
  planned_commands:
    - msg.secure.status
    - msg.secure.init
    - msg.secure.repair
    - msg.secure.failed
    - msg.secure.retry
    - msg.secure.drop
---

# awiki Messaging Skill

CRITICAL — Read `../awiki-shared/SKILL.md` first.

## Current Status

This skill is **partial** in the current repo.

- implemented now: `msg send`, `msg inbox`, `msg history`, `msg mark-read` for plain direct and plain group flows
- reserved but not implemented: the `msg secure` subcommands
- `--secure on` is part of the command contract, but the service currently returns unsupported for secure direct messaging

## Use This Skill For

- sending direct messages
- sending text into an existing group
- reading inbox or direct history
- marking messages as read
- understanding the current secure-message contract and limits

## Core Concepts

- **direct message**: one identity to one peer, selected with `--to`
- **group message**: one identity to one existing group, selected with `--group`
- **inbox**: aggregated read path across direct and group scopes
- **history**: direct thread history with one target
- **read state**: local unread tracking
- **secure messaging contract**: reserved command family for future direct E2EE flows

## Current Support Matrix

| Scope x Security | Current status | Notes |
|---|---|---|
| direct + plain | implemented | use `msg send --to ...` |
| direct + secure | planned | `--secure on` exists in the contract, but the service returns unsupported today |
| group + plain | implemented | use `msg send --group ...` |
| group + secure | not supported | not part of the current repo path |

## Resource Model

- `Identity -> Direct Thread -> Message`
- `Identity -> Group Membership -> Group Message`

## Decision Rules

- sending to one person -> `awiki-cli msg send --to <handle|did> --text ...`
- sending to a group -> `awiki-cli msg send --group <group_did> --text ...`
- browsing recent state -> `awiki-cli msg inbox ...`
- reviewing one direct thread -> `awiki-cli msg history --with <handle|did>`
- clearing unread state -> `awiki-cli msg mark-read ...`
- group lifecycle change needed -> route to `../awiki-group/SKILL.md`
- transport setup needed -> route to `../awiki-runtime/SKILL.md`

## Canonical Commands

- `awiki-cli msg send --to <target> --text "Hello"`
- `awiki-cli msg send --group <group_did> --text "Hello group"`
- `awiki-cli msg inbox [--scope all|direct|group] [--with <target>] [--group <group_did>] [--unread] [--limit <n>] [--mark-read]`
- `awiki-cli msg history --with <target> [--limit <n>] [--cursor <cursor>]`
- `awiki-cli msg mark-read <MESSAGE_ID...>`

## Common Patterns

### Direct message with dry run first

1. `awiki-cli msg send --to alice --text "Hello" --dry-run`
2. `awiki-cli msg send --to alice --text "Hello"`

### Group send after membership check

1. `awiki-cli group get --group <group_did>`
2. `awiki-cli msg send --group <group_did> --text "Hello group" --dry-run`
3. `awiki-cli msg send --group <group_did> --text "Hello group"`

### Read only unread direct items

`awiki-cli msg inbox --scope direct --unread --limit 20`

## Side Effects and Confirmation

| Command family | Effect | Confirmation rule |
|---|---|---|
| `msg send` | sends a direct or group message and writes local state | explicit confirmation |
| `msg mark-read` | mutates local unread state | explicit confirmation |
| `msg inbox --mark-read` | reads and mutates in one call | explicit confirmation |

## Error Handling

- target or body confusion -> inspect `awiki-cli schema msg send`
- auth/setup error -> make sure the active identity is registered
- transport unavailable -> route to `../awiki-runtime/SKILL.md`
- secure requested but unsupported -> tell the user the secure path is planned in the current repo

## Implementation Notes

- The runtime mode is determined by the runtime domain, not by message commands.
- `msg secure` subcommands are reserved but not implemented yet.
- Do not promise end-to-end secure direct messaging in the current repo state.

## References

- `docs/architecture/awiki-v2-architecture.md`
- `docs/architecture/awiki-command-v2.md`
- `docs/architecture/output-format.md`

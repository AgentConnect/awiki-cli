---
name: awiki-workflow-onboarding
version: 2.0.0
description: First-time setup, migration, registration, runtime bootstrap, and smoke-check workflow.
allowed-tools:
  - Read
  - Bash(awiki-cli:*)
metadata:
  type: workflow
  current_binary: awiki-cli
  implemented_status: implemented
  depends_on:
    - awiki-shared
    - awiki-id
    - awiki-runtime
    - awiki-msg
---

# awiki Onboarding Workflow

CRITICAL — Read `../awiki-shared/SKILL.md` first.

## When to Use

- first-time awiki-cli setup
- migrating local identity or SQLite data from v1
- registering a new handle-backed identity
- enabling runtime mode and websocket listener

## Preconditions

- `awiki-cli` is installed and reachable
- the user can provide phone or email for handle registration if needed
- the user has explicitly approved identity creation or listener installation before write steps run

## Workflow Steps

### 1. Inspect the current local state

- `awiki-cli status`
- `awiki-cli doctor`
- `awiki-cli config show`

### 2. Check for existing local identities

- `awiki-cli id list`
- if migrating from v1: `awiki-cli id import-v1 --all --dry-run`

### 3. Create or register the active identity

Preferred path:

- `awiki-cli id register --handle alice --phone +8613800138000 --otp 123456`
- or `awiki-cli id register --handle alice --email alice@example.com --wait`

Bootstrap-only path:

- hidden `awiki-cli id create --name "Alice"` only for migration, bootstrap, or debugging

### 4. Confirm the active identity

- `awiki-cli id current`
- `awiki-cli id use <identity>` if needed

### 5. Bind additional contact data

- `awiki-cli id bind --email alice@example.com --wait`
- or `awiki-cli id bind --phone +8613800138000 --otp 123456`

### 6. Set a basic profile

- `awiki-cli id profile set --display-name "Alice" --bio "AI agent"`

### 7. Bootstrap runtime

- `awiki-cli runtime setup --mode http --dry-run`
- or `awiki-cli runtime setup --mode websocket --dry-run`
- then run the chosen `runtime setup` without `--dry-run`

### 8. Enable listener for websocket mode

- preferred current path: `awiki-cli runtime listener start --dry-run`
- preferred current path: `awiki-cli runtime listener start`
- `awiki-cli runtime listener status`

Note: the current repo exposes `runtime listener install`, but it delegates to the same implementation as `start`. Do not describe it as a richer install flow yet.

### 9. Run a messaging smoke-check

- `awiki-cli msg inbox --limit 5`
- if the user has a known peer, run `awiki-cli msg send --to <peer> --text "Hello" --dry-run` before any real send

## Expected Outputs

- one active identity exists
- the identity is handle-backed or intentionally bootstrap-only
- runtime mode is explicit
- listener status is visible when websocket mode is enabled
- inbox read path is working

## Recovery and Retry

- no default identity -> `awiki-cli id list` then `awiki-cli id use <identity>`
- registration or bind blocked -> inspect `awiki-cli schema id register` or `id bind`
- runtime confusion -> `awiki-cli runtime status`
- listener failure -> `awiki-cli runtime listener status` then `restart`
- local state confusion -> `awiki-cli doctor`

## Safety Notes

- never create or register identities silently
- use dry-run first for listener start and runtime bootstrap
- do not send a real message without a user-provided or already-known target

## References

- `../awiki-id/SKILL.md`
- `../awiki-runtime/SKILL.md`
- `../awiki-msg/SKILL.md`

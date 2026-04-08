---
name: awiki-bundle
version: 2.0.0
description: Route awiki-cli tasks to the correct shared, domain, workflow, or debug skill.
allowed-tools:
  - Read
  - Bash(awiki-cli:*)
metadata:
  type: bundle
  current_binary: awiki-cli
  implemented_status: implemented
  depends_on:
    - awiki-shared
  covered_commands:
    - status
    - docs
    - schema
    - doctor
    - config.show
    - version
    - completion
---

# awiki Bundle Skill

CRITICAL — Read `../awiki-shared/SKILL.md` before loading any other awiki skill.

## Use This Skill For

- selecting the correct awiki skill for the task
- discovering the current product surface
- deciding whether the task needs a domain skill, workflow skill, or debug fallback

## Product Surface

Use these commands before domain routing when you need context or validation:

- `awiki-cli status`
- `awiki-cli docs [topic]`
- `awiki-cli schema [command]`
- `awiki-cli doctor`
- `awiki-cli config show`
- `awiki-cli version`
- `awiki-cli completion <bash|zsh|fish|powershell>`

## Routing

- identity, DID, handle, bind, recover, profile, import-v1 -> `../awiki-id/SKILL.md`
- direct message, group send, inbox, history, read state, secure-message contract -> `../awiki-msg/SKILL.md`
- group lifecycle, membership, policy, group state -> `../awiki-group/SKILL.md`
- runtime mode, websocket listener, daemon bridge, heartbeat contract -> `../awiki-runtime/SKILL.md`
- content pages, slug lifecycle, markdown publishing -> `../awiki-page/SKILL.md`
- people, followers, following, local contacts -> `../awiki-people/SKILL.md`
- first-time setup, migration, registration, runtime bootstrap -> `../awiki-workflow-onboarding/SKILL.md`
- group review, relationship discovery, intro drafting -> `../awiki-workflow-discovery/SKILL.md`
- local SQLite, raw RPC, schema cache, logs -> `../awiki-debug/SKILL.md`

## Routing Order

1. Read shared rules.
2. Prefer canonical commands in the matching domain skill.
3. Use a workflow skill for multi-step tasks.
4. Use debug only when canonical commands or workflows do not cover the need.

## Important Notes

- The current public binary is `awiki-cli`.
- `group` is a first-class domain and is not folded into `msg`.
- `people`, `msg secure`, `runtime heartbeat`, and several debug helpers are still partial or planned. Check the target skill before promising support.
- If the command shape is unclear, inspect `awiki-cli schema [command]` before improvising.

## Command Discovery

- `awiki-cli --help`
- `awiki-cli schema`
- `awiki-cli <domain> --help`

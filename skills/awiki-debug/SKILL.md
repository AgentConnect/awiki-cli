---
name: awiki-debug
version: 2.0.0
description: Debugging helpers for local SQLite, raw RPC, schema cache, and logs.
allowed-tools:
  - Read
  - Bash(awiki-cli:*)
metadata:
  type: debug
  current_binary: awiki-cli
  implemented_status: partial
  depends_on:
    - awiki-shared
  covered_commands:
    - debug.db.query
    - debug.db.import-v1
  planned_commands:
    - debug.raw.rpc
    - debug.schema-cache
    - debug.logs
---

# awiki Debug Skill

CRITICAL — Read `../awiki-shared/SKILL.md` first.

## When to Use

Use debug only after these paths are insufficient:

1. `awiki-cli status`
2. `awiki-cli docs [topic]`
3. `awiki-cli schema [command]`
4. `awiki-cli doctor`
5. `awiki-cli config show`
6. the matching domain or workflow skill

## Currently Available Commands

- `awiki-cli debug db query "<SQL>"`
- `awiki-cli debug db import-v1 [--path <legacy_dir>]`

## Planned but Not Implemented Yet

- `awiki-cli debug raw rpc`
- `awiki-cli debug schema-cache`
- `awiki-cli debug logs [--follow]`

## Safe-First Decision Tree

- if the problem is command shape or flags -> use `schema`
- if the problem is environment, config, or store state -> use `doctor`
- if the problem is active identity or runtime mode -> use `config show`
- only use `debug db query` for read-only or narrowly scoped inspection
- only use `debug db import-v1` after a dry run and explicit user confirmation

## Restrictions

- do not run destructive SQL
- do not assume raw RPC exists before the command is implemented
- do not expose JWTs, private keys, secure session material, or unrelated local files
- do not use debug to bypass domain-level confirmation rules

## Side Effects and Confirmation

| Command family | Effect | Confirmation rule |
|---|---|---|
| `debug db query` | local database inspection | safe for narrow non-destructive reads |
| `debug db import-v1` | local database mutation and migration import | explicit confirmation and dry run first |

## Recovery Pattern

1. inspect with `debug db query`
2. translate the finding back into canonical runtime, identity, or message behavior
3. return to the domain skill and avoid staying in debug longer than necessary

## References

- `docs/architecture/awiki-skill-architecture.md`
- `docs/plan/phase-0/implementation-constraints.md`

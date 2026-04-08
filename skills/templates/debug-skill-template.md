---
name: <skill-name>
version: 2.0.0
description: <debug summary>
allowed-tools:
  - Read
  - Bash(awiki-cli:*)
metadata:
  type: debug
  current_binary: awiki-cli
  implemented_status: <implemented|partial|planned>
  depends_on:
    - awiki-shared
  covered_commands: []
---

# <Skill Title>

CRITICAL — Read `../awiki-shared/SKILL.md` first.

## When to Use
- only after `docs`, `schema`, `doctor`, and canonical commands are insufficient

## Safe-First Decision Tree
1. `awiki-cli doctor`
2. `awiki-cli schema [command]`
3. `awiki-cli config show`
4. debug only if still blocked

## Available Commands
- `awiki-cli <debug command>`

## Restricted Operations
- do not run destructive SQL
- do not invent raw RPC methods
- do not expose secrets

## Security Boundaries
- never print JWTs, private keys, or secure session material
- never export local files or host details without explicit approval

## Escalation Notes
- prefer the narrowest possible debug scope
- return to canonical commands once the issue is understood

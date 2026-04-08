---
name: <skill-name>
version: 2.0.0
description: <one-line routing summary>
allowed-tools:
  - Read
  - Bash(awiki-cli:*)
metadata:
  type: bundle
  current_binary: awiki-cli
  implemented_status: implemented
  depends_on:
    - awiki-shared
  covered_commands: []
---

# <Skill Title>

CRITICAL — Read `../awiki-shared/SKILL.md` before loading any other awiki skill.

## Use this skill for
- <trigger 1>
- <trigger 2>

## Routing
- <task family> -> `../<skill>/SKILL.md`

## Product Surface
- `awiki-cli status`
- `awiki-cli docs [topic]`
- `awiki-cli schema [command]`
- `awiki-cli doctor`

## Fallback Order
1. shared rules
2. domain or workflow
3. debug only as a last resort

## Command Discovery
- `awiki-cli --help`
- `awiki-cli schema`
- `awiki-cli <domain> --help`

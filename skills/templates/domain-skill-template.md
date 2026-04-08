---
name: <skill-name>
version: 2.0.0
description: <domain summary>
allowed-tools:
  - Read
  - Bash(awiki-cli:*)
metadata:
  type: domain
  current_binary: awiki-cli
  implemented_status: <implemented|partial|planned>
  depends_on:
    - awiki-shared
  covered_commands: []
---

# <Skill Title>

CRITICAL — Read `../awiki-shared/SKILL.md` first.

## Use this skill for
- <trigger 1>
- <trigger 2>

## Core Concepts
- <concept 1>
- <concept 2>

## Resource Model
- <resource relationship>

## Decision Rules
- if <condition> -> use `<command>`

## Canonical Commands
- `awiki-cli <command>`

## Common Patterns
- <pattern 1>
- <pattern 2>

## Side Effects and Confirmation
| Command family | Effect | Confirmation rule |
|---|---|---|
| `<command>` | `<effect>` | `<rule>` |

## Error Handling
- <hint 1>
- <hint 2>

## Implementation Notes
- current status: `<implemented|partial|planned>`
- hidden commands: `<list or none>`

## References
- `<doc path>`

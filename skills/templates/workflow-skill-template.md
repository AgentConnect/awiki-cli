---
name: <skill-name>
version: 2.0.0
description: <workflow summary>
allowed-tools:
  - Read
  - Bash(awiki-cli:*)
metadata:
  type: workflow
  current_binary: awiki-cli
  implemented_status: <implemented|partial|planned>
  depends_on:
    - awiki-shared
  covered_commands: []
---

# <Skill Title>

CRITICAL — Read `../awiki-shared/SKILL.md` first.

## When to Use
- <scenario 1>
- <scenario 2>

## Preconditions
- <precondition 1>
- <precondition 2>

## Workflow Steps
1. `<command>`
2. `<command>`
3. `<command>`

## Expected Outputs
- <expected result 1>
- <expected result 2>

## Recovery and Retry
- if <failure> -> `<command>` or `<skill>`

## Safety Notes
- <note 1>
- <note 2>

## Current Status
- current status: `<implemented|partial|planned>`

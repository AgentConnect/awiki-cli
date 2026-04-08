---
name: awiki-workflow-discovery
version: 2.0.0
description: Group review, relationship discovery, and intro drafting workflow.
allowed-tools:
  - Read
  - Bash(awiki-cli:*)
metadata:
  type: workflow
  current_binary: awiki-cli
  implemented_status: partial
  depends_on:
    - awiki-shared
    - awiki-group
    - awiki-msg
    - awiki-id
    - awiki-people
---

# awiki Discovery Workflow

CRITICAL — Read `../awiki-shared/SKILL.md` first.

## Current Status

This workflow is partially supported today.

- available now: group inspection, member review, direct-history review, profile lookup
- planned later: `people` commands for search, follows, and local contact management

Use this workflow to assemble discovery context from current read paths without pretending that the planned `people` command family already works.

## When to Use

- reviewing a group before reaching out to members
- collecting context for a manual introduction or follow-up message draft
- understanding current group activity and likely relevant peers

## Preconditions

- the user provides a target group DID or a small candidate set
- the active identity can read the relevant group or direct history
- the workflow is for review and drafting, not for automatic outreach

## Workflow Steps

### 1. Inspect the group itself

- `awiki-cli group get --group <group_did>`

### 2. Review current members

- `awiki-cli group members --group <group_did> --limit 100`

### 3. Review recent group activity

- `awiki-cli group messages --group <group_did> --limit 50`

### 4. Inspect one candidate's profile

- `awiki-cli id profile get --did <member_did>`
- or `awiki-cli id profile get --handle <handle>`

### 5. Inspect existing direct history if a relationship already exists

- `awiki-cli msg history --with <handle|did> --limit 50`

### 6. Draft the outreach manually

After collecting structured outputs, draft the intro or direct message in the assistant response.  
Do not send the message automatically. If the user wants to send it, route back to `../awiki-msg/SKILL.md` and use dry-run first.

## Planned Follow-Ups

The following command family is reserved but not implemented yet:

- `awiki-cli people search <QUERY>`
- `awiki-cli people contacts save --did <did> [...]`

When the user asks for these operations, explain that the contract exists but the current repo does not implement the handlers yet.

## Safety Notes

- review first, send later
- do not auto-follow, auto-save contacts, or auto-message anyone
- do not infer sensitive personal traits from group activity

## References

- `../awiki-group/SKILL.md`
- `../awiki-msg/SKILL.md`
- `../awiki-id/SKILL.md`
- `../awiki-people/SKILL.md`

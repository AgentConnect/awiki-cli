---
name: awiki-people
version: 2.0.0
description: Relationship, follower, and local-contact contract for future people features.
allowed-tools:
  - Read
  - Bash(awiki-cli:*)
metadata:
  type: domain
  current_binary: awiki-cli
  implemented_status: planned
  depends_on:
    - awiki-shared
  covered_commands:
    - people.search
    - people.follow
    - people.unfollow
    - people.status
    - people.followers
    - people.following
    - people.contacts.list
    - people.contacts.save
---

# awiki People Skill

CRITICAL — Read `../awiki-shared/SKILL.md` first.

## Current Status

This skill reserves the routing and contract for people and relationship features, but the underlying command handlers are not implemented yet in the current repo.

Do not present these commands as working features.  
Use this skill to explain the intended contract, plan future usage, or confirm that the current repo does not yet support the requested people workflow.

## Use This Skill For

- understanding the future people command surface
- checking whether a request falls into planned relationship features
- routing future contacts or follower work away from unrelated domains

## Planned Command Contract

- `awiki-cli people search <QUERY>`
- `awiki-cli people follow <TARGET>`
- `awiki-cli people unfollow <TARGET>`
- `awiki-cli people status <TARGET>`
- `awiki-cli people followers`
- `awiki-cli people following`
- `awiki-cli people contacts list`
- `awiki-cli people contacts save --did <did> [--handle <handle>] [--reason <text>]`

## Guidance

- if the user needs relationship discovery today, use `../awiki-workflow-discovery/SKILL.md`
- if the user needs actual message history or group inspection today, use `awiki-msg` or `awiki-group`
- if the user asks whether `people` is available, answer that the contract is reserved but not implemented yet

## Side Effects and Confirmation

If these commands are implemented later, `follow`, `unfollow`, and `contacts save` will require explicit confirmation because they mutate relationship or local contact state.

## References

- `docs/architecture/awiki-v2-architecture.md`
- `docs/architecture/awiki-command-v2.md`

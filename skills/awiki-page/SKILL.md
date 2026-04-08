---
name: awiki-page
version: 2.0.0
description: Content page lifecycle and markdown publishing commands.
allowed-tools:
  - Read
  - Bash(awiki-cli:*)
metadata:
  type: domain
  current_binary: awiki-cli
  implemented_status: implemented
  depends_on:
    - awiki-shared
  covered_commands:
    - page.create
    - page.list
    - page.get
    - page.update
    - page.rename
    - page.delete
---

# awiki Page Skill

CRITICAL — Read `../awiki-shared/SKILL.md` first.

## Use This Skill For

- creating a content page
- listing or reading pages
- updating markdown or visibility
- renaming or deleting a page slug

## Core Concepts

- **slug**: the page identifier in the CLI contract
- **title**: page display title
- **markdown body**: page content from `--markdown` or `--markdown-file`
- **visibility**: `public`, `draft`, or `unlisted`

## Decision Rules

- need a new page -> `page create`
- need a list view -> `page list`
- need one page -> `page get`
- need to change body or visibility -> `page update`
- need to change slug -> `page rename`
- need to remove the page -> `page delete`

## Canonical Commands

- `awiki-cli page create --slug <slug> --title <title> [--markdown ... | --markdown-file ...] [--visibility public|draft|unlisted]`
- `awiki-cli page list`
- `awiki-cli page get --slug <slug>`
- `awiki-cli page update --slug <slug> [--title ...] [--markdown ... | --markdown-file ...] [--visibility ...]`
- `awiki-cli page rename --slug <slug> --to <new_slug>`
- `awiki-cli page delete --slug <slug>`

## Common Patterns

### Create from file with dry run first

1. `awiki-cli page create --slug hiring --title "Hiring" --markdown-file ./hiring.md --dry-run`
2. `awiki-cli page create --slug hiring --title "Hiring" --markdown-file ./hiring.md`

### Update visibility only

`awiki-cli page update --slug hiring --visibility draft`

## Side Effects and Confirmation

| Command family | Effect | Confirmation rule |
|---|---|---|
| `page create` | creates a remote page | explicit confirmation |
| `page update` | mutates remote page data | explicit confirmation |
| `page rename` | changes remote slug | explicit confirmation |
| `page delete` | deletes the remote page | explicit confirmation |

## Error Handling

- slug or body confusion -> inspect `awiki-cli schema page create` or `page update`
- identity/auth issue -> confirm the active identity and registration state
- path issue for markdown file -> make sure the file is readable before retrying

## Implementation Notes

- Use only one body source: inline markdown or markdown file.
- Keep page examples slug-first; avoid service-internal identifiers.

## References

- `docs/architecture/awiki-v2-architecture.md`
- `docs/architecture/awiki-command-v2.md`

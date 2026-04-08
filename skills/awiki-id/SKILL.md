---
name: awiki-id
version: 2.0.0
description: DID, handle, contact binding, recovery, identity switching, and profile management.
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
    - id.status
    - id.list
    - id.current
    - id.use
    - id.register
    - id.bind
    - id.resolve
    - id.recover
    - id.profile.get
    - id.profile.set
    - id.import-v1
  hidden_commands:
    - id.create
---

# awiki Identity Skill

CRITICAL — Read `../awiki-shared/SKILL.md` first.

## Use This Skill For

- creating or importing a local identity
- registering or recovering a handle-backed identity
- binding phone or email contact information
- switching the default identity
- reading or updating the DID profile

## Core Concepts

- **identity**: the local awiki identity container selected with `--identity`
- **DID**: the protocol identifier used by the services
- **handle**: the human-friendly public identifier
- **contact binding**: adding phone or email to an existing identity
- **current identity**: the default local identity used when `--identity` is omitted

## Lifecycle

`status -> create/register/import -> bind -> profile set -> current/use`

## Decision Rules

- no local identity yet -> prefer `awiki-cli id register ...`; use hidden `id create` only for bootstrap, migration, or debug
- local identity exists but no handle-backed user state -> `awiki-cli id register ...`
- handle exists but contact info is incomplete -> `awiki-cli id bind ...`
- lost handle but still have recovery phone -> `awiki-cli id recover ...`
- need to inspect multiple local identities -> `awiki-cli id list`
- need to switch the default identity -> `awiki-cli id use <identity>`
- need to inspect public profile data -> `awiki-cli id profile get ...`

## Canonical Commands

- `awiki-cli id status`
- `awiki-cli id list`
- `awiki-cli id current`
- `awiki-cli id use <identity>`
- `awiki-cli id register --handle <handle> (--phone <phone> [--otp <code>] | --email <email> [--wait])`
- `awiki-cli id bind (--phone <phone> [--otp <code>] | --email <email> [--wait])`
- `awiki-cli id resolve (--handle <handle> | --did <did>)`
- `awiki-cli id recover --handle <handle> --phone <phone> --otp <code>`
- `awiki-cli id profile get [--self | --handle <handle> | --did <did>]`
- `awiki-cli id profile set [--display-name ...] [--bio ...] [--tags ...] [--markdown ...] [--markdown-file ...]`
- `awiki-cli id import-v1 [--name <identity> | --all]`

## Common Patterns

### Preferred registration flow

1. `awiki-cli id status`
2. `awiki-cli id register --handle alice --phone +8613800138000 --otp 123456`
3. `awiki-cli id current`
4. `awiki-cli id bind --email alice@example.com --wait`
5. `awiki-cli id profile set --display-name "Alice"`

### Import from v1 before switching

1. `awiki-cli id import-v1 --all --dry-run`
2. `awiki-cli id import-v1 --all`
3. `awiki-cli id list`
4. `awiki-cli id use <identity>`

## Side Effects and Confirmation

| Command family | Effect | Confirmation rule |
|---|---|---|
| `id register` | creates or completes remote user registration | explicit confirmation |
| `id bind` | mutates remote identity contact data | explicit confirmation |
| `id recover` | rebinds a handle using recovery flow | explicit confirmation |
| `id use` | changes local default identity | explicit confirmation |
| `id profile set` | mutates remote DID profile | explicit confirmation |
| `id import-v1` | imports local credentials and metadata | explicit confirmation |
| `id create` | creates local DID material only | explicit confirmation and only for internal/bootstrap use |

## Error Handling

- registration or bind shape confusion -> inspect `awiki-cli schema id register` or `awiki-cli schema id bind`
- auth or token confusion -> recover or re-register the identity
- missing identity -> use `awiki-cli id list` and `awiki-cli id current`
- local store confusion -> use `awiki-cli doctor`

## Implementation Notes

- `id create` is hidden on purpose.
- Public guidance must stay handle-first.
- `user_id` is not part of the public contract for this skill.

## References

- `docs/architecture/awiki-v2-architecture.md`
- `docs/architecture/awiki-command-v2.md`

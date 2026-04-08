# awiki Skills

This directory contains the awiki skill set for the current `awiki-cli` product surface.

## Structure

- `awiki-bundle/`: routing entrypoint
- `awiki-shared/`: shared rules and safety boundaries
- `awiki-*/`: domain, workflow, and debug skills
- `manifests/skills.yaml`: structured skill index
- `templates/`: authoring templates for future skills

## Current Source of Truth

Skill maintenance currently follows two coordinated sources:

1. `internal/cmdmeta/catalog.go` for command contract and implementation status
2. `skills/manifests/skills.yaml` for skill routing, grouping, and ownership

If they disagree:

- command existence and implementation status come from `cmdmeta`
- skill ownership and routing come from `skills.yaml`

## Maintenance Rules

- Use `awiki-cli` in command examples
- Keep shared rules in `awiki-shared` only
- Mark unimplemented capabilities as `partial` or `planned`
- Prefer canonical commands over shortcuts or imagined aliases
- Route multi-step tasks through workflow skills
- Use debug only as a last resort

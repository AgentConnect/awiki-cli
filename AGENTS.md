# Repository Guidelines

## Project Structure & Module Organization

This repository currently serves as the design home for **awiki-cli**, a rewrite of the Python CLI client. Architecture and behavior notes live under `docs/architecture/`:

- `awiki-v2-architecture.md` — overall CLI architecture
- `awiki-command-v2.md` — command model and command flow
- `websocket-host-notification-v1.md` — websocket listener to host notification normalization and sink contract
- `openclaw-host-adapter-v1.md` — OpenClaw `/hooks/agent` adapter contract for awiki-cli host notifications
- `cli-init.md` / `overall-init.md` — bootstrap and initialization flows
- `output-format.md` — terminal output contract

Cross-repo references are required for implementation work:

- `../message-service/docs/api/` — messaging APIs
- `../user-service/docs/api/` — user APIs
- `../awiki-agent-id-message/` — legacy Python CLI being rewritten
- `../cli/` — Feishu CLI reference

## Build, Test, and Development Commands

This repo does not yet ship executable Python code, but contributors should use **UV** for all Python workflows when code is added.

- `uv sync` — install project dependencies
- `uv run pytest` — run tests
- `uv run python -m awiki_cli ...` — run the CLI locally

If you add a new command or API integration, update the matching architecture doc and verify the sibling service API docs still match the behavior.

## Coding Style & Naming Conventions

- Follow **Google Python Style Guide**.
- Use 4-space indentation.
- Keep code comments, docstrings, logs, and user-facing documentation in **English**.
- Prefer `snake_case` for Python modules, functions, and variables; `PascalCase` for classes.
- Name design docs by topic, e.g. `auth-bootstrap.md`, `message-sync.md`.

## Testing Guidelines

- Place unit tests under `tests/`.
- Name test files `test_<module>.py`.
- Cover command parsing, API client behavior, and output formatting.
- For cross-service flows, validate request/response assumptions against `../user-service` and `../message-service` docs before merging.
- After completing any development task, run the unit test suite before handing off the change to catch unintended regressions. For the current Go implementation, use `go test ./...`; when coverage gates are relevant, also run `make coverage-check`.
- When adding new production code or changing existing behavior, add or update the corresponding unit tests in the same change. New functionality should not be delivered without regression coverage for its stable behavior and key error paths.

## Commit & Pull Request Guidelines

Git history is currently minimal (`Initial commit`), so use short, imperative commit messages such as:

- `Add auth command architecture draft`
- `Document message sync workflow`

For pull requests, include:

1. A short summary of the change
2. A list of affected docs or commands
3. Any related service/API paths
4. Terminal screenshots or sample output when output behavior changes

## Contributor Notes

This project is a rewrite. Prefer aligning new behavior with the intended V2 architecture, while checking `../awiki-agent-id-message/` and `../cli/` for proven interaction patterns.

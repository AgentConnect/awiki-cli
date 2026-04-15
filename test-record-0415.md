# Test Record 0415

## Session
- Date: 2026-04-15
- Workspace: `/Users/cs/work/agents/awiki-space/awiki-cli/awiki-cli-dev`
- Binary: `./bin/awiki-cli`
- Active identity: `changshan-cli`
- Goal: websocket realtime listener testing in isolated workspace

## Issue Log

### 2026-04-15 1. `id status` panics when there is no local identity
- Reproduction:
  - `source ./awiki-cli-dev/activate.sh`
  - Ensure isolated workspace has zero local identities
  - Run `./bin/awiki-cli id status --format json`
- Observed:
  - CLI panics with `panic: runtime error: invalid memory address or nil pointer dereference`
  - Stack points to `internal/cli/id.go:557` via `identityMetaFromData`
- Impact:
  - Read-only identity inspection is not safe in a fresh workspace
- Status: open

### 2026-04-15 2. `runtime listener start` fails before `runtime listener install`
- Reproduction:
  - `source ./awiki-cli-dev/activate.sh`
  - `./bin/awiki-cli runtime listener enable --format json`
  - `./bin/awiki-cli runtime listener start --format json`
- Observed:
  - CLI returns `listener service is not installed`
  - Hint says to run `awiki-cli runtime listener install` first
- Impact:
  - `start` cannot bootstrap the service from an uninstalled state in this workspace
  - Runtime operation required an extra install step
- Status: open

### 2026-04-15 3. Transient warning right after listener start: socket not yet available
- Reproduction:
  - `./bin/awiki-cli runtime listener install --format json`
  - `./bin/awiki-cli runtime listener start --format json`
  - Immediately run `./bin/awiki-cli runtime listener status --format json`
- Observed:
  - Listener reports `running: true`
  - Warning: `listener socket is not available`
- Follow-up:
  - After about 2 seconds, `bridge_available` becomes `true`
  - Unix socket file exists and websocket session reports `connected: true`
- Impact:
  - There is a short readiness gap between process start and socket availability
- Status: observed / may be expected, keep watching

## Runtime Listener Notes
- Listener was intentionally disabled in isolated config before this test session.
- Runtime mode confirmed: `websocket`.
- Listener enable completed successfully.
- Listener install completed successfully.
- Listener is now running in the isolated workspace.
- Verified after startup stabilization:
  - `installed: true`
  - `running: true`
  - `bridge_available: true`
  - active session connected for `changshan-cli`

### 2026-04-15 4. Root cause analysis for transient `listener socket is not available`
- Measured restart timeline in isolated workspace:
  - `t=154ms`: `running=true`, `bridge_available=false`, warning present
  - `t=585ms`: `running=true`, `bridge_available=true`, no warning
  - `t=938ms`: first websocket session shows `connected=true`
- Code-level findings:
  - `internal/runtime/listener/service.go:34-55`
    - `serviceProgram.Start()` spawns `supervisor.Run(ctx)` in a goroutine and returns immediately.
    - This lets the service manager mark the process as started before listener initialization is finished.
  - `internal/runtime/listener/service.go:178-199`
    - `StartService()` waits only for service-manager state via `waitForServiceStatus(resolved, true)`.
    - It does **not** wait for socket readiness or websocket session readiness.
  - `internal/runtime/listener/server.go:117-133`
    - Actual initialization order inside `Supervisor.Run()` is:
      1. write pid/status
      2. create unix socket via `startSocket()`
      3. bootstrap known websocket sessions via `startKnownSessions()`
    - So the command can return while step 2 or 3 is still in progress.
  - `internal/runtime/listener/manager.go:56-60`
    - Status warning is emitted whenever listener is enabled but `BridgeEndpointAvailable(socketPath)` is false.
  - `internal/runtime/bridge_unix.go:47-49`
    - `BridgeEndpointAvailable()` only checks `os.Stat(socketPath)`.
- Conclusion:
  - This is primarily a **startup readiness race**, not a persistent runtime failure.
  - `runtime listener start/restart` reports success too early: it tracks service-manager `running`, but not bridge/socket readiness.
  - There is also a second smaller readiness phase after socket creation, where the websocket session is not yet connected.
- Suggested fix direction:
  - Preferred: make listener startup block on a readiness signal from `Supervisor` after `startSocket()` and initial `startKnownSessions()` complete.
  - Minimum fix: update `waitForServiceStatus(..., true)` so websocket listener start waits for `status.BridgeAvailable == true` before returning success.
  - Optional stronger fix: also wait until the default/current identity session reaches `connected=true`, or expose a dedicated readiness field distinct from service-manager `running`.
- Status: root cause identified

## Fix Result

### 2026-04-15 5. Listener startup readiness fix applied
- Code change:
  - Updated `internal/runtime/listener/service.go`
  - `waitForServiceStatus()` now waits for `bridge_available=true` in websocket mode before reporting start/restart success
  - Added helper logic to keep readiness rules explicit and testable
- Regression test added:
  - `internal/runtime/listener/service_test.go`
  - `TestWaitForServiceStatusWithWaitsForBridgeAvailability`
- Verification:
  - `CGO_ENABLED=0 go test ./internal/runtime/listener -run TestWaitForServiceStatusWithWaitsForBridgeAvailability -count=1`
  - `CGO_ENABLED=0 go test ./internal/runtime/listener -count=1`
  - Rebuilt `./bin/awiki-cli`
  - Re-ran `runtime listener restart` followed by immediate `runtime listener status`
- Observed after fix:
  - `runtime listener restart` now returns with `bridge_available=true`
  - Immediate `runtime listener status` also shows `bridge_available=true`
  - The transient `listener socket is not available` warning is no longer reproduced in the local websocket restart path
- Status: fixed locally

### 2026-04-15 6. Host notify webhook E2E requires extra local scaffolding in this environment
- Observed:
  - `openclaw` CLI binary is not installed locally by default
  - `runtime.host_notify.enabled` has no dedicated CLI toggle in the current command surface
- Impact:
  - Local webhook E2E testing cannot be completed through public awiki-cli commands alone in this environment
- Local workaround for test only:
  - Added a mock `openclaw` CLI at `~/.npm-global/bin/openclaw`
  - Manually updated isolated `config.yaml` to set `runtime.host_notify.enabled = true`
- Status: open UX/runtime gap

### 2026-04-15 7. Websocket two-handle messaging and webhook callback fan-out verified locally
- Setup:
  - Started local webhook hub: `scripts/host_notify_webhook_server.py` on `127.0.0.1:18900`
  - Started per-user callback receivers on `127.0.0.1:19001` and `127.0.0.1:19002`
  - Switched default identity to `changshan-cli02`
  - Enabled `runtime.host_notify` in isolated config and pointed OpenClaw hook URL to the local webhook hub
- Local test harness note:
  - Because real `openclaw` is not installed here, a local mock `openclaw` binary was used for webhook E2E
  - Callback routing in the mock fan-out path used the OpenClaw-style `to` target (`changshan-cli` / `changshan-cli02`)
- Messaging verification:
  - `changshan-cli02 -> changshan-cli`: delivered and visible in `msg inbox`
  - `changshan-cli -> changshan-cli02`: delivered and visible in `msg inbox`
- Callback verification:
  - `awiki-cli-dev/callback-logs/changshan-cli.jsonl` received only the message addressed to `changshan-cli`
  - `awiki-cli-dev/callback-logs/changshan-cli02.jsonl` received only the message addressed to `changshan-cli02`
- Status: verified locally with test harness

### 2026-04-15 8. Added host notify enable/disable switch and changed default enablement to on
- Product behavior change:
  - `runtime.host_notify.enabled` default changed from `false` to `true`
  - New commands added:
    - `awiki-cli runtime host-notify enable`
    - `awiki-cli runtime host-notify disable`
- UX simplification:
  - `runtime host-notify config set --sink ...` now also writes `host_notify.enabled = true`
  - New workspaces no longer need a separate manual enable step for host notifications
- Verification:
  - `CGO_ENABLED=0 go test ./internal/config -count=1`
  - `CGO_ENABLED=0 go test ./internal/runtime -count=1`
  - `CGO_ENABLED=0 go test ./internal/cli -count=1`
  - `./bin/awiki-cli schema 'runtime host-notify enable' --format json`
- Status: implemented locally

### 2026-04-15 9. Webhook server callback registry persistence added and verified
- Code change:
  - Updated `scripts/host_notify_webhook_server.py`
  - Callback registrations are now persisted to a JSON state file
  - Default state file path:
    - when `AWIKI_CLI_WORKSPACE_HOME_DIR` is set: `<workspace>/runtime/host-notify-webhook-callbacks.json`
    - otherwise: `~/.awiki-cli/runtime/host-notify-webhook-callbacks.json`
- Verification:
  - Registered a callback against an explicit test state file
  - Restarted the webhook server
  - Confirmed `GET /callbacks` still returned the pre-registered callback after restart
- Status: fixed locally

### 2026-04-15 10. `id status` fresh-workspace panic fixed
- Verification:
  - Ran `HOME=<empty-home> AWIKI_CLI_WORKSPACE_HOME_DIR=<empty-workspace> ./bin/awiki-cli id status --format json`
  - Command now returns `active_identity: null`, `identity_count: 0`, and summary `No default identity is configured yet`
  - No panic occurs
- Regression coverage:
  - `internal/cli/id_test.go`
  - `TestIdentityMetaFromDataSkipsTypedNilIdentitySummary`
- Status: fixed locally

### 2026-04-15 11. `runtime listener start` now auto-installs missing service before start
- Code change:
  - Updated `internal/runtime/listener/service.go`
  - `StartService()` now calls install automatically when the listener service is missing
- Regression coverage:
  - `internal/runtime/listener/service_test.go`
  - `TestStartServiceAutoInstallsWhenMissing`
- Additional runtime fix:
  - Hidden commands `awiki-cli runtime listener run` and `awiki-cli runtime listener service-run` are now exempt from strict version blocking so the service manager can launch the listener reliably
- Manual verification:
  - Initialized a fresh isolated workspace
  - Uninstalled the listener service
  - Ran `runtime listener start` with only the foreground command using `AWIKI_CLI_DISABLE_STRICT_VERSION=true`
  - Verified the command auto-installed the service and returned `installed=true`, `running=true`, `bridge_available=true`
- Status: fixed locally

## Remaining Unresolved Test Issues
- Real OpenClaw end-to-end verification still depends on a real local `openclaw` installation; current webhook E2E used a local mock harness

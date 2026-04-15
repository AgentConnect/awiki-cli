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

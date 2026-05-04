package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/output"
	runtimecfg "github.com/agentconnect/awiki-cli/internal/runtime"
	"github.com/agentconnect/awiki-cli/internal/runtime/hermesbridge"
	listenerrt "github.com/agentconnect/awiki-cli/internal/runtime/listener"
	"github.com/agentconnect/awiki-cli/internal/runtime/openclawnotify"
	"github.com/agentconnect/awiki-cli/internal/store"
	"github.com/spf13/cobra"
)

var (
	listenerStatusForFunc = listenerrt.StatusFor
	listenerStopFunc      = listenerrt.Stop
	runtimeBootstrapFunc  = func(app *App, resolved *appconfig.Resolved) (listenerrt.Status, error) {
		return app.ensureRuntimeBootstrap(resolved)
	}
)

const (
	hermesNotifySecretEnv  = "AWIKI_HOST_NOTIFY_HERMES_SECRET"
	legacyWebhookSecretEnv = "AWIKI_HOST_NOTIFY_WEBHOOK_SECRET"
	defaultHermesNotifyURL = "http://127.0.0.1:8765/notify/host-event"
)

func runtimeFormat(resolved *appconfig.Resolved) string {
	if resolved == nil || strings.TrimSpace(resolved.OutputFormat) == "" {
		return "json"
	}
	return resolved.OutputFormat
}

func (a *App) ensureRuntimeBootstrap(resolved *appconfig.Resolved) (listenerrt.Status, error) {
	db, err := store.Open(resolved.Paths)
	if err != nil {
		return listenerrt.Status{}, err
	}
	defer db.Close()
	if err := store.EnsureSchema(context.Background(), db); err != nil {
		return listenerrt.Status{}, err
	}
	return listenerrt.ApplyRuntimePolicy(resolved)
}

func (a *App) runtimeExit(err error, hint string) error {
	if err == nil {
		return nil
	}
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) {
		return err
	}
	switch {
	case errors.Is(err, identity.ErrUserRegistrationRequired):
		return output.NewExitError("identity_required", 3, err.Error(), "Complete user setup with `awiki-cli id register --handle <handle> ...` or recover an existing handle before starting realtime runtime.")
	case strings.Contains(err.Error(), "must be websocket"), strings.Contains(err.Error(), "unsupported mode"), strings.Contains(err.Error(), "runtime mode"):
		return output.NewExitError("invalid_argument", 2, err.Error(), refineWorkspaceWriteHint(err, hint))
	default:
		return output.NewExitError("internal_error", 1, err.Error(), refineWorkspaceWriteHint(err, hint))
	}
}

func (a *App) refreshListenerForHostNotifyChange(resolved *appconfig.Resolved) (*listenerrt.Status, []string, error) {
	status, err := listenerStatusForFunc(resolved)
	if err != nil {
		return nil, nil, err
	}
	runtimeResolved := runtimecfg.Resolve(resolved)
	if runtimeResolved.Mode != runtimecfg.ModeWebSocket || !runtimeResolved.Listener.Enabled {
		return &status, []string{"Host notify changes will apply the next time the websocket listener is enabled."}, nil
	}
	if !status.Running {
		return &status, []string{"Host notify changes will apply the next time the listener starts."}, nil
	}
	if _, err := listenerStopFunc(resolved); err != nil {
		return nil, nil, fmt.Errorf("stop listener to apply host notify config: %w", err)
	}
	restarted, err := runtimeBootstrapFunc(a, resolved)
	if err != nil {
		return nil, nil, fmt.Errorf("restart listener to apply host notify config: %w", err)
	}
	warnings := append([]string{"Listener restarted to apply host notify configuration."}, restarted.Warnings...)
	return &restarted, warnings, nil
}

func (a *App) runRuntimeStatus(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	status, err := listenerrt.StatusFor(resolved)
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	data := map[string]any{
		"runtime":  runtimecfg.Resolve(resolved),
		"listener": status,
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Runtime status loaded", status.Warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeApply(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	currentStatus, _ := listenerrt.StatusFor(resolved)
	if a.globals.DryRun {
		data := map[string]any{
			"plan": map[string]any{
				"action":   "runtime_apply",
				"runtime":  runtimecfg.Resolve(resolved),
				"listener": currentStatus,
			},
		}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: runtime apply planned", nil, identityMetaFromResolved(resolved))
	}
	status, err := a.ensureRuntimeBootstrap(resolved)
	if err != nil {
		return a.runtimeExit(err, "Check listener service permissions, runtime.listener settings, and local sqlite access.")
	}
	data := map[string]any{
		"runtime":  runtimecfg.Resolve(resolved),
		"listener": status,
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Runtime state applied", status.Warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeSetup(cmd *cobra.Command, args []string) error {
	mode, _ := cmd.Flags().GetString("mode")
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	if strings.TrimSpace(mode) == "" {
		mode = resolved.RuntimeMode
	}
	if mode != runtimecfg.ModeHTTP && mode != runtimecfg.ModeWebSocket {
		return output.NewExitError("invalid_argument", 2, "runtime setup requires --mode http|websocket.", "Use runtime setup --mode websocket or runtime setup --mode http.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":         "runtime_setup",
			"mode":           mode,
			"workspace_home": resolved.Paths.WorkspaceHomeDir,
			"runtime_dir":    resolved.Paths.StateDir,
			"database_file":  resolved.Paths.DatabaseFile,
			"writes":         []string{resolved.Paths.ConfigFile, resolved.Paths.DatabaseFile},
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: runtime setup planned", nil, identityMetaFromResolved(resolved))
	}
	if err := appconfig.UpdateRuntimeSettings(resolved.Paths, mode, resolved.RuntimeSocketPath); err != nil {
		return a.runtimeExit(err, "Check write permissions for config.yaml.")
	}
	resolved, err = a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	listenerStatus, err := a.ensureRuntimeBootstrap(resolved)
	if err != nil {
		return a.runtimeExit(err, "Check listener service permissions, runtime.listener settings, and local sqlite access.")
	}
	data := map[string]any{
		"action":   "runtime_setup",
		"mode":     mode,
		"paths":    resolved.Paths,
		"listener": listenerStatus,
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Runtime setup completed", nil, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeModeGet(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	data := map[string]any{"runtime": runtimecfg.Resolve(resolved)}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Runtime mode loaded", nil, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeModeSet(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return output.NewExitError("invalid_argument", 2, "runtime mode set requires exactly one mode.", "Usage: awiki-cli runtime mode set <http|websocket>")
	}
	mode := strings.ToLower(strings.TrimSpace(args[0]))
	if mode != runtimecfg.ModeHTTP && mode != runtimecfg.ModeWebSocket {
		return output.NewExitError("invalid_argument", 2, "unsupported runtime mode", "Use runtime mode set http or runtime mode set websocket.")
	}
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":      "runtime_mode_set",
			"mode":        mode,
			"config_file": resolved.Paths.ConfigFile,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: runtime mode change planned", nil, identityMetaFromResolved(resolved))
	}
	if err := appconfig.UpdateRuntimeSettings(resolved.Paths, mode, resolved.RuntimeSocketPath); err != nil {
		return a.runtimeExit(err, "Check write permissions for config.yaml.")
	}
	resolved, err = a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	listenerStatus, err := a.ensureRuntimeBootstrap(resolved)
	if err != nil {
		return a.runtimeExit(err, "Check listener service permissions, runtime.listener settings, and local sqlite access.")
	}
	data := map[string]any{
		"action":   "runtime_mode_set",
		"mode":     mode,
		"listener": listenerStatus,
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, fmt.Sprintf("Runtime mode set to %s", mode), nil, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeListenerStatus(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	status, err := listenerrt.StatusFor(resolved)
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	data := map[string]any{"listener": status}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Listener status loaded", status.Warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeListenerStart(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":      "listener_start",
			"mode":        resolved.RuntimeMode,
			"socket_path": resolved.RuntimeSocketPath,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: listener start planned", nil, identityMetaFromResolved(resolved))
	}
	status, err := listenerrt.Start(resolved)
	if err != nil {
		return a.runtimeExit(err, "Set runtime.mode to websocket and check listener service permissions before starting the listener.")
	}
	data := map[string]any{"listener": status}
	summary := "Listener started"
	if listenerrt.HasDisconnectedSessions(status.Sessions) {
		summary = "Listener started, but some websocket sessions are disconnected"
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, summary, status.Warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeListenerStop(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action": "listener_stop",
			"pid":    "from_pid_file",
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: listener stop planned", nil, identityMetaFromResolved(resolved))
	}
	status, err := listenerrt.Stop(resolved)
	if err != nil {
		return a.runtimeExit(err, "Check the pid file and socket path under ~/.awiki-cli/runtime.")
	}
	data := map[string]any{"listener": status}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Listener stopped", status.Warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeListenerRestart(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":      "listener_restart",
			"mode":        resolved.RuntimeMode,
			"socket_path": resolved.RuntimeSocketPath,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: listener restart planned", nil, identityMetaFromResolved(resolved))
	}
	status, err := listenerrt.Restart(resolved)
	if err != nil {
		return a.runtimeExit(err, "Set runtime.mode to websocket and install the listener service before restarting it.")
	}
	data := map[string]any{"listener": status}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Listener restarted", status.Warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeListenerInstall(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":      "listener_install",
			"mode":        resolved.RuntimeMode,
			"socket_path": resolved.RuntimeSocketPath,
			"service":     true,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: listener install planned", nil, identityMetaFromResolved(resolved))
	}
	status, err := listenerrt.Install(resolved)
	if err != nil {
		return a.runtimeExit(err, "Check service manager permissions for listener installation.")
	}
	data := map[string]any{"listener": status}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Listener service installed", status.Warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeListenerUninstall(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":  "listener_uninstall",
			"service": true,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: listener uninstall planned", nil, identityMetaFromResolved(resolved))
	}
	status, err := listenerrt.Uninstall(resolved)
	if err != nil {
		return a.runtimeExit(err, "Check service manager permissions for listener removal.")
	}
	data := map[string]any{"listener": status}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Listener service uninstalled", status.Warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeListenerRun(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return err
	}
	return listenerrt.RunForeground(resolved)
}

func (a *App) runRuntimeListenerServiceRun(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return err
	}
	return listenerrt.RunService(resolved)
}

func (a *App) runRuntimeListenerConfigShow(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	data := map[string]any{
		"listener": map[string]any{
			"enabled":      resolved.RuntimeListenerEnabled,
			"auto_install": resolved.RuntimeListenerAutoInstall,
			"auto_start":   resolved.RuntimeListenerAutoStart,
		},
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Listener config loaded", nil, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeListenerConfigSet(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	var enabledPtr, autoInstallPtr, autoStartPtr *bool
	if cmd.Flags().Changed("enabled") {
		value, _ := cmd.Flags().GetBool("enabled")
		enabledPtr = &value
	}
	if cmd.Flags().Changed("auto-install") {
		value, _ := cmd.Flags().GetBool("auto-install")
		autoInstallPtr = &value
	}
	if cmd.Flags().Changed("auto-start") {
		value, _ := cmd.Flags().GetBool("auto-start")
		autoStartPtr = &value
	}
	if enabledPtr == nil && autoInstallPtr == nil && autoStartPtr == nil {
		return output.NewExitError("invalid_argument", 2, "listener config set requires at least one changed flag.", "Use --enabled, --auto-install, or --auto-start.")
	}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":       "listener_config_set",
			"enabled":      enabledPtr,
			"auto_install": autoInstallPtr,
			"auto_start":   autoStartPtr,
			"config_file":  resolved.Paths.ConfigFile,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: listener config change planned", nil, identityMetaFromResolved(resolved))
	}
	if err := appconfig.UpdateRuntimeListenerSettings(resolved.Paths, enabledPtr, autoInstallPtr, autoStartPtr); err != nil {
		return a.runtimeExit(err, "Check write permissions for config.yaml.")
	}
	resolved, err = a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli config show` to inspect the updated configuration.")
	}
	data := map[string]any{
		"listener": map[string]any{
			"enabled":      resolved.RuntimeListenerEnabled,
			"auto_install": resolved.RuntimeListenerAutoInstall,
			"auto_start":   resolved.RuntimeListenerAutoStart,
		},
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Listener config updated", nil, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeListenerEnable(cmd *cobra.Command, args []string) error {
	return a.setRuntimeListenerEnabled(cmd, true)
}

func (a *App) runRuntimeListenerDisable(cmd *cobra.Command, args []string) error {
	return a.setRuntimeListenerEnabled(cmd, false)
}

func (a *App) setRuntimeListenerEnabled(cmd *cobra.Command, enabled bool) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":      "listener_enable_toggle",
			"enabled":     enabled,
			"config_file": resolved.Paths.ConfigFile,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: listener enablement change planned", nil, identityMetaFromResolved(resolved))
	}
	if err := appconfig.UpdateRuntimeListenerSettings(resolved.Paths, &enabled, nil, nil); err != nil {
		return a.runtimeExit(err, "Check write permissions for config.yaml.")
	}
	resolved, err = a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli config show` to inspect the updated configuration.")
	}
	status, err := a.ensureRuntimeBootstrap(resolved)
	if err != nil {
		return a.runtimeExit(err, "Check listener service permissions, runtime.listener settings, and local sqlite access.")
	}
	data := map[string]any{
		"listener": status,
	}
	summary := "Listener enabled and runtime applied"
	if !enabled {
		summary = "Listener disabled and runtime applied"
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, summary, status.Warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeHostNotifyConfigShow(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	view, err := hostNotifyConfigView(resolved)
	if err != nil {
		return a.runtimeExit(err, "Check OpenClaw route registry and host notify configuration.")
	}
	data := map[string]any{
		"host_notify": view,
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Host notify config loaded", hostNotifyGuidanceWarnings(resolved), identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeHostNotifyConfigSet(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	if !cmd.Flags().Changed("sink") {
		return output.NewExitError("invalid_argument", 2, "host-notify config set requires --sink.", "Use --sink noop|log|file|openclaw|hermes.")
	}
	sink, _ := cmd.Flags().GetString("sink")
	switch sink {
	case "noop", "log", "file", "openclaw", "hermes", "webhook":
	default:
		return output.NewExitError("invalid_argument", 2, "unsupported host notify sink", "Use --sink noop, log, file, openclaw, or hermes.")
	}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":      "host_notify_config_set",
			"sink":        sink,
			"config_file": resolved.Paths.ConfigFile,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: host notify config change planned", nil, identityMetaFromResolved(resolved))
	}
	if err := appconfig.UpdateHostNotifySink(resolved.Paths, sink); err != nil {
		return a.runtimeExit(err, "Check write permissions for config.yaml.")
	}
	resolved, err = a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli config show` to inspect the updated configuration.")
	}
	listenerStatus, warnings, err := a.refreshListenerForHostNotifyChange(resolved)
	if err != nil {
		return a.runtimeExit(err, "Host notify config was updated, but the listener could not be restarted to apply it.")
	}
	view, err := hostNotifyConfigView(resolved)
	if err != nil {
		return a.runtimeExit(err, "Check OpenClaw route registry and host notify configuration.")
	}
	data := map[string]any{
		"host_notify": view,
	}
	if listenerStatus != nil {
		data["listener"] = listenerStatus
	}
	warnings = append(warnings, hostNotifyGuidanceWarnings(resolved)...)
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Host notify config updated", warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeHostNotifyEnable(cmd *cobra.Command, args []string) error {
	return a.setRuntimeHostNotifyEnabled(cmd, true)
}

func (a *App) runRuntimeHostNotifyDisable(cmd *cobra.Command, args []string) error {
	return a.setRuntimeHostNotifyEnabled(cmd, false)
}

func (a *App) setRuntimeHostNotifyEnabled(cmd *cobra.Command, enabled bool) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":      "host_notify_enable_toggle",
			"enabled":     enabled,
			"config_file": resolved.Paths.ConfigFile,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: host notify enablement change planned", nil, identityMetaFromResolved(resolved))
	}
	if err := appconfig.UpdateHostNotifyEnabled(resolved.Paths, enabled); err != nil {
		return a.runtimeExit(err, "Check write permissions for config.yaml.")
	}
	resolved, err = a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli config show` to inspect the updated configuration.")
	}
	listenerStatus, warnings, err := a.refreshListenerForHostNotifyChange(resolved)
	if err != nil {
		return a.runtimeExit(err, "Host notify setting was updated, but the listener could not be restarted to apply it.")
	}
	view, err := hostNotifyConfigView(resolved)
	if err != nil {
		return a.runtimeExit(err, "Check OpenClaw route registry and host notify configuration.")
	}
	data := map[string]any{
		"host_notify": view,
	}
	if listenerStatus != nil {
		data["listener"] = listenerStatus
	}
	summary := "Host notify enabled"
	if !enabled {
		summary = "Host notify disabled"
	}
	warnings = append(warnings, hostNotifyGuidanceWarnings(resolved)...)
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, summary, warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeHostNotifyOpenClawSet(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	var hookURLPtr *string
	if cmd.Flags().Changed("hook-url") {
		value, _ := cmd.Flags().GetString("hook-url")
		hookURLPtr = &value
	}
	if hookURLPtr == nil {
		return output.NewExitError("invalid_argument", 2, "openclaw set requires at least one changed flag.", "Use --hook-url.")
	}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":      "host_notify_openclaw_set",
			"hook_url":    hookURLPtr,
			"config_file": resolved.Paths.ConfigFile,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: OpenClaw host notify config change planned", nil, identityMetaFromResolved(resolved))
	}
	if err := appconfig.UpdateOpenClawSettings(resolved.Paths, hookURLPtr); err != nil {
		return a.runtimeExit(err, "Check write permissions for config.yaml.")
	}
	resolved, err = a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli config show` to inspect the updated configuration.")
	}
	listenerStatus, warnings, err := a.refreshListenerForHostNotifyChange(resolved)
	if err != nil {
		return a.runtimeExit(err, "OpenClaw host notify config was updated, but the listener could not be restarted to apply it.")
	}
	settings, err := openclawnotify.ResolveSettings(resolved)
	if err != nil {
		return a.runtimeExit(err, "Check the OpenClaw hook URL configuration.")
	}
	data := map[string]any{
		"openclaw": map[string]any{
			"hook_url":                     settings.HookURL,
			"hook_url_source":              settings.HookURLSource,
			"detected_webhook_port":        settings.DetectedWebhookPort,
			"detected_webhook_source":      settings.DetectedWebhookPortInfo.Source,
			"detected_webhook_path":        settings.DetectedWebhookPath,
			"detected_webhook_path_source": settings.DetectedWebhookPathInfo.Source,
		},
	}
	if listenerStatus != nil {
		data["listener"] = listenerStatus
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "OpenClaw host notify config updated", warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeHostNotifyOpenClawSetToken(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	value, _ := cmd.Flags().GetString("value")
	if strings.TrimSpace(value) == "" {
		return output.NewExitError("invalid_argument", 2, "openclaw set-token requires --value.", "Use --value <token>.")
	}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":      "host_notify_openclaw_set_token",
			"configured":  true,
			"config_file": resolved.Paths.ConfigFile,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: OpenClaw token update planned", nil, identityMetaFromResolved(resolved))
	}
	if err := appconfig.SetOpenClawToken(resolved.Paths, value); err != nil {
		return a.runtimeExit(err, "Check write permissions for config.yaml.")
	}
	resolved, err = a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli config show` to inspect the updated configuration.")
	}
	listenerStatus, warnings, err := a.refreshListenerForHostNotifyChange(resolved)
	if err != nil {
		return a.runtimeExit(err, "OpenClaw token was updated, but the listener could not be restarted to apply it.")
	}
	data := map[string]any{"openclaw": map[string]any{"token_configured": true}}
	if listenerStatus != nil {
		data["listener"] = listenerStatus
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "OpenClaw token updated", warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeHostNotifyOpenClawClearToken(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":      "host_notify_openclaw_clear_token",
			"config_file": resolved.Paths.ConfigFile,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: OpenClaw token clear planned", nil, identityMetaFromResolved(resolved))
	}
	if err := appconfig.ClearOpenClawToken(resolved.Paths); err != nil {
		return a.runtimeExit(err, "Check write permissions for config.yaml.")
	}
	resolved, err = a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli config show` to inspect the updated configuration.")
	}
	listenerStatus, warnings, err := a.refreshListenerForHostNotifyChange(resolved)
	if err != nil {
		return a.runtimeExit(err, "OpenClaw token was cleared, but the listener could not be restarted to apply it.")
	}
	data := map[string]any{"openclaw": map[string]any{"token_configured": false}}
	if listenerStatus != nil {
		data["listener"] = listenerStatus
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "OpenClaw token cleared", warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeHostNotifyHermesGuide(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	deliver, _ := cmd.Flags().GetString("deliver")
	deliver = resolveHermesDeliverTarget(resolved, deliver)
	if !hermesbridge.IsSupportedDeliverTarget(deliver) {
		return output.NewExitError("invalid_argument", 2, fmt.Sprintf("unsupported Hermes deliver target %q", deliver), fmt.Sprintf("Use --deliver with one of: %s.", strings.Join(hermesbridge.SupportedDeliverTargets(), ", ")))
	}
	notifyURL := resolveHermesNotifyURL(resolved)
	secretSource := resolveHermesSecretSource(resolved, notifyURL)
	data := map[string]any{
		"hermes_guide": buildHermesHostNotifyGuideView(resolved, notifyURL, deliver, secretSource),
	}
	if routeState, routeErr := hermesbridge.InspectRoute(resolveHermesHomeDir(), hermesbridge.DefaultWebhookRouteName); routeErr == nil {
		data["local_hermes"] = routeState
	}
	warnings := append([]string{}, hostNotifyGuidanceWarningsFor(resolved, deliver)...)
	currentHostNotify := runtimecfg.Resolve(resolved).HostNotify
	if currentHostNotify.Sink != "hermes" {
		warnings = append(warnings, fmt.Sprintf("Current host notify sink is %q. Run `awiki-cli runtime host-notify hermes setup` to switch awiki-cli over to the fully managed local Hermes flow.", currentHostNotify.Sink))
	}
	if secretSource == "unset" {
		warnings = append(warnings, "awiki-cli does not have a Hermes notify secret yet. `awiki-cli runtime host-notify hermes setup` will generate and persist one automatically.")
	}
	if deliver == "log" {
		warnings = append(warnings, "This guide is using `deliver: \"log\"` for probe-only verification. Switch to a real messaging platform such as `feishu` or `telegram` for end-user delivery.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Hermes host notify guide generated", warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeHostNotifyHermesStatus(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	hostNotifyView, err := hostNotifyConfigView(resolved)
	if err != nil {
		return a.runtimeExit(err, "Check host notify configuration and local OpenClaw route registry state.")
	}
	routeState, routeErr := hermesbridge.InspectRoute(resolveHermesHomeDir(), hermesbridge.DefaultWebhookRouteName)
	bridgeStatus, bridgeErr := hermesbridge.StatusFor(resolved)
	expectedDeliver := resolveHermesDeliverTarget(resolved, "")
	data := map[string]any{
		"host_notify": hostNotifyView,
		"readiness": map[string]any{
			"awiki_sink_is_hermes":           runtimecfg.Resolve(resolved).HostNotify.Sink == "hermes",
			"awiki_host_notify_enabled":      runtimecfg.Resolve(resolved).HostNotify.Enabled,
			"awiki_secret_configured":        resolveHermesSecretSource(resolved, resolveHermesNotifyURL(resolved)) != "unset",
			"hermes_route_configured":        routeErr == nil && routeState.RouteConfigured,
			"hermes_route_matches_deliver":   routeErr == nil && routeState.Deliver == expectedDeliver,
			"hermes_route_uses_home_channel": routeErr == nil && routeState.DeliverUsesHomeChannel,
			"home_channel_configured":        routeErr == nil && (expectedDeliver == "log" || routeState.HomeChannelConfigured),
			"bridge_running":                 bridgeStatus.Running,
			"bridge_available":               bridgeStatus.BridgeAvailable,
		},
	}
	warnings := append([]string{}, hostNotifyGuidanceWarningsFor(resolved, expectedDeliver)...)
	if routeErr == nil {
		data["local_hermes"] = routeState
		warnings = append(warnings, routeState.Warnings...)
	} else {
		warnings = append(warnings, fmt.Sprintf("Failed to inspect local Hermes config: %v", routeErr))
	}
	if bridgeErr == nil {
		data["bridge"] = bridgeStatus
		warnings = append(warnings, bridgeStatus.Warnings...)
	} else {
		warnings = append(warnings, fmt.Sprintf("Failed to inspect Hermes bridge status: %v", bridgeErr))
	}
	ready := runtimecfg.Resolve(resolved).HostNotify.Sink == "hermes" &&
		runtimecfg.Resolve(resolved).HostNotify.Enabled &&
		resolveHermesSecretSource(resolved, resolveHermesNotifyURL(resolved)) != "unset" &&
		routeErr == nil && routeState.RouteConfigured && routeState.Deliver == expectedDeliver &&
		(expectedDeliver == "log" || (routeState.DeliverUsesHomeChannel && routeState.HomeChannelConfigured)) &&
		bridgeStatus.Running && bridgeStatus.BridgeAvailable
	data["ready"] = ready
	summary := "Hermes host notify readiness loaded"
	if ready {
		summary = fmt.Sprintf("Hermes host notify is ready for awiki -> Hermes -> %s delivery", hermesbridge.DeliverDisplayName(expectedDeliver))
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, summary, dedupeStrings(warnings), identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeHostNotifyHermesSetup(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	notifyURL := resolveHermesNotifyURL(resolved)
	if cmd.Flags().Changed("notify-url") {
		value, _ := cmd.Flags().GetString("notify-url")
		notifyURL = strings.TrimSpace(value)
	}
	if strings.TrimSpace(notifyURL) == "" {
		return output.NewExitError("invalid_argument", 2, "hermes setup requires a notify URL.", "Use --notify-url or configure runtime.host_notify.hermes.notify_url first.")
	}
	if _, _, _, err := hermesbridge.ValidateLocalNotifyURL(notifyURL); err != nil {
		return output.NewExitError("invalid_argument", 2, err.Error(), "Use a local notify URL such as http://127.0.0.1:8765/notify/host-event for the fully managed Hermes flow.")
	}
	deliver, _ := cmd.Flags().GetString("deliver")
	deliver = resolveHermesDeliverTarget(resolved, deliver)
	if !hermesbridge.IsSupportedDeliverTarget(deliver) {
		return output.NewExitError("invalid_argument", 2, fmt.Sprintf("unsupported Hermes deliver target %q", deliver), fmt.Sprintf("Use --deliver with one of: %s.", strings.Join(hermesbridge.SupportedDeliverTargets(), ", ")))
	}
	secretValue, secretSourceBefore, err := resolveHermesSecretValue(resolved, notifyURL)
	if err != nil {
		return a.runtimeExit(err, "Check awiki-cli host notify secret sources.")
	}
	if cmd.Flags().Changed("secret") {
		value, _ := cmd.Flags().GetString("secret")
		if strings.TrimSpace(value) == "" {
			return output.NewExitError("invalid_argument", 2, "hermes setup requires a non-empty --secret when the flag is provided.", "Use --secret <secret>.")
		}
		secretValue = strings.TrimSpace(value)
		secretSourceBefore = "flag"
	} else if strings.TrimSpace(secretValue) == "" {
		secretValue = generateHermesNotifySecret()
		secretSourceBefore = "generated"
	}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":                  "host_notify_hermes_setup",
			"notify_url":              notifyURL,
			"secret_source":           secretSourceBefore,
			"deliver":                 deliver,
			"previous_sink":           runtimecfg.Resolve(resolved).HostNotify.Sink,
			"host_notify_enabled":     true,
			"awiki_config_file":       resolved.Paths.ConfigFile,
			"hermes_config_file":      filepath.Join(resolveHermesHomeDir(), "config.yaml"),
			"manages_local_hermes":    true,
			"starts_local_bridge":     true,
			"route_uses_home_channel": true,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: Hermes host notify setup planned", nil, identityMetaFromResolved(resolved))
	}
	if err := appconfig.ConfigureHermesHostNotify(resolved.Paths, notifyURL, &secretValue, deliver, true); err != nil {
		return a.runtimeExit(err, "Check write permissions for config.yaml.")
	}
	resolved, err = a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli config show` to inspect the updated configuration.")
	}
	routeState, err := hermesbridge.EnsureRoute(hermesbridge.EnsureRouteOptions{
		HermesHome: resolveHermesHomeDir(),
		RouteName:  hermesbridge.DefaultWebhookRouteName,
		Deliver:    deliver,
	})
	if err != nil {
		return a.runtimeExit(err, "Check local Hermes installation and ~/.hermes/config.yaml permissions.")
	}
	listenerStatus, warnings, err := a.refreshListenerForHostNotifyChange(resolved)
	if err != nil {
		return a.runtimeExit(err, "Hermes host notify setup was written, but the listener could not be restarted to apply it.")
	}
	bridgeStatus, err := hermesbridge.Apply(resolved)
	if err != nil {
		return a.runtimeExit(err, "Hermes was configured, but the local Hermes bridge could not be started.")
	}
	view, err := hostNotifyConfigView(resolved)
	if err != nil {
		return a.runtimeExit(err, "Check host notify configuration and local OpenClaw route registry state.")
	}
	data := map[string]any{
		"host_notify":  view,
		"local_hermes": routeState,
		"bridge":       bridgeStatus,
		"next_steps": []string{
			fmt.Sprintf("If you have not done it yet, send `/sethome` to Hermes from the desired %s chat.", hermesbridge.DeliverDisplayName(deliver)),
			"Use `awiki-cli runtime host-notify hermes status` to verify end-to-end readiness.",
		},
	}
	if listenerStatus != nil {
		data["listener"] = listenerStatus
	}
	warnings = append(warnings, hostNotifyGuidanceWarningsFor(resolved, deliver)...)
	warnings = append(warnings, routeState.Warnings...)
	warnings = append(warnings, bridgeStatus.Warnings...)
	if deliver != "log" && !routeState.HomeChannelConfigured {
		if routeState.HomeChannelKey != "" {
			warnings = append(warnings, fmt.Sprintf("Hermes route is ready, but %s is still missing. Run /sethome in %s to complete delivery targeting.", routeState.HomeChannelKey, hermesbridge.DeliverDisplayName(deliver)))
		} else {
			warnings = append(warnings, fmt.Sprintf("Hermes route is ready, but awiki-cli could not verify a home channel for %s. Set a home channel in Hermes before expecting delivery.", hermesbridge.DeliverDisplayName(deliver)))
		}
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Hermes host notify setup completed", dedupeStrings(warnings), identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeHostNotifyHermesBridgeServiceRun(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfig()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	return hermesbridge.RunService(resolved)
}

func (a *App) runRuntimeHostNotifyHermesSet(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	var notifyURLPtr *string
	var deliverPtr *string
	if cmd.Flags().Changed("notify-url") {
		value, _ := cmd.Flags().GetString("notify-url")
		notifyURLPtr = &value
	}
	if cmd.Flags().Changed("deliver") {
		value, _ := cmd.Flags().GetString("deliver")
		normalized := resolveHermesDeliverTarget(resolved, value)
		deliverPtr = &normalized
	}
	if notifyURLPtr == nil && deliverPtr == nil {
		return output.NewExitError("invalid_argument", 2, "hermes set requires at least one changed flag.", "Use --notify-url or --deliver.")
	}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":      "host_notify_hermes_set",
			"notify_url":  notifyURLPtr,
			"deliver":     deliverPtr,
			"config_file": resolved.Paths.ConfigFile,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: Hermes host notify config change planned", nil, identityMetaFromResolved(resolved))
	}
	if deliverPtr != nil && !hermesbridge.IsSupportedDeliverTarget(*deliverPtr) {
		return output.NewExitError("invalid_argument", 2, fmt.Sprintf("unsupported Hermes deliver target %q", *deliverPtr), fmt.Sprintf("Use --deliver with one of: %s.", strings.Join(hermesbridge.SupportedDeliverTargets(), ", ")))
	}
	if err := appconfig.UpdateHermesSettings(resolved.Paths, notifyURLPtr, deliverPtr); err != nil {
		return a.runtimeExit(err, "Check write permissions for config.yaml.")
	}
	resolved, err = a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli config show` to inspect the updated configuration.")
	}
	listenerStatus, warnings, err := a.refreshListenerForHostNotifyChange(resolved)
	if err != nil {
		return a.runtimeExit(err, "Hermes host notify config was updated, but the listener could not be restarted to apply it.")
	}
	data := map[string]any{
		"hermes": runtimecfg.Resolve(resolved).HostNotify.Hermes,
	}
	if listenerStatus != nil {
		data["listener"] = listenerStatus
	}
	warnings = append(warnings, hostNotifyGuidanceWarningsFor(resolved, resolveHermesDeliverTarget(resolved, ""))...)
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Hermes host notify config updated", warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeHostNotifyHermesSetSecret(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	value, _ := cmd.Flags().GetString("value")
	if strings.TrimSpace(value) == "" {
		return output.NewExitError("invalid_argument", 2, "hermes set-secret requires --value.", "Use --value <secret>.")
	}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":      "host_notify_hermes_set_secret",
			"configured":  true,
			"config_file": resolved.Paths.ConfigFile,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: Hermes secret update planned", nil, identityMetaFromResolved(resolved))
	}
	if err := appconfig.SetHermesSecret(resolved.Paths, value); err != nil {
		return a.runtimeExit(err, "Check write permissions for config.yaml.")
	}
	resolved, err = a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli config show` to inspect the updated configuration.")
	}
	listenerStatus, warnings, err := a.refreshListenerForHostNotifyChange(resolved)
	if err != nil {
		return a.runtimeExit(err, "Hermes secret was updated, but the listener could not be restarted to apply it.")
	}
	data := map[string]any{"hermes": map[string]any{"secret_configured": true}}
	if listenerStatus != nil {
		data["listener"] = listenerStatus
	}
	warnings = append(warnings, hostNotifyGuidanceWarningsFor(resolved, resolveHermesDeliverTarget(resolved, ""))...)
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Hermes secret updated", warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeHostNotifyHermesClearSecret(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":      "host_notify_hermes_clear_secret",
			"config_file": resolved.Paths.ConfigFile,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: Hermes secret clear planned", nil, identityMetaFromResolved(resolved))
	}
	if err := appconfig.ClearHermesSecret(resolved.Paths); err != nil {
		return a.runtimeExit(err, "Check write permissions for config.yaml.")
	}
	resolved, err = a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli config show` to inspect the updated configuration.")
	}
	listenerStatus, warnings, err := a.refreshListenerForHostNotifyChange(resolved)
	if err != nil {
		return a.runtimeExit(err, "Hermes secret was cleared, but the listener could not be restarted to apply it.")
	}
	data := map[string]any{"hermes": map[string]any{"secret_configured": false}}
	if listenerStatus != nil {
		data["listener"] = listenerStatus
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Hermes secret cleared", warnings, identityMetaFromResolved(resolved))
}

func hostNotifyConfigView(resolved *appconfig.Resolved) (map[string]any, error) {
	config := runtimecfg.Resolve(resolved).HostNotify
	settings, err := openclawnotify.ResolveSettings(resolved)
	if err != nil {
		return nil, err
	}
	routes, err := openclawnotify.LoadRoutes(resolved.Paths)
	if err != nil {
		return nil, err
	}
	hermesSecretSource := resolveHermesSecretSource(resolved, config.Hermes.NotifyURL)
	return map[string]any{
		"enabled":             config.Enabled,
		"sink":                config.Sink,
		"file_path":           config.FilePath,
		"route_registry_path": openclawnotify.RoutesPath(resolved.Paths),
		"routes":              routes,
		"openclaw": map[string]any{
			"hook_url":                     settings.HookURL,
			"hook_url_source":              settings.HookURLSource,
			"detected_webhook_port":        settings.DetectedWebhookPort,
			"detected_webhook_source":      settings.DetectedWebhookPortInfo.Source,
			"detected_webhook_path":        settings.DetectedWebhookPath,
			"detected_webhook_path_source": settings.DetectedWebhookPathInfo.Source,
			"token_configured":             settings.TokenConfigured,
			"token_source":                 settings.TokenSource,
		},
		"hermes": map[string]any{
			"notify_url":          config.Hermes.NotifyURL,
			"deliver":             resolveHermesDeliverTarget(resolved, ""),
			"secret_configured":   hermesSecretSource != "unset",
			"secret_source":       hermesSecretSource,
			"secret_env_fallback": hermesNotifySecretEnv,
			"secret_env_legacy":   legacyWebhookSecretEnv,
		},
	}, nil
}

func hostNotifyGuidanceWarnings(resolved *appconfig.Resolved) []string {
	return hostNotifyGuidanceWarningsFor(resolved, "")
}

func hostNotifyGuidanceWarningsFor(resolved *appconfig.Resolved, deliverOverride string) []string {
	if resolved == nil {
		return nil
	}
	config := runtimecfg.Resolve(resolved).HostNotify
	if config.Sink != "hermes" {
		return nil
	}
	deliver := resolveHermesDeliverTarget(resolved, deliverOverride)
	homeChannelKey := resolveHermesHomeChannelKey(deliver)
	targetWarning := "Prefer the platform home channel (or /sethome in Hermes) instead of hard-coding deliver_extra.chat_id in Hermes routes."
	if deliver != "log" && homeChannelKey != "" {
		targetWarning = fmt.Sprintf("For %s delivery, prefer setting %s (or using /sethome in Hermes) instead of hard-coding deliver_extra.chat_id in Hermes routes.", hermesbridge.DeliverDisplayName(deliver), homeChannelKey)
	}
	return []string{
		"Hermes sink only forwards notifications to the Hermes adapter. Final delivery targets are configured in Hermes, not in awiki-cli.",
		targetWarning,
		"`awiki-cli runtime host-notify hermes setup` now also updates the local Hermes notify route and starts the local bridge automatically.",
	}
}

func resolveHermesNotifyURL(resolved *appconfig.Resolved) string {
	if resolved != nil {
		if value := strings.TrimSpace(resolved.HostNotifyHermesNotifyURL); value != "" {
			return value
		}
		if strings.TrimSpace(resolved.Paths.ConfigFile) != "" {
			fileConfig, exists, err := appconfig.ReadFileConfig(resolved.Paths.ConfigFile)
			if err == nil && exists {
				if value := strings.TrimSpace(fileConfig.Runtime.HostNotify.Hermes.NotifyURL); value != "" {
					return value
				}
				if value := strings.TrimSpace(fileConfig.Runtime.HostNotify.LegacyWebhook.NotifyURL); value != "" {
					return value
				}
			}
		}
	}
	return defaultHermesNotifyURL
}

func buildHermesHostNotifyGuideView(resolved *appconfig.Resolved, notifyURL string, deliver string, secretSource string) map[string]any {
	currentHostNotify := runtimecfg.Resolve(resolved).HostNotify
	homeChannelKey := resolveHermesHomeChannelKey(deliver)
	targeting := []string{}
	if homeChannelKey != "" {
		targeting = append(targeting, fmt.Sprintf("Prefer %s for the default delivery target.", homeChannelKey))
	}
	if deliver != "log" {
		targeting = append(targeting, fmt.Sprintf("Or send /sethome or /set-home to Hermes from the desired %s chat.", hermesbridge.DeliverDisplayName(deliver)))
	}
	targeting = append(targeting, "Avoid hard-coding deliver_extra.chat_id unless you explicitly want a fixed destination.")
	routeYAML := fmt.Sprintf(`platforms:
  webhook:
    enabled: true
    extra:
      port: 8644
      secret: "${HERMES_WEBHOOK_SECRET}"
      routes:
        notify:
          secret: "${HERMES_ROUTE_SECRET}"
          events: []
          prompt: "{notify_payload}"
          skills: ["notify"]
          deliver: %q
`, deliver)
	adapterCommand := `python3 scripts/hermes_notify_adapter.py \
  --host 0.0.0.0 \
  --port 8765 \
  --notify-secret "<NOTIFY_SECRET>" \
  --hermes-webhook-url "http://127.0.0.1:8644/webhooks/notify" \
  --hermes-route-secret "<HERMES_ROUTE_SECRET>" \
  --log-level INFO`
	setupCommand := "awiki-cli runtime host-notify hermes setup"
	if deliver != resolveHermesDeliverTarget(resolved, "") {
		setupCommand += " --deliver " + deliver
	}
	return map[string]any{
		"delivery_model": "awiki-cli only forwards host notify events to the Hermes adapter. Final delivery targets are configured in Hermes.",
		"awiki_cli": map[string]any{
			"current": map[string]any{
				"enabled":           currentHostNotify.Enabled,
				"sink":              currentHostNotify.Sink,
				"notify_url":        notifyURL,
				"deliver":           resolveHermesDeliverTarget(resolved, ""),
				"secret_configured": secretSource != "unset",
				"secret_source":     secretSource,
			},
			"recommended_setup_command": setupCommand,
			"verify_commands": []string{
				"awiki-cli runtime host-notify config show",
				"awiki-cli runtime host-notify hermes status",
			},
		},
		"hermes": map[string]any{
			"notify_route_name":   "notify",
			"webhook_port":        8644,
			"webhook_secret_env":  "HERMES_WEBHOOK_SECRET",
			"route_secret_env":    "HERMES_ROUTE_SECRET",
			"recommended_route":   routeYAML,
			"adapter_notify_url":  "http://127.0.0.1:8765/notify/host-event",
			"adapter_healthcheck": "curl -sS http://127.0.0.1:8765/healthz",
			"adapter_run_command": adapterCommand,
			"deliver_target":      deliver,
			"awiki_expected_url":  notifyURL,
			"managed_by_setup":    "awiki-cli runtime host-notify hermes setup will write the local Hermes notify route and restart the local bridge for you.",
			"targeting":           targeting,
		},
	}
}

func resolveHermesDeliverTarget(resolved *appconfig.Resolved, override string) string {
	if value := strings.TrimSpace(override); value != "" {
		return strings.ToLower(value)
	}
	if resolved != nil {
		if value := strings.TrimSpace(resolved.HostNotifyHermesDeliver); value != "" {
			return strings.ToLower(value)
		}
		if strings.TrimSpace(resolved.Paths.ConfigFile) != "" {
			fileConfig, exists, err := appconfig.ReadFileConfig(resolved.Paths.ConfigFile)
			if err == nil && exists {
				if value := strings.TrimSpace(fileConfig.Runtime.HostNotify.Hermes.Deliver); value != "" {
					return strings.ToLower(value)
				}
			}
		}
	}
	return hermesbridge.NormalizeDeliverTarget("")
}

func resolveHermesHomeChannelKey(deliver string) string {
	return hermesbridge.HomeChannelEnvKey(deliver)
}

func resolveHermesSecretSource(resolved *appconfig.Resolved, notifyURL string) string {
	_, source, err := resolveHermesSecretValue(resolved, notifyURL)
	if err != nil {
		return "unset"
	}
	return source
}

func resolveHermesSecretValue(resolved *appconfig.Resolved, notifyURL string) (string, string, error) {
	if resolved != nil && strings.TrimSpace(resolved.Paths.ConfigFile) != "" {
		fileConfig, exists, err := appconfig.ReadFileConfig(resolved.Paths.ConfigFile)
		if err != nil {
			return "", "unset", err
		}
		if exists {
			if value := strings.TrimSpace(fileConfig.Runtime.HostNotify.Hermes.Secret); value != "" {
				return value, "config_file", nil
			}
			if value := strings.TrimSpace(fileConfig.Runtime.HostNotify.LegacyWebhook.Secret); value != "" {
				return value, "config_file", nil
			}
		}
	}
	if strings.TrimSpace(notifyURL) != "" {
		if value := strings.TrimSpace(os.Getenv(hermesNotifySecretEnv)); value != "" {
			return value, "environment", nil
		}
		if value := strings.TrimSpace(os.Getenv(legacyWebhookSecretEnv)); value != "" {
			return value, "environment", nil
		}
	}
	return "", "unset", nil
}

func resolveHermesHomeDir() string {
	home, err := hermesbridge.ResolveHermesHome()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".hermes")
	}
	return home
}

func generateHermesNotifySecret() string {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "awiki-hermes-secret"
	}
	return hex.EncodeToString(buf)
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

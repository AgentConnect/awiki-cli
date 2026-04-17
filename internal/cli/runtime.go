package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/output"
	runtimecfg "github.com/agentconnect/awiki-cli/internal/runtime"
	listenerrt "github.com/agentconnect/awiki-cli/internal/runtime/listener"
	"github.com/agentconnect/awiki-cli/internal/runtime/openclawnotify"
	"github.com/agentconnect/awiki-cli/internal/store"
	"github.com/spf13/cobra"
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
		return output.NewExitError("invalid_argument", 2, err.Error(), hint)
	default:
		return output.NewExitError("internal_error", 1, err.Error(), hint)
	}
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
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Listener started", status.Warnings, identityMetaFromResolved(resolved))
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
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Host notify config loaded", nil, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeHostNotifyConfigSet(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.runtimeExit(err, "Run `awiki-cli doctor` to inspect runtime configuration.")
	}
	format := normalizedFormat(runtimeFormat(resolved))
	if !cmd.Flags().Changed("sink") {
		return output.NewExitError("invalid_argument", 2, "host-notify config set requires --sink.", "Use --sink noop|log|file|openclaw.")
	}
	sink, _ := cmd.Flags().GetString("sink")
	switch sink {
	case "noop", "log", "file", "openclaw":
	default:
		return output.NewExitError("invalid_argument", 2, "unsupported host notify sink", "Use --sink noop, log, file, or openclaw.")
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
	view, err := hostNotifyConfigView(resolved)
	if err != nil {
		return a.runtimeExit(err, "Check OpenClaw route registry and host notify configuration.")
	}
	data := map[string]any{
		"host_notify": view,
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Host notify config updated", nil, identityMetaFromResolved(resolved))
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
	view, err := hostNotifyConfigView(resolved)
	if err != nil {
		return a.runtimeExit(err, "Check OpenClaw route registry and host notify configuration.")
	}
	data := map[string]any{
		"host_notify": view,
	}
	summary := "Host notify enabled"
	if !enabled {
		summary = "Host notify disabled"
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, summary, nil, identityMetaFromResolved(resolved))
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
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "OpenClaw host notify config updated", nil, identityMetaFromResolved(resolved))
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
	data := map[string]any{"openclaw": map[string]any{"token_configured": true}}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "OpenClaw token updated", nil, identityMetaFromResolved(resolved))
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
	data := map[string]any{"openclaw": map[string]any{"token_configured": false}}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "OpenClaw token cleared", nil, identityMetaFromResolved(resolved))
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
	}, nil
}

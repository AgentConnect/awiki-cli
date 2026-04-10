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
	"github.com/agentconnect/awiki-cli/internal/store"
	"github.com/spf13/cobra"
)

func runtimeFormat(resolved *appconfig.Resolved) string {
	if resolved == nil || strings.TrimSpace(resolved.OutputFormat) == "" {
		return "json"
	}
	return resolved.OutputFormat
}

func (a *App) runtimeExit(err error, hint string) error {
	if err == nil {
		return nil
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
	db, err := store.Open(resolved.Paths)
	if err != nil {
		return a.runtimeExit(err, "Check write permissions for the local sqlite database.")
	}
	defer db.Close()
	if err := store.EnsureSchema(context.Background(), db); err != nil {
		return a.runtimeExit(err, "Initialize the local sqlite schema before enabling runtime.")
	}
	data := map[string]any{
		"action": "runtime_setup",
		"mode":   mode,
		"paths":  resolved.Paths,
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
	data := map[string]any{
		"action": "runtime_mode_set",
		"mode":   mode,
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
		return a.runtimeExit(err, "Set runtime.mode to websocket before starting the listener.")
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
		return a.runtimeExit(err, "Set runtime.mode to websocket before restarting the listener.")
	}
	data := map[string]any{"listener": status}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Listener restarted", status.Warnings, identityMetaFromResolved(resolved))
}

func (a *App) runRuntimeListenerInstall(cmd *cobra.Command, args []string) error {
	return a.runRuntimeListenerStart(cmd, args)
}

func (a *App) runRuntimeListenerUninstall(cmd *cobra.Command, args []string) error {
	return a.runRuntimeListenerStop(cmd, args)
}

func (a *App) runRuntimeListenerRun(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return err
	}
	return listenerrt.RunForeground(resolved)
}

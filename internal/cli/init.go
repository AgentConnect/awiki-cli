package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/output"
	listenerrt "github.com/agentconnect/awiki-cli/internal/runtime/listener"
	"github.com/agentconnect/awiki-cli/internal/store"
	"github.com/agentconnect/awiki-cli/internal/upgrade"
	"github.com/spf13/cobra"
)

var (
	initApplyRuntimePolicyFunc = listenerrt.ApplyRuntimePolicy
	initUpgradeInspectFunc     = func(ctx context.Context, resolved *appconfig.Resolved) (*upgrade.Inspection, error) {
		return upgrade.Inspect(ctx, resolved, buildinfo.Version)
	}
	initUpgradeIfNeededFunc = func(ctx context.Context, resolved *appconfig.Resolved) error {
		return upgrade.UpgradeIfNeeded(ctx, resolved, buildinfo.Version)
	}
)

// runInit initializes the awiki-cli workspace and an optional minimal config.
//
// It uses the same workspace root resolution as the rest of the CLI:
// AWIKI_CLI_WORKSPACE_HOME_DIR is the only supported override and the default
// fallback remains ~/.awiki-cli.
func (a *App) runInit(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfig()
	if err != nil {
		return a.configCommandExit(err)
	}

	var warnings []string
	if !a.globals.DryRun {
		var migratedLegacy bool
		resolved, migratedLegacy, err = a.maybeUpgradeLegacyBeforeInit(context.Background(), resolved)
		if err != nil {
			return a.configCommandExit(err)
		}
		if migratedLegacy {
			warnings = append(warnings, "Legacy workspace data was migrated before initialization.")
		}
	}

	format := normalizedFormat(resolved.OutputFormat)
	rootSource := resolved.Sources["workspace_home_dir"]
	upgradeDir := filepath.Join(resolved.Paths.WorkspaceHomeDir, "upgrade")

	dirs := []string{
		resolved.Paths.WorkspaceHomeDir,
		filepath.Dir(resolved.Paths.ConfigFile),
		resolved.Paths.DataDir,
		resolved.Paths.StateDir,
		resolved.Paths.CacheDir,
		resolved.Paths.LogsDir,
		resolved.Paths.IdentityDir,
		upgradeDir,
	}

	if a.globals.DryRun {
		data := map[string]any{
			"plan": map[string]any{
				"action":        "init_workspace",
				"root_dir":      resolved.Paths.WorkspaceHomeDir,
				"root_source":   rootSource,
				"directories":   dirs,
				"config_file":   resolved.Paths.ConfigFile,
				"config_exists": resolved.ConfigExists,
				"config_error":  resolved.ConfigError,
			},
		}
		return a.renderSuccess(
			cmd.CommandPath(),
			format,
			a.globals.JQ,
			data,
			"Dry run: workspace initialization planned",
			nil,
			identityMetaFromResolved(resolved),
		)
	}

	if resolved.ConfigError != "" {
		return output.NewExitError(
			"invalid_argument",
			2,
			"config.yaml exists but failed to parse; fix or remove it before running init.",
			"Run `awiki-cli config show` to inspect the parse error, then correct the YAML syntax.",
		)
	}

	for _, dir := range dirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return output.NewExitError(
				"internal_error",
				1,
				err.Error(),
				"Check directory permissions for the awiki-cli workspace.",
			)
		}
	}

	if !resolved.ConfigExists {
		cfg := appconfig.FileConfig{}
		cfg.Identity.Active = resolved.ActiveIdentity
		cfg.Runtime.Mode = resolved.RuntimeMode
		cfg.Runtime.Listener.Enabled = boolPtr(resolved.RuntimeListenerEnabled)
		cfg.Runtime.Listener.AutoInstall = boolPtr(resolved.RuntimeListenerAutoInstall)
		cfg.Runtime.Listener.AutoStart = boolPtr(resolved.RuntimeListenerAutoStart)
		cfg.Output.Format = resolved.OutputFormat
		noColor := resolved.NoColor
		cfg.Output.NoColor = &noColor
		cfg.Services.ServiceBaseURL = resolved.ServiceBaseURL
		cfg.Services.DIDDomain = resolved.DIDDomain
		cfg.Services.ANPServiceEndpoint = resolved.ANPServiceEndpoint
		cfg.Services.ANPServiceDID = resolved.ANPServiceDID
		cfg.Services.CABundle = resolved.CABundle

		if err := appconfig.WriteFileConfig(resolved.Paths.ConfigFile, cfg); err != nil {
			return output.NewExitError(
				"internal_error",
				1,
				err.Error(),
				refineWorkspaceWriteHint(err, "Check write permissions for config.yaml under the awiki-cli workspace."),
			)
		}
		resolved.ConfigExists = true
	}
	db, err := store.Open(resolved.Paths)
	if err != nil {
		return output.NewExitError(
			"internal_error",
			1,
			err.Error(),
			"Check write permissions for the local sqlite database.",
		)
	}
	defer db.Close()
	if err := store.EnsureSchema(context.Background(), db); err != nil {
		return output.NewExitError(
			"internal_error",
			1,
			err.Error(),
			"Initialize the local sqlite schema before enabling runtime.",
		)
	}
	listenerStatus, err := initApplyRuntimePolicyFunc(resolved)
	if err != nil {
		return output.NewExitError(
			"internal_error",
			1,
			err.Error(),
			"Check listener service permissions and runtime.listener settings.",
		)
	}

	result := map[string]any{
		"workspace": map[string]any{
			"root_dir":      resolved.Paths.WorkspaceHomeDir,
			"root_source":   rootSource,
			"paths":         resolved.Paths,
			"config_file":   resolved.Paths.ConfigFile,
			"config_exists": resolved.ConfigExists,
		},
		"listener": listenerStatus,
	}

	return a.renderSuccess(
		cmd.CommandPath(),
		format,
		a.globals.JQ,
		result,
		"Workspace initialized",
		warnings,
		identityMetaFromResolved(resolved),
	)
}

func boolPtr(value bool) *bool {
	result := value
	return &result
}

func (a *App) maybeUpgradeLegacyBeforeInit(ctx context.Context, resolved *appconfig.Resolved) (*appconfig.Resolved, bool, error) {
	if resolved == nil {
		return nil, false, nil
	}
	inspection, err := initUpgradeInspectFunc(ctx, resolved)
	if err != nil {
		return nil, false, err
	}
	detection := inspection.Detection
	if detection.HasWorkspace || !detection.HasLegacy {
		return resolved, false, nil
	}
	if err := initUpgradeIfNeededFunc(ctx, resolved); err != nil {
		return nil, false, err
	}
	refreshed, err := a.resolveConfig()
	if err != nil {
		return nil, false, err
	}
	return refreshed, true, nil
}

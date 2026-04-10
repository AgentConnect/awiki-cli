package cli

import (
	"os"
	"path/filepath"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/output"
	"github.com/spf13/cobra"
)

// runInit initializes the awiki-cli workspace and an optional minimal config.
//
// It uses the same workspace root resolution as the rest of the CLI:
// AWIKI_WORKSPACE_HOME is preferred, AWIKI_HOME is accepted as a root alias,
// and the default fallback remains ~/.awiki-cli.
func (a *App) runInit(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfig()
	if err != nil {
		return output.NewExitError(
			"internal_error",
			1,
			err.Error(),
			"Run `awiki-cli doctor` to inspect configuration and environment.",
		)
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
		cfg.Output.Format = resolved.OutputFormat
		noColor := resolved.NoColor
		cfg.Output.NoColor = &noColor
		cfg.Services.UserServiceURL = resolved.UserServiceURL
		cfg.Services.MessageServiceURL = resolved.MessageServiceURL
		cfg.Services.MessageServiceWSURL = resolved.MessageServiceWSURL
		cfg.Services.DIDDomain = resolved.DIDDomain
		cfg.Services.ANPServiceEndpoint = resolved.ANPServiceEndpoint
		cfg.Services.ANPServiceDID = resolved.ANPServiceDID
		cfg.Services.CABundle = resolved.CABundle

		if err := appconfig.WriteFileConfig(resolved.Paths.ConfigFile, cfg); err != nil {
			return output.NewExitError(
				"internal_error",
				1,
				err.Error(),
				"Check write permissions for config.yaml under the awiki-cli workspace.",
			)
		}
		resolved.ConfigExists = true
	}

	result := map[string]any{
		"workspace": map[string]any{
			"root_dir":      resolved.Paths.WorkspaceHomeDir,
			"root_source":   rootSource,
			"paths":         resolved.Paths,
			"config_file":   resolved.Paths.ConfigFile,
			"config_exists": resolved.ConfigExists,
		},
	}

	return a.renderSuccess(
		cmd.CommandPath(),
		format,
		a.globals.JQ,
		result,
		"Workspace initialized",
		nil,
		identityMetaFromResolved(resolved),
	)
}

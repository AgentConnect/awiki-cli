package cli

import (
	"os"
	"path/filepath"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/output"
	"github.com/spf13/cobra"
)

// runInit initializes the awiki-cli workdir and a minimal config.json.
//
// It is the primary entrypoint for users to discover and prepare the workdir:
// - Without flags, it uses the same root resolution as other commands.
// - With --home, it treats the flag as the desired root for this run and
//   records it in the default root's home.json pointer.
func (a *App) runInit(cmd *cobra.Command, args []string) error {
	homeFlag, _ := cmd.Flags().GetString("home")
	homeFlag = strings.TrimSpace(homeFlag)

	home, err := os.UserHomeDir()
	if err != nil {
		return output.NewExitError(
			"internal_error",
			1,
			err.Error(),
			"Resolve a valid user home directory before running awiki-cli.",
		)
	}

	// If --home is provided, treat it as the canonical workdir root for this
	// init invocation by temporarily overriding AWIKI_HOME. This makes the
	// root resolution behavior consistent with other commands.
	var previousAWIKIHome string
	if homeFlag != "" {
		expanded := appconfig.ExpandHome(home, homeFlag)
		previousAWIKIHome = os.Getenv("AWIKI_HOME")
		if err := os.Setenv("AWIKI_HOME", expanded); err != nil {
			return output.NewExitError(
				"internal_error",
				1,
				err.Error(),
				"Set AWIKI_HOME for this process before running init.",
			)
		}
		defer os.Setenv("AWIKI_HOME", previousAWIKIHome)
	}

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
	defaultRoot := appconfig.DefaultRootDir(home)
	pointerPath := filepath.Join(defaultRoot, "home.json")
	rootSource := resolved.Sources["root_dir"]

	dirs := []string{
		resolved.Paths.RootDir,
		filepath.Dir(resolved.Paths.ConfigFile),
		resolved.Paths.DataDir,
		resolved.Paths.StateDir,
		resolved.Paths.CacheDir,
		resolved.Paths.IdentityDir,
		resolved.Paths.LogsDir,
	}

	willWritePointer := homeFlag != ""

	if a.globals.DryRun {
		data := map[string]any{
			"plan": map[string]any{
				"action":        "init_workdir",
				"root_dir":      resolved.Paths.RootDir,
				"root_source":   rootSource,
				"directories":   dirs,
				"config_file":   resolved.Paths.ConfigFile,
				"config_exists": resolved.ConfigExists,
				"config_error":  resolved.ConfigError,
				"home_pointer": map[string]any{
					"path":       pointerPath,
					"will_write": willWritePointer,
				},
			},
		}
		return a.renderSuccess(
			cmd.CommandPath(),
			format,
			a.globals.JQ,
			data,
			"Dry run: workdir initialization planned",
			nil,
			identityMetaFromResolved(resolved),
		)
	}

	// Avoid silently overwriting a broken config file – require the user to
	// fix or remove it first.
	if resolved.ConfigError != "" {
		return output.NewExitError(
			"invalid_argument",
			2,
			"config.json exists but failed to parse; fix or remove it before running init.",
			"Run `awiki-cli config show` to inspect the parse error, then correct the JSON syntax.",
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
				"Check directory permissions for the awiki-cli work directory.",
			)
		}
	}

	// If there is no config.json yet, create a minimal one based on the
	// currently resolved defaults so future reads remain consistent.
	if !resolved.ConfigExists {
		cfg := appconfig.FileConfig{}
		cfg.Services.Domain = resolved.DIDDomain
		cfg.Identity.Active = resolved.ActiveIdentity
		cfg.Runtime.Mode = resolved.RuntimeMode
		cfg.Output.Format = resolved.OutputFormat
		noColor := resolved.NoColor
		cfg.Output.NoColor = &noColor
		cfg.Update.DisableStrictVersion = resolved.UpdateDisableStrictVersion
		cfg.Update.MetadataCacheTTLSeconds = resolved.UpdateMetadataCacheTTLSeconds

		if err := appconfig.WriteFileConfig(resolved.Paths.ConfigFile, cfg); err != nil {
			return output.NewExitError(
				"internal_error",
				1,
				err.Error(),
				"Check write permissions for config.json under the awiki-cli work directory.",
			)
		}
		resolved.ConfigExists = true
	}

	if willWritePointer {
		if err := appconfig.WriteHomePointer(home, resolved.Paths.RootDir); err != nil {
			return output.NewExitError(
				"internal_error",
				1,
				err.Error(),
				"Check write permissions under the default awiki-cli home directory.",
			)
		}
	}

	result := map[string]any{
		"workdir": map[string]any{
			"root_dir":      resolved.Paths.RootDir,
			"root_source":   rootSource,
			"paths":         resolved.Paths,
			"config_file":   resolved.Paths.ConfigFile,
			"config_exists": resolved.ConfigExists,
		},
		"home_pointer": map[string]any{
			"path":    pointerPath,
			"written": willWritePointer,
		},
	}

	return a.renderSuccess(
		cmd.CommandPath(),
		format,
		a.globals.JQ,
		result,
		"Workdir initialized",
		nil,
		identityMetaFromResolved(resolved),
	)
}

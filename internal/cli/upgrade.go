package cli

import (
	"fmt"

	"github.com/agentconnect/awiki-cli/internal/output"
	"github.com/agentconnect/awiki-cli/internal/update"
	"github.com/spf13/cobra"
)

func (a *App) runUpgrade(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfig()
	if err != nil {
		return output.NewExitError("internal_error", 1, err.Error(), "Check your local configuration and environment variables.")
	}

	decision, err := update.Check(resolved)
	if err != nil {
		return output.NewExitError(
			"internal_error",
			1,
			err.Error(),
			"Failed to check npm metadata for awiki-cli. Please ensure you have network access and try again.",
		)
	}

	format := normalizedFormat(resolved.OutputFormat)

	data := map[string]any{
		"current_version":       decision.CurrentVersion,
		"latest_version":        decision.LatestVersion,
		"min_supported_version": decision.MinSupportedVersion,
		"strict_disabled":       decision.StrictDisabled,
		"dev_build":             decision.DevBuild,
		"has_newer_version":     decision.HasNewerVersion,
		"blocked":               decision.Blocked,
		"upgrade_hint":          "To upgrade awiki-cli, run: npm install -g @agentconnect/awiki-cli@latest",
	}

	summary := "awiki-cli is up to date"
	var warnings []string

	if decision.Blocked {
		summary = "awiki-cli version is below the minimum supported version"
		warnings = append(warnings, fmt.Sprintf(
			"awiki-cli %s is below the minimum supported version %s. Upgrading is required before using remote APIs.",
			decision.CurrentVersion,
			decision.MinSupportedVersion,
		))
	} else if decision.HasNewerVersion {
		if decision.LatestVersion != "" {
			summary = fmt.Sprintf("A newer awiki-cli version (%s) is available", decision.LatestVersion)
		} else {
			summary = "A newer awiki-cli version may be available"
		}
		warnings = append(warnings, "Upgrading is recommended to stay on a supported version.")
	}

	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, summary, warnings, identityMetaFromResolved(resolved))
}

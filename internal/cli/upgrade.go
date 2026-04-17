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
			"Failed to check awiki update metadata. Please ensure the update service is reachable and try again.",
		)
	}

	format := normalizedFormat(resolved.OutputFormat)
	data := map[string]any{
		"current_version":       decision.CurrentVersion,
		"latest_version":        decision.LatestVersion,
		"min_supported_version": decision.MinSupportedVersion,
		"channel":               decision.Channel,
		"published_at":          decision.PublishedAt,
		"source":                decision.Source,
		"strict_disabled":       decision.StrictDisabled,
		"dev_build":             decision.DevBuild,
		"has_newer_version":     decision.HasNewerVersion,
		"blocked":               decision.Blocked,
		"artifact_available":    decision.ArtifactAvailable,
		"artifact_url":          decision.ArtifactURL,
		"artifact_sha256":       decision.ArtifactSHA256,
		"skill_bundle_version":  decision.SkillBundleVersion,
		"skill_bundle_sha256":   decision.SkillBundleSHA256,
		"root_skill_sha256":     decision.RootSkillSHA256,
		"upgrade_hint":          "Inspect the current update status now. Automatic binary installation will be implemented in a later phase.",
		"upgrade_attempted":     false,
	}

	summary := "awiki-cli is up to date"
	var warnings []string
	if decision.Blocked {
		summary = "awiki-cli version is below the minimum supported version"
		warnings = append(warnings, fmt.Sprintf(
			"awiki-cli %s is below the minimum supported version %s. An upgrade is required before using remote APIs.",
			decision.CurrentVersion,
			decision.MinSupportedVersion,
		))
	} else if decision.HasNewerVersion {
		if decision.LatestVersion != "" {
			summary = fmt.Sprintf("A newer awiki-cli version (%s) is available", decision.LatestVersion)
		} else {
			summary = "A newer awiki-cli version may be available"
		}
		warnings = append(warnings, "Upgrade execution is not enabled in this phase. Use your current bootstrap install flow if you need to reinstall awiki-cli.")
	}

	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, summary, warnings, identityMetaFromResolved(resolved))
}

package cli

import (
	"fmt"
	"os"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	"github.com/agentconnect/awiki-cli/internal/output"
	"github.com/agentconnect/awiki-cli/internal/update"
	"github.com/agentconnect/awiki-cli/internal/upgrader"
	"github.com/spf13/cobra"
)

func (a *App) runUpgrade(cmd *cobra.Command, args []string) error {
	return a.runUpgradeCheck(cmd, args)
}

func (a *App) runUpgradeCheck(cmd *cobra.Command, args []string) error {
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
	manager, err := upgrader.NewManager(resolved)
	if err != nil {
		return output.NewExitError("internal_error", 1, err.Error(), "Failed to resolve self update workspace paths.")
	}
	var state *upgrader.ReleaseState
	warnings := make([]string, 0)
	if !a.globals.DryRun {
		state, err = manager.RecordCheck(decision)
		if err != nil {
			return output.NewExitError("internal_error", 1, err.Error(), "Failed to record local self update state.")
		}
	} else {
		warnings = append(warnings, "Dry run: release-state.json was not updated.")
	}
	data := upgradeDecisionData(decision)
	data["release_state"] = state
	data["paths"] = manager.Paths()
	format := normalizedFormat(resolved.OutputFormat)
	summary := "awiki-cli is up to date"
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
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, summary, warnings, identityMetaFromResolved(resolved))
}

func (a *App) runUpgradeStatus(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfig()
	if err != nil {
		return output.NewExitError("internal_error", 1, err.Error(), "Check your local configuration and environment variables.")
	}
	manager, err := upgrader.NewManager(resolved)
	if err != nil {
		return output.NewExitError("internal_error", 1, err.Error(), "Failed to resolve self update workspace paths.")
	}
	state, err := manager.LoadReleaseState()
	if err != nil {
		return output.NewExitError("internal_error", 1, err.Error(), "Failed to read local self update state.")
	}
	paths := manager.Paths()
	data := map[string]any{
		"current_binary_version":  buildinfo.Current(),
		"current_executable_path": currentExecutablePath(),
		"managed_binary_path":     paths.CurrentBinaryPath,
		"managed_binary_target":   upgrader.ResolveCurrentTarget(paths.CurrentBinaryPath),
		"release_state":           state,
		"paths":                   paths,
	}
	format := normalizedFormat(resolved.OutputFormat)
	summary := "No local self update state has been recorded yet"
	if state != nil {
		summary = "Loaded local self update state"
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, summary, nil, identityMetaFromResolved(resolved))
}

func (a *App) runUpgradeApply(cmd *cobra.Command, args []string) error {
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
	manager, err := upgrader.NewManager(resolved)
	if err != nil {
		return output.NewExitError("internal_error", 1, err.Error(), "Failed to resolve self update workspace paths.")
	}
	format := normalizedFormat(resolved.OutputFormat)
	if a.globals.DryRun {
		data := upgradeDecisionData(decision)
		data["paths"] = manager.Paths()
		data["plan"] = []string{
			"download artifact to staging",
			"verify sha256",
			"extract archive",
			"install binary to versions/<version>/",
			"switch managed current binary",
			"write release-state.json",
		}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: self update apply plan", nil, identityMetaFromResolved(resolved))
	}
	if _, err := manager.RecordCheck(decision); err != nil {
		return output.NewExitError("internal_error", 1, err.Error(), "Failed to record local self update state.")
	}
	result, err := manager.Apply(cmd.Context(), decision)
	if err != nil {
		return output.NewExitError("upgrade_failed", 1, err.Error(), "Inspect `awiki-cli upgrade status` for the last failed stage and retry when the artifact is available.")
	}
	data := upgradeDecisionData(decision)
	data["apply_result"] = result
	data["paths"] = manager.Paths()
	warnings := []string{
		"The current process may still be running the previous binary until you start a new awiki-cli process.",
	}
	summary := fmt.Sprintf("Installed awiki-cli %s into the managed versions directory", result.InstalledVersion)
	if result.AlreadyCurrent {
		summary = fmt.Sprintf("awiki-cli %s is already the active managed binary", result.InstalledVersion)
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, summary, warnings, identityMetaFromResolved(resolved))
}

func upgradeDecisionData(decision update.Decision) map[string]any {
	return map[string]any{
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
	}
}

func currentExecutablePath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	return path
}

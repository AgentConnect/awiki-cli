package cli

import (
	"fmt"
	"os"
	"os/exec"

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
		"upgrade_hint":          "To upgrade awiki-cli, run: npm install -g @awiki/cli@latest",
	}

	upgradeAttempted := false

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

	// 自动执行 npm 全局升级：在存在新版本或已低于最小支持版本时触发。
	// 若开启了全局 --dry-run，则只报告状态而不执行实际升级。
	if !a.globals.DryRun && (decision.Blocked || decision.HasNewerVersion) {
		upgradeAttempted = true
		if err := runNpmGlobalInstall(cmd); err != nil {
			hint := "Ensure npm is installed, your PATH is configured, and you have permission to install global packages, then retry `awiki-cli upgrade`."
			return output.NewExitError("upgrade_failed", 1, err.Error(), hint)
		}
		warnings = append(warnings, "Attempted to upgrade via `npm install -g @awiki/cli@latest`. Open a new shell and run `awiki-cli version` to verify.")
	}

	data["upgrade_attempted"] = upgradeAttempted

	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, summary, warnings, identityMetaFromResolved(resolved))
}

// runNpmGlobalInstall runs `npm install -g @awiki/cli@latest` in the current environment,
// streaming stdout/stderr directly to the caller's terminal.
func runNpmGlobalInstall(cmd *cobra.Command) error {
	npmCmd := exec.CommandContext(cmd.Context(), "npm", "install", "-g", "@awiki/cli@latest")
	npmCmd.Stdout = os.Stdout
	npmCmd.Stderr = os.Stderr
	if err := npmCmd.Run(); err != nil {
		return fmt.Errorf("failed to run `npm install -g @awiki/cli@latest`: %w", err)
	}
	return nil
}

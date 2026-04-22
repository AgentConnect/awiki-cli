package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/output"
	"github.com/agentconnect/awiki-cli/internal/traceutil"
	"github.com/agentconnect/awiki-cli/internal/update"
	"github.com/spf13/cobra"
)

const npmMirrorRegistryURL = "https://registry.npmmirror.com"

type npmInstallAttempt struct {
	args []string
}

var runNpmInstallAttempt = func(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	npmCmd := exec.CommandContext(ctx, "npm", args...)
	npmCmd.Stdout = stdout
	npmCmd.Stderr = stderr
	return npmCmd.Run()
}

func (a *App) runUpgrade(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfig()
	if err != nil {
		return output.NewExitError("internal_error", 1, err.Error(), "Check your local configuration and environment variables.")
	}

	decision, checkErr := update.CheckFresh(cmd.Context(), resolved)

	format := normalizedFormat(resolved.OutputFormat)

	data, summary, warnings := buildUpgradeStatus(decision, checkErr)
	upgradeAttempted := false

	// 自动执行 npm 全局升级：在存在新版本或已低于最小支持版本时触发。
	// 若开启了全局 --dry-run，则只报告状态而不执行实际升级。
	if checkErr == nil && !a.globals.DryRun && (decision.Blocked || decision.HasNewerVersion) {
		upgradeAttempted = true
		if err := runNpmGlobalInstall(cmd); err != nil {
			hint := "Ensure npm is installed, your PATH is configured, you have permission to install global packages, and you can reach registry.npmjs.org or registry.npmmirror.com, then retry `awiki-cli upgrade`."
			return output.NewExitError("upgrade_failed", 1, err.Error(), hint)
		}
		warnings = append(warnings, "Attempted to upgrade via npm with registry.npmjs.org first and registry.npmmirror.com fallback. Open a new shell and run `awiki-cli version` to verify.")
	}

	data["upgrade_attempted"] = upgradeAttempted

	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, summary, warnings, identityMetaFromResolved(resolved))
}

func buildUpgradeStatus(decision update.Decision, checkErr error) (map[string]any, string, []string) {
	data := map[string]any{
		"current_version":       decision.CurrentVersion,
		"latest_version":        decision.LatestVersion,
		"min_supported_version": decision.MinSupportedVersion,
		"strict_disabled":       decision.StrictDisabled,
		"dev_build":             decision.DevBuild,
		"has_newer_version":     decision.HasNewerVersion,
		"blocked":               decision.Blocked,
		"upgrade_hint":          directUpgradeHint(),
	}
	if decision.MetadataSource != "" {
		data["update_metadata_source"] = decision.MetadataSource
	}

	if checkErr != nil {
		data["update_check_status"] = "unavailable"
		data["update_check_error"] = checkErr.Error()
		return data,
			"Unable to check for awiki-cli updates",
			[]string{
				fmt.Sprintf("Failed to fetch npm metadata for awiki-cli: %v", checkErr),
				"Showing local version status only. Retry `awiki-cli upgrade` when network access is available.",
			}
	}

	data["update_check_status"] = "ok"
	if decision.MetadataSource == "cache_stale" {
		data["update_check_status"] = "stale_cache"
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
	if decision.MetadataSource == "cache_stale" {
		warnings = append(warnings, "Remote npm registries were unavailable; showing cached update metadata instead.")
	}

	return data, summary, warnings
}

func npmGlobalInstallAttempts() []npmInstallAttempt {
	return []npmInstallAttempt{
		{
			args: []string{"install", "-g", "@awiki/cli@latest"},
		},
		{
			args: []string{"install", "-g", "@awiki/cli@latest", "--registry=" + npmMirrorRegistryURL},
		},
	}
}

func formatNpmInstallCommand(args []string) string {
	return "npm " + strings.Join(args, " ")
}

func directNpmInstallCommand() string {
	return formatNpmInstallCommand(npmGlobalInstallAttempts()[0].args)
}

func mirrorNpmInstallCommand() string {
	return formatNpmInstallCommand(npmGlobalInstallAttempts()[1].args)
}

func directUpgradeHint() string {
	return fmt.Sprintf(
		"To upgrade awiki-cli, run: %s. If registry.npmjs.org is unreachable, retry with: %s",
		directNpmInstallCommand(),
		mirrorNpmInstallCommand(),
	)
}

// runNpmGlobalInstall runs `npm install -g @awiki/cli@latest` in the current environment,
// trying registry.npmjs.org first and falling back to registry.npmmirror.com.
func runNpmGlobalInstall(cmd *cobra.Command) error {
	var stdout io.Writer = os.Stdout
	var stderr io.Writer = os.Stderr
	ctx := context.Background()
	if cmd != nil {
		if cmdCtx := cmd.Context(); cmdCtx != nil {
			ctx = cmdCtx
		}
		stdout = cmd.OutOrStdout()
		stderr = cmd.ErrOrStderr()
	}

	finish := traceutil.PhaseContext(ctx, "npm_upgrade_install")
	defer finish()

	var errs []string
	for i, attempt := range npmGlobalInstallAttempts() {
		if i > 0 {
			traceutil.MarkFallback(ctx, "npm_upgrade_install:registry.npmjs.org", errsToError(errs))
			fmt.Fprintf(stderr, "[awiki-cli] npm install via registry.npmjs.org failed; retrying with %s\n", npmMirrorRegistryURL)
		}

		err := runNpmInstallAttempt(ctx, stdout, stderr, attempt.args)
		if err == nil {
			return nil
		}
		errs = append(errs, fmt.Sprintf("%s: %v", formatNpmInstallCommand(attempt.args), err))
	}

	return fmt.Errorf("failed to upgrade awiki-cli via npm registries: %s", strings.Join(errs, "; "))
}

func errsToError(errs []string) error {
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf(strings.Join(errs, "; "))
}

package cli

import (
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/output"
	"github.com/spf13/cobra"
)

func (a *App) runConfigSet(cmd *cobra.Command, args []string) error {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return a.configCommandExit(err)
	}
	format := normalizedFormat(resolved.OutputFormat)
	if !cmd.Flags().Changed("did-domain") {
		return output.NewExitError("invalid_argument", 2, "config set requires --did-domain.", "Use --did-domain <domain>.")
	}
	rawDidDomain, _ := cmd.Flags().GetString("did-domain")
	didDomain, err := appconfig.NormalizeDIDDomain(rawDidDomain)
	if err != nil {
		return output.NewExitError("invalid_argument", 2, err.Error(), "Use a bare domain such as tenant.example.")
	}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":      "config_set_did_domain",
			"did_domain":  didDomain,
			"config_file": resolved.Paths.ConfigFile,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: DID domain update planned", nil, identityMetaFromResolved(resolved))
	}
	if err := appconfig.UpdateDIDDomain(resolved.Paths, didDomain); err != nil {
		return a.configCommandExit(err)
	}
	resolved, err = a.resolveConfigForWorkspace()
	if err != nil {
		return a.configCommandExit(err)
	}
	format = normalizedFormat(resolved.OutputFormat)
	data := map[string]any{
		"did_domain":  resolved.DIDDomain,
		"config_file": resolved.Paths.ConfigFile,
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "DID domain updated", nil, identityMetaFromResolved(resolved))
}

package cli

import (
	"fmt"
	"time"

	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/message"
	"github.com/spf13/cobra"
)

func (a *App) runGroupE2EEStatus(cmd *cobra.Command, args []string) error {
	group, _ := cmd.Flags().GetString("group")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	provider := message.NewDefaultMLSExecProvider(service.Config())
	plan := map[string]any{
		"action":               "group.e2ee.status",
		"identity":             a.globals.Identity,
		"runtime_mode":         service.Config().RuntimeMode,
		"profile":              message.GroupE2EEProfile,
		"security_profile":     message.GroupE2EESecurityProfile,
		"artifact_mode":        message.GroupE2EEContractArtifactMode,
		"provider":             "exec",
		"binary":               provider.BinaryPath,
		"mls_data_dir":         provider.DataDir,
		"group":                group,
		"discovery_advertised": false,
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": plan}, "Dry run: group e2ee status planned", nil, a.identityMeta())
	}
	agentDID, identityErr := activeIdentityDID(service, a.globals.Identity)
	warnings := []string(nil)
	if identityErr != nil {
		warnings = append(warnings, fmt.Sprintf("Active identity DID unavailable: %v", identityErr))
	}
	provider.Timeout = 5 * time.Second
	resp, callErr := provider.Call(cmd.Context(), "group", "status", message.MLSRequest{
		APIVersion:          "anp-mls/v1",
		RequestID:           fmt.Sprintf("group-e2ee-status-%d", time.Now().UnixNano()),
		AgentDID:            agentDID,
		ContractTestEnabled: true,
		Params:              map[string]any{"group_did": group},
	})
	data := map[string]any{"plan": plan, "available": callErr == nil}
	if callErr != nil {
		warnings = append(warnings, fmt.Sprintf("anp-mls exec provider unavailable: %v", callErr))
	} else if resp != nil {
		data["mls"] = resp.Result
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Group E2EE contract-test status inspected", warnings, a.identityMeta())
}

func (a *App) runGroupE2EEPublishKeyPackage(cmd *cobra.Command, args []string) error {
	device, _ := cmd.Flags().GetString("device")
	contractTest, _ := cmd.Flags().GetBool("contract-test")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	provider := message.NewDefaultMLSExecProvider(service.Config())
	plan := map[string]any{
		"action":             "group.e2ee.publish_key_package",
		"identity":           a.globals.Identity,
		"runtime_mode":       service.Config().RuntimeMode,
		"provider":           "exec",
		"binary":             provider.BinaryPath,
		"mls_data_dir":       provider.DataDir,
		"device":             device,
		"contract_test_only": contractTest,
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": plan}, "Dry run: group e2ee key package publish planned", nil, a.identityMeta())
	}
	ownerDID, identityErr := activeIdentityDID(service, a.globals.Identity)
	if identityErr != nil {
		return a.messageExit(identityErr, "Create or select an identity before generating a group E2EE KeyPackage.")
	}
	resp, callErr := provider.Call(cmd.Context(), "key-package", "generate", message.MLSRequest{
		APIVersion:          "anp-mls/v1",
		RequestID:           fmt.Sprintf("group-e2ee-key-package-%d", time.Now().UnixNano()),
		AgentDID:            ownerDID,
		DeviceID:            device,
		ContractTestEnabled: contractTest,
		Params: map[string]any{
			"owner_did": ownerDID,
			"device_id": device,
		},
	})
	if callErr != nil {
		return a.messageExit(callErr, "Install anp-mls or rerun with --dry-run while group E2EE remains contract-test only.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": plan, "mls": resp.Result}, "Generated group E2EE contract-test KeyPackage", nil, a.identityMeta())
}

func (a *App) runGroupE2EEPending(cmd *cobra.Command, args []string) error {
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	provider := message.NewDefaultMLSExecProvider(service.Config())
	data := map[string]any{
		"plan": map[string]any{
			"action":       "group.e2ee.pending",
			"identity":     a.globals.Identity,
			"runtime_mode": service.Config().RuntimeMode,
			"provider":     "exec",
			"mls_data_dir": provider.DataDir,
		},
		"pending": []any{},
		"note":    "contract-test skeleton only; real OpenMLS pending queue will be added with MLS state integration",
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Group E2EE pending queue inspected", nil, a.identityMeta())
}

func (a *App) runGroupE2EERepair(cmd *cobra.Command, args []string) error {
	group, _ := cmd.Flags().GetString("group")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	provider := message.NewDefaultMLSExecProvider(service.Config())
	plan := map[string]any{
		"action":       "group.e2ee.repair",
		"identity":     a.globals.Identity,
		"runtime_mode": service.Config().RuntimeMode,
		"provider":     "exec",
		"mls_data_dir": provider.DataDir,
		"group":        group,
		"scope":        "replay pending notices and verify local MLS DB summary",
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": plan}, "Group E2EE repair planned", nil, a.identityMeta())
}

func activeIdentityDID(service *message.Service, name string) (string, error) {
	record, err := identity.NewManager(service.Config().Paths).Load(name)
	if err != nil {
		return "", err
	}
	return record.DID, nil
}

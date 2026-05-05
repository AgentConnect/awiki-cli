package cli

import (
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
		"provider":             "exec",
		"binary":               provider.BinaryPath,
		"mls_data_dir":         provider.DataDir,
		"group":                group,
		"discovery_advertised": false,
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": plan}, "Dry run: group e2ee status planned", nil, a.identityMeta())
	}
	result, statusErr := service.InspectGroupE2EEStatus(cmd.Context(), a.globals.Identity, group, 50)
	if statusErr != nil {
		return a.messageExit(statusErr, "Install anp-mls, set AWIKI_ANP_MLS_BINARY, and ensure message-service group E2EE APIs are enabled for focused validation.")
	}
	result.Data["plan"] = plan
	return a.renderMessageResult(cmd, format, result)
}

func (a *App) runGroupE2EEPublishKeyPackage(cmd *cobra.Command, args []string) error {
	device, _ := cmd.Flags().GetString("device")
	group, _ := cmd.Flags().GetString("group")
	recovery, _ := cmd.Flags().GetBool("recovery")
	purpose, _ := cmd.Flags().GetString("purpose")
	contractTest, _ := cmd.Flags().GetBool("contract-test")
	if recovery {
		purpose = "recovery"
	}
	if purpose == "" {
		purpose = "normal"
	}
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
		"group":              group,
		"recovery":           purpose == "recovery",
		"purpose":            purpose,
		"contract_test_only": contractTest,
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": plan}, "Dry run: group e2ee key package publish planned", nil, a.identityMeta())
	}
	result, publishErr := service.PublishGroupE2EEKeyPackageWithPurpose(cmd.Context(), a.globals.Identity, device, group, purpose, contractTest)
	if publishErr != nil {
		return a.messageExit(publishErr, "Install anp-mls, set AWIKI_ANP_MLS_BINARY, pass --group when --recovery is used, and ensure message-service group E2EE APIs are enabled.")
	}
	result.Data["plan"] = plan
	return a.renderMessageResult(cmd, format, result)
}

func (a *App) runGroupE2EEPending(cmd *cobra.Command, args []string) error {
	group, _ := cmd.Flags().GetString("group")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	provider := message.NewDefaultMLSExecProvider(service.Config())
	plan := map[string]any{
		"action":       "group.e2ee.pending",
		"identity":     a.globals.Identity,
		"runtime_mode": service.Config().RuntimeMode,
		"provider":     "exec",
		"mls_data_dir": provider.DataDir,
		"group":        group,
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": plan}, "Dry run: group e2ee pending planned", nil, a.identityMeta())
	}
	result, pendingErr := service.PullGroupE2EENotices(cmd.Context(), a.globals.Identity, group, 50)
	if pendingErr != nil {
		return a.messageExit(pendingErr, "Ensure message-service group E2EE test flag is enabled for focused validation; discovery remains hidden by default.")
	}
	result.Data["plan"] = plan
	return a.renderMessageResult(cmd, format, result)
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
		"scope":        "compare local MLS status to service head, safely finalize accepted pending commits, replay welcome/commit notices, and fail closed on unrecoverable gaps",
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": plan}, "Dry run: group e2ee repair planned", nil, a.identityMeta())
	}
	result, repairErr := service.RepairGroupE2EENotices(cmd.Context(), a.globals.Identity, group, 50)
	if repairErr != nil {
		return a.messageExit(repairErr, "Install anp-mls, set AWIKI_ANP_MLS_BINARY, and ensure message-service group E2EE APIs are enabled for focused validation.")
	}
	result.Data["plan"] = plan
	return a.renderMessageResult(cmd, format, result)
}

func (a *App) runGroupE2EEProcessLeaveRequest(cmd *cobra.Command, args []string) error {
	group, _ := cmd.Flags().GetString("group")
	member, _ := cmd.Flags().GetString("member")
	leaveRequestID, _ := cmd.Flags().GetString("leave-request-id")
	reason, _ := cmd.Flags().GetString("reason")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	provider := message.NewDefaultMLSExecProvider(service.Config())
	request := message.GroupE2EEProcessLeaveRequest{
		IdentityName:   a.globals.Identity,
		Group:          group,
		Member:         member,
		LeaveRequestID: leaveRequestID,
		ReasonText:     reason,
	}
	plan := map[string]any{
		"action":           "group.e2ee.process_leave_request",
		"identity":         a.globals.Identity,
		"runtime_mode":     service.Config().RuntimeMode,
		"provider":         "exec",
		"mls_data_dir":     provider.DataDir,
		"group":            group,
		"member":           member,
		"leave_request_id": leaveRequestID,
		"request":          request,
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": plan}, "Dry run: group e2ee leave request process planned", nil, a.identityMeta())
	}
	result, processErr := service.ProcessGroupE2EELeaveRequest(cmd.Context(), request)
	if processErr != nil {
		return a.messageExit(processErr, "Ensure the leave request exists, the active identity can remove members, and anp-mls/message-service group E2EE APIs are enabled.")
	}
	result.Data["plan"] = plan
	return a.renderMessageResult(cmd, format, result)
}

func (a *App) runGroupE2EERecoverMember(cmd *cobra.Command, args []string) error {
	group, _ := cmd.Flags().GetString("group")
	member, _ := cmd.Flags().GetString("member")
	device, _ := cmd.Flags().GetString("device")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	provider := message.NewDefaultMLSExecProvider(service.Config())
	request := message.GroupE2EERecoverMemberRequest{
		IdentityName: a.globals.Identity,
		Group:        group,
		Member:       member,
		DeviceID:     device,
	}
	plan := map[string]any{
		"action":               "group.e2ee.recover_member",
		"identity":             a.globals.Identity,
		"runtime_mode":         service.Config().RuntimeMode,
		"provider":             "exec",
		"mls_data_dir":         provider.DataDir,
		"group":                group,
		"member":               member,
		"device":               device,
		"p4_membership_mutate": false,
		"orchestration":        []string{"lease recovery KeyPackage", "anp-mls recover-member-prepare", "hidden group.e2ee.recover_member", "finalize on accept", "abort on deterministic rejection"},
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": plan}, "Dry run: group e2ee recover-member planned", nil, a.identityMeta())
	}
	result, recoverErr := service.RecoverGroupE2EEMember(cmd.Context(), request)
	if recoverErr != nil {
		return a.messageExit(recoverErr, "Ensure the target remains an active P4 member, has published a --recovery --group KeyPackage, and anp-mls/message-service PR-B3 APIs are enabled.")
	}
	result.Data["plan"] = plan
	return a.renderMessageResult(cmd, format, result)
}

func (a *App) runGroupE2EEUpdateKey(cmd *cobra.Command, args []string) error {
	group, _ := cmd.Flags().GetString("group")
	member, _ := cmd.Flags().GetString("member")
	device, _ := cmd.Flags().GetString("device")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	provider := message.NewDefaultMLSExecProvider(service.Config())
	request := message.GroupE2EEUpdateKeyRequest{
		IdentityName: a.globals.Identity,
		Group:        group,
		Member:       member,
		DeviceID:     device,
	}
	plan := map[string]any{
		"action":                 "group.e2ee.update_key",
		"identity":               a.globals.Identity,
		"runtime_mode":           service.Config().RuntimeMode,
		"provider":               "exec",
		"mls_data_dir":           provider.DataDir,
		"group":                  group,
		"member":                 member,
		"device":                 device,
		"key_package_purpose":    "update",
		"hidden_awiki_extension": true,
		"p4_membership_mutate":   false,
		"orchestration":          []string{"lease purpose=update KeyPackage", "anp-mls update-member-prepare", "hidden group.e2ee.update", "finalize on accept", "abort on deterministic rejection"},
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": plan}, "Dry run: group e2ee update-key planned", nil, a.identityMeta())
	}
	result, updateErr := service.UpdateGroupE2EEKey(cmd.Context(), request)
	if updateErr != nil {
		return a.messageExit(updateErr, "Ensure the target is an active group member, has published `group e2ee publish-key-package --purpose update --group <group>`, and message-service/anp-mls update APIs are enabled.")
	}
	result.Data["plan"] = plan
	return a.renderMessageResult(cmd, format, result)
}

func (a *App) runGroupE2EERejoin(cmd *cobra.Command, args []string) error {
	group, _ := cmd.Flags().GetString("group")
	member, _ := cmd.Flags().GetString("member")
	role, _ := cmd.Flags().GetString("role")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := message.GroupMemberRequest{IdentityName: a.globals.Identity, Group: group, Member: member, Role: role, E2EE: true}
	plan := map[string]any{
		"action":               "group.e2ee.rejoin",
		"canonical_command":    "group add --e2ee",
		"identity":             a.globals.Identity,
		"runtime_mode":         service.Config().RuntimeMode,
		"group":                group,
		"member":               member,
		"role":                 role,
		"key_package_purpose":  "normal",
		"recovery_command":     "group e2ee recover-member is only for active-member crypto recovery, not removed/left rejoin",
		"external_commit":      false,
		"p4_membership_mutate": true,
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": plan}, "Dry run: group e2ee rejoin planned", nil, a.identityMeta())
	}
	result, rejoinErr := service.AddGroupMember(cmd.Context(), request)
	if rejoinErr != nil {
		return a.messageExit(rejoinErr, "Removed/left rejoin requires a fresh normal KeyPackage published after removal/leave, then owner-only `group add --e2ee`; do not use recover-member for removed/left members.")
	}
	result.Data["plan"] = plan
	return a.renderMessageResult(cmd, format, result)
}

func activeIdentityDID(service *message.Service, name string) (string, error) {
	record, err := identity.NewManager(service.Config().Paths).Load(name)
	if err != nil {
		return "", err
	}
	return record.DID, nil
}

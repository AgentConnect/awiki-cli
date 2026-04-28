package cli

import (
	"strconv"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/message"
	"github.com/spf13/cobra"
)

func (a *App) runGroupCreate(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	description, _ := cmd.Flags().GetString("description")
	discoverability, _ := cmd.Flags().GetString("discoverability")
	admissionMode, _ := cmd.Flags().GetString("admission-mode")
	slug, _ := cmd.Flags().GetString("slug")
	goal, _ := cmd.Flags().GetString("goal")
	rules, _ := cmd.Flags().GetString("rules")
	messagePrompt, _ := cmd.Flags().GetString("message-prompt")
	docURL, _ := cmd.Flags().GetString("doc-url")
	maxMembers, _ := cmd.Flags().GetString("max-members")
	attachmentsAllowed := boolFlagPtr(cmd, "attachments-allowed")
	memberMaxMessages := int64FlagPtr(cmd, "member-max-messages")
	memberMaxTotalChars := int64FlagPtr(cmd, "member-max-total-chars")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := message.GroupCreateRequest{
		IdentityName:        a.globals.Identity,
		Name:                name,
		Description:         description,
		Discoverability:     discoverability,
		AdmissionMode:       admissionMode,
		Slug:                slug,
		Goal:                goal,
		Rules:               rules,
		MessagePrompt:       messagePrompt,
		DocURL:              docURL,
		AttachmentsAllowed:  attachmentsAllowed,
		MaxMembers:          maxMembers,
		MemberMaxMessages:   memberMaxMessages,
		MemberMaxTotalChars: memberMaxTotalChars,
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{"action": "group.create", "identity": a.globals.Identity, "runtime_mode": service.Config().RuntimeMode, "request": request}}, "Dry run: group create planned", nil, a.identityMeta())
	}
	result, err := service.CreateGroup(cmd.Context(), request)
	if err != nil {
		return a.messageExit(err, "Ensure the active identity is registered and the message service is reachable.")
	}
	return a.renderMessageResult(cmd, format, result)
}

func (a *App) runGroupShow(cmd *cobra.Command, args []string) error {
	group, _ := cmd.Flags().GetString("group")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := message.GroupGetRequest{IdentityName: a.globals.Identity, Group: group}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{"action": "group.show", "identity": a.globals.Identity, "runtime_mode": service.Config().RuntimeMode, "group": group}}, "Dry run: group show planned", nil, a.identityMeta())
	}
	result, err := service.GetGroup(cmd.Context(), request)
	if err != nil {
		return a.messageExit(err, "Make sure the group exists and the active identity can access it.")
	}
	return a.renderMessageResult(cmd, format, result)
}

func (a *App) runGroupJoin(cmd *cobra.Command, args []string) error {
	group, _ := cmd.Flags().GetString("group")
	reason, _ := cmd.Flags().GetString("reason")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := message.GroupJoinRequest{IdentityName: a.globals.Identity, Group: group, ReasonText: reason}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{"action": "group.join", "identity": a.globals.Identity, "runtime_mode": service.Config().RuntimeMode, "request": request}}, "Dry run: group join planned", nil, a.identityMeta())
	}
	result, err := service.JoinGroup(cmd.Context(), request)
	if err != nil {
		return a.messageExit(err, "Make sure the group exists and allows open join for the active identity.")
	}
	return a.renderMessageResult(cmd, format, result)
}

func (a *App) runGroupAdd(cmd *cobra.Command, args []string) error {
	return a.runGroupMemberMutation(cmd, "add", "add")
}

func (a *App) runGroupKick(cmd *cobra.Command, args []string) error {
	return a.runGroupMemberMutation(cmd, "kick", "remove")
}

func (a *App) runGroupMemberMutation(cmd *cobra.Command, publicAction string, memberAction string) error {
	group, _ := cmd.Flags().GetString("group")
	member, _ := cmd.Flags().GetString("member")
	role, _ := cmd.Flags().GetString("role")
	reason, _ := cmd.Flags().GetString("reason")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := message.GroupMemberRequest{IdentityName: a.globals.Identity, Group: group, Member: member, Role: role, ReasonText: reason}
	if a.globals.DryRun {
		plan := map[string]any{"action": "group." + publicAction, "identity": a.globals.Identity, "runtime_mode": service.Config().RuntimeMode, "request": request}
		if completed := message.CompleteBareHandle(member, service.Config().DIDDomain); completed != strings.TrimSpace(member) {
			plan["member_handle"] = completed
		}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": plan}, "Dry run: group membership change planned", nil, a.identityMeta())
	}
	var result *message.CommandResult
	if memberAction == "add" {
		result, err = service.AddGroupMember(cmd.Context(), request)
	} else {
		result, err = service.RemoveGroupMember(cmd.Context(), request)
	}
	if err != nil {
		return a.messageExit(err, "Make sure the group and member exist and the active identity has the required role.")
	}
	if result == nil {
		return commandResultMissing(cmd.CommandPath())
	}
	switch publicAction {
	case "add":
		result.Summary = "Added member to group"
	case "kick":
		result.Summary = "Removed member from group"
	}
	return a.renderMessageResult(cmd, format, result)
}

func (a *App) runGroupLeave(cmd *cobra.Command, args []string) error {
	group, _ := cmd.Flags().GetString("group")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := message.GroupLeaveRequest{IdentityName: a.globals.Identity, Group: group}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{"action": "group.leave", "identity": a.globals.Identity, "runtime_mode": service.Config().RuntimeMode, "request": request}}, "Dry run: group leave planned", nil, a.identityMeta())
	}
	result, err := service.LeaveGroup(cmd.Context(), request)
	if err != nil {
		return a.messageExit(err, "Make sure the group exists and the active identity is still a member.")
	}
	return a.renderMessageResult(cmd, format, result)
}

func (a *App) runGroupUpdate(cmd *cobra.Command, args []string) error {
	group, _ := cmd.Flags().GetString("group")
	name, _ := cmd.Flags().GetString("name")
	description, _ := cmd.Flags().GetString("description")
	discoverability, _ := cmd.Flags().GetString("discoverability")
	admissionMode, _ := cmd.Flags().GetString("admission-mode")
	slug, _ := cmd.Flags().GetString("slug")
	goal, _ := cmd.Flags().GetString("goal")
	rules, _ := cmd.Flags().GetString("rules")
	messagePrompt, _ := cmd.Flags().GetString("message-prompt")
	docURL, _ := cmd.Flags().GetString("doc-url")
	maxMembers, _ := cmd.Flags().GetString("max-members")
	attachmentsAllowed := boolFlagPtr(cmd, "attachments-allowed")
	memberMaxMessages := int64FlagPtr(cmd, "member-max-messages")
	memberMaxTotalChars := int64FlagPtr(cmd, "member-max-total-chars")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := message.GroupUpdateRequest{
		IdentityName:        a.globals.Identity,
		Group:               group,
		Name:                name,
		Description:         description,
		Discoverability:     discoverability,
		AdmissionMode:       admissionMode,
		Slug:                slug,
		Goal:                goal,
		Rules:               rules,
		MessagePrompt:       messagePrompt,
		DocURL:              docURL,
		AttachmentsAllowed:  attachmentsAllowed,
		MaxMembers:          maxMembers,
		MemberMaxMessages:   memberMaxMessages,
		MemberMaxTotalChars: memberMaxTotalChars,
	}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{"action": "group.update", "identity": a.globals.Identity, "runtime_mode": service.Config().RuntimeMode, "request": request}}, "Dry run: group update planned", nil, a.identityMeta())
	}
	result, err := service.UpdateGroup(cmd.Context(), request)
	if err != nil {
		return a.messageExit(err, "Make sure the active identity has permission to update the target group.")
	}
	return a.renderMessageResult(cmd, format, result)
}

func (a *App) runGroupMembers(cmd *cobra.Command, args []string) error {
	group, _ := cmd.Flags().GetString("group")
	limit, _ := cmd.Flags().GetInt("limit")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := message.GroupMembersRequest{IdentityName: a.globals.Identity, Group: group, Limit: limit}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{"action": "group.list_members", "identity": a.globals.Identity, "runtime_mode": service.Config().RuntimeMode, "request": request}}, "Dry run: group members planned", nil, a.identityMeta())
	}
	result, err := service.GroupMembers(cmd.Context(), request)
	if err != nil {
		return a.messageExit(err, "Make sure the group exists and the active identity can access its member list.")
	}
	return a.renderMessageResult(cmd, format, result)
}

func (a *App) runGroupMessages(cmd *cobra.Command, args []string) error {
	group, _ := cmd.Flags().GetString("group")
	limit, _ := cmd.Flags().GetInt("limit")
	cursor, _ := cmd.Flags().GetString("cursor")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := message.GroupMessagesRequest{IdentityName: a.globals.Identity, Group: group, Limit: limit, Cursor: cursor}
	if a.globals.DryRun {
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, map[string]any{"plan": map[string]any{"action": "group.list_messages", "identity": a.globals.Identity, "runtime_mode": service.Config().RuntimeMode, "request": request}}, "Dry run: group messages planned", nil, a.identityMeta())
	}
	result, err := service.GroupMessages(cmd.Context(), request)
	if err != nil {
		return a.messageExit(err, "Make sure the group exists and the active identity can access its messages.")
	}
	return a.renderMessageResult(cmd, format, result)
}

func boolFlagPtr(cmd *cobra.Command, name string) *bool {
	if !cmd.Flags().Changed(name) {
		return nil
	}
	value, err := cmd.Flags().GetBool(name)
	if err != nil {
		return nil
	}
	return &value
}

func int64FlagPtr(cmd *cobra.Command, name string) *int64 {
	if !cmd.Flags().Changed(name) {
		return nil
	}
	value, err := cmd.Flags().GetInt(name)
	if err != nil {
		return nil
	}
	result := int64(value)
	return &result
}

func parseInt64OrEmpty(value string) *int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

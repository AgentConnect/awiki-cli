package cli

import (
	"errors"
	"os"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/message"
	"github.com/agentconnect/awiki-cli/internal/output"
	"github.com/spf13/cobra"
)

var quietMessageWarningPrefixes = []string{
	"Group lifecycle commands use HTTP transport even when runtime.mode is websocket.",
}

func (a *App) messageService() (*message.Service, output.Format, error) {
	resolved, err := a.resolveConfigForWorkspace()
	if err != nil {
		return nil, output.FormatJSON, err
	}
	format := normalizedFormat(resolved.OutputFormat)
	service, err := message.NewService(resolved)
	if err != nil {
		return nil, format, err
	}
	return service, format, nil
}

func (a *App) messageExit(err error, hint string) error {
	if err == nil {
		return nil
	}
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) {
		return err
	}
	var serviceErr *message.ServiceError
	if errors.As(err, &serviceErr) {
		switch {
		case serviceErr.StatusCode == 400 || serviceErr.RPCCode == -32602:
			return output.NewExitError("invalid_argument", 2, err.Error(), hint)
		case serviceErr.StatusCode == 401 || serviceErr.RPCCode == -32000 || serviceErr.RPCCode == 1401:
			return output.NewExitError("auth_required", 3, err.Error(), "Use an identity with a valid JWT or DID WBA auth material.")
		case serviceErr.StatusCode == 404 || serviceErr.RPCCode == -32002:
			return output.NewExitError("not_found", 5, err.Error(), hint)
		case serviceErr.StatusCode == 409 || serviceErr.RPCCode == -32003 || serviceErr.RPCCode == -32004:
			return output.NewExitError("conflict", 1, err.Error(), hint)
		case serviceErr.RPCCode == 6000 || serviceErr.RPCCode == 6005 || serviceErr.RPCCode == 6007 || serviceErr.RPCCode == 6012:
			return output.NewExitError("not_found", 5, err.Error(), hint)
		case serviceErr.RPCCode == 6006 || serviceErr.RPCCode == 6008 || serviceErr.RPCCode == 6009 || serviceErr.RPCCode == 6010 || serviceErr.RPCCode == 6011 || serviceErr.RPCCode == 6013:
			return output.NewExitError("invalid_argument", 2, err.Error(), hint)
		}
	}
	switch {
	case errors.Is(err, message.ErrTargetRequired),
		errors.Is(err, message.ErrGroupRequired),
		errors.Is(err, message.ErrMemberRequired),
		errors.Is(err, message.ErrGroupOwnerCannotLeave),
		errors.Is(err, message.ErrTextRequired),
		errors.Is(err, message.ErrFilePathRequired),
		errors.Is(err, message.ErrMimeTypeWithoutFile),
		errors.Is(err, message.ErrMessageIDRequired),
		errors.Is(err, message.ErrOutputPathRequired),
		errors.Is(err, message.ErrDownloadTargetNeeded),
		errors.Is(err, message.ErrDownloadTargetConflict),
		errors.Is(err, message.ErrAttachmentIDRequired),
		errors.Is(err, message.ErrAttachmentMessageInvalid):
		return output.NewExitError("invalid_argument", 2, err.Error(), hint)
	case errors.Is(err, message.ErrMessageNotFound), errors.Is(err, message.ErrAttachmentNotFound):
		return output.NewExitError("not_found", 5, err.Error(), hint)
	case errors.Is(err, identity.ErrUserRegistrationRequired):
		return output.NewExitError("identity_required", 3, err.Error(), "Complete user setup with `awiki-cli id register --handle <handle> ...` or recover an existing handle before using msg commands.")
	case errors.Is(err, message.ErrSecureNotSupported):
		return output.NewExitError("unsupported_mode", 1, err.Error(), "Direct secure messaging is planned for Phase 5.")
	case errors.Is(err, message.ErrTransportUnavailable):
		return output.NewExitError("transport_unavailable", 1, err.Error(), "Start the websocket listener/daemon or switch runtime.mode back to http.")
	default:
		type rpcCoder interface{ Error() string }
		var _ rpcCoder = err
		return output.NewExitError("internal_error", 1, err.Error(), hint)
	}
}

func (a *App) renderMessageResult(cmd *cobra.Command, format output.Format, result *message.CommandResult) error {
	if result == nil {
		return commandResultMissing(cmd.CommandPath())
	}
	return a.renderSuccess(
		cmd.CommandPath(),
		format,
		a.globals.JQ,
		result.Data,
		result.Summary,
		a.filterMessageWarningsForDisplay(result.Warnings),
		a.identityMeta(),
	)
}

func (a *App) filterMessageWarningsForDisplay(warnings []string) []string {
	if a != nil && a.globals.Verbose {
		return warnings
	}
	filtered := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if shouldHideMessageWarning(warning) {
			continue
		}
		filtered = append(filtered, warning)
	}
	return filtered
}

func shouldHideMessageWarning(warning string) bool {
	warning = strings.TrimSpace(warning)
	for _, prefix := range quietMessageWarningPrefixes {
		if strings.HasPrefix(warning, prefix) {
			return true
		}
	}
	return false
}

func (a *App) runMsgSend(cmd *cobra.Command, args []string) error {
	to, _ := cmd.Flags().GetString("to")
	group, _ := cmd.Flags().GetString("group")
	text, _ := cmd.Flags().GetString("text")
	textFile, _ := cmd.Flags().GetString("text-file")
	filePath, _ := cmd.Flags().GetString("file")
	mimeType, _ := cmd.Flags().GetString("mime-type")
	messageType, _ := cmd.Flags().GetString("type")
	secure, _ := cmd.Flags().GetString("secure")
	hasAttachment := strings.TrimSpace(filePath) != ""
	if strings.TrimSpace(group) == "" && strings.TrimSpace(to) == "" {
		return output.NewExitError("invalid_argument", 2, "msg send requires either --to or --group.", "Usage: awiki-cli msg send --to <handle|did> --text \"Hello\" or awiki-cli msg send --group <group_did> --text \"Hello group\"")
	}
	if strings.TrimSpace(group) != "" && strings.TrimSpace(to) != "" {
		return output.NewExitError("invalid_argument", 2, "msg send accepts either --to or --group, but not both.", "Choose direct messaging with --to or group messaging with --group.")
	}
	if strings.TrimSpace(text) == "" && strings.TrimSpace(textFile) != "" {
		raw, err := os.ReadFile(textFile)
		if err != nil {
			return output.NewExitError("invalid_argument", 2, err.Error(), "Make sure --text-file points to a readable file.")
		}
		text = string(raw)
	}
	if !hasAttachment && strings.TrimSpace(mimeType) != "" {
		return output.NewExitError("invalid_argument", 2, message.ErrMimeTypeWithoutFile.Error(), "Use --mime-type only together with --file.")
	}
	if hasAttachment && cmd.Flags().Changed("type") {
		return output.NewExitError("invalid_argument", 2, "msg send does not accept --type together with --file.", "Attachment sends always use attachment manifests.")
	}
	if !hasAttachment && strings.TrimSpace(text) == "" {
		return output.NewExitError("invalid_argument", 2, "msg send requires --text or --text-file.", "Provide the message body via --text or --text-file.")
	}
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := message.SendRequest{
		IdentityName: a.globals.Identity,
		Target:       to,
		Group:        group,
		Text:         text,
		MessageType:  messageType,
		SecureMode:   secure,
		FilePath:     filePath,
		MIMEType:     mimeType,
	}
	if a.globals.DryRun {
		action := "direct.send"
		target := map[string]any{"did": to, "kind": "direct"}
		if completed := message.CompleteBareHandle(to, service.Config().DIDDomain); completed != strings.TrimSpace(to) {
			target["handle"] = completed
		}
		if strings.TrimSpace(group) != "" {
			action = "group.send"
			target = map[string]any{"did": group, "kind": "group"}
		}
		if hasAttachment {
			action = "attachment.send"
		}
		data := map[string]any{
			"plan": map[string]any{
				"action":       action,
				"identity":     a.globals.Identity,
				"target":       target,
				"message_type": defaultString(messageType, "text"),
				"runtime_mode": service.Config().RuntimeMode,
				"transport":    service.Config().RuntimeMode,
				"local_writes": []string{"messages"},
			},
		}
		if hasAttachment {
			data["plan"].(map[string]any)["message_type"] = "attachment_manifest"
			data["plan"].(map[string]any)["transport"] = "http"
			data["plan"].(map[string]any)["attachment"] = map[string]any{
				"path":      filePath,
				"mime_type": mimeType,
				"caption":   text,
			}
		}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: message send planned", nil, a.identityMeta())
	}
	result, err := service.Send(cmd.Context(), request)
	if err != nil {
		return a.messageExit(err, "Ensure the target exists, the active identity is valid, and runtime mode is configured correctly.")
	}
	return a.renderMessageResult(cmd, format, result)
}

func (a *App) runMsgAttachmentDownload(cmd *cobra.Command, args []string) error {
	with, _ := cmd.Flags().GetString("with")
	group, _ := cmd.Flags().GetString("group")
	messageID, _ := cmd.Flags().GetString("message-id")
	attachmentID, _ := cmd.Flags().GetString("attachment-id")
	outputPath, _ := cmd.Flags().GetString("output")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := message.AttachmentDownloadRequest{
		IdentityName: a.globals.Identity,
		With:         with,
		Group:        group,
		MessageID:    messageID,
		AttachmentID: attachmentID,
		OutputPath:   outputPath,
	}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":        "download_attachment",
			"identity":      a.globals.Identity,
			"with":          with,
			"group":         group,
			"message_id":    messageID,
			"attachment_id": attachmentID,
			"output":        outputPath,
			"transport":     "http",
		}}
		if completed := message.CompleteBareHandle(with, service.Config().DIDDomain); completed != strings.TrimSpace(with) {
			data["plan"].(map[string]any)["with_handle"] = completed
		}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: attachment download planned", nil, a.identityMeta())
	}
	result, err := service.DownloadAttachment(cmd.Context(), request)
	if err != nil {
		return a.messageExit(err, "Make sure the message id, attachment id, and target context are correct.")
	}
	return a.renderMessageResult(cmd, format, result)
}

func (a *App) runMsgInbox(cmd *cobra.Command, args []string) error {
	scope, _ := cmd.Flags().GetString("scope")
	with, _ := cmd.Flags().GetString("with")
	group, _ := cmd.Flags().GetString("group")
	unread, _ := cmd.Flags().GetBool("unread")
	limit, _ := cmd.Flags().GetInt("limit")
	markRead, _ := cmd.Flags().GetBool("mark-read")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := message.InboxRequest{
		IdentityName: a.globals.Identity,
		Scope:        scope,
		With:         with,
		Group:        group,
		Limit:        limit,
		UnreadOnly:   unread,
		MarkRead:     markRead,
	}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":       "inbox.get",
			"identity":     a.globals.Identity,
			"runtime_mode": service.Config().RuntimeMode,
			"scope":        scope,
			"with":         with,
			"group":        group,
			"limit":        limit,
			"mark_read":    markRead,
		}}
		if completed := message.CompleteBareHandle(with, service.Config().DIDDomain); completed != strings.TrimSpace(with) {
			data["plan"].(map[string]any)["with_handle"] = completed
		}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: inbox read planned", nil, a.identityMeta())
	}
	result, err := service.Inbox(cmd.Context(), request)
	if err != nil {
		return a.messageExit(err, "Make sure the active identity is valid and runtime mode is available.")
	}
	return a.renderMessageResult(cmd, format, result)
}

func (a *App) runMsgHistory(cmd *cobra.Command, args []string) error {
	with, _ := cmd.Flags().GetString("with")
	limit, _ := cmd.Flags().GetInt("limit")
	cursor, _ := cmd.Flags().GetString("cursor")
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := message.HistoryRequest{
		IdentityName: a.globals.Identity,
		With:         with,
		Limit:        limit,
		Cursor:       cursor,
	}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":       "direct.get_history",
			"identity":     a.globals.Identity,
			"runtime_mode": service.Config().RuntimeMode,
			"with":         with,
			"limit":        limit,
			"cursor":       cursor,
		}}
		if completed := message.CompleteBareHandle(with, service.Config().DIDDomain); completed != strings.TrimSpace(with) {
			data["plan"].(map[string]any)["with_handle"] = completed
		}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: direct history read planned", nil, a.identityMeta())
	}
	result, err := service.History(cmd.Context(), request)
	if err != nil {
		return a.messageExit(err, "Make sure the peer exists and runtime mode is available.")
	}
	return a.renderMessageResult(cmd, format, result)
}

func (a *App) runMsgMarkRead(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return output.NewExitError("invalid_argument", 2, "msg mark-read requires at least one message id.", "Usage: awiki-cli msg mark-read <MESSAGE_ID...>")
	}
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := message.MarkReadRequest{
		IdentityName: a.globals.Identity,
		MessageIDs:   args,
	}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":       "inbox.mark_read",
			"identity":     a.globals.Identity,
			"runtime_mode": service.Config().RuntimeMode,
			"message_ids":  args,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: mark-read planned", nil, a.identityMeta())
	}
	result, err := service.MarkRead(cmd.Context(), request)
	if err != nil {
		return a.messageExit(err, "Make sure the message ids are valid and runtime mode is available.")
	}
	return a.renderMessageResult(cmd, format, result)
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

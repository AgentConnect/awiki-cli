package cli

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/message"
	"github.com/agentconnect/awiki-cli/internal/output"
	"github.com/spf13/cobra"
)

func (a *App) messageService() (*message.Service, output.Format, error) {
	resolved, err := a.resolveConfig()
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
	var serviceErr *message.ServiceError
	if errors.As(err, &serviceErr) {
		switch {
		case serviceErr.StatusCode == 400 || serviceErr.RPCCode == -32602:
			return output.NewExitError("invalid_argument", 2, err.Error(), hint)
		case serviceErr.StatusCode == 401 || serviceErr.RPCCode == -32000:
			return output.NewExitError("auth_required", 3, err.Error(), "Use an identity with a valid JWT or DID WBA auth material.")
		case serviceErr.StatusCode == 404 || serviceErr.RPCCode == -32002:
			return output.NewExitError("not_found", 5, err.Error(), hint)
		case serviceErr.StatusCode == 409 || serviceErr.RPCCode == -32003 || serviceErr.RPCCode == -32004:
			return output.NewExitError("conflict", 1, err.Error(), hint)
		}
	}
	switch {
	case errors.Is(err, message.ErrTargetRequired), errors.Is(err, message.ErrTextRequired), errors.Is(err, message.ErrGroupNotSupported), errors.Is(err, message.ErrMessageNotFound):
		return output.NewExitError("invalid_argument", 2, err.Error(), hint)
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

func (a *App) runMsgSend(cmd *cobra.Command, args []string) error {
	to, _ := cmd.Flags().GetString("to")
	group, _ := cmd.Flags().GetString("group")
	text, _ := cmd.Flags().GetString("text")
	textFile, _ := cmd.Flags().GetString("text-file")
	messageType, _ := cmd.Flags().GetString("type")
	secure, _ := cmd.Flags().GetString("secure")
	if strings.TrimSpace(group) != "" {
		return a.messageExit(message.ErrGroupNotSupported, "Use direct messaging for now; group messaging will be implemented after direct plain flows.")
	}
	if strings.TrimSpace(to) == "" {
		return output.NewExitError("invalid_argument", 2, "msg send requires --to for direct messaging.", "Usage: awiki-cli msg send --to <handle|did> --text \"Hello\"")
	}
	if strings.TrimSpace(text) == "" && strings.TrimSpace(textFile) != "" {
		raw, err := os.ReadFile(textFile)
		if err != nil {
			return output.NewExitError("invalid_argument", 2, err.Error(), "Make sure --text-file points to a readable file.")
		}
		text = string(raw)
	}
	if strings.TrimSpace(text) == "" {
		return output.NewExitError("invalid_argument", 2, "msg send requires --text or --text-file.", "Provide the message body via --text or --text-file.")
	}
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := message.SendRequest{
		IdentityName: a.globals.Identity,
		Target:       to,
		Text:         text,
		MessageType:  messageType,
		SecureMode:   secure,
	}
	if a.globals.DryRun {
		data := map[string]any{
			"plan": map[string]any{
				"action":       "direct.send",
				"identity":     a.globals.Identity,
				"target":       to,
				"message_type": defaultString(messageType, "text"),
				"runtime_mode": service.Config().RuntimeMode,
				"transport":    service.Config().RuntimeMode,
				"local_writes": []string{"messages"},
			},
		}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: direct send planned", nil, a.identityMeta())
	}
	result, err := service.Send(context.Background(), request)
	if err != nil {
		return a.messageExit(err, "Ensure the target exists, the active identity is valid, and runtime mode is configured correctly.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result.Data, result.Summary, result.Warnings, a.identityMeta())
}

func (a *App) runMsgInbox(cmd *cobra.Command, args []string) error {
	scope, _ := cmd.Flags().GetString("scope")
	with, _ := cmd.Flags().GetString("with")
	group, _ := cmd.Flags().GetString("group")
	unread, _ := cmd.Flags().GetBool("unread")
	limit, _ := cmd.Flags().GetInt("limit")
	markRead, _ := cmd.Flags().GetBool("mark-read")
	if strings.TrimSpace(group) != "" || strings.EqualFold(scope, "group") {
		return a.messageExit(message.ErrGroupNotSupported, "Direct inbox is implemented first. Group inbox will follow after direct messaging is stable.")
	}
	service, format, err := a.messageService()
	if err != nil {
		return a.messageExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := message.InboxRequest{
		IdentityName: a.globals.Identity,
		Scope:        scope,
		With:         with,
		Limit:        limit,
		UnreadOnly:   unread,
		MarkRead:     markRead,
	}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":       "inbox.get",
			"identity":     a.globals.Identity,
			"runtime_mode": service.Config().RuntimeMode,
			"with":         with,
			"limit":        limit,
			"mark_read":    markRead,
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: inbox read planned", nil, a.identityMeta())
	}
	result, err := service.Inbox(context.Background(), request)
	if err != nil {
		return a.messageExit(err, "Make sure the active identity is valid and runtime mode is available.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result.Data, result.Summary, result.Warnings, a.identityMeta())
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
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: direct history read planned", nil, a.identityMeta())
	}
	result, err := service.History(context.Background(), request)
	if err != nil {
		return a.messageExit(err, "Make sure the peer exists and runtime mode is available.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result.Data, result.Summary, result.Warnings, a.identityMeta())
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
	result, err := service.MarkRead(context.Background(), request)
	if err != nil {
		return a.messageExit(err, "Make sure the message ids are valid and runtime mode is available.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result.Data, result.Summary, result.Warnings, a.identityMeta())
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

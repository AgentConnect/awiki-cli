package cli

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/mail"
	"github.com/agentconnect/awiki-cli/internal/output"
	"github.com/spf13/cobra"
)

func (a *App) mailService() (*mail.Service, output.Format, error) {
	resolved, err := a.resolveConfig()
	if err != nil {
		return nil, output.FormatJSON, err
	}
	format := normalizedFormat(resolved.OutputFormat)
	service, err := mail.NewService(resolved)
	if err != nil {
		return nil, format, err
	}
	return service, format, nil
}

func (a *App) mailExit(err error, hint string) error {
	if err == nil {
		return nil
	}
	var serviceErr *mail.ServiceError
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
	case errors.Is(err, mail.ErrMessageIDRequired), errors.Is(err, mail.ErrRecipientRequired), errors.Is(err, mail.ErrSubjectRequired), errors.Is(err, mail.ErrBodyRequired), errors.Is(err, mail.ErrAttachmentIndexZero):
		return output.NewExitError("invalid_argument", 2, err.Error(), hint)
	case errors.Is(err, identity.ErrUserRegistrationRequired):
		return output.NewExitError("identity_required", 3, err.Error(), "Complete user setup with `awiki-cli id register --handle <handle> ...` or recover an existing handle before using msg mail commands.")
	default:
		return output.NewExitError("internal_error", 1, err.Error(), hint)
	}
}

func (a *App) runMailInbox(cmd *cobra.Command, args []string) error {
	folder, _ := cmd.Flags().GetString("folder")
	unread, _ := cmd.Flags().GetBool("unread")
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")

	service, format, err := a.mailService()
	if err != nil {
		return a.mailExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}

	request := mail.InboxRequest{
		IdentityName: a.globals.Identity,
		Folder:       folder,
		Limit:        limit,
		Offset:       offset,
		UnreadOnly:   unread,
	}

	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":       "mail.getInbox",
			"identity":     a.globals.Identity,
			"folder":       defaultString(folder, "inbox"),
			"limit":        limit,
			"offset":       offset,
			"unread_only":  unread,
			"remote_calls": []string{"POST /mail/rpc mail.getInbox"},
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: mail inbox planned", nil, a.identityMeta())
	}

	result, err := service.Inbox(context.Background(), request)
	if err != nil {
		return a.mailExit(err, "Ensure the active identity is valid and mail service is reachable.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result.Data, result.Summary, result.Warnings, a.identityMeta())
}

func (a *App) runMailRead(cmd *cobra.Command, args []string) error {
	messageID, _ := cmd.Flags().GetString("id")
	if strings.TrimSpace(messageID) == "" {
		return output.NewExitError("invalid_argument", 2, "mail read requires --id.", "Usage: awiki-cli msg mail read --id <MESSAGE_ID>")
	}
	service, format, err := a.mailService()
	if err != nil {
		return a.mailExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := mail.ReadRequest{IdentityName: a.globals.Identity, MessageID: messageID}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":       "mail.getMessage",
			"identity":     a.globals.Identity,
			"message_id":   messageID,
			"remote_calls": []string{"POST /mail/rpc mail.getMessage"},
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: mail read planned", nil, a.identityMeta())
	}
	result, err := service.Read(context.Background(), request)
	if err != nil {
		return a.mailExit(err, "Ensure the message id is valid and mail service is reachable.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result.Data, result.Summary, result.Warnings, a.identityMeta())
}

func (a *App) runMailMarkRead(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return output.NewExitError("invalid_argument", 2, "mail mark-read requires at least one message id.", "Usage: awiki-cli msg mail mark-read <MESSAGE_ID...>")
	}
	service, format, err := a.mailService()
	if err != nil {
		return a.mailExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := mail.MarkReadRequest{IdentityName: a.globals.Identity, MessageIDs: args, IsRead: true}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":       "mail.markRead",
			"identity":     a.globals.Identity,
			"message_ids":  args,
			"remote_calls": []string{"POST /mail/rpc mail.markRead"},
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: mail mark-read planned", nil, a.identityMeta())
	}
	result, err := service.MarkRead(context.Background(), request)
	if err != nil {
		return a.mailExit(err, "Ensure the message ids are valid and mail service is reachable.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result.Data, result.Summary, result.Warnings, a.identityMeta())
}

func (a *App) runMailAccount(cmd *cobra.Command, args []string) error {
	service, format, err := a.mailService()
	if err != nil {
		return a.mailExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := mail.AccountRequest{IdentityName: a.globals.Identity}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":       "mail.getMailbox",
			"identity":     a.globals.Identity,
			"remote_calls": []string{"POST /mail/rpc mail.getMailbox"},
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: mail account lookup planned", nil, a.identityMeta())
	}
	result, err := service.Account(context.Background(), request)
	if err != nil {
		return a.mailExit(err, "Ensure the mail service is reachable.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result.Data, result.Summary, result.Warnings, a.identityMeta())
}

func (a *App) runMailSend(cmd *cobra.Command, args []string) error {
	toRaw, _ := cmd.Flags().GetString("to")
	ccRaw, _ := cmd.Flags().GetString("cc")
	subject, _ := cmd.Flags().GetString("subject")
	body, _ := cmd.Flags().GetString("body")
	html, _ := cmd.Flags().GetString("html")

	to := splitMailList(toRaw)
	cc := splitMailList(ccRaw)
	if len(to) == 0 {
		return output.NewExitError("invalid_argument", 2, "mail send requires --to.", "Usage: awiki-cli msg mail send --to alice@example.com --subject \"Hello\" --body \"Hi\"")
	}
	if strings.TrimSpace(subject) == "" {
		return output.NewExitError("invalid_argument", 2, "mail send requires --subject.", "Provide a subject with --subject.")
	}
	if strings.TrimSpace(body) == "" {
		return output.NewExitError("invalid_argument", 2, "mail send requires --body.", "Provide the plain text body with --body.")
	}

	service, format, err := a.mailService()
	if err != nil {
		return a.mailExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := mail.SendRequest{
		IdentityName: a.globals.Identity,
		To:           to,
		CC:           cc,
		Subject:      subject,
		BodyText:     body,
		BodyHTML:     html,
	}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":       "mail.send",
			"identity":     a.globals.Identity,
			"to":           to,
			"cc":           cc,
			"subject":      subject,
			"has_html":     strings.TrimSpace(html) != "",
			"remote_calls": []string{"POST /mail/rpc mail.send"},
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: mail send planned", nil, a.identityMeta())
	}

	result, err := service.Send(context.Background(), request)
	if err != nil {
		return a.mailExit(err, "Ensure the mail service is reachable and the active identity is valid.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result.Data, result.Summary, result.Warnings, a.identityMeta())
}

func (a *App) runMailAttachmentDownload(cmd *cobra.Command, args []string) error {
	messageID, _ := cmd.Flags().GetString("message-id")
	index, _ := cmd.Flags().GetInt("attachment-index")
	outputPath, _ := cmd.Flags().GetString("output")
	if strings.TrimSpace(messageID) == "" {
		return output.NewExitError("invalid_argument", 2, "mail attachment download requires --message-id.", "Usage: awiki-cli msg mail attachment download --message-id <MESSAGE_ID> --attachment-index 0")
	}
	if index < 0 {
		return output.NewExitError("invalid_argument", 2, "attachment index must be >= 0.", "Use --attachment-index 0 for the first attachment.")
	}
	service, format, err := a.mailService()
	if err != nil {
		return a.mailExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	request := mail.AttachmentRequest{IdentityName: a.globals.Identity, MessageID: messageID, AttachmentIndex: index}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":           "mail.getAttachment",
			"identity":         a.globals.Identity,
			"message_id":       messageID,
			"attachment_index": index,
			"output":           outputPath,
			"remote_calls":     []string{"POST /mail/rpc mail.getAttachment"},
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: mail attachment download planned", nil, a.identityMeta())
	}

	result, err := service.Attachment(context.Background(), request)
	if err != nil {
		return a.mailExit(err, "Ensure the message id is valid and mail service is reachable.")
	}

	payload := result.Data
	filename := defaultString(stringFromAny(payload["filename"]), fmt.Sprintf("attachment_%d", index))
	contentB64 := stringFromAny(payload["content_base64"])
	contentType := defaultString(stringFromAny(payload["content_type"]), "application/octet-stream")
	size := payload["size"]

	if contentB64 == "" {
		return output.NewExitError("internal_error", 1, "attachment content is empty", "Try fetching the attachment again or verify the mail service response.")
	}

	content, err := base64.StdEncoding.DecodeString(contentB64)
	if err != nil {
		return output.NewExitError("internal_error", 1, fmt.Sprintf("attachment base64 decode failed: %v", err), "Ensure the mail service response is valid.")
	}

	outPath := outputPath
	if strings.TrimSpace(outPath) == "" {
		outPath = filename
	}
	if dir := filepath.Dir(outPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return output.NewExitError("internal_error", 1, err.Error(), "Check write permissions for the output directory.")
		}
	}
	if err := os.WriteFile(outPath, content, 0o600); err != nil {
		return output.NewExitError("internal_error", 1, err.Error(), "Check write permissions for the output file.")
	}

	data := map[string]any{
		"message_id":       messageID,
		"attachment_index": index,
		"filename":         filename,
		"content_type":     contentType,
		"size":             size,
		"path":             outPath,
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, fmt.Sprintf("Attachment saved to %s", outPath), result.Warnings, a.identityMeta())
}

func (a *App) runMailNotify(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")

	service, format, err := a.mailService()
	if err != nil {
		return a.mailExit(err, "Run `awiki-cli doctor` to inspect configuration and identity state.")
	}
	if a.globals.DryRun {
		data := map[string]any{"plan": map[string]any{
			"action":       "mail.notifications",
			"identity":     a.globals.Identity,
			"limit":        limit,
			"remote_calls": []string{}, // local sqlite only
		}}
		return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, data, "Dry run: mail notifications planned", nil, a.identityMeta())
	}

	result, err := service.Notifications(context.Background(), a.globals.Identity, limit)
	if err != nil {
		return a.mailExit(err, "Ensure the runtime listener is running in websocket mode and has received notifications.")
	}
	return a.renderSuccess(cmd.CommandPath(), format, a.globals.JQ, result.Data, result.Summary, nil, a.identityMeta())
}

func splitMailList(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t' || r == ' '
	})
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			values = append(values, item)
		}
	}
	return values
}

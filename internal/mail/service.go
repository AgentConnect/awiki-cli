package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/authsdk"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/store"
	"github.com/agentconnect/awiki-cli/internal/traceutil"
	"github.com/agentconnect/awiki-cli/internal/transportcfg"
)

const didAuthRPCEndpoint = "/user-service/did-auth/rpc"

type Service struct {
	resolved *appconfig.Resolved
	manager  *identity.Manager
	client   *Client
}

func NewService(resolved *appconfig.Resolved) (*Service, error) {
	if resolved == nil {
		return nil, fmt.Errorf("resolved config is required")
	}
	client, err := NewClient(resolved)
	if err != nil {
		return nil, err
	}
	return &Service{
		resolved: resolved,
		manager:  identity.NewManager(resolved.Paths),
		client:   client,
	}, nil
}

func (s *Service) Config() *appconfig.Resolved {
	return s.resolved
}

// Notifications returns recent local mail-notification records from the local websocket cache.
//
// This does not call the remote mail-service. Instead, it reads from the same
// sqlite database used by the runtime listener for direct/group messages and
// surfaces rows recognized as local mail notifications, including both:
// - legacy rows with content_type = "mail.notification"
// - current rows with metadata.source_kind = "mail"
func (s *Service) Notifications(ctx context.Context, identityName string, limit int) (*CommandResult, error) {
	if limit <= 0 {
		limit = 20
	}
	record, err := s.requireActiveIdentity(identityName)
	if err != nil {
		return nil, err
	}
	finish := traceutil.LocalDBPhase(ctx, "read_mail_notifications")
	defer finish()
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	rows, err := store.ListNotifications(ctx, db, record.DID, limit)
	if err != nil {
		return nil, err
	}
	rows = normalizeNotificationRows(rows)
	summary := fmt.Sprintf("Loaded %d mail notification(s)", len(rows))
	data := map[string]any{
		"notifications": rows,
		"total":         len(rows),
	}
	return &CommandResult{Data: data, Summary: summary}, nil
}

func (s *Service) Inbox(ctx context.Context, request InboxRequest) (*CommandResult, error) {
	if strings.TrimSpace(request.Folder) == "" {
		request.Folder = "inbox"
	}
	if request.Limit <= 0 {
		request.Limit = 20
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	auth, err := s.authSession(ctx, record)
	if err != nil {
		return nil, err
	}
	params := map[string]any{
		"folder":      request.Folder,
		"limit":       request.Limit,
		"offset":      request.Offset,
		"unread_only": request.UnreadOnly,
	}
	var result map[string]any
	if err := s.client.AuthenticatedRPCCallProfile(ctx, transportcfg.ProfileRPCReadHeavy, MailRPCEndpoint, "mail.getInbox", params, auth, &result); err != nil {
		return nil, err
	}
	total := intValueFromAny(result["total"], 0)
	count := listLength(result["messages"])
	summary := fmt.Sprintf("Loaded %d messages from %s", count, request.Folder)
	if total > 0 && count == 0 {
		summary = fmt.Sprintf("Loaded %d messages", total)
	}
	return &CommandResult{Data: result, Summary: summary}, nil
}

func normalizeNotificationRows(rows []map[string]any) []map[string]any {
	if len(rows) == 0 {
		return rows
	}
	normalized := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		normalized = append(normalized, normalizeNotificationRow(row))
	}
	return normalized
}

func normalizeNotificationRow(row map[string]any) map[string]any {
	if !isLocalMailNotificationRow(row) {
		return row
	}
	metadata := parseNotificationMetadata(row["metadata"])
	mailboxAddress := defaultString(stringFromAny(metadata["mailbox_address"]), stringFromAny(row["thread_id"]))
	if strings.HasPrefix(mailboxAddress, "mail:") {
		mailboxAddress = strings.TrimPrefix(mailboxAddress, "mail:")
	}
	subject := defaultString(stringFromAny(metadata["subject"]), stringFromAny(row["title"]))
	if strings.HasPrefix(subject, "[邮件] ") {
		subject = strings.TrimPrefix(subject, "[邮件] ")
	}
	if strings.TrimSpace(subject) == "" {
		subject = "(no subject)"
	}
	fromAddr := stringFromAny(metadata["from_addr"])
	preview := stringFromAny(metadata["preview"])
	hasAttachments := boolFromAny(metadata["has_attachments"])

	normalized := make(map[string]any, len(row))
	for key, value := range row {
		normalized[key] = value
	}
	normalized["source_kind"] = "mail"
	normalized["title"] = "[邮件] " + subject
	normalized["content"] = buildNotificationContent(mailboxAddress, fromAddr, subject, preview, hasAttachments)
	return normalized
}

func isLocalMailNotificationRow(row map[string]any) bool {
	if row == nil {
		return false
	}
	if strings.TrimSpace(stringFromAny(row["content_type"])) == "mail.notification" {
		return true
	}
	metadata := parseNotificationMetadata(row["metadata"])
	return strings.TrimSpace(stringFromAny(metadata["source_kind"])) == "mail"
}

func parseNotificationMetadata(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(typed), &parsed); err != nil {
			return nil
		}
		return parsed
	default:
		return nil
	}
}

func buildNotificationContent(mailboxAddress string, fromAddr string, subject string, preview string, hasAttachments bool) string {
	contentLines := []string{
		fmt.Sprintf("[邮件] 收件邮箱: %s", mailboxAddress),
	}
	if fromAddr != "" {
		contentLines = append(contentLines, fmt.Sprintf("发件人: %s", fromAddr))
	}
	if subject != "" {
		contentLines = append(contentLines, fmt.Sprintf("主题: %s", subject))
	}
	if preview != "" {
		contentLines = append(contentLines, "", preview)
	}
	if hasAttachments {
		contentLines = append(contentLines, "", "(这封邮件包含附件)")
	}
	return strings.Join(contentLines, "\n")
}

func (s *Service) Read(ctx context.Context, request ReadRequest) (*CommandResult, error) {
	if strings.TrimSpace(request.MessageID) == "" {
		return nil, ErrMessageIDRequired
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	auth, err := s.authSession(ctx, record)
	if err != nil {
		return nil, err
	}
	params := map[string]any{"message_id": request.MessageID}
	var result map[string]any
	if err := s.client.AuthenticatedRPCCallProfile(ctx, transportcfg.ProfileRPCReadHeavy, MailRPCEndpoint, "mail.getMessage", params, auth, &result); err != nil {
		return nil, err
	}
	summary := fmt.Sprintf("Loaded message %s", request.MessageID)
	return &CommandResult{Data: result, Summary: summary}, nil
}

func (s *Service) MarkRead(ctx context.Context, request MarkReadRequest) (*CommandResult, error) {
	if len(request.MessageIDs) == 0 {
		return nil, ErrMessageIDRequired
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	auth, err := s.authSession(ctx, record)
	if err != nil {
		return nil, err
	}
	params := map[string]any{"message_ids": request.MessageIDs, "is_read": request.IsRead}
	var result map[string]any
	if err := s.client.AuthenticatedRPCCallProfile(ctx, transportcfg.ProfileRPCDefault, MailRPCEndpoint, "mail.markRead", params, auth, &result); err != nil {
		return nil, err
	}
	updated := intValueFromAny(result["updated"], 0)
	summary := fmt.Sprintf("Marked %d message(s) as read", updated)
	return &CommandResult{Data: result, Summary: summary}, nil
}

func (s *Service) Account(ctx context.Context, request AccountRequest) (*CommandResult, error) {
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	auth, err := s.authSession(ctx, record)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := s.client.AuthenticatedRPCCallProfile(ctx, transportcfg.ProfileRPCDefault, MailRPCEndpoint, "mail.getMailbox", map[string]any{}, auth, &result); err != nil {
		return nil, err
	}
	return &CommandResult{Data: result, Summary: "Loaded mailbox account"}, nil
}

func (s *Service) Attachment(ctx context.Context, request AttachmentRequest) (*CommandResult, error) {
	if strings.TrimSpace(request.MessageID) == "" {
		return nil, ErrMessageIDRequired
	}
	if request.AttachmentIndex < 0 {
		return nil, ErrAttachmentIndexZero
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	auth, err := s.authSession(ctx, record)
	if err != nil {
		return nil, err
	}
	params := map[string]any{
		"message_id":       request.MessageID,
		"attachment_index": request.AttachmentIndex,
	}
	var result map[string]any
	if err := s.client.AuthenticatedRPCCallProfile(ctx, transportcfg.ProfileRPCReadHeavy, MailRPCEndpoint, "mail.getAttachment", params, auth, &result); err != nil {
		return nil, err
	}
	filename := defaultString(stringFromAny(result["filename"]), fmt.Sprintf("attachment_%d", request.AttachmentIndex))
	summary := fmt.Sprintf("Fetched attachment %s", filename)
	return &CommandResult{Data: result, Summary: summary}, nil
}

func (s *Service) Send(ctx context.Context, request SendRequest) (*CommandResult, error) {
	if len(request.To) == 0 {
		return nil, ErrRecipientRequired
	}
	if strings.TrimSpace(request.Subject) == "" {
		return nil, ErrSubjectRequired
	}
	if strings.TrimSpace(request.BodyText) == "" {
		return nil, ErrBodyRequired
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	auth, err := s.authSession(ctx, record)
	if err != nil {
		return nil, err
	}
	params := map[string]any{
		"to":        request.To,
		"cc":        request.CC,
		"subject":   request.Subject,
		"body_text": request.BodyText,
		"body_html": nil,
	}
	if strings.TrimSpace(request.BodyHTML) != "" {
		params["body_html"] = request.BodyHTML
	}
	var result map[string]any
	if err := s.client.AuthenticatedRPCCallProfile(ctx, transportcfg.ProfileRPCDefault, MailRPCEndpoint, "mail.send", params, auth, &result); err != nil {
		return nil, err
	}
	summary := "Mail send request accepted"
	return &CommandResult{Data: result, Summary: summary}, nil
}

func (s *Service) requireActiveIdentity(requested string) (*identity.StoredIdentity, error) {
	identityName := strings.TrimSpace(requested)
	if identityName == "" {
		identityName = strings.TrimSpace(s.resolved.ActiveIdentity)
	}
	if identityName == "" {
		current, err := s.manager.Current()
		if err != nil {
			return nil, err
		}
		identityName = current.IdentityName
		s.resolved.ActiveIdentity = identityName
	}
	record, err := s.manager.Load(identityName)
	if err != nil {
		return nil, err
	}
	userState := identity.EvaluateStoredIdentityUserState(record)
	if !userState.ReadyForMessaging {
		return nil, identity.UserRegistrationError(record.IdentityName, userState)
	}
	return record, nil
}

func (s *Service) authSession(ctx context.Context, record *identity.StoredIdentity) (*authsdk.Session, error) {
	if record == nil {
		return nil, fmt.Errorf("active identity is required")
	}
	paths, err := s.manager.PathsForIdentity(record.IdentityName)
	if err != nil {
		return nil, err
	}
	session := authsdk.NewSession(
		paths.DIDDocumentPath,
		paths.Key1PrivatePath,
		record.IdentityName,
		record.DID,
		record.JWTToken,
		func(token string) error { return s.manager.UpdateJWT(record.IdentityName, token) },
	)
	baseURL := strings.TrimSpace(s.resolved.ServiceBaseURL)
	token := strings.TrimSpace(record.JWTToken)
	if baseURL != "" {
		session.RememberScope(baseURL)
		session.RememberScope(appconfig.JoinBaseURL(baseURL, didAuthRPCEndpoint))
	}
	if strings.TrimSpace(s.resolved.MailServiceURL) != "" {
		session.RememberScope(s.resolved.MailServiceURL)
	}
	if token != "" && baseURL != "" {
		// 统一的 service_base_url 与 did-auth 端点
		session.SetBearer(baseURL, token)
		session.SetBearer(
			appconfig.JoinBaseURL(baseURL, didAuthRPCEndpoint),
			token,
		)
	}
	if token != "" && strings.TrimSpace(s.resolved.MailServiceURL) != "" {
		// mail 服务可能是独立域名，但复用相同 JWT
		session.SetBearer(s.resolved.MailServiceURL, token)
	}
	if token == "" {
		refreshCtx, cancel := transportcfg.WithProfileTimeout(ctx, transportcfg.ProfileAuthRefresh)
		defer cancel()
		finish := traceutil.EnsureJWTPhase(ctx, "mail_bootstrap")
		defer finish()
		requestURL := appconfig.JoinBaseURL(baseURL, didAuthRPCEndpoint)
		if _, err := session.EnsureJWT(refreshCtx, s.client.client, requestURL); err != nil {
			return nil, fmt.Errorf("active identity does not have a JWT yet: %w", err)
		}
		record.JWTToken = session.CurrentJWT()
	}
	return session, nil
}

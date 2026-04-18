package mail

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agentconnect/awiki-cli/internal/authsdk"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/store"
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

// Notifications returns recent mail.notification records from the local websocket cache.
//
// This does not call the remote mail-service. Instead, it reads from the same
// sqlite database used by the runtime listener for direct/group messages and
// surfaces entries where content_type = "mail.notification".
func (s *Service) Notifications(ctx context.Context, identityName string, limit int) (*CommandResult, error) {
	if limit <= 0 {
		limit = 20
	}
	record, err := s.requireActiveIdentity(identityName)
	if err != nil {
		return nil, err
	}
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
	auth, err := s.authSession(record)
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
	if err := s.client.AuthenticatedRPCCall(ctx, MailRPCEndpoint, "mail.getInbox", params, auth, &result); err != nil {
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

func (s *Service) Read(ctx context.Context, request ReadRequest) (*CommandResult, error) {
	if strings.TrimSpace(request.MessageID) == "" {
		return nil, ErrMessageIDRequired
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	auth, err := s.authSession(record)
	if err != nil {
		return nil, err
	}
	params := map[string]any{"message_id": request.MessageID}
	var result map[string]any
	if err := s.client.AuthenticatedRPCCall(ctx, MailRPCEndpoint, "mail.getMessage", params, auth, &result); err != nil {
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
	auth, err := s.authSession(record)
	if err != nil {
		return nil, err
	}
	params := map[string]any{"message_ids": request.MessageIDs, "is_read": request.IsRead}
	var result map[string]any
	if err := s.client.AuthenticatedRPCCall(ctx, MailRPCEndpoint, "mail.markRead", params, auth, &result); err != nil {
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
	auth, err := s.authSession(record)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := s.client.AuthenticatedRPCCall(ctx, MailRPCEndpoint, "mail.getMailbox", map[string]any{}, auth, &result); err != nil {
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
	auth, err := s.authSession(record)
	if err != nil {
		return nil, err
	}
	params := map[string]any{
		"message_id":       request.MessageID,
		"attachment_index": request.AttachmentIndex,
	}
	var result map[string]any
	if err := s.client.AuthenticatedRPCCall(ctx, MailRPCEndpoint, "mail.getAttachment", params, auth, &result); err != nil {
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
	auth, err := s.authSession(record)
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
	if err := s.client.AuthenticatedRPCCall(ctx, MailRPCEndpoint, "mail.send", params, auth, &result); err != nil {
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

func (s *Service) authSession(record *identity.StoredIdentity) (*authsdk.Session, error) {
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
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		requestURL := appconfig.JoinBaseURL(baseURL, didAuthRPCEndpoint)
		if _, err := session.EnsureJWT(ctx, s.client.client, requestURL); err != nil {
			return nil, fmt.Errorf("active identity does not have a JWT yet: %w", err)
		}
		record.JWTToken = session.CurrentJWT()
	}
	return session, nil
}

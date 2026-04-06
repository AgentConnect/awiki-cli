package message

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/runtime"
	"github.com/agentconnect/awiki-cli/internal/store"
)

type Service struct {
	resolved *appconfig.Resolved
	manager  *identity.Manager
	remote   *identity.RemoteClient
}

func NewService(resolved *appconfig.Resolved) (*Service, error) {
	remote, err := identity.NewRemoteClient(resolved)
	if err != nil {
		return nil, err
	}
	return &Service{
		resolved: resolved,
		manager:  identity.NewManager(resolved.Paths),
		remote:   remote,
	}, nil
}

func (s *Service) Config() *appconfig.Resolved {
	return s.resolved
}

func (s *Service) runtimeConfig() runtime.Resolved {
	return runtime.Resolve(s.resolved)
}

func (s *Service) Send(ctx context.Context, request SendRequest) (*CommandResult, error) {
	if strings.TrimSpace(request.Target) == "" {
		return nil, ErrTargetRequired
	}
	if strings.TrimSpace(request.Text) == "" {
		return nil, ErrTextRequired
	}
	if request.SecureMode == "on" {
		return nil, ErrSecureNotSupported
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	targetDID, targetHandle, err := s.resolveTarget(ctx, request.Target)
	if err != nil {
		return nil, err
	}
	transport, warnings, err := s.transportFor(record)
	if err != nil {
		return nil, err
	}
	request.Target = targetDID
	result, err := transport.SendDirect(ctx, request)
	if err != nil {
		if fallback, fallbackWarnings, fallbackErr := s.httpFallbackSend(ctx, record, request, targetDID); fallbackErr == nil {
			warnings = append(warnings, fallbackWarnings...)
			return s.persistSendResult(ctx, record, targetDID, targetHandle, request, fallback, warnings)
		}
		return nil, err
	}
	return s.persistSendResult(ctx, record, targetDID, targetHandle, request, result, warnings)
}

func (s *Service) Inbox(ctx context.Context, request InboxRequest) (*CommandResult, error) {
	if request.Limit <= 0 {
		request.Limit = 20
	}
	if request.Scope == "" {
		request.Scope = "all"
	}
	if request.Scope == "group" {
		return nil, ErrGroupNotSupported
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	peerDID := ""
	peerHandle := ""
	if strings.TrimSpace(request.With) != "" {
		peerDID, peerHandle, err = s.resolveTarget(ctx, request.With)
		if err != nil {
			return nil, err
		}
		request.With = peerDID
	}

	mode := s.runtimeConfig()
	warnings := make([]string, 0)
	var raw map[string]any
	switch mode.Mode {
	case runtime.ModeWebSocket:
		transport := NewWSProxyTransport(s.resolved, record.IdentityName)
		raw, err = transport.GetInbox(ctx, request)
		if err != nil {
			cached, cacheErr := s.readInboxFromCache(ctx, record, peerDID, request.Limit, request.UnreadOnly)
			if cacheErr == nil && len(cached) > 0 {
				return &CommandResult{
					Data: map[string]any{
						"messages": cached,
						"total":    len(cached),
						"source":   "local_ws_cache_fallback",
						"with":     peerHandleOrDid(peerHandle, peerDID),
					},
					Summary:  "Loaded inbox from local websocket cache",
					Warnings: []string{err.Error()},
				}, nil
			}
			httpTransport, httpWarnings, httpErr := s.httpTransport(record)
			if httpErr != nil {
				return nil, err
			}
			raw, err = httpTransport.GetInbox(ctx, request)
			if err != nil {
				return nil, err
			}
			warnings = append(warnings, "WebSocket transport unavailable; used HTTP fallback.")
			warnings = append(warnings, httpWarnings...)
		}
	default:
		httpTransport, httpWarnings, httpErr := s.httpTransport(record)
		if httpErr != nil {
			return nil, httpErr
		}
		raw, err = httpTransport.GetInbox(ctx, request)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, httpWarnings...)
	}
	messages, total := s.persistInboxMessages(ctx, record, raw)
	if request.MarkRead && len(messages) > 0 {
		messageIDs := collectMessageIDs(messages)
		if len(messageIDs) > 0 {
			if _, markErr := s.MarkRead(ctx, MarkReadRequest{IdentityName: record.IdentityName, MessageIDs: messageIDs}); markErr == nil {
				raw["mark_read"] = true
			}
		}
	}
	return &CommandResult{
		Data: map[string]any{
			"messages": messages,
			"total":    total,
			"source":   sourceWithDefault(raw, mode.Mode),
			"with":     peerHandleOrDid(peerHandle, peerDID),
		},
		Summary:  fmt.Sprintf("Loaded %d direct inbox messages", total),
		Warnings: warnings,
	}, nil
}

func (s *Service) History(ctx context.Context, request HistoryRequest) (*CommandResult, error) {
	if strings.TrimSpace(request.With) == "" {
		return nil, ErrTargetRequired
	}
	if request.Limit <= 0 {
		request.Limit = 50
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	peerDID, peerHandle, err := s.resolveTarget(ctx, request.With)
	if err != nil {
		return nil, err
	}
	request.With = peerDID

	mode := s.runtimeConfig()
	warnings := make([]string, 0)
	var raw map[string]any
	switch mode.Mode {
	case runtime.ModeWebSocket:
		transport := NewWSProxyTransport(s.resolved, record.IdentityName)
		raw, err = transport.GetHistory(ctx, request)
		if err != nil {
			cached, cacheErr := s.readHistoryFromCache(ctx, record, peerDID, request.Limit)
			if cacheErr == nil && len(cached) > 0 {
				return &CommandResult{
					Data: map[string]any{
						"messages": cached,
						"total":    len(cached),
						"source":   "local_ws_cache_fallback",
						"with":     peerHandleOrDid(peerHandle, peerDID),
					},
					Summary:  "Loaded history from local websocket cache",
					Warnings: []string{err.Error()},
				}, nil
			}
			httpTransport, httpWarnings, httpErr := s.httpTransport(record)
			if httpErr != nil {
				return nil, err
			}
			raw, err = httpTransport.GetHistory(ctx, request)
			if err != nil {
				return nil, err
			}
			warnings = append(warnings, "WebSocket transport unavailable; used HTTP fallback.")
			warnings = append(warnings, httpWarnings...)
		}
	default:
		httpTransport, httpWarnings, httpErr := s.httpTransport(record)
		if httpErr != nil {
			return nil, httpErr
		}
		raw, err = httpTransport.GetHistory(ctx, request)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, httpWarnings...)
	}
	messages, total := s.persistHistoryMessages(ctx, record, peerDID, raw)
	return &CommandResult{
		Data: map[string]any{
			"messages": messages,
			"total":    total,
			"source":   sourceWithDefault(raw, mode.Mode),
			"with":     peerHandleOrDid(peerHandle, peerDID),
		},
		Summary:  fmt.Sprintf("Loaded %d direct history messages", total),
		Warnings: warnings,
	}, nil
}

func (s *Service) MarkRead(ctx context.Context, request MarkReadRequest) (*CommandResult, error) {
	if len(request.MessageIDs) == 0 {
		return nil, ErrMessageNotFound
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	transport, warnings, err := s.transportFor(record)
	if err != nil {
		return nil, err
	}
	result, err := transport.MarkRead(ctx, request)
	if err != nil {
		if httpTransport, httpWarnings, httpErr := s.httpTransport(record); httpErr == nil {
			result, err = httpTransport.MarkRead(ctx, request)
			if err == nil {
				warnings = append(warnings, "WebSocket transport unavailable; used HTTP fallback.")
				warnings = append(warnings, httpWarnings...)
			}
		}
	}
	if err != nil {
		return nil, err
	}
	db, openErr := store.Open(s.resolved.Paths)
	if openErr == nil {
		defer db.Close()
		if _, markErr := store.MarkMessagesRead(ctx, db, record.DID, request.MessageIDs); markErr != nil {
			warnings = append(warnings, fmt.Sprintf("Failed to mark local messages read: %v", markErr))
		}
	}
	return &CommandResult{
		Data: map[string]any{
			"action":        "mark_read",
			"updated_count": intValueFromAny(result["updated_count"], 0),
			"message_ids":   request.MessageIDs,
		},
		Summary:  fmt.Sprintf("Marked %d messages as read", intValueFromAny(result["updated_count"], 0)),
		Warnings: warnings,
	}, nil
}

func (s *Service) transportFor(record *identity.StoredIdentity) (Transport, []string, error) {
	mode := s.runtimeConfig()
	if mode.Mode == runtime.ModeWebSocket {
		return NewWSProxyTransport(s.resolved, record.IdentityName), nil, nil
	}
	transport, warnings, err := s.httpTransport(record)
	return transport, warnings, err
}

func (s *Service) httpTransport(record *identity.StoredIdentity) (*HTTPTransport, []string, error) {
	auth, err := newAuthContext(record, s.manager)
	if err != nil {
		return nil, nil, err
	}
	return NewHTTPTransport(s.resolved, auth, http.DefaultClient), nil, nil
}

func (s *Service) httpFallbackSend(ctx context.Context, record *identity.StoredIdentity, request SendRequest, targetDID string) (*directSendResult, []string, error) {
	transport, warnings, err := s.httpTransport(record)
	if err != nil {
		return nil, nil, err
	}
	request.Target = targetDID
	result, err := transport.SendDirect(ctx, request)
	return result, warnings, err
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
	return s.manager.Load(identityName)
}

func (s *Service) resolveTarget(ctx context.Context, target string) (string, string, error) {
	target = strings.TrimSpace(target)
	if strings.HasPrefix(target, "did:") {
		return target, "", nil
	}
	var lookup map[string]any
	if err := s.remote.RPCCall(ctx, "/user-service/handle/rpc", "lookup", map[string]any{"handle": target}, "", &lookup); err != nil {
		return "", "", err
	}
	did := stringFromAny(lookup["did"])
	if did == "" {
		return "", "", fmt.Errorf("%w: %s", ErrTargetRequired, target)
	}
	return did, target, nil
}

func (s *Service) persistSendResult(ctx context.Context, record *identity.StoredIdentity, targetDID string, targetHandle string, request SendRequest, result *directSendResult, warnings []string) (*CommandResult, error) {
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	if err := store.StoreMessage(ctx, db, store.MessageRecord{
		MsgID:          result.MessageID,
		OwnerDID:       record.DID,
		ThreadID:       store.MakeThreadID(record.DID, targetDID, ""),
		Direction:      1,
		SenderDID:      record.DID,
		ReceiverDID:    targetDID,
		ContentType:    contentTypeForMessageType(request.MessageType),
		Content:        request.Text,
		ServerSeq:      nil,
		SentAt:         result.AcceptedAt,
		IsRead:         true,
		Metadata:       metadataString(map[string]any{"delivery_state": result.DeliveryState, "operation_id": result.OperationID, "target_handle": targetHandle}),
		CredentialName: record.IdentityName,
	}); err != nil {
		warnings = append(warnings, fmt.Sprintf("Failed to persist local message: %v", err))
	}
	return &CommandResult{
		Data: map[string]any{
			"action": "send_message",
			"target": map[string]any{
				"did":    targetDID,
				"handle": targetHandle,
				"kind":   "direct",
			},
			"message": map[string]any{
				"id":      result.MessageID,
				"type":    request.MessageType,
				"secure":  false,
				"sent_at": result.AcceptedAt,
			},
			"delivery": result,
		},
		Summary:  fmt.Sprintf("Sent a direct %s message", request.MessageType),
		Warnings: warnings,
	}, nil
}

func (s *Service) persistInboxMessages(ctx context.Context, record *identity.StoredIdentity, raw map[string]any) ([]map[string]any, int) {
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return nil, 0
	}
	defer db.Close()
	_ = store.EnsureSchema(ctx, db)
	messages := messagesFromResult(raw["messages"])
	storable := make([]store.MessageRecord, 0, len(messages))
	for _, message := range messages {
		msgID := stringFromAny(message["id"])
		if msgID == "" {
			continue
		}
		senderDID := stringFromAny(message["sender_did"])
		receiverDID := stringFromAny(message["receiver_did"])
		peerDID := senderDID
		if peerDID == record.DID {
			peerDID = receiverDID
		}
		storable = append(storable, store.MessageRecord{
			MsgID:          msgID,
			OwnerDID:       record.DID,
			ThreadID:       store.MakeThreadID(record.DID, peerDID, ""),
			Direction:      0,
			SenderDID:      senderDID,
			ReceiverDID:    receiverDID,
			ContentType:    stringFromAny(message["content_type"]),
			Content:        stringFromAny(message["content"]),
			ServerSeq:      int64PtrFromAny(message["server_seq"]),
			SentAt:         stringFromAny(message["sent_at"]),
			IsRead:         boolFromAny(message["is_read"]),
			SenderName:     stringFromAny(message["sender_name"]),
			Metadata:       metadataString(message),
			CredentialName: record.IdentityName,
		})
	}
	_ = store.StoreMessagesBatch(ctx, db, storable)
	return messages, intValueFromAny(raw["total"], len(messages))
}

func (s *Service) persistHistoryMessages(ctx context.Context, record *identity.StoredIdentity, peerDID string, raw map[string]any) ([]map[string]any, int) {
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return nil, 0
	}
	defer db.Close()
	_ = store.EnsureSchema(ctx, db)
	messages := messagesFromResult(raw["messages"])
	storable := make([]store.MessageRecord, 0, len(messages))
	for _, message := range messages {
		msgID := stringFromAny(message["id"])
		if msgID == "" {
			continue
		}
		senderDID := stringFromAny(message["sender_did"])
		receiverDID := stringFromAny(message["receiver_did"])
		direction := 0
		if senderDID == record.DID {
			direction = 1
		}
		storable = append(storable, store.MessageRecord{
			MsgID:          msgID,
			OwnerDID:       record.DID,
			ThreadID:       store.MakeThreadID(record.DID, peerDID, ""),
			Direction:      direction,
			SenderDID:      senderDID,
			ReceiverDID:    receiverDID,
			ContentType:    stringFromAny(message["content_type"]),
			Content:        stringFromAny(message["content"]),
			ServerSeq:      int64PtrFromAny(message["server_seq"]),
			SentAt:         stringFromAny(message["sent_at"]),
			IsRead:         boolFromAny(message["is_read"]),
			SenderName:     stringFromAny(message["sender_name"]),
			Metadata:       metadataString(message),
			CredentialName: record.IdentityName,
		})
	}
	_ = store.StoreMessagesBatch(ctx, db, storable)
	return messages, intValueFromAny(raw["total"], len(messages))
}

func (s *Service) readInboxFromCache(ctx context.Context, record *identity.StoredIdentity, peerDID string, limit int, unreadOnly bool) ([]map[string]any, error) {
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	return store.ListInboxMessages(ctx, db, record.DID, limit, peerDID, unreadOnly)
}

func (s *Service) readHistoryFromCache(ctx context.Context, record *identity.StoredIdentity, peerDID string, limit int) ([]map[string]any, error) {
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	threadID := store.MakeThreadID(record.DID, peerDID, "")
	return store.ListThreadMessages(ctx, db, record.DID, threadID, limit)
}

func messagesFromResult(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]map[string]any); ok {
			return typed
		}
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if message, ok := item.(map[string]any); ok {
			result = append(result, message)
		}
	}
	return result
}

func collectMessageIDs(messages []map[string]any) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		if id := stringFromAny(message["id"]); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func contentTypeForMessageType(messageType string) string {
	switch strings.ToLower(strings.TrimSpace(messageType)) {
	case "", "text":
		return "text/plain"
	case "event":
		return "application/json"
	default:
		return "text/plain"
	}
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func generateOperationID() string {
	return strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000"), ".", "")
}

func metadataString(value any) string {
	if value == nil {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func peerHandleOrDid(handle string, did string) string {
	if handle != "" {
		return handle
	}
	return did
}

func sourceWithDefault(result map[string]any, mode string) string {
	if source := stringFromAny(result["source"]); source != "" {
		return source
	}
	if mode == runtime.ModeWebSocket {
		return "local_ws_cache"
	}
	return "remote_http"
}

func decodeMapInto(source map[string]any, destination *directSendResult) {
	if source == nil || destination == nil {
		return
	}
	raw, err := json.Marshal(source)
	if err != nil {
		return
	}
	_ = json.Unmarshal(raw, destination)
}

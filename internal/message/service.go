package message

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/agentconnect/awiki-cli/internal/authsdk"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/runtime"
	"github.com/agentconnect/awiki-cli/internal/store"
	"github.com/agentconnect/awiki-cli/internal/traceutil"
	"github.com/agentconnect/awiki-cli/internal/transportcfg"
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
	if request.HasAttachment() {
		if strings.TrimSpace(request.Group) != "" {
			return s.sendGroupAttachment(ctx, request)
		}
		return s.sendDirectAttachment(ctx, request)
	}
	if strings.TrimSpace(request.Group) != "" {
		return s.sendGroup(ctx, request)
	}
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
		if isSessionUnauthorized(err) {
			_ = s.refreshJWT(ctx, record)
		}
		if fallback, fallbackWarnings, fallbackErr := s.httpFallbackSend(ctx, record, request, targetDID); fallbackErr == nil {
			traceutil.MarkFallback(ctx, "websocket_to_http", err)
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
		return s.groupInbox(ctx, request)
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	if request.Scope == "all" {
		return s.allInbox(ctx, record, request)
	}
	originalWith := strings.TrimSpace(request.With)
	targetIsHandle := originalWith != "" && !strings.HasPrefix(originalWith, "did:")
	peerDID := ""
	peerHandle := ""
	if originalWith != "" {
		peerDID, peerHandle, err = s.resolveTarget(ctx, request.With)
		if err != nil {
			if targetIsHandle {
				peerHandle = normalizeHandleValue(originalWith)
				cachedDIDs, cacheErr := s.peerDIDsForHandleFromStore(ctx, record.DID, peerHandle, "")
				if cacheErr == nil && len(cachedDIDs) > 0 {
					cached, readErr := s.readInboxFromCacheByPeerDIDs(ctx, record, cachedDIDs, request.Limit, request.UnreadOnly)
					if readErr == nil {
						return &CommandResult{
							Data: map[string]any{
								"messages": cached,
								"total":    len(cached),
								"source":   "local_handle_history_cache",
								"with":     peerHandleOrDid(peerHandle, peerDID),
							},
							Summary: "Loaded inbox from local handle history cache",
						}, nil
					}
				}
			}
			return nil, err
		}
		peerHandle = normalizeHandleValue(peerHandle)
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
			wsErr := err
			cached, cacheErr := s.readInboxFromCache(ctx, record, peerDID, request.Limit, request.UnreadOnly)
			if targetIsHandle {
				cachedDIDs, didErr := s.peerDIDsForHandleFromStore(ctx, record.DID, peerHandle, peerDID)
				if didErr == nil && len(cachedDIDs) > 0 {
					cached, cacheErr = s.readInboxFromCacheByPeerDIDs(ctx, record, cachedDIDs, request.Limit, request.UnreadOnly)
				}
			}
			if cacheErr == nil && len(cached) > 0 {
				return &CommandResult{
					Data: map[string]any{
						"messages": cached,
						"total":    len(cached),
						"source":   "local_ws_cache_fallback",
						"with":     peerHandleOrDid(peerHandle, peerDID),
					},
					Summary:  "Loaded inbox from local websocket cache",
					Warnings: []string{websocketCacheFallbackWarning(wsErr)},
				}, nil
			}
			httpTransport, httpWarnings, httpErr := s.httpTransport(record)
			if httpErr != nil {
				return nil, err
			}
			raw, err = httpTransport.GetInbox(ctx, request)
			if err != nil {
				if isSessionUnauthorized(err) {
					if refreshErr := s.refreshJWT(ctx, record); refreshErr == nil {
						httpTransport, httpWarnings, httpErr = s.httpTransport(record)
						if httpErr != nil {
							return nil, httpErr
						}
						warnings = append(warnings, httpWarnings...)
						raw, err = httpTransport.GetInbox(ctx, request)
					}
				}
				if err != nil {
					return nil, err
				}
			}
			traceutil.MarkFallback(ctx, "websocket_to_http", wsErr)
			warnings = append(warnings, websocketHTTPFallbackWarning(wsErr))
			warnings = append(warnings, httpWarnings...)
		}
	default:
		httpTransport, httpWarnings, httpErr := s.httpTransport(record)
		if httpErr != nil {
			return nil, httpErr
		}
		raw, err = httpTransport.GetInbox(ctx, request)
		if err != nil {
			if isSessionUnauthorized(err) {
				if refreshErr := s.refreshJWT(ctx, record); refreshErr == nil {
					httpTransport, httpWarnings, httpErr = s.httpTransport(record)
					if httpErr != nil {
						return nil, httpErr
					}
					warnings = append(warnings, httpWarnings...)
					raw, err = httpTransport.GetInbox(ctx, request)
				}
			}
			if err != nil {
				return nil, err
			}
		}
		warnings = append(warnings, httpWarnings...)
	}
	messages, total, persistWarnings := s.persistInboxMessages(ctx, record, raw, peerHandle)
	warnings = append(warnings, persistWarnings...)
	if targetIsHandle {
		cachedDIDs, didErr := s.peerDIDsForHandleFromStore(ctx, record.DID, peerHandle, peerDID)
		if didErr != nil {
			warnings = append(warnings, fmt.Sprintf("Failed to expand handle history: %v", didErr))
		} else if len(cachedDIDs) > 0 {
			cached, cacheErr := s.readInboxFromCacheByPeerDIDs(ctx, record, cachedDIDs, request.Limit, request.UnreadOnly)
			if cacheErr == nil {
				messages = cached
				total = len(cached)
				raw["source"] = sourceWithDefault(raw, mode.Mode) + "+handle_history"
			} else {
				warnings = append(warnings, fmt.Sprintf("Failed to load handle history from cache: %v", cacheErr))
			}
		}
	}
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
	originalWith := strings.TrimSpace(request.With)
	targetIsHandle := originalWith != "" && !strings.HasPrefix(originalWith, "did:")
	peerDID, peerHandle, err := s.resolveTarget(ctx, request.With)
	if err != nil {
		if targetIsHandle {
			peerHandle = normalizeHandleValue(originalWith)
			cachedDIDs, cacheErr := s.peerDIDsForHandleFromStore(ctx, record.DID, peerHandle, "")
			if cacheErr == nil && len(cachedDIDs) > 0 {
				cached, readErr := s.readHistoryFromCacheByPeerDIDs(ctx, record, cachedDIDs, request.Limit)
				if readErr == nil {
					return &CommandResult{
						Data: map[string]any{
							"messages":      cached,
							"total":         len(cached),
							"source":        "local_handle_history_cache",
							"with":          peerHandle,
							"resolved_dids": cachedDIDs,
						},
						Summary: "Loaded history from local handle history cache",
					}, nil
				}
			}
		}
		return nil, err
	}
	peerHandle = normalizeHandleValue(peerHandle)
	request.With = peerDID

	mode := s.runtimeConfig()
	warnings := make([]string, 0)
	var raw map[string]any
	switch mode.Mode {
	case runtime.ModeWebSocket:
		transport := NewWSProxyTransport(s.resolved, record.IdentityName)
		raw, err = transport.GetHistory(ctx, request)
		if err != nil {
			wsErr := err
			cached, cacheErr := s.readHistoryFromCache(ctx, record, peerDID, request.Limit)
			if targetIsHandle {
				cachedDIDs, didErr := s.peerDIDsForHandleFromStore(ctx, record.DID, peerHandle, peerDID)
				if didErr == nil && len(cachedDIDs) > 0 {
					cached, cacheErr = s.readHistoryFromCacheByPeerDIDs(ctx, record, cachedDIDs, request.Limit)
				}
			}
			if cacheErr == nil && len(cached) > 0 {
				return &CommandResult{
					Data: map[string]any{
						"messages": cached,
						"total":    len(cached),
						"source":   "local_ws_cache_fallback",
						"with":     peerHandleOrDid(peerHandle, peerDID),
					},
					Summary:  "Loaded history from local websocket cache",
					Warnings: []string{websocketCacheFallbackWarning(wsErr)},
				}, nil
			}
			httpTransport, httpWarnings, httpErr := s.httpTransport(record)
			if httpErr != nil {
				return nil, err
			}
			raw, err = httpTransport.GetHistory(ctx, request)
			if err != nil {
				if isSessionUnauthorized(err) {
					if refreshErr := s.refreshJWT(ctx, record); refreshErr == nil {
						httpTransport, httpWarnings, httpErr = s.httpTransport(record)
						if httpErr != nil {
							return nil, httpErr
						}
						warnings = append(warnings, httpWarnings...)
						raw, err = httpTransport.GetHistory(ctx, request)
					}
				}
				if err != nil {
					return nil, err
				}
			}
			traceutil.MarkFallback(ctx, "websocket_to_http", wsErr)
			warnings = append(warnings, websocketHTTPFallbackWarning(wsErr))
			warnings = append(warnings, httpWarnings...)
		}
	default:
		httpTransport, httpWarnings, httpErr := s.httpTransport(record)
		if httpErr != nil {
			return nil, httpErr
		}
		raw, err = httpTransport.GetHistory(ctx, request)
		if err != nil {
			if isSessionUnauthorized(err) {
				if refreshErr := s.refreshJWT(ctx, record); refreshErr == nil {
					httpTransport, httpWarnings, httpErr = s.httpTransport(record)
					if httpErr != nil {
						return nil, httpErr
					}
					warnings = append(warnings, httpWarnings...)
					raw, err = httpTransport.GetHistory(ctx, request)
				}
			}
			if err != nil {
				return nil, err
			}
		}
		warnings = append(warnings, httpWarnings...)
	}
	messages, total, persistWarnings := s.persistHistoryMessages(ctx, record, peerDID, peerHandle, raw)
	warnings = append(warnings, persistWarnings...)
	if targetIsHandle {
		cachedDIDs, didErr := s.peerDIDsForHandleFromStore(ctx, record.DID, peerHandle, peerDID)
		if didErr != nil {
			warnings = append(warnings, fmt.Sprintf("Failed to expand handle history: %v", didErr))
		} else if len(cachedDIDs) > 0 {
			cached, cacheErr := s.readHistoryFromCacheByPeerDIDs(ctx, record, cachedDIDs, request.Limit)
			if cacheErr == nil {
				messages = cached
				total = len(cached)
				raw["source"] = sourceWithDefault(raw, mode.Mode) + "+handle_history"
				raw["resolved_dids"] = cachedDIDs
			} else {
				warnings = append(warnings, fmt.Sprintf("Failed to load handle history from cache: %v", cacheErr))
			}
		}
	}
	return &CommandResult{
		Data: map[string]any{
			"messages":      messages,
			"total":         total,
			"source":        sourceWithDefault(raw, mode.Mode),
			"with":          peerHandleOrDid(peerHandle, peerDID),
			"resolved_dids": raw["resolved_dids"],
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
	db, openErr := store.Open(s.resolved.Paths)
	if openErr == nil {
		defer db.Close()
		_ = store.EnsureSchema(ctx, db)
	}
	directIDs := make([]string, 0, len(request.MessageIDs))
	groupIDs := make([]string, 0, len(request.MessageIDs))
	localOnlyIDs := make([]string, 0, len(request.MessageIDs))
	if db != nil {
		rows, queryErr := store.ListMessagesByIDs(ctx, db, record.DID, request.MessageIDs)
		if queryErr == nil {
			known := make(map[string]map[string]any, len(rows))
			for _, row := range rows {
				known[stringFromAny(row["msg_id"])] = row
			}
			for _, id := range request.MessageIDs {
				row, ok := known[id]
				if ok && isLocalMailNotificationMessage(row) {
					localOnlyIDs = append(localOnlyIDs, id)
					continue
				}
				if ok && (stringFromAny(row["group_did"]) != "" || stringFromAny(row["group_id"]) != "") {
					groupIDs = append(groupIDs, id)
					continue
				}
				directIDs = append(directIDs, id)
			}
		} else {
			directIDs = append(directIDs, request.MessageIDs...)
		}
	} else {
		directIDs = append(directIDs, request.MessageIDs...)
	}
	warnings := make([]string, 0)
	updatedCount := 0
	if len(directIDs) > 0 {
		transport, transportWarnings, transportErr := s.transportFor(record)
		if transportErr != nil {
			return nil, transportErr
		}
		result, markErr := transport.MarkRead(ctx, MarkReadRequest{IdentityName: request.IdentityName, MessageIDs: directIDs})
		if markErr != nil {
			fallbackCause := markErr
			if isSessionUnauthorized(markErr) {
				if refreshErr := s.refreshJWT(ctx, record); refreshErr == nil {
					if httpTransport, httpWarnings, httpErr := s.httpTransport(record); httpErr == nil {
						result, markErr = httpTransport.MarkRead(ctx, MarkReadRequest{IdentityName: request.IdentityName, MessageIDs: directIDs})
						if markErr == nil {
							traceutil.MarkFallback(ctx, "websocket_to_http", fallbackCause)
							transportWarnings = append(transportWarnings, websocketHTTPFallbackWarning(fallbackCause))
							transportWarnings = append(transportWarnings, httpWarnings...)
						}
					}
				}
			} else {
				if httpTransport, httpWarnings, httpErr := s.httpTransport(record); httpErr == nil {
					result, markErr = httpTransport.MarkRead(ctx, MarkReadRequest{IdentityName: request.IdentityName, MessageIDs: directIDs})
					if markErr == nil {
						traceutil.MarkFallback(ctx, "websocket_to_http", fallbackCause)
						transportWarnings = append(transportWarnings, websocketHTTPFallbackWarning(fallbackCause))
						transportWarnings = append(transportWarnings, httpWarnings...)
					}
				}
			}
		}
		if markErr != nil {
			return nil, markErr
		}
		warnings = append(warnings, transportWarnings...)
		updatedCount += intValueFromAny(result["updated_count"], len(directIDs))
	}
	if db != nil {
		localIDs := append(append(append([]string{}, directIDs...), groupIDs...), localOnlyIDs...)
		if len(localIDs) > 0 {
			if count, markErr := store.MarkMessagesRead(ctx, db, record.DID, localIDs); markErr != nil {
				warnings = append(warnings, fmt.Sprintf("Failed to mark local messages read: %v", markErr))
			} else if updatedCount == 0 {
				updatedCount = int(count)
			} else {
				updatedCount += len(groupIDs) + len(localOnlyIDs)
			}
		}
	}
	return &CommandResult{
		Data: map[string]any{
			"action":        "mark_read",
			"updated_count": updatedCount,
			"message_ids":   request.MessageIDs,
		},
		Summary:  fmt.Sprintf("Marked %d messages as read", updatedCount),
		Warnings: compactWarnings(warnings),
	}, nil
}

func (s *Service) groupInbox(ctx context.Context, request InboxRequest) (*CommandResult, error) {
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	groupMessages, err := s.readGroupInboxFromCache(ctx, record, request.Group, request.Limit, request.UnreadOnly)
	if err != nil {
		return nil, err
	}
	if request.MarkRead {
		ids := collectMessageIDs(groupMessages)
		if len(ids) > 0 {
			_, _ = s.MarkRead(ctx, MarkReadRequest{IdentityName: request.IdentityName, MessageIDs: ids})
			for _, message := range groupMessages {
				message["is_read"] = true
			}
		}
	}
	return &CommandResult{
		Data: map[string]any{
			"messages": groupMessages,
			"total":    len(groupMessages),
			"source":   "local_group_cache",
			"group":    request.Group,
		},
		Summary:  fmt.Sprintf("Loaded %d group inbox messages", len(groupMessages)),
		Warnings: nil,
	}, nil
}

// isSessionUnauthorized reports whether a message-service error indicates an
// unauthorized or expired session. For HTTP transport, the primary 1401
// handling now lives inside HTTPTransport.rpcCall; this helper is kept as a
// secondary guard around higher-level fallbacks (e.g. websocket → HTTP).
func isSessionUnauthorized(err error) bool {
	if err == nil {
		return false
	}
	var serviceErr *ServiceError
	if errors.As(err, &serviceErr) {
		if serviceErr.StatusCode == http.StatusUnauthorized {
			return true
		}
		if serviceErr.RPCCode == 1401 {
			return true
		}
	}
	return false
}

// refreshJWT uses did-auth to obtain a new JWT for the given identity and
// persist it in the local identity store. For message-service RPCs, the
// first-line auto-refresh now happens inside HTTPTransport.rpcCall; this
// helper is only used as a fallback when the transport-level refresh fails
// and the caller decides to make one more attempt.
func (s *Service) refreshJWT(ctx context.Context, record *identity.StoredIdentity) error {
	if s == nil || record == nil || s.manager == nil || s.remote == nil || s.resolved == nil {
		return fmt.Errorf("refreshJWT: identity context is not available")
	}
	paths, err := s.manager.PathsForIdentity(record.IdentityName)
	if err != nil {
		return err
	}
	session := authsdk.NewSession(
		paths.DIDDocumentPath,
		paths.Key1PrivatePath,
		record.IdentityName,
		record.DID,
		record.JWTToken,
		func(token string) error { return s.manager.UpdateJWT(record.IdentityName, token) },
	)
	didAuthURL := appconfig.JoinBaseURL(s.resolved.ServiceBaseURL, "/user-service/did-auth/rpc")
	rememberAuthScopes(session, s.resolved)
	ctxWithTimeout, cancel := transportcfg.WithProfileTimeout(ctx, transportcfg.ProfileAuthRefresh)
	defer cancel()
	finish := traceutil.EnsureJWTPhase(ctx, "message_fallback_refresh")
	defer finish()
	if _, err := session.EnsureJWT(ctxWithTimeout, s.remote.Client(), didAuthURL); err != nil {
		return err
	}
	record.JWTToken = session.CurrentJWT()
	return nil
}

func (s *Service) allInbox(ctx context.Context, record *identity.StoredIdentity, request InboxRequest) (*CommandResult, error) {
	groupMessages, groupErr := s.readAllLocalGroupInbox(ctx, record, request.Limit, request.UnreadOnly)
	warnings := make([]string, 0)
	if groupErr != nil {
		warnings = append(warnings, fmt.Sprintf("Failed to read local group inbox cache: %v", groupErr))
	}
	directMessages := []map[string]any(nil)
	source := "local_direct_cache+local_group_cache"
	if s.runtimeConfig().Mode == runtime.ModeWebSocket {
		cachedDirect, directErr := s.readUnifiedDirectInboxFromCache(ctx, record, request.Limit, request.UnreadOnly)
		if directErr == nil {
			directMessages = normalizeMailNotificationMessages(cachedDirect)
		} else {
			warnings = append(warnings, fmt.Sprintf("Failed to read local direct inbox cache: %v", directErr))
		}
	}
	if directMessages == nil {
		directRequest := request
		directRequest.Scope = "direct"
		directRequest.Group = ""
		directResult, err := s.Inbox(ctx, directRequest)
		if err != nil {
			return nil, err
		}
		mailNotifications, mailErr := s.readAllLocalMailNotifications(ctx, record, request.Limit, request.UnreadOnly)
		if mailErr != nil {
			warnings = append(warnings, fmt.Sprintf("Failed to read local mail notification cache: %v", mailErr))
		}
		directMessages = normalizeMailNotificationMessages(messagesFromResult(directResult.Data["messages"]))
		directMessages = mergeInboxMessages(request.Limit, directMessages, normalizeMailNotificationMessages(mailNotifications))
		warnings = append(warnings, directResult.Warnings...)
		source = "remote_http+local_group_cache+local_mail_cache"
	}
	merged := mergeInboxMessages(request.Limit, directMessages, groupMessages)
	if request.MarkRead {
		ids := collectMessageIDs(merged)
		if len(ids) > 0 {
			_, _ = s.MarkRead(ctx, MarkReadRequest{IdentityName: request.IdentityName, MessageIDs: ids})
			for _, message := range merged {
				message["is_read"] = true
			}
		}
	}
	return &CommandResult{
		Data: map[string]any{
			"messages": merged,
			"total":    len(merged),
			"source":   source,
		},
		Summary:  fmt.Sprintf("Loaded %d inbox messages", len(merged)),
		Warnings: compactWarnings(warnings),
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
	if auth != nil && auth.session != nil {
		rememberAuthScopes(auth.session, s.resolved)
	}
	if auth != nil && auth.session != nil && strings.TrimSpace(record.JWTToken) != "" {
		token := strings.TrimSpace(record.JWTToken)
		auth.session.SetBearer(s.resolved.ServiceBaseURL, token)
		auth.session.SetBearer(
			appconfig.JoinBaseURL(s.resolved.ServiceBaseURL, "/user-service/did-auth/rpc"),
			token,
		)
		auth.session.SetBearer(
			appconfig.JoinBaseURL(s.resolved.ServiceBaseURL, MessageRPCEndpoint),
			token,
		)
		auth.session.SetBearer(s.resolved.ANPServiceEndpoint, token)
	}
	client := http.DefaultClient
	if s.remote != nil && s.remote.Client() != nil {
		client = s.remote.Client()
	}
	return NewHTTPTransport(s.resolved, auth, client), nil, nil
}

func rememberAuthScopes(session *authsdk.Session, resolved *appconfig.Resolved) {
	if session == nil || resolved == nil {
		return
	}
	session.RememberScope(resolved.ServiceBaseURL)
	session.RememberScope(appconfig.JoinBaseURL(resolved.ServiceBaseURL, "/user-service/did-auth/rpc"))
	session.RememberScope(appconfig.JoinBaseURL(resolved.ServiceBaseURL, MessageRPCEndpoint))
	session.RememberScope(resolved.ANPServiceEndpoint)
}

func (s *Service) groupControlTransport(record *identity.StoredIdentity) (*HTTPTransport, []string, error) {
	transport, warnings, err := s.httpTransport(record)
	if err != nil {
		return nil, nil, err
	}
	if s.runtimeConfig().Mode == runtime.ModeWebSocket {
		warnings = append(warnings, "Group lifecycle commands use HTTP transport even when runtime.mode is websocket.")
	}
	return transport, warnings, nil
}

func groupControlSource(result map[string]any) string {
	return sourceWithDefault(result, runtime.ModeHTTP)
}

func transportSource(mode string) string {
	return sourceWithDefault(nil, mode)
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

func (s *Service) resolveTarget(ctx context.Context, target string) (string, string, error) {
	target = strings.TrimSpace(target)
	if strings.HasPrefix(target, "did:") {
		return target, "", nil
	}
	finish := traceutil.HandleLookupPhase(ctx, "target_resolve")
	defer finish()
	var lookup map[string]any
	if err := s.remote.RPCCall(ctx, "/user-service/handle/rpc", "lookup", map[string]any{"handle": target}, "", &lookup); err != nil {
		return "", "", err
	}
	did := stringFromAny(lookup["did"])
	if did == "" {
		return "", "", fmt.Errorf("%w: %s", ErrTargetRequired, target)
	}
	resolvedHandle := normalizeHandleValue(defaultString(stringFromAny(lookup["handle"]), target))
	return did, resolvedHandle, nil
}

func (s *Service) persistSendResult(ctx context.Context, record *identity.StoredIdentity, targetDID string, targetHandle string, request SendRequest, result *directSendResult, warnings []string) (*CommandResult, error) {
	finish := traceutil.LocalDBPhase(ctx, "persist_direct_send")
	defer finish()
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

func (s *Service) persistInboxMessages(ctx context.Context, record *identity.StoredIdentity, raw map[string]any, knownHandle string) ([]map[string]any, int, []string) {
	finish := traceutil.LocalDBPhase(ctx, "persist_inbox_messages")
	defer finish()
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return nil, 0, nil
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
	contactFinish := traceutil.PhaseContext(ctx, "contact_sync")
	defer contactFinish()
	warnings := s.syncDirectPeerHandles(ctx, db, record.DID, messages, knownHandle, "msg.inbox")
	return messages, intValueFromAny(raw["total"], len(messages)), warnings
}

func (s *Service) persistHistoryMessages(ctx context.Context, record *identity.StoredIdentity, peerDID string, knownHandle string, raw map[string]any) ([]map[string]any, int, []string) {
	finish := traceutil.LocalDBPhase(ctx, "persist_history_messages")
	defer finish()
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return nil, 0, nil
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
	contactFinish := traceutil.PhaseContext(ctx, "contact_sync")
	defer contactFinish()
	warnings := s.syncDirectPeerHandles(ctx, db, record.DID, messages, knownHandle, "msg.history")
	return messages, intValueFromAny(raw["total"], len(messages)), warnings
}

func (s *Service) readInboxFromCache(ctx context.Context, record *identity.StoredIdentity, peerDID string, limit int, unreadOnly bool) ([]map[string]any, error) {
	finish := traceutil.LocalDBPhase(ctx, "read_inbox_cache")
	defer finish()
	return s.readInboxFromCacheWithOptions(ctx, record, peerDID, limit, unreadOnly, false)
}

func (s *Service) readUnifiedDirectInboxFromCache(ctx context.Context, record *identity.StoredIdentity, limit int, unreadOnly bool) ([]map[string]any, error) {
	finish := traceutil.LocalDBPhase(ctx, "read_unified_direct_inbox_cache")
	defer finish()
	return s.readInboxFromCacheWithOptions(ctx, record, "", limit, unreadOnly, true)
}

func (s *Service) readInboxFromCacheWithOptions(ctx context.Context, record *identity.StoredIdentity, peerDID string, limit int, unreadOnly bool, includeLocalNotifications bool) ([]map[string]any, error) {
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	return store.ListInboxMessages(ctx, db, record.DID, limit, peerDID, unreadOnly, includeLocalNotifications)
}

func (s *Service) readAllLocalMailNotifications(ctx context.Context, record *identity.StoredIdentity, limit int, unreadOnly bool) ([]map[string]any, error) {
	finish := traceutil.LocalDBPhase(ctx, "read_mail_notification_cache")
	defer finish()
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	return store.ListNotificationInboxMessages(ctx, db, record.DID, limit, unreadOnly)
}

func (s *Service) readHistoryFromCache(ctx context.Context, record *identity.StoredIdentity, peerDID string, limit int) ([]map[string]any, error) {
	finish := traceutil.LocalDBPhase(ctx, "read_history_cache")
	defer finish()
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

func (s *Service) readInboxFromCacheByPeerDIDs(ctx context.Context, record *identity.StoredIdentity, peerDIDs []string, limit int, unreadOnly bool) ([]map[string]any, error) {
	finish := traceutil.LocalDBPhase(ctx, "read_inbox_cache_by_peer_dids")
	defer finish()
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	return store.ListDirectMessagesByPeerDIDs(ctx, db, record.DID, peerDIDs, limit, unreadOnly, true)
}

func (s *Service) readHistoryFromCacheByPeerDIDs(ctx context.Context, record *identity.StoredIdentity, peerDIDs []string, limit int) ([]map[string]any, error) {
	finish := traceutil.LocalDBPhase(ctx, "read_history_cache_by_peer_dids")
	defer finish()
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	return store.ListDirectMessagesByPeerDIDs(ctx, db, record.DID, peerDIDs, limit, false, false)
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

func normalizeMailNotificationMessages(messages []map[string]any) []map[string]any {
	if len(messages) == 0 {
		return messages
	}
	normalized := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		normalized = append(normalized, normalizeMailNotificationMessage(message))
	}
	return normalized
}

func normalizeMailNotificationMessage(message map[string]any) map[string]any {
	if !isLocalMailNotificationMessage(message) {
		return message
	}
	metadata := parseMessageMetadata(message["metadata"])
	mailboxAddress := defaultString(stringFromAny(metadata["mailbox_address"]), stringFromAny(message["thread_id"]))
	if strings.HasPrefix(mailboxAddress, "mail:") {
		mailboxAddress = strings.TrimPrefix(mailboxAddress, "mail:")
	}
	subject := defaultString(stringFromAny(metadata["subject"]), stringFromAny(message["title"]))
	if strings.HasPrefix(subject, "[邮件] ") {
		subject = strings.TrimPrefix(subject, "[邮件] ")
	}
	if strings.TrimSpace(subject) == "" {
		subject = "(no subject)"
	}
	fromAddr := stringFromAny(metadata["from_addr"])
	preview := stringFromAny(metadata["preview"])
	hasAttachments := boolFromAny(metadata["has_attachments"])

	normalized := make(map[string]any, len(message))
	for key, value := range message {
		normalized[key] = value
	}
	normalized["source_kind"] = "mail"
	normalized["title"] = "[邮件] " + subject
	normalized["content"] = buildNormalizedMailNotificationContent(mailboxAddress, fromAddr, subject, preview, hasAttachments)
	return normalized
}

func isLocalMailNotificationMessage(message map[string]any) bool {
	if message == nil {
		return false
	}
	if strings.TrimSpace(stringFromAny(message["content_type"])) == "mail.notification" {
		return true
	}
	metadata := parseMessageMetadata(message["metadata"])
	return strings.TrimSpace(stringFromAny(metadata["source_kind"])) == "mail"
}

func parseMessageMetadata(value any) map[string]any {
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

func buildNormalizedMailNotificationContent(mailboxAddress string, fromAddr string, subject string, preview string, hasAttachments bool) string {
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
	case attachmentMessageType:
		return attachmentManifestContentType
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

func decodeMapInto(source map[string]any, destination any) {
	if source == nil || destination == nil {
		return
	}
	raw, err := json.Marshal(source)
	if err != nil {
		return
	}
	_ = json.Unmarshal(raw, destination)
}

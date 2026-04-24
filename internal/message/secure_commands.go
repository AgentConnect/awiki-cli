package message

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/store"
)

func (s *Service) SecureStatus(ctx context.Context, request SecureStatusRequest) (*CommandResult, error) {
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
	}
	sessions, err := s.listSecureSessions(record, peerDID)
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
	outboxRows, err := store.ListE2EEOutbox(ctx, db, record.DID, record.IdentityName, "")
	if err != nil {
		return nil, err
	}
	if peerDID != "" {
		filtered := make([]map[string]any, 0, len(outboxRows))
		for _, row := range outboxRows {
			if stringFromAny(row["peer_did"]) == peerDID {
				filtered = append(filtered, row)
			}
		}
		outboxRows = filtered
	}
	outboxSummary := map[string]int{}
	for _, row := range outboxRows {
		status := defaultString(stringFromAny(row["local_status"]), "unknown")
		outboxSummary[status]++
	}
	return &CommandResult{
		Data: map[string]any{
			"with":     peerHandleOrDid(peerHandle, peerDID),
			"sessions": sessions,
			"outbox": map[string]any{
				"total":     len(outboxRows),
				"by_status": outboxSummary,
				"records":   redactSecureOutboxRowsForStatus(outboxRows),
			},
		},
		Summary: fmt.Sprintf("Loaded %d secure session(s) and %d secure outbox record(s)", len(sessions), len(outboxRows)),
	}, nil
}

func (s *Service) SecureFailed(ctx context.Context, request SecureStatusRequest) (*CommandResult, error) {
	record, err := s.requireActiveIdentity(request.IdentityName)
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
	rows, err := store.ListE2EEOutbox(ctx, db, record.DID, record.IdentityName, "failed")
	if err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"failed": rows,
			"total":  len(rows),
		},
		Summary: fmt.Sprintf("Loaded %d failed secure outbox record(s)", len(rows)),
	}, nil
}

func (s *Service) SecureDrop(ctx context.Context, request SecureOutboxActionRequest) (*CommandResult, error) {
	record, err := s.requireActiveIdentity(request.IdentityName)
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
	if _, err := store.GetE2EEOutbox(ctx, db, request.OutboxID, record.DID, record.IdentityName); err != nil {
		return nil, err
	}
	if err := store.UpdateE2EEOutboxStatus(ctx, db, request.OutboxID, record.DID, record.IdentityName, "dropped"); err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"outbox_id": request.OutboxID,
			"status":    "dropped",
		},
		Summary: fmt.Sprintf("Dropped secure outbox record %s", request.OutboxID),
	}, nil
}

func (s *Service) SecureRetry(ctx context.Context, request SecureOutboxActionRequest) (*CommandResult, error) {
	record, err := s.requireActiveIdentity(request.IdentityName)
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
	row, err := store.GetE2EEOutbox(ctx, db, request.OutboxID, record.DID, record.IdentityName)
	if err != nil {
		return nil, err
	}
	if err := store.UpdateE2EEOutboxStatus(ctx, db, request.OutboxID, record.DID, record.IdentityName, "queued"); err != nil {
		return nil, err
	}
	warnings := FlushQueuedSecureOutbox(ctx, s.resolved, s.manager, record, stringFromAny(row["peer_did"]), func(method string, params map[string]any) (map[string]any, error) {
		transport, _, err := s.httpTransport(record)
		if err != nil {
			return nil, err
		}
		return transport.rpcMapCall(ctx, method, params)
	})
	row, _ = store.GetE2EEOutbox(ctx, db, request.OutboxID, record.DID, record.IdentityName)
	return &CommandResult{
		Data: map[string]any{
			"outbox_id": request.OutboxID,
			"record":    row,
		},
		Summary:  fmt.Sprintf("Retried secure outbox record %s", request.OutboxID),
		Warnings: warnings,
	}, nil
}

func (s *Service) SecureInit(ctx context.Context, request SecurePeerRequest) (*CommandResult, error) {
	record, peerDID, peerHandle, warnings, err := s.prepareSecurePeerAction(ctx, request)
	if err != nil {
		return nil, err
	}
	session, ok, err := s.loadSecureSessionState(record, peerDID)
	if err != nil {
		return nil, err
	}
	if ok {
		return &CommandResult{
			Data: map[string]any{
				"target":  map[string]any{"did": peerDID, "handle": peerHandle, "kind": "direct"},
				"session": session,
				"reused":  true,
			},
			Summary:  fmt.Sprintf("Secure session already exists for %s", peerHandleOrDid(peerHandle, peerDID)),
			Warnings: compactWarnings(warnings),
		}, nil
	}
	transport, httpWarnings, err := s.httpTransport(record)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, httpWarnings...)
	client, err := s.secureE2EEClient(ctx, record, transport)
	if err != nil {
		return nil, err
	}
	messageID := "secure-init-" + generateOperationID()
	resultMap, err := client.SendJSON(ctx, peerDID, BuildSecureInitPayload(), messageID, messageID)
	if err != nil {
		return nil, err
	}
	session, _, _ = s.loadSecureSessionState(record, peerDID)
	return &CommandResult{
		Data: map[string]any{
			"target":  map[string]any{"did": peerDID, "handle": peerHandle, "kind": "direct"},
			"session": session,
			"delivery": map[string]any{
				"message_id":   defaultString(stringFromAny(resultMap["message_id"]), messageID),
				"operation_id": defaultString(stringFromAny(resultMap["operation_id"]), messageID),
				"target_did":   defaultString(stringFromAny(resultMap["target_did"]), peerDID),
			},
			"initialized": true,
		},
		Summary:  fmt.Sprintf("Initialized secure session with %s", peerHandleOrDid(peerHandle, peerDID)),
		Warnings: compactWarnings(warnings),
	}, nil
}

func (s *Service) SecureRepair(ctx context.Context, request SecurePeerRequest) (*CommandResult, error) {
	record, peerDID, peerHandle, warnings, err := s.prepareSecurePeerAction(ctx, request)
	if err != nil {
		return nil, err
	}
	resetCount, err := s.resetSecurePeerState(ctx, record, peerDID)
	if err != nil {
		return nil, err
	}
	initResult, err := s.SecureInit(ctx, request)
	if err != nil {
		return nil, err
	}
	initResult.Data["repair"] = map[string]any{
		"peer_did":      peerDID,
		"peer_handle":   peerHandle,
		"reset_records": resetCount,
	}
	initResult.Summary = fmt.Sprintf("Repaired secure session with %s", peerHandleOrDid(peerHandle, peerDID))
	initResult.Warnings = compactWarnings(append(warnings, initResult.Warnings...))
	return initResult, nil
}

func (s *Service) listSecureSessions(record *identity.StoredIdentity, peerDID string) ([]map[string]any, error) {
	paths, err := s.manager.PathsForIdentity(record.IdentityName)
	if err != nil {
		return nil, err
	}
	root := filepath.Join(paths.IdentityDir, "p5-e2ee-sessions")
	entries, err := filepath.Glob(filepath.Join(root, "*.json"))
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(entries))
	for _, path := range entries {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var session map[string]any
		if err := json.Unmarshal(raw, &session); err != nil {
			return nil, err
		}
		if peerDID != "" && stringFromAny(session["peer_did"]) != peerDID {
			continue
		}
		result = append(result, redactSecureSessionForStatus(session))
	}
	sort.Slice(result, func(i, j int) bool {
		return stringFromAny(result[i]["peer_did"]) < stringFromAny(result[j]["peer_did"])
	})
	return result, nil
}

func redactSecureSessionForStatus(session map[string]any) map[string]any {
	summary := map[string]any{
		"session_id":                 stringFromAny(session["session_id"]),
		"suite":                      stringFromAny(session["suite"]),
		"peer_did":                   stringFromAny(session["peer_did"]),
		"status":                     stringFromAny(session["status"]),
		"is_initiator":               boolFromAny(session["is_initiator"]),
		"send_n":                     intValueFromAny(session["send_n"], 0),
		"recv_n":                     intValueFromAny(session["recv_n"], 0),
		"previous_send_chain_length": intValueFromAny(session["previous_send_chain_length"], 0),
		"skipped_key_count":          countStatusArrayItems(session["skipped_message_keys"]),
	}
	return summary
}

func countStatusArrayItems(value any) int {
	switch typed := value.(type) {
	case []any:
		return len(typed)
	case []map[string]any:
		return len(typed)
	default:
		return 0
	}
}

func redactSecureOutboxRowsForStatus(rows []map[string]any) []map[string]any {
	redacted := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		redacted = append(redacted, map[string]any{
			"outbox_id":         stringFromAny(row["outbox_id"]),
			"peer_did":          stringFromAny(row["peer_did"]),
			"session_id":        stringFromAny(row["session_id"]),
			"original_type":     stringFromAny(row["original_type"]),
			"local_status":      stringFromAny(row["local_status"]),
			"attempt_count":     intValueFromAny(row["attempt_count"], 0),
			"sent_msg_id":       stringFromAny(row["sent_msg_id"]),
			"sent_server_seq":   row["sent_server_seq"],
			"last_error_code":   stringFromAny(row["last_error_code"]),
			"retry_hint":        stringFromAny(row["retry_hint"]),
			"failed_msg_id":     stringFromAny(row["failed_msg_id"]),
			"failed_server_seq": row["failed_server_seq"],
			"last_attempt_at":   stringFromAny(row["last_attempt_at"]),
			"created_at":        stringFromAny(row["created_at"]),
			"updated_at":        stringFromAny(row["updated_at"]),
		})
	}
	return redacted
}

func (s *Service) prepareSecurePeerAction(ctx context.Context, request SecurePeerRequest) (*identity.StoredIdentity, string, string, []string, error) {
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, "", "", nil, err
	}
	if record.E2EEAgreementPrivatePEM == "" || record.Key1PrivatePEM == "" {
		return nil, "", "", nil, fmt.Errorf("secure direct messaging requires DID signing and X25519 E2EE private keys")
	}
	if strings.TrimSpace(request.With) == "" {
		return nil, "", "", nil, ErrTargetRequired
	}
	peerDID, peerHandle, err := s.resolveTarget(ctx, request.With)
	if err != nil {
		return nil, "", "", nil, err
	}
	return record, peerDID, peerHandle, s.maybePublishSecurePrekeys(ctx, record), nil
}

func (s *Service) loadSecureSessionState(record *identity.StoredIdentity, peerDID string) (map[string]any, bool, error) {
	sessions, err := s.listSecureSessions(record, peerDID)
	if err != nil {
		return nil, false, err
	}
	if len(sessions) == 0 {
		return nil, false, nil
	}
	return sessions[0], true, nil
}

func (s *Service) resetSecurePeerState(ctx context.Context, record *identity.StoredIdentity, peerDID string) (int, error) {
	paths, err := s.manager.PathsForIdentity(record.IdentityName)
	if err != nil {
		return 0, err
	}
	sessionStore, err := anpsdk.NewFileSessionStore(filepath.Join(paths.IdentityDir, "p5-e2ee-sessions"))
	if err != nil {
		return 0, err
	}
	session, ok, err := sessionStore.FindByPeerDID(peerDID)
	if err != nil {
		return 0, err
	}
	resetCount := 0
	if ok {
		if err := sessionStore.DeleteSession(session.SessionID); err != nil {
			return 0, err
		}
		resetCount++
	}
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return resetCount, err
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return resetCount, err
	}
	rows, err := store.ListE2EEOutbox(ctx, db, record.DID, record.IdentityName, "failed")
	if err != nil {
		return resetCount, err
	}
	for _, row := range rows {
		if stringFromAny(row["peer_did"]) != peerDID {
			continue
		}
		if err := store.UpdateE2EEOutboxStatus(ctx, db, stringFromAny(row["outbox_id"]), record.DID, record.IdentityName, "queued"); err != nil {
			return resetCount, err
		}
		resetCount++
	}
	return resetCount, nil
}

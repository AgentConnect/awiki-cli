package message

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/store"
)

const secureAckSystemType = "awiki.direct.secure_ack.v1"
const secureInitSystemType = "awiki.direct.secure_init.v1"

func BuildSecureAckPayload(sessionID string, ackedMessageID string) map[string]any {
	return map[string]any{
		"system_type":      secureAckSystemType,
		"session_id":       strings.TrimSpace(sessionID),
		"acked_message_id": strings.TrimSpace(ackedMessageID),
	}
}

func BuildSecureInitPayload() map[string]any {
	return map[string]any{
		"system_type": secureInitSystemType,
		"reason":      "manual_init",
	}
}

func IsSecureAckPlaintext(plaintext map[string]any) bool {
	if stringFromAny(plaintext["application_content_type"]) != "application/json" {
		return false
	}
	payload, err := mapFromAny(plaintext["payload"])
	if err != nil {
		return false
	}
	return stringFromAny(payload["system_type"]) == secureAckSystemType
}

func IsSecureInitPlaintext(plaintext map[string]any) bool {
	if stringFromAny(plaintext["application_content_type"]) != "application/json" {
		return false
	}
	payload, err := mapFromAny(plaintext["payload"])
	if err != nil {
		return false
	}
	return stringFromAny(payload["system_type"]) == secureInitSystemType
}

func secureAckSessionID(plaintext map[string]any) string {
	payload, err := mapFromAny(plaintext["payload"])
	if err != nil {
		return ""
	}
	return stringFromAny(payload["session_id"])
}

func isPendingConfirmationError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "pending confirmation") || strings.Contains(message, "pending-confirmation")
}

func queueSecureOutboxRecord(ctx context.Context, resolved *appconfig.Resolved, manager *identity.Manager, record *identity.StoredIdentity, peerDID string, originalType string, plaintext string) (string, error) {
	if record == nil {
		return "", fmt.Errorf("identity record is required")
	}
	db, err := store.Open(resolved.Paths)
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return "", err
	}
	return store.QueueE2EEOutbox(ctx, db, store.E2EEOutboxRecord{
		OwnerDID:       record.DID,
		PeerDID:        peerDID,
		SessionID:      currentSecureSessionID(manager, record, peerDID),
		OriginalType:   defaultString(originalType, "text"),
		Plaintext:      plaintext,
		LocalStatus:    "queued",
		CredentialName: record.IdentityName,
		Metadata:       metadataString(map[string]any{"reason": "pending_confirmation"}),
	})
}

func currentSecureSessionID(manager *identity.Manager, record *identity.StoredIdentity, peerDID string) string {
	if manager == nil || record == nil {
		return ""
	}
	paths, err := manager.PathsForIdentity(record.IdentityName)
	if err != nil {
		return ""
	}
	sessionStore, err := anpsdk.NewFileSessionStore(filepath.Join(paths.IdentityDir, "p5-e2ee-sessions"))
	if err != nil {
		return ""
	}
	session, ok, err := sessionStore.FindByPeerDID(peerDID)
	if err != nil || !ok {
		return ""
	}
	return strings.TrimSpace(session.SessionID)
}

func FlushQueuedSecureOutbox(ctx context.Context, resolved *appconfig.Resolved, manager *identity.Manager, record *identity.StoredIdentity, peerDID string, rpc func(string, map[string]any) (map[string]any, error)) []string {
	if record == nil {
		return nil
	}
	db, err := store.Open(resolved.Paths)
	if err != nil {
		return compactWarnings([]string{fmt.Sprintf("Failed to open secure outbox store: %v", err)})
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return compactWarnings([]string{fmt.Sprintf("Failed to ensure secure outbox schema: %v", err)})
	}
	rows, err := store.ListE2EEOutbox(ctx, db, record.DID, record.IdentityName, "queued")
	if err != nil {
		return compactWarnings([]string{fmt.Sprintf("Failed to list secure outbox: %v", err)})
	}
	if len(rows) == 0 {
		return nil
	}
	client, err := NewSecureE2EEClientForRecord(ctx, manager, record, rpc)
	if err != nil {
		return compactWarnings([]string{fmt.Sprintf("Failed to initialize secure outbox sender: %v", err)})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return stringFromAny(rows[i]["created_at"]) < stringFromAny(rows[j]["created_at"])
	})
	warnings := make([]string, 0)
	for _, row := range rows {
		if strings.TrimSpace(peerDID) != "" && stringFromAny(row["peer_did"]) != strings.TrimSpace(peerDID) {
			continue
		}
		outboxID := stringFromAny(row["outbox_id"])
		targetDID := stringFromAny(row["peer_did"])
		originalType := defaultString(stringFromAny(row["original_type"]), "text")
		plaintext := stringFromAny(row["plaintext"])
		if outboxID == "" || targetDID == "" {
			continue
		}
		var resultMap map[string]any
		switch originalType {
		case "text", "":
			resultMap, err = client.SendText(ctx, targetDID, plaintext, outboxID, outboxID)
		case "json":
			var payload map[string]any
			if err := json.Unmarshal([]byte(plaintext), &payload); err != nil {
				_ = store.SetE2EEOutboxFailureByID(ctx, db, outboxID, record.DID, record.IdentityName, "invalid_payload", "drop", metadataString(map[string]any{"detail": err.Error()}))
				warnings = append(warnings, fmt.Sprintf("Failed to parse queued secure JSON payload %s: %v", outboxID, err))
				continue
			}
			resultMap, err = client.SendJSON(ctx, targetDID, payload, outboxID, outboxID)
		default:
			_ = store.SetE2EEOutboxFailureByID(ctx, db, outboxID, record.DID, record.IdentityName, "unsupported_original_type", "drop", metadataString(map[string]any{"original_type": originalType}))
			warnings = append(warnings, fmt.Sprintf("Queued secure outbox %s uses unsupported original_type=%s", outboxID, originalType))
			continue
		}
		if err != nil {
			_ = store.SetE2EEOutboxFailureByID(ctx, db, outboxID, record.DID, record.IdentityName, "send_failed", "retry", metadataString(map[string]any{"detail": err.Error()}))
			warnings = append(warnings, fmt.Sprintf("Failed to flush queued secure outbox %s: %v", outboxID, err))
			continue
		}
		sessionID := currentSecureSessionID(manager, record, targetDID)
		sentMsgID := stringFromAny(resultMap["message_id"])
		if sentMsgID == "" {
			sentMsgID = outboxID
		}
		metadata := metadataString(map[string]any{
			"target_did":     targetDID,
			"operation_id":   stringFromAny(resultMap["operation_id"]),
			"delivery_state": stringFromAny(resultMap["delivery_state"]),
			"flushed_from":   "queued",
		})
		if err := store.MarkE2EEOutboxSent(ctx, db, outboxID, record.DID, sessionID, sentMsgID, nil, metadata); err != nil {
			warnings = append(warnings, fmt.Sprintf("Failed to mark secure outbox %s sent: %v", outboxID, err))
			continue
		}
		if err := store.StoreMessage(ctx, db, store.MessageRecord{
			MsgID:          sentMsgID,
			OwnerDID:       record.DID,
			ThreadID:       store.MakeThreadID(record.DID, targetDID, ""),
			Direction:      1,
			SenderDID:      record.DID,
			ReceiverDID:    targetDID,
			ContentType:    contentTypeForMessageType(originalType),
			Content:        plaintext,
			SentAt:         stringFromAny(resultMap["accepted_at"]),
			IsRead:         true,
			IsE2EE:         true,
			Metadata:       metadata,
			CredentialName: record.IdentityName,
		}); err != nil {
			warnings = append(warnings, fmt.Sprintf("Failed to persist flushed secure outbox %s: %v", outboxID, err))
		}
	}
	return compactWarnings(warnings)
}

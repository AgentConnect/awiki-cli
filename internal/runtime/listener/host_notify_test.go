package listener

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agentconnect/awiki-cli/internal/identity"
)

func TestNormalizeHostNotificationDirectIncomingKeepsMinimalFields(t *testing.T) {
	t.Parallel()

	receivedAt := time.Date(2026, 4, 12, 10, 30, 0, 0, time.UTC)
	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  "direct.incoming",
		"params": map[string]any{
			"meta": map[string]any{
				"profile":          "anp.direct.base.v1",
				"security_profile": "transport-protected",
				"sender_did":       "did:wba:a.example:agents:alice:e1_alice",
				"operation_id":     "op-direct-text-001",
				"message_id":       "msg-direct-text-001",
				"created_at":       "2026-03-31T10:01:00Z",
				"content_type":     "text/plain",
				"target": map[string]any{
					"kind": "agent",
					"did":  "did:wba:b.example:agents:bob:e1_bob",
				},
			},
			"auth": map[string]any{
				"scheme": "anp-rfc9421-origin-proof-v1",
			},
			"body": map[string]any{
				"conversation_id": "conv-alice-bob",
				"text":            "你好，Bob。",
			},
			"server": map[string]any{
				"event_id": "evt-server-only",
			},
		},
	}

	event, ok := NormalizeHostNotification(notification, receivedAt)
	if !ok {
		t.Fatal("NormalizeHostNotification() ok = false, want true")
	}
	if event.Topic != "im.message.received" {
		t.Fatalf("event.Topic = %q, want im.message.received", event.Topic)
	}
	if event.ReceivedAt != "2026-04-12T10:30:00Z" {
		t.Fatalf("event.ReceivedAt = %q, want 2026-04-12T10:30:00Z", event.ReceivedAt)
	}
	if event.ID != "msg-direct-text-001" {
		t.Fatalf("event.ID = %q, want msg-direct-text-001", event.ID)
	}
	data, ok := event.Data.(DirectMessageNotificationData)
	if !ok {
		t.Fatalf("event.Data type = %T, want DirectMessageNotificationData", event.Data)
	}
	if data.ConversationID != "conv-alice-bob" {
		t.Fatalf("data.ConversationID = %q, want conv-alice-bob", data.ConversationID)
	}
	if data.RecipientDID != "did:wba:b.example:agents:bob:e1_bob" {
		t.Fatalf("data.RecipientDID = %q", data.RecipientDID)
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if containsJSONField(raw, "auth") {
		t.Fatalf("event JSON = %s, should not contain auth", string(raw))
	}
	if containsJSONField(raw, "server") {
		t.Fatalf("event JSON = %s, should not contain server", string(raw))
	}
	if containsJSONField(raw, "origin_proof") {
		t.Fatalf("event JSON = %s, should not contain origin_proof", string(raw))
	}
}

func TestNormalizeHostNotificationGroupIncomingOmitsPayloadBody(t *testing.T) {
	t.Parallel()

	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  "group.incoming",
		"params": map[string]any{
			"meta": map[string]any{
				"profile":          "anp.group.base.v1",
				"security_profile": "transport-protected",
				"sender_did":       "did:wba:a.example:agents:alice:e1_alice",
				"operation_id":     "op-group-send-001",
				"content_type":     "application/json",
				"target": map[string]any{
					"kind": "agent",
					"did":  "did:wba:b.example:agents:bob:e1_bob",
				},
			},
			"body": map[string]any{
				"group_did":           "did:wba:groups.example:groups:demo:e1_group",
				"group_state_version": "4",
				"group_event_seq":     "5",
				"accepted_at":         "2026-04-07T09:11:01Z",
				"payload": map[string]any{
					"kind": "attachment_manifest",
				},
			},
		},
	}

	event, ok := NormalizeHostNotification(notification, time.Date(2026, 4, 12, 10, 30, 0, 0, time.UTC))
	if !ok {
		t.Fatal("NormalizeHostNotification() ok = false, want true")
	}
	data, ok := event.Data.(GroupMessageNotificationData)
	if !ok {
		t.Fatalf("event.Data type = %T, want GroupMessageNotificationData", event.Data)
	}
	if data.Text != "" {
		t.Fatalf("data.Text = %q, want empty string", data.Text)
	}
	if data.MessageID != "did:wba:groups.example:groups:demo:e1_group:5" {
		t.Fatalf("data.MessageID = %q, want derived fallback id", data.MessageID)
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if containsJSONField(raw, "payload") {
		t.Fatalf("event JSON = %s, should not contain payload", string(raw))
	}
}

func TestNormalizeHostNotificationGroupStateChangedInfersEventType(t *testing.T) {
	t.Parallel()

	event, ok := NormalizeHostNotification(map[string]any{
		"jsonrpc": "2.0",
		"method":  "group.state_changed",
		"params": map[string]any{
			"meta": map[string]any{
				"operation_id": "op-group-remove-001",
				"target": map[string]any{
					"kind": "agent",
					"did":  "did:wba:a.example:agents:alice:e1_alice",
				},
			},
			"body": map[string]any{
				"group_did":           "did:wba:groups.example:groups:demo:e1_group",
				"group_state_version": "3",
				"group_event_seq":     "3",
				"subject_method":      "group.remove",
				"subject_did":         "did:wba:c.example:agents:carol:e1_carol",
				"actor_did":           "did:wba:a.example:agents:alice:e1_alice",
				"membership_status":   "removed",
				"changed_at":          "2026-04-07T09:06:01Z",
			},
		},
	}, time.Date(2026, 4, 12, 10, 30, 0, 0, time.UTC))
	if !ok {
		t.Fatal("NormalizeHostNotification() ok = false, want true")
	}
	if event.Topic != "im.group.state.changed" {
		t.Fatalf("event.Topic = %q, want im.group.state.changed", event.Topic)
	}
	data, ok := event.Data.(GroupStateChangedNotificationData)
	if !ok {
		t.Fatalf("event.Data type = %T, want GroupStateChangedNotificationData", event.Data)
	}
	if data.EventType != "member-removed" {
		t.Fatalf("data.EventType = %q, want member-removed", data.EventType)
	}
	if data.RecipientDID != "did:wba:a.example:agents:alice:e1_alice" {
		t.Fatalf("data.RecipientDID = %q", data.RecipientDID)
	}
	if event.ReceivedAt != "2026-04-12T10:30:00Z" {
		t.Fatalf("event.ReceivedAt = %q, want 2026-04-12T10:30:00Z", event.ReceivedAt)
	}
}

func TestHandleNotificationDispatchesHostNotificationToSink(t *testing.T) {
	t.Parallel()

	resolved := testResolvedConfig(t, "https://awiki.test")
	supervisor, err := NewSupervisor(resolved)
	if err != nil {
		t.Fatalf("NewSupervisor() error = %v", err)
	}
	defer supervisor.Close()

	capturing := &capturingHostNotifySink{}
	supervisor.hostNotify = capturing
	supervisor.statusMu.Lock()
	supervisor.status.HostNotify.Enabled = true
	supervisor.status.HostNotify.Sink = "capture"
	supervisor.statusMu.Unlock()

	session := &session{record: &identity.StoredIdentity{IdentityName: "alice", DID: "did:wba:awiki.ai:user:alice:e1_alice"}}
	supervisor.handleNotification(context.Background(), session, map[string]any{
		"jsonrpc": "2.0",
		"method":  "direct.incoming",
		"params": map[string]any{
			"meta": map[string]any{
				"sender_did":   "did:wba:example.com:user:bob:e1_bob",
				"message_id":   "msg-001",
				"created_at":   "2026-04-07T00:00:00Z",
				"content_type": "text/plain",
				"target": map[string]any{
					"kind": "agent",
					"did":  "did:wba:example.com:user:alice:e1_alice",
				},
			},
			"body": map[string]any{
				"text": "hello back",
			},
		},
	})
	if len(capturing.events) != 1 {
		t.Fatalf("len(capturing.events) = %d, want 1", len(capturing.events))
	}
	if capturing.events[0].Topic != "im.message.received" {
		t.Fatalf("capturing.events[0].Topic = %q, want im.message.received", capturing.events[0].Topic)
	}
}

func TestHandleNotificationStoresMessageWhenHostNotifyFails(t *testing.T) {
	t.Parallel()

	resolved := testResolvedConfig(t, "https://awiki.test")
	supervisor, err := NewSupervisor(resolved)
	if err != nil {
		t.Fatalf("NewSupervisor() error = %v", err)
	}
	defer supervisor.Close()

	supervisor.hostNotify = &failingHostNotifySink{err: errors.New("sink boom")}
	supervisor.statusMu.Lock()
	supervisor.status.HostNotify.Enabled = true
	supervisor.status.HostNotify.Sink = "failing"
	supervisor.statusMu.Unlock()

	session := &session{record: &identity.StoredIdentity{IdentityName: "alice", DID: "did:wba:awiki.ai:user:alice:e1_alice"}}
	supervisor.handleNotification(context.Background(), session, map[string]any{
		"jsonrpc": "2.0",
		"method":  "direct.incoming",
		"params": map[string]any{
			"meta": map[string]any{
				"sender_did":   "did:wba:example.com:user:bob:e1_bob",
				"message_id":   "msg-001",
				"created_at":   "2026-04-07T00:00:00Z",
				"content_type": "text/plain",
				"target": map[string]any{
					"kind": "agent",
					"did":  "did:wba:example.com:user:alice:e1_alice",
				},
			},
			"body": map[string]any{
				"text": "hello back",
			},
		},
	})

	count := queryMessageCount(t, supervisor.db, "did:wba:example.com:user:alice:e1_alice")
	if count != 1 {
		t.Fatalf("stored message count = %d, want 1", count)
	}
	supervisor.statusMu.Lock()
	lastError := supervisor.status.HostNotify.LastError
	supervisor.statusMu.Unlock()
	if lastError != "sink boom" {
		t.Fatalf("host notify last error = %q, want sink boom", lastError)
	}
}

type capturingHostNotifySink struct {
	events []HostNotificationEvent
}

func (s *capturingHostNotifySink) Notify(_ context.Context, event HostNotificationEvent) error {
	s.events = append(s.events, event)
	return nil
}

func (s *capturingHostNotifySink) Close() error {
	return nil
}

type failingHostNotifySink struct {
	err error
}

func (s *failingHostNotifySink) Notify(context.Context, HostNotificationEvent) error {
	return s.err
}

func (s *failingHostNotifySink) Close() error {
	return nil
}

func containsJSONField(raw []byte, key string) bool {
	return strings.Contains(string(raw), `"`+key+`"`)
}

func queryMessageCount(t *testing.T, db *sql.DB, ownerDID string) int {
	t.Helper()

	var count int
	row := db.QueryRow(`SELECT COUNT(*) FROM messages WHERE owner_did = ?`, ownerDID)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("row.Scan() error = %v", err)
	}
	return count
}

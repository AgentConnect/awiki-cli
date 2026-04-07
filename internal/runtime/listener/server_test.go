package listener

import (
	"strings"
	"testing"
)

func TestMessageRecordFromDirectIncomingUsesProtocolFieldsOnly(t *testing.T) {
	t.Parallel()

	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  "direct.incoming",
		"params": map[string]any{
			"meta": map[string]any{
				"sender_did":   "did:wba:example.com:user:bob:e1_yyy",
				"message_id":   "msg-001",
				"created_at":   "2026-04-07T00:00:00Z",
				"content_type": "text/plain",
				"target": map[string]any{
					"kind": "agent",
					"did":  "did:wba:example.com:user:alice:e1_xxx",
				},
			},
			"auth": map[string]any{
				"scheme": "anp-rfc9421-origin-proof-v1",
				"sender_proof": map[string]any{
					"contentDigest":  "sha-256=:digest:",
					"signatureInput": "sig1=(\"@method\");created=1;keyid=\"did:wba:example.com:user:bob:e1_yyy#key-1\"",
					"signature":      "sig1=:signature:",
				},
			},
			"body": map[string]any{
				"text": "hello back",
			},
		},
	}

	record, ok := messageRecordFromDirectIncoming(notification, "alice")
	if !ok {
		t.Fatalf("messageRecordFromDirectIncoming() ok = false, want true")
	}
	if record.OwnerDID != "did:wba:example.com:user:alice:e1_xxx" {
		t.Fatalf("record.OwnerDID = %q", record.OwnerDID)
	}
	if record.SenderDID != "did:wba:example.com:user:bob:e1_yyy" {
		t.Fatalf("record.SenderDID = %q", record.SenderDID)
	}
	if record.SentAt != "2026-04-07T00:00:00Z" {
		t.Fatalf("record.SentAt = %q", record.SentAt)
	}
	if !strings.Contains(record.Metadata, "anp-rfc9421-origin-proof-v1") {
		t.Fatalf("record.Metadata = %q, want auth payload", record.Metadata)
	}
	if strings.Contains(record.Metadata, "\"server\"") {
		t.Fatalf("record.Metadata = %q, should not depend on server wrapper", record.Metadata)
	}
}

func TestMessageRecordFromDirectIncomingRejectsNonDirectNotification(t *testing.T) {
	t.Parallel()

	_, ok := messageRecordFromDirectIncoming(map[string]any{"method": "group.incoming"}, "alice")
	if ok {
		t.Fatalf("messageRecordFromDirectIncoming() ok = true, want false")
	}
}

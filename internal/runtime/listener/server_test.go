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

func TestMessageRecordFromGroupIncomingUsesProtocolFieldsOnly(t *testing.T) {
	t.Parallel()

	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  "group.incoming",
		"params": map[string]any{
			"meta": map[string]any{
				"sender_did":   "did:wba:example.com:user:bob:e1_bob",
				"message_id":   "msg-group-001",
				"content_type": "text/plain",
				"target": map[string]any{
					"kind": "agent",
					"did":  "did:wba:example.com:user:alice:e1_alice",
				},
			},
			"body": map[string]any{
				"text":            "hello group",
				"group_did":       "did:wba:example.com:groups:demo:e1_group",
				"group_event_seq": "5",
				"accepted_at":     "2026-04-07T09:11:01Z",
			},
		},
	}

	record, ok := messageRecordFromGroupIncoming(notification, "alice")
	if !ok {
		t.Fatalf("messageRecordFromGroupIncoming() ok = false, want true")
	}
	if record.GroupDID != "did:wba:example.com:groups:demo:e1_group" {
		t.Fatalf("record.GroupDID = %q", record.GroupDID)
	}
	if record.ThreadID != "group:did:wba:example.com:groups:demo:e1_group" {
		t.Fatalf("record.ThreadID = %q", record.ThreadID)
	}
	if record.Content != "hello group" {
		t.Fatalf("record.Content = %q", record.Content)
	}
}

func TestRecordsFromGroupStateChangedBuildsMemberAndSystemMessage(t *testing.T) {
	t.Parallel()

	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  "group.state_changed",
		"params": map[string]any{
			"meta": map[string]any{
				"target": map[string]any{
					"kind": "agent",
					"did":  "did:wba:example.com:user:alice:e1_alice",
				},
			},
			"body": map[string]any{
				"event_id":          "evt-3",
				"group_did":         "did:wba:example.com:groups:demo:e1_group",
				"group_event_seq":   "3",
				"subject_method":    "group.remove",
				"subject_did":       "did:wba:example.com:user:carol:e1_carol",
				"actor_did":         "did:wba:example.com:user:alice:e1_alice",
				"membership_status": "removed",
				"changed_at":        "2026-04-07T09:06:01Z",
			},
		},
	}

	groupRecord, memberRecord, messageRecord, ok := recordsFromGroupStateChanged(notification, "alice")
	if !ok {
		t.Fatalf("recordsFromGroupStateChanged() ok = false, want true")
	}
	if groupRecord == nil || groupRecord.GroupDID != "did:wba:example.com:groups:demo:e1_group" {
		t.Fatalf("groupRecord = %#v", groupRecord)
	}
	if memberRecord == nil || memberRecord.Status != "removed" {
		t.Fatalf("memberRecord = %#v", memberRecord)
	}
	if messageRecord == nil || messageRecord.ContentType != "group_system_member_kicked" {
		t.Fatalf("messageRecord = %#v", messageRecord)
	}
}

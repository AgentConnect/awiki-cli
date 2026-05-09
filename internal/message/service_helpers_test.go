package message

import (
	"encoding/json"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/runtime"
)

func TestMessageServiceHelperContracts(t *testing.T) {
	t.Parallel()

	messages := messagesFromResult([]any{map[string]any{"id": "msg-1"}, map[string]any{"msg_id": "msg-2"}})
	if len(messages) != 2 || messages[0]["id"] != "msg-1" || messages[1]["msg_id"] != "msg-2" {
		t.Fatalf("messagesFromResult() = %#v, want two decoded messages", messages)
	}
	if ids := collectMessageIDs(messages); len(ids) != 2 || ids[0] != "msg-1" || ids[1] != "msg-2" {
		t.Fatalf("collectMessageIDs() = %#v, want [msg-1 msg-2]", ids)
	}
	markMessagesReadInResult(messages, []string{"msg-1", "msg-2"})
	if messages[0]["is_read"] != true || messages[1]["is_read"] != true {
		t.Fatalf("markMessagesReadInResult() messages = %#v, want both read", messages)
	}

	if got := contentTypeForMessageType(attachmentMessageType); got != attachmentManifestContentType {
		t.Fatalf("contentTypeForMessageType(attachment_manifest) = %q", got)
	}
	if got := contentTypeForMessageType("event"); got != "application/json" {
		t.Fatalf("contentTypeForMessageType(event) = %q", got)
	}
	if got := contentTypeForMessageType("unknown"); got != "text/plain" {
		t.Fatalf("contentTypeForMessageType(unknown) = %q", got)
	}

	metadata := metadataString(map[string]any{"delivery_state": "accepted"})
	if !json.Valid([]byte(metadata)) {
		t.Fatalf("metadataString() = %q, want JSON", metadata)
	}
	if got := peerHandleOrDid("bob", "did:bob"); got != "bob" {
		t.Fatalf("peerHandleOrDid(handle) = %q", got)
	}
	if got := peerHandleOrDid("", "did:bob"); got != "did:bob" {
		t.Fatalf("peerHandleOrDid(did) = %q", got)
	}
	if got := CompleteBareHandle("Alice", "Tenant.Example."); got != "alice.tenant.example" {
		t.Fatalf("CompleteBareHandle(bare) = %q, want alice.tenant.example", got)
	}
	if got := CompleteBareHandle("alice.other.example", "tenant.example"); got != "alice.other.example" {
		t.Fatalf("CompleteBareHandle(full) = %q, want unchanged full handle", got)
	}
	if got := CompleteBareHandle("wba://Alice", "tenant.example"); got != "alice.tenant.example" {
		t.Fatalf("CompleteBareHandle(wba bare) = %q, want alice.tenant.example", got)
	}
	if got := CompleteBareHandle("did:wba:tenant.example:user:alice:e1", "tenant.example"); got != "did:wba:tenant.example:user:alice:e1" {
		t.Fatalf("CompleteBareHandle(did) = %q, want unchanged did", got)
	}
	if got := sourceWithDefault(map[string]any{}, runtime.ModeWebSocket); got != "local_ws_cache" {
		t.Fatalf("sourceWithDefault(ws) = %q", got)
	}
	if got := sourceWithDefault(map[string]any{}, runtime.ModeHTTP); got != "remote_http" {
		t.Fatalf("sourceWithDefault(http) = %q", got)
	}
}

func TestDecodeMapIntoHandlesNilAndTypedDestination(t *testing.T) {
	t.Parallel()

	type sample struct {
		MessageID string `json:"message_id"`
		Limit     int    `json:"limit"`
	}

	var decoded sample
	decodeMapInto(map[string]any{"message_id": "msg-1", "limit": 3}, &decoded)
	if decoded.MessageID != "msg-1" || decoded.Limit != 3 {
		t.Fatalf("decoded = %#v, want populated struct", decoded)
	}
	decodeMapInto(nil, &decoded)
	decodeMapInto(map[string]any{"message_id": "msg-2"}, nil)
}

func TestMergeDirectHistoryMessagesPrefersCacheAndOrdersBySeq(t *testing.T) {
	t.Parallel()

	remote := []map[string]any{
		{"id": "remote-old", "server_seq": int64(3), "content": "old remote"},
		{"id": "cached-new", "server_seq": int64(10), "content": "stale remote"},
	}
	cached := []map[string]any{
		{"msg_id": "cached-new", "server_seq": int64(10), "content": "fresh cache"},
		{"msg_id": "cached-reply", "server_seq": int64(11), "content": "reply cache"},
	}

	merged := mergeDirectHistoryMessages(remote, cached, 10)
	if len(merged) != 3 {
		t.Fatalf("len(merged) = %d, want 3: %#v", len(merged), merged)
	}
	if messageIdentity(merged[0]) != "cached-reply" || merged[0]["content"] != "reply cache" {
		t.Fatalf("merged[0] = %#v, want newest cached reply", merged[0])
	}
	if messageIdentity(merged[1]) != "cached-new" || merged[1]["content"] != "fresh cache" {
		t.Fatalf("merged[1] = %#v, want cached row to win duplicate", merged[1])
	}
	if messageIdentity(merged[2]) != "remote-old" {
		t.Fatalf("merged[2] = %#v, want remote-old", merged[2])
	}

	limited := mergeDirectHistoryMessages(remote, cached, 2)
	if len(limited) != 2 || messageIdentity(limited[0]) != "cached-reply" || messageIdentity(limited[1]) != "cached-new" {
		t.Fatalf("limited = %#v, want top 2 newest merged rows", limited)
	}
}

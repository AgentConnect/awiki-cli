package message

import (
	"encoding/json"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/runtime"
)

func TestMessageServiceHelperContracts(t *testing.T) {
	t.Parallel()

	messages := messagesFromResult([]any{map[string]any{"id": "msg-1"}, map[string]any{"id": "msg-2"}})
	if len(messages) != 2 || messages[0]["id"] != "msg-1" || messages[1]["id"] != "msg-2" {
		t.Fatalf("messagesFromResult() = %#v, want two decoded messages", messages)
	}
	if ids := collectMessageIDs(messages); len(ids) != 2 || ids[0] != "msg-1" || ids[1] != "msg-2" {
		t.Fatalf("collectMessageIDs() = %#v, want [msg-1 msg-2]", ids)
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

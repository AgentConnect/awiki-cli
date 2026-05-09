package message

import (
	"testing"

	"github.com/agentconnect/awiki-cli/internal/identity"
)

func TestBuildInboxRPCParamsUsesDefaultLimit(t *testing.T) {
	t.Parallel()

	record := &identity.StoredIdentity{DID: "did:wba:example.com:user:alice:e1_alice"}
	params := BuildInboxRPCParams(record, InboxRequest{})
	body := mustMapValue(t, params["body"], "params.body")
	if body["limit"] != 20 {
		t.Fatalf("body.limit = %#v, want 20", body["limit"])
	}
	if body["user_did"] != record.DID {
		t.Fatalf("body.user_did = %#v, want %q", body["user_did"], record.DID)
	}
}

func TestBuildHistoryRPCParamsValidatesTargetAndOptionalCursor(t *testing.T) {
	t.Parallel()

	record := &identity.StoredIdentity{DID: "did:wba:example.com:user:alice:e1_alice"}
	if _, err := BuildHistoryRPCParams(record, HistoryRequest{}); err != ErrTargetRequired {
		t.Fatalf("BuildHistoryRPCParams() error = %v, want %v", err, ErrTargetRequired)
	}

	params, err := BuildHistoryRPCParams(record, HistoryRequest{With: "did:bob", Limit: 0})
	if err != nil {
		t.Fatalf("BuildHistoryRPCParams() error = %v", err)
	}
	body := mustMapValue(t, params["body"], "params.body")
	if body["limit"] != 50 {
		t.Fatalf("body.limit = %#v, want 50", body["limit"])
	}
	if _, ok := body["since_seq"]; ok {
		t.Fatalf("body.since_seq = %#v, want absent", body["since_seq"])
	}

	params, err = BuildHistoryRPCParams(record, HistoryRequest{With: "did:bob", Cursor: "7", Limit: 3, Skip: 4})
	if err != nil {
		t.Fatalf("BuildHistoryRPCParams(cursor) error = %v", err)
	}
	body = mustMapValue(t, params["body"], "params.body")
	if body["since_seq"] != "7" || body["limit"] != 3 || body["skip"] != 4 {
		t.Fatalf("body = %#v, want cursor, explicit limit, and skip", body)
	}
}

func TestBuildMarkReadRPCParamsRequiresMessageIDs(t *testing.T) {
	t.Parallel()

	record := &identity.StoredIdentity{DID: "did:wba:example.com:user:alice:e1_alice"}
	if _, err := BuildMarkReadRPCParams(record, MarkReadRequest{}); err == nil {
		t.Fatal("BuildMarkReadRPCParams() error = nil, want validation error")
	}

	params, err := BuildMarkReadRPCParams(record, MarkReadRequest{MessageIDs: []string{"msg-1", "msg-2"}})
	if err != nil {
		t.Fatalf("BuildMarkReadRPCParams() error = %v", err)
	}
	body := mustMapValue(t, params["body"], "params.body")
	ids, ok := body["message_ids"].([]string)
	if !ok || len(ids) != 2 {
		t.Fatalf("body.message_ids = %#v, want []string len=2", body["message_ids"])
	}
}

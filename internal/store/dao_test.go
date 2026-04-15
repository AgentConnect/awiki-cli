package store

import (
	"context"
	"testing"
)

func TestStoreMessageAndThreadView(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	if err := StoreMessage(ctx, db, MessageRecord{
		MsgID:          "msg-1",
		OwnerDID:       "did:wba:awiki.ai:user:a",
		ThreadID:       MakeThreadID("did:wba:awiki.ai:user:a", "did:wba:awiki.ai:user:b", ""),
		Direction:      0,
		SenderDID:      "did:wba:awiki.ai:user:b",
		ReceiverDID:    "did:wba:awiki.ai:user:a",
		ContentType:    "text",
		Content:        "hello",
		IsRead:         false,
		CredentialName: "default",
	}); err != nil {
		t.Fatalf("StoreMessage() error = %v", err)
	}
	got, err := GetMessageByID(ctx, db, "msg-1", "did:wba:awiki.ai:user:a", "")
	if err != nil {
		t.Fatalf("GetMessageByID() error = %v", err)
	}
	if got["content"] != "hello" {
		t.Fatalf("unexpected message content: %#v", got)
	}
	rows, err := ExecuteSQL(ctx, db, "SELECT thread_id, unread_count FROM threads")
	if err != nil {
		t.Fatalf("ExecuteSQL() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("unexpected threads rows: %#v", rows)
	}
}

func TestRebindOwnerDIDAndClearE2EEData(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	if _, err := QueueE2EEOutbox(ctx, db, E2EEOutboxRecord{
		OutboxID:       "out-1",
		OwnerDID:       "did:old",
		PeerDID:        "did:peer",
		Plaintext:      "secret",
		LocalStatus:    "queued",
		CredentialName: "default",
	}); err != nil {
		t.Fatalf("QueueE2EEOutbox() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO e2ee_sessions
    (owner_did, peer_did, session_id, is_initiator, send_chain_key, recv_chain_key, send_seq, recv_seq, expires_at, created_at, active_at, peer_confirmed, credential_name, updated_at)
VALUES ('did:old', 'did:peer', 'sess-1', 1, 'send', 'recv', 0, 0, NULL, '2026-01-01T00:00:00Z', NULL, 0, 'default', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert e2ee session error = %v", err)
	}
	if err := UpsertContact(ctx, db, ContactRecord{OwnerDID: "did:old", DID: "did:peer", Name: "Peer", Handle: "peer"}); err != nil {
		t.Fatalf("UpsertContact() error = %v", err)
	}
	result, err := RebindOwnerDID(ctx, db, "did:old", "did:new")
	if err != nil {
		t.Fatalf("RebindOwnerDID() error = %v", err)
	}
	if result["contacts"] != 1 {
		t.Fatalf("unexpected rebind counts: %#v", result)
	}
	if result["contact_handle_bindings"] != 1 {
		t.Fatalf("unexpected alias rebind counts: %#v", result)
	}
	cleared, err := ClearOwnerE2EEData(ctx, db, "did:old")
	if err != nil {
		t.Fatalf("ClearOwnerE2EEData() error = %v", err)
	}
	if cleared["e2ee_outbox"] != 1 || cleared["e2ee_sessions"] != 1 {
		t.Fatalf("unexpected clear result: %#v", cleared)
	}
}

func TestUpsertContactRebindsCurrentHandleAndPreservesHistory(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	if err := UpsertContact(ctx, db, ContactRecord{
		OwnerDID:       "did:owner",
		DID:            "did:peer-old",
		Handle:         "alice",
		SourceType:     "listener.direct_incoming",
		CredentialName: "default",
	}); err != nil {
		t.Fatalf("UpsertContact(old) error = %v", err)
	}
	if err := UpsertContact(ctx, db, ContactRecord{
		OwnerDID:       "did:owner",
		DID:            "did:peer-new",
		Handle:         "alice",
		SourceType:     "listener.direct_incoming",
		CredentialName: "default",
	}); err != nil {
		t.Fatalf("UpsertContact(new) error = %v", err)
	}

	current, err := GetCurrentContactByHandle(ctx, db, "did:owner", "alice")
	if err != nil {
		t.Fatalf("GetCurrentContactByHandle() error = %v", err)
	}
	if current["did"] != "did:peer-new" {
		t.Fatalf("current contact did = %#v, want did:peer-new", current["did"])
	}
	oldContact, err := GetContactByDID(ctx, db, "did:owner", "did:peer-old")
	if err != nil {
		t.Fatalf("GetContactByDID(old) error = %v", err)
	}
	if stringFromAny(oldContact["handle"]) != "" {
		t.Fatalf("old contact handle = %#v, want cleared handle", oldContact["handle"])
	}
	handle, err := ResolveContactHandleByDID(ctx, db, "did:owner", "did:peer-old")
	if err != nil {
		t.Fatalf("ResolveContactHandleByDID(old) error = %v", err)
	}
	if handle != "alice" {
		t.Fatalf("historical handle = %q, want alice", handle)
	}
	dids, err := ListDIDsByHandle(ctx, db, "did:owner", "alice")
	if err != nil {
		t.Fatalf("ListDIDsByHandle() error = %v", err)
	}
	if len(dids) != 2 || dids[0] != "did:peer-new" || dids[1] != "did:peer-old" {
		t.Fatalf("ListDIDsByHandle() = %#v, want [did:peer-new did:peer-old]", dids)
	}
}

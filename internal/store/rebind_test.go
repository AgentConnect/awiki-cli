package store

import (
	"context"
	"path/filepath"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func TestRebindLocalIdentityStateReturnsZeroCountsWhenDatabaseIsMissing(t *testing.T) {
	t.Parallel()

	paths := appconfig.Paths{DatabaseFile: filepath.Join(t.TempDir(), "missing.db")}
	storeRebind, e2eeCleanup, err := RebindLocalIdentityState(context.Background(), paths, "did:old", "did:new")
	if err != nil {
		t.Fatalf("RebindLocalIdentityState() error = %v", err)
	}
	for key, got := range storeRebind {
		if got != 0 {
			t.Fatalf("storeRebind[%s] = %d, want 0", key, got)
		}
	}
	for key, got := range e2eeCleanup {
		if got != 0 {
			t.Fatalf("e2eeCleanup[%s] = %d, want 0", key, got)
		}
	}
}

func TestRebindLocalIdentityStateRebindsDatabaseAndCleansE2EEData(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := appconfig.Paths{DatabaseFile: filepath.Join(root, "awiki-cli.db")}
	db, err := Open(paths)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	ctx := context.Background()
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	if err := StoreMessage(ctx, db, MessageRecord{
		MsgID:          "msg-1",
		OwnerDID:       "did:old",
		ThreadID:       MakeThreadID("did:old", "did:peer", ""),
		Direction:      0,
		SenderDID:      "did:peer",
		ReceiverDID:    "did:old",
		Content:        "hello",
		CredentialName: "default",
	}); err != nil {
		t.Fatalf("StoreMessage() error = %v", err)
	}
	if err := UpsertContact(ctx, db, ContactRecord{
		OwnerDID:       "did:old",
		DID:            "did:peer",
		Handle:         "alice",
		CredentialName: "default",
	}); err != nil {
		t.Fatalf("UpsertContact() error = %v", err)
	}
	if _, err := AppendRelationshipEvent(ctx, db, RelationshipEventRecord{
		OwnerDID:       "did:old",
		TargetDID:      "did:peer",
		EventType:      "recommended",
		CredentialName: "default",
	}); err != nil {
		t.Fatalf("AppendRelationshipEvent() error = %v", err)
	}
	if err := UpsertGroup(ctx, db, GroupRecord{
		OwnerDID:       "did:old",
		GroupID:        "group-1",
		Name:           "Group One",
		CredentialName: "default",
	}); err != nil {
		t.Fatalf("UpsertGroup() error = %v", err)
	}
	if err := UpsertGroupMember(ctx, db, GroupMemberRecord{
		OwnerDID:       "did:old",
		GroupID:        "group-1",
		UserID:         "user-1",
		MemberDID:      "did:peer",
		CredentialName: "default",
	}); err != nil {
		t.Fatalf("UpsertGroupMember() error = %v", err)
	}
	if _, err := QueueE2EEOutbox(ctx, db, E2EEOutboxRecord{
		OutboxID:       "out-1",
		OwnerDID:       "did:old",
		PeerDID:        "did:peer",
		Plaintext:      "secret",
		CredentialName: "default",
	}); err != nil {
		t.Fatalf("QueueE2EEOutbox() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO e2ee_sessions
    (owner_did, peer_did, session_id, is_initiator, send_chain_key, recv_chain_key,
     send_seq, recv_seq, expires_at, created_at, active_at, peer_confirmed, credential_name, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"did:old",
		"did:peer",
		"session-1",
		1,
		"send-key",
		"recv-key",
		0,
		0,
		nil,
		"2026-01-01T00:00:00Z",
		nil,
		0,
		"default",
		"2026-01-01T00:00:00Z",
	); err != nil {
		t.Fatalf("ExecContext(e2ee_sessions) error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close() error = %v", err)
	}

	storeRebind, e2eeCleanup, err := RebindLocalIdentityState(ctx, paths, "did:old", "did:new")
	if err != nil {
		t.Fatalf("RebindLocalIdentityState() error = %v", err)
	}
	if storeRebind["messages"] != 1 || storeRebind["contacts"] != 1 || storeRebind["contact_handle_bindings"] != 1 || storeRebind["relationship_events"] != 1 || storeRebind["groups"] != 1 || storeRebind["group_members"] != 1 {
		t.Fatalf("storeRebind = %#v, want all rebinding counts = 1", storeRebind)
	}
	if e2eeCleanup["e2ee_outbox"] != 1 || e2eeCleanup["e2ee_sessions"] != 1 {
		t.Fatalf("e2eeCleanup = %#v, want both cleanup counts = 1", e2eeCleanup)
	}

	verifyDB, err := OpenReadOnly(paths.DatabaseFile)
	if err != nil {
		t.Fatalf("OpenReadOnly() error = %v", err)
	}
	defer verifyDB.Close()
	messageRow, err := GetMessageByID(ctx, verifyDB, "msg-1", "did:new", "")
	if err != nil {
		t.Fatalf("GetMessageByID(new owner) error = %v", err)
	}
	if messageRow["content"] != "hello" {
		t.Fatalf("messageRow[content] = %#v, want hello", messageRow["content"])
	}
	if _, err := GetMessageByID(ctx, verifyDB, "msg-1", "did:old", ""); err == nil {
		t.Fatal("GetMessageByID(old owner) error = nil, want no rows after rebind")
	}
	var outboxCount int
	if err := verifyDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM e2ee_outbox WHERE owner_did = ?`, "did:old").Scan(&outboxCount); err != nil {
		t.Fatalf("QueryRowContext(outbox) error = %v", err)
	}
	if outboxCount != 0 {
		t.Fatalf("outboxCount = %d, want 0", outboxCount)
	}
}

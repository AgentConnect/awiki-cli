package store

import (
	"context"
	"path/filepath"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
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

func TestStoreMessageUpdatesCachedRawWireWithDecryptedContent(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	ownerDID := "did:wba:awiki.ai:user:bob"
	peerDID := "did:wba:awiki.ai:user:alice"
	threadID := MakeThreadID(ownerDID, peerDID, "")
	if err := StoreMessage(ctx, db, MessageRecord{
		MsgID:          "msg-secure-1",
		OwnerDID:       ownerDID,
		ThreadID:       threadID,
		Direction:      0,
		SenderDID:      peerDID,
		ReceiverDID:    ownerDID,
		ContentType:    "application/anp-direct-cipher+json",
		Content:        `{"ciphertext_b64u":"raw"}`,
		IsRead:         true,
		CredentialName: "bob",
	}); err != nil {
		t.Fatalf("StoreMessage(raw) error = %v", err)
	}
	if err := StoreMessage(ctx, db, MessageRecord{
		MsgID:          "msg-secure-1",
		OwnerDID:       ownerDID,
		ThreadID:       threadID,
		Direction:      0,
		SenderDID:      peerDID,
		ReceiverDID:    ownerDID,
		ContentType:    "text/plain",
		Content:        "decrypted hello",
		ServerSeq:      int64Ptr(42),
		IsRead:         false,
		IsE2EE:         true,
		Metadata:       `{"decryption_state":"decrypted"}`,
		CredentialName: "bob",
	}); err != nil {
		t.Fatalf("StoreMessage(decrypted) error = %v", err)
	}
	got, err := GetMessageByID(ctx, db, "msg-secure-1", ownerDID, "")
	if err != nil {
		t.Fatalf("GetMessageByID() error = %v", err)
	}
	if got["content_type"] != "text/plain" {
		t.Fatalf("content_type = %#v, want text/plain", got["content_type"])
	}
	if got["content"] != "decrypted hello" {
		t.Fatalf("content = %#v, want decrypted hello", got["content"])
	}
	if got["is_e2ee"] != int64(1) {
		t.Fatalf("is_e2ee = %#v, want 1", got["is_e2ee"])
	}
	if got["is_read"] != int64(1) {
		t.Fatalf("is_read = %#v, want preserved read state", got["is_read"])
	}
	if got["server_seq"] != int64(42) {
		t.Fatalf("server_seq = %#v, want 42", got["server_seq"])
	}
}

func TestStoreMessagePreservesDecryptedContentWhenRawWireArrivesLater(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	ownerDID := "did:wba:awiki.ai:user:bob"
	peerDID := "did:wba:awiki.ai:user:alice"
	threadID := MakeThreadID(ownerDID, peerDID, "")
	if err := StoreMessage(ctx, db, MessageRecord{
		MsgID:          "msg-secure-2",
		OwnerDID:       ownerDID,
		ThreadID:       threadID,
		Direction:      0,
		SenderDID:      peerDID,
		ReceiverDID:    ownerDID,
		ContentType:    "text/plain",
		Content:        "already decrypted",
		ServerSeq:      int64Ptr(42),
		IsE2EE:         true,
		Metadata:       `{"decryption_state":"decrypted"}`,
		CredentialName: "bob",
	}); err != nil {
		t.Fatalf("StoreMessage(decrypted) error = %v", err)
	}
	if err := StoreMessage(ctx, db, MessageRecord{
		MsgID:          "msg-secure-2",
		OwnerDID:       ownerDID,
		ThreadID:       threadID,
		Direction:      0,
		SenderDID:      peerDID,
		ReceiverDID:    ownerDID,
		ContentType:    "application/anp-direct-cipher+json",
		Content:        `{"ciphertext_b64u":"raw"}`,
		ServerSeq:      int64Ptr(43),
		Metadata:       `{"content_type":"application/anp-direct-cipher+json"}`,
		CredentialName: "bob",
	}); err != nil {
		t.Fatalf("StoreMessage(raw) error = %v", err)
	}
	got, err := GetMessageByID(ctx, db, "msg-secure-2", ownerDID, "")
	if err != nil {
		t.Fatalf("GetMessageByID() error = %v", err)
	}
	if got["content_type"] != "text/plain" {
		t.Fatalf("content_type = %#v, want text/plain", got["content_type"])
	}
	if got["content"] != "already decrypted" {
		t.Fatalf("content = %#v, want already decrypted", got["content"])
	}
	if got["metadata"] != `{"decryption_state":"decrypted"}` {
		t.Fatalf("metadata = %#v, want decrypted metadata preserved", got["metadata"])
	}
	if got["server_seq"] != int64(43) {
		t.Fatalf("server_seq = %#v, want newest server seq", got["server_seq"])
	}
}

func TestStoreMessagePreservesGroupPlaintextWhenRawWireArrivesLater(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	ownerDID := "did:wba:awiki.ai:user:alice"
	groupDID := "did:wba:awiki.ai:groups:e2ee"
	threadID := MakeThreadID(ownerDID, "", groupDID)
	if err := StoreMessage(ctx, db, MessageRecord{
		MsgID:          "group-e2ee-1",
		OwnerDID:       ownerDID,
		ThreadID:       threadID,
		Direction:      1,
		SenderDID:      ownerDID,
		GroupID:        groupDID,
		GroupDID:       groupDID,
		ContentType:    "text/plain",
		Content:        "already plaintext",
		ServerSeq:      int64Ptr(7),
		IsE2EE:         true,
		Metadata:       `{"security_profile":"group-e2ee"}`,
		CredentialName: "alice",
	}); err != nil {
		t.Fatalf("StoreMessage(plaintext) error = %v", err)
	}
	if err := StoreMessage(ctx, db, MessageRecord{
		MsgID:          "group-e2ee-1",
		OwnerDID:       ownerDID,
		ThreadID:       threadID,
		Direction:      1,
		SenderDID:      ownerDID,
		GroupID:        groupDID,
		GroupDID:       groupDID,
		ContentType:    "application/anp-group-cipher+json",
		Content:        `{"private_message_b64u":"raw"}`,
		ServerSeq:      int64Ptr(8),
		Metadata:       `{"content_type":"application/anp-group-cipher+json"}`,
		CredentialName: "alice",
	}); err != nil {
		t.Fatalf("StoreMessage(raw group cipher) error = %v", err)
	}
	got, err := GetMessageByID(ctx, db, "group-e2ee-1", ownerDID, "")
	if err != nil {
		t.Fatalf("GetMessageByID() error = %v", err)
	}
	if got["content_type"] != "text/plain" {
		t.Fatalf("content_type = %#v, want text/plain", got["content_type"])
	}
	if got["content"] != "already plaintext" {
		t.Fatalf("content = %#v, want already plaintext", got["content"])
	}
	if got["metadata"] != `{"security_profile":"group-e2ee"}` {
		t.Fatalf("metadata = %#v, want plaintext metadata preserved", got["metadata"])
	}
	if got["server_seq"] != int64(8) {
		t.Fatalf("server_seq = %#v, want newest server seq", got["server_seq"])
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

func TestListDIDsByHandleFallsBackToContactsWithoutHistoryBindings(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO contacts
    (owner_did, did, handle, first_seen_at, last_seen_at, metadata)
VALUES (?, ?, ?, ?, ?, ?)`,
		"did:owner",
		"did:peer",
		"alice",
		"2026-01-01T00:00:00Z",
		"2026-01-01T00:00:00Z",
		`{"source":"seed"}`,
	); err != nil {
		t.Fatalf("ExecContext(insert contacts) error = %v", err)
	}

	dids, err := ListDIDsByHandle(ctx, db, "did:owner", "alice")
	if err != nil {
		t.Fatalf("ListDIDsByHandle() error = %v", err)
	}
	if len(dids) != 1 || dids[0] != "did:peer" {
		t.Fatalf("ListDIDsByHandle() = %#v, want [did:peer]", dids)
	}
}

func TestExecuteSQLRejectsUnsafeStatementsAndAllowsScopedUpdate(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	if err := StoreMessage(ctx, db, MessageRecord{
		MsgID:          "msg-1",
		OwnerDID:       "did:owner",
		ThreadID:       MakeThreadID("did:owner", "did:peer", ""),
		Direction:      0,
		SenderDID:      "did:peer",
		ReceiverDID:    "did:owner",
		Content:        "hello",
		CredentialName: "default",
	}); err != nil {
		t.Fatalf("StoreMessage() error = %v", err)
	}

	for _, statement := range []string{"", "SELECT 1; SELECT 2", "DROP TABLE messages", "DELETE FROM messages"} {
		if _, err := ExecuteSQL(ctx, db, statement); err == nil {
			t.Fatalf("ExecuteSQL(%q) error = nil, want unsafe SQL error", statement)
		}
	}

	rows, err := ExecuteSQL(ctx, db, `UPDATE messages SET is_read = 1 WHERE owner_did = ? AND msg_id = ?`, "did:owner", "msg-1")
	if err != nil {
		t.Fatalf("ExecuteSQL(update) error = %v", err)
	}
	if len(rows) != 1 || rows[0]["rows_affected"] != int64(1) {
		t.Fatalf("ExecuteSQL(update) rows = %#v, want rows_affected=1", rows)
	}
	messageRow, err := GetMessageByID(ctx, db, "msg-1", "did:owner", "")
	if err != nil {
		t.Fatalf("GetMessageByID() error = %v", err)
	}
	if messageRow["is_read"] != int64(1) {
		t.Fatalf("messageRow[is_read] = %#v, want 1", messageRow["is_read"])
	}
}

func TestMergeRecoveredHandleLocalStateMovesOnlyTargetOwners(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	paths := appconfig.Paths{DatabaseFile: filepath.Join(root, "awiki-cli.db")}
	db, err := Open(paths)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}

	oldOne := "did:owner:old-1"
	oldTwo := "did:owner:old-2"
	newOwner := "did:owner:new"
	otherOwner := "did:owner:other"
	peerDID := "did:peer:bob"
	if err := StoreMessage(ctx, db, MessageRecord{
		MsgID:          "msg-1",
		OwnerDID:       oldOne,
		ThreadID:       MakeThreadID(oldOne, peerDID, ""),
		Direction:      0,
		SenderDID:      peerDID,
		ReceiverDID:    oldOne,
		ContentType:    "text",
		Content:        "hello old one",
		SentAt:         "2026-04-20T10:00:00Z",
		StoredAt:       "2026-04-20T10:00:00Z",
		CredentialName: "zhuocheng",
	}); err != nil {
		t.Fatalf("StoreMessage(oldOne) error = %v", err)
	}
	if err := StoreMessage(ctx, db, MessageRecord{
		MsgID:          "msg-2",
		OwnerDID:       oldTwo,
		ThreadID:       MakeThreadID(oldTwo, peerDID, ""),
		Direction:      1,
		SenderDID:      oldTwo,
		ReceiverDID:    peerDID,
		ContentType:    "text",
		Content:        "hello old two",
		SentAt:         "2026-04-20T11:00:00Z",
		StoredAt:       "2026-04-20T11:00:00Z",
		CredentialName: "zhuocheng-2",
	}); err != nil {
		t.Fatalf("StoreMessage(oldTwo) error = %v", err)
	}
	if err := StoreMessage(ctx, db, MessageRecord{
		MsgID:          "msg-other",
		OwnerDID:       otherOwner,
		ThreadID:       MakeThreadID(otherOwner, peerDID, ""),
		Direction:      0,
		SenderDID:      peerDID,
		ReceiverDID:    otherOwner,
		ContentType:    "text",
		Content:        "hello other",
		CredentialName: "lzc",
	}); err != nil {
		t.Fatalf("StoreMessage(otherOwner) error = %v", err)
	}
	if err := UpsertContact(ctx, db, ContactRecord{
		OwnerDID:       oldOne,
		DID:            "did:peer:old",
		Handle:         "alice",
		FirstSeenAt:    "2026-04-20T10:00:00Z",
		LastSeenAt:     "2026-04-20T10:00:00Z",
		CredentialName: "zhuocheng",
	}); err != nil {
		t.Fatalf("UpsertContact(oldOne) error = %v", err)
	}
	if err := UpsertContact(ctx, db, ContactRecord{
		OwnerDID:       oldTwo,
		DID:            "did:peer:new",
		Handle:         "alice",
		FirstSeenAt:    "2026-04-20T11:00:00Z",
		LastSeenAt:     "2026-04-20T11:00:00Z",
		CredentialName: "zhuocheng-2",
	}); err != nil {
		t.Fatalf("UpsertContact(oldTwo) error = %v", err)
	}
	if err := UpsertContact(ctx, db, ContactRecord{
		OwnerDID:       otherOwner,
		DID:            "did:peer:else",
		Handle:         "else",
		FirstSeenAt:    "2026-04-20T09:00:00Z",
		LastSeenAt:     "2026-04-20T09:00:00Z",
		CredentialName: "lzc",
	}); err != nil {
		t.Fatalf("UpsertContact(otherOwner) error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO relationship_events
    (event_id, owner_did, target_did, target_handle, event_type, created_at, updated_at, credential_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"evt-1",
		oldOne,
		"did:peer:old",
		"alice",
		"follow",
		"2026-04-20T10:00:00Z",
		"2026-04-20T10:00:00Z",
		"zhuocheng",
	); err != nil {
		t.Fatalf("insert relationship_events error = %v", err)
	}
	if err := UpsertGroup(ctx, db, GroupRecord{
		OwnerDID:         oldTwo,
		GroupID:          "group:one",
		GroupDID:         "did:group:one",
		Name:             "Group One",
		GroupOwnerDID:    oldTwo,
		MembershipStatus: "active",
		LastMessageAt:    "2026-04-20T11:30:00Z",
		StoredAt:         "2026-04-20T11:30:00Z",
		CredentialName:   "zhuocheng-2",
	}); err != nil {
		t.Fatalf("UpsertGroup(oldTwo) error = %v", err)
	}
	if err := UpsertGroupMember(ctx, db, GroupMemberRecord{
		OwnerDID:         oldTwo,
		GroupID:          "group:one",
		UserID:           "user-z",
		MemberDID:        oldTwo,
		MemberHandle:     "zhuocheng",
		Status:           "active",
		SentMessageCount: int64PtrFromAny(int64(3)),
		LastSyncedAt:     "2026-04-20T11:30:00Z",
		CredentialName:   "zhuocheng-2",
	}); err != nil {
		t.Fatalf("UpsertGroupMember(oldTwo) error = %v", err)
	}
	if _, err := QueueE2EEOutbox(ctx, db, E2EEOutboxRecord{
		OutboxID:       "out-1",
		OwnerDID:       oldOne,
		PeerDID:        peerDID,
		Plaintext:      "secret",
		LocalStatus:    "queued",
		CredentialName: "zhuocheng",
	}); err != nil {
		t.Fatalf("QueueE2EEOutbox(oldOne) error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO e2ee_sessions
    (owner_did, peer_did, session_id, is_initiator, send_chain_key, recv_chain_key, send_seq, recv_seq, expires_at, created_at, active_at, peer_confirmed, credential_name, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		oldTwo,
		peerDID,
		"sess-1",
		1,
		"send",
		"recv",
		0,
		0,
		nil,
		"2026-04-20T11:00:00Z",
		nil,
		0,
		"zhuocheng-2",
		"2026-04-20T11:00:00Z",
	); err != nil {
		t.Fatalf("insert e2ee_sessions error = %v", err)
	}

	storeMerge, cleanup, err := MergeRecoveredHandleLocalState(ctx, paths, []string{oldOne, oldTwo}, newOwner, "zhuocheng")
	if err != nil {
		t.Fatalf("MergeRecoveredHandleLocalState() error = %v", err)
	}
	if storeMerge["messages"] != 2 || storeMerge["contacts"] != 2 || storeMerge["contact_handle_bindings"] != 2 {
		t.Fatalf("unexpected store merge counts: %#v", storeMerge)
	}
	if cleanup["e2ee_outbox"] != 1 || cleanup["e2ee_sessions"] != 1 {
		t.Fatalf("unexpected e2ee cleanup counts: %#v", cleanup)
	}

	msgOne, err := GetMessageByID(ctx, db, "msg-1", newOwner, "")
	if err != nil {
		t.Fatalf("GetMessageByID(msg-1) error = %v", err)
	}
	if got := stringFromAny(msgOne["receiver_did"]); got != newOwner {
		t.Fatalf("msg-1 receiver_did = %q, want %q", got, newOwner)
	}
	if got := stringFromAny(msgOne["thread_id"]); got != MakeThreadID(newOwner, peerDID, "") {
		t.Fatalf("msg-1 thread_id = %q, want %q", got, MakeThreadID(newOwner, peerDID, ""))
	}
	msgTwo, err := GetMessageByID(ctx, db, "msg-2", newOwner, "")
	if err != nil {
		t.Fatalf("GetMessageByID(msg-2) error = %v", err)
	}
	if got := stringFromAny(msgTwo["sender_did"]); got != newOwner {
		t.Fatalf("msg-2 sender_did = %q, want %q", got, newOwner)
	}
	if got := stringFromAny(msgTwo["credential_name"]); got != "zhuocheng" {
		t.Fatalf("msg-2 credential_name = %q, want zhuocheng", got)
	}

	current, err := GetCurrentContactByHandle(ctx, db, newOwner, "alice")
	if err != nil {
		t.Fatalf("GetCurrentContactByHandle() error = %v", err)
	}
	if got := stringFromAny(current["did"]); got != "did:peer:new" {
		t.Fatalf("current contact did = %q, want did:peer:new", got)
	}
	dids, err := ListDIDsByHandle(ctx, db, newOwner, "alice")
	if err != nil {
		t.Fatalf("ListDIDsByHandle() error = %v", err)
	}
	if len(dids) != 2 || dids[0] != "did:peer:new" || dids[1] != "did:peer:old" {
		t.Fatalf("ListDIDsByHandle() = %#v, want [did:peer:new did:peer:old]", dids)
	}

	groupSnapshot, err := GetGroupSnapshot(ctx, db, newOwner, "group:one")
	if err != nil {
		t.Fatalf("GetGroupSnapshot() error = %v", err)
	}
	if got := stringFromAny(groupSnapshot["group_owner_did"]); got != newOwner {
		t.Fatalf("group_owner_did = %q, want %q", got, newOwner)
	}
	members, err := ListCachedGroupMembers(ctx, db, newOwner, "group:one", 10)
	if err != nil {
		t.Fatalf("ListCachedGroupMembers() error = %v", err)
	}
	if len(members) != 1 || stringFromAny(members[0]["member_did"]) != newOwner {
		t.Fatalf("group members = %#v, want member_did %q", members, newOwner)
	}

	otherMsg, err := GetMessageByID(ctx, db, "msg-other", otherOwner, "")
	if err != nil {
		t.Fatalf("GetMessageByID(msg-other) error = %v", err)
	}
	if got := stringFromAny(otherMsg["owner_did"]); got != otherOwner {
		t.Fatalf("other message owner_did = %q, want %q", got, otherOwner)
	}

	for _, owner := range []string{oldOne, oldTwo} {
		var remaining int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE owner_did = ?`, owner).Scan(&remaining); err != nil {
			t.Fatalf("count old messages error = %v", err)
		}
		if remaining != 0 {
			t.Fatalf("old owner %s still has %d message rows", owner, remaining)
		}
	}
}

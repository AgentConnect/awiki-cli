package store

import (
	"context"
	"database/sql"
	"testing"
)

func insertQueryTestMessage(t *testing.T, ctx context.Context, db *sql.DB, record MessageRecord) {
	t.Helper()
	if err := StoreMessage(ctx, db, record); err != nil {
		t.Fatalf("StoreMessage(%s) error = %v", record.MsgID, err)
	}
}

func TestListDirectMessagesByPeerDIDsFiltersUnreadInboxOnlyAndDeduplicates(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}

	ownerDID := "did:owner"
	peerOne := "did:peer-1"
	peerTwo := "did:peer-2"
	insertQueryTestMessage(t, ctx, db, MessageRecord{
		MsgID:          "direct-unread",
		OwnerDID:       ownerDID,
		ThreadID:       MakeThreadID(ownerDID, peerOne, ""),
		Direction:      0,
		SenderDID:      peerOne,
		ReceiverDID:    ownerDID,
		Content:        "incoming unread",
		SentAt:         "2026-01-01T00:00:01Z",
		CredentialName: "default",
	})
	insertQueryTestMessage(t, ctx, db, MessageRecord{
		MsgID:          "direct-outgoing",
		OwnerDID:       ownerDID,
		ThreadID:       MakeThreadID(ownerDID, peerOne, ""),
		Direction:      1,
		SenderDID:      ownerDID,
		ReceiverDID:    peerOne,
		Content:        "outgoing",
		IsRead:         true,
		SentAt:         "2026-01-01T00:00:02Z",
		CredentialName: "default",
	})
	insertQueryTestMessage(t, ctx, db, MessageRecord{
		MsgID:          "direct-read",
		OwnerDID:       ownerDID,
		ThreadID:       MakeThreadID(ownerDID, peerTwo, ""),
		Direction:      0,
		SenderDID:      peerTwo,
		ReceiverDID:    ownerDID,
		Content:        "incoming read",
		IsRead:         true,
		SentAt:         "2026-01-01T00:00:03Z",
		CredentialName: "default",
	})
	insertQueryTestMessage(t, ctx, db, MessageRecord{
		MsgID:          "group-message",
		OwnerDID:       ownerDID,
		ThreadID:       MakeThreadID(ownerDID, "", "group-1"),
		Direction:      0,
		SenderDID:      peerOne,
		ReceiverDID:    ownerDID,
		GroupID:        "group-1",
		Content:        "group content",
		SentAt:         "2026-01-01T00:00:04Z",
		CredentialName: "default",
	})

	rows, err := ListDirectMessagesByPeerDIDs(ctx, db, ownerDID, []string{"  " + peerOne + "  ", peerOne, "", peerTwo}, 0, false, false)
	if err != nil {
		t.Fatalf("ListDirectMessagesByPeerDIDs() error = %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3 direct rows", len(rows))
	}
	gotOrder := []string{rows[0]["msg_id"].(string), rows[1]["msg_id"].(string), rows[2]["msg_id"].(string)}
	wantOrder := []string{"direct-read", "direct-outgoing", "direct-unread"}
	for index, want := range wantOrder {
		if gotOrder[index] != want {
			t.Fatalf("rows[%d][msg_id] = %q, want %q; full order=%#v", index, gotOrder[index], want, gotOrder)
		}
	}

	filtered, err := ListDirectMessagesByPeerDIDs(ctx, db, ownerDID, []string{peerOne, peerTwo}, 0, true, true)
	if err != nil {
		t.Fatalf("ListDirectMessagesByPeerDIDs(unread inbox) error = %v", err)
	}
	if len(filtered) != 1 || filtered[0]["msg_id"] != "direct-unread" {
		t.Fatalf("filtered rows = %#v, want only direct-unread", filtered)
	}

	empty, err := ListDirectMessagesByPeerDIDs(ctx, db, ownerDID, nil, 0, false, false)
	if err != nil {
		t.Fatalf("ListDirectMessagesByPeerDIDs(nil peers) error = %v", err)
	}
	if empty != nil {
		t.Fatalf("ListDirectMessagesByPeerDIDs(nil peers) = %#v, want nil", empty)
	}
}

func TestGroupQueryFunctionsApplyFiltersDefaultsAndOrdering(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}

	ownerDID := "did:owner"
	groupID := "group-1"
	otherGroupID := "group-2"
	insertQueryTestMessage(t, ctx, db, MessageRecord{
		MsgID:          "group-1-seq1",
		OwnerDID:       ownerDID,
		ThreadID:       MakeThreadID(ownerDID, "", groupID),
		Direction:      0,
		SenderDID:      "did:peer-a",
		GroupID:        groupID,
		Content:        "first unread",
		ServerSeq:      int64Ptr(1),
		SentAt:         "2026-01-01T00:00:01Z",
		CredentialName: "default",
	})
	insertQueryTestMessage(t, ctx, db, MessageRecord{
		MsgID:          "group-1-seq2",
		OwnerDID:       ownerDID,
		ThreadID:       MakeThreadID(ownerDID, "", groupID),
		Direction:      0,
		SenderDID:      "did:peer-b",
		GroupID:        groupID,
		Content:        "second read",
		IsRead:         true,
		ServerSeq:      int64Ptr(2),
		SentAt:         "2026-01-01T00:00:02Z",
		CredentialName: "default",
	})
	insertQueryTestMessage(t, ctx, db, MessageRecord{
		MsgID:          "group-1-seq3",
		OwnerDID:       ownerDID,
		ThreadID:       MakeThreadID(ownerDID, "", groupID),
		Direction:      0,
		SenderDID:      "did:peer-c",
		GroupID:        groupID,
		Content:        "third unread",
		ServerSeq:      int64Ptr(3),
		SentAt:         "2026-01-01T00:00:03Z",
		CredentialName: "default",
	})
	insertQueryTestMessage(t, ctx, db, MessageRecord{
		MsgID:          "group-2-seq9",
		OwnerDID:       ownerDID,
		ThreadID:       MakeThreadID(ownerDID, "", otherGroupID),
		Direction:      0,
		SenderDID:      "did:peer-d",
		GroupID:        otherGroupID,
		Content:        "other group",
		ServerSeq:      int64Ptr(9),
		SentAt:         "2026-01-01T00:00:04Z",
		CredentialName: "default",
	})
	joinEnabled := true
	memberCount := int64(2)
	if err := UpsertGroup(ctx, db, GroupRecord{
		OwnerDID:         ownerDID,
		GroupID:          groupID,
		Name:             "Group One",
		JoinEnabled:      &joinEnabled,
		MemberCount:      &memberCount,
		CredentialName:   "default",
		MembershipStatus: "active",
	}); err != nil {
		t.Fatalf("UpsertGroup() error = %v", err)
	}
	if err := UpsertGroupMember(ctx, db, GroupMemberRecord{
		OwnerDID:       ownerDID,
		GroupID:        groupID,
		UserID:         "user-admin",
		MemberDID:      "did:admin",
		MemberHandle:   "zoe",
		Role:           "admin",
		CredentialName: "default",
	}); err != nil {
		t.Fatalf("UpsertGroupMember(admin) error = %v", err)
	}
	if err := UpsertGroupMember(ctx, db, GroupMemberRecord{
		OwnerDID:       ownerDID,
		GroupID:        groupID,
		UserID:         "user-member",
		MemberDID:      "did:member",
		MemberHandle:   "alice",
		Role:           "member",
		CredentialName: "default",
	}); err != nil {
		t.Fatalf("UpsertGroupMember(member) error = %v", err)
	}

	groupInbox, err := ListGroupInboxMessages(ctx, db, ownerDID, 0, groupID, true)
	if err != nil {
		t.Fatalf("ListGroupInboxMessages() error = %v", err)
	}
	if len(groupInbox) != 2 || groupInbox[0]["msg_id"] != "group-1-seq3" || groupInbox[1]["msg_id"] != "group-1-seq1" {
		t.Fatalf("group inbox = %#v, want unread seq3 then seq1", groupInbox)
	}

	sinceSeq := int64(1)
	groupRows, err := ListGroupMessages(ctx, db, ownerDID, groupID, 0, &sinceSeq)
	if err != nil {
		t.Fatalf("ListGroupMessages() error = %v", err)
	}
	if len(groupRows) != 2 || groupRows[0]["msg_id"] != "group-1-seq3" || groupRows[1]["msg_id"] != "group-1-seq2" {
		t.Fatalf("group rows = %#v, want seq3 then seq2", groupRows)
	}
	if _, err := ListGroupMessages(ctx, db, ownerDID, "", 0, nil); err == nil {
		t.Fatal("ListGroupMessages(empty groupID) error = nil, want error")
	}

	snapshot, err := GetGroupSnapshot(ctx, db, ownerDID, groupID)
	if err != nil {
		t.Fatalf("GetGroupSnapshot() error = %v", err)
	}
	if snapshot["name"] != "Group One" {
		t.Fatalf("snapshot[name] = %#v, want Group One", snapshot["name"])
	}

	members, err := ListCachedGroupMembers(ctx, db, ownerDID, groupID, 0)
	if err != nil {
		t.Fatalf("ListCachedGroupMembers() error = %v", err)
	}
	if len(members) != 2 || members[0]["member_handle"] != "zoe" || members[1]["member_handle"] != "alice" {
		t.Fatalf("members = %#v, want admin zoe then member alice", members)
	}
	if _, err := ListCachedGroupMembers(ctx, db, ownerDID, "", 0); err == nil {
		t.Fatal("ListCachedGroupMembers(empty groupID) error = nil, want error")
	}
}

func TestMessageQueryHelpersLookupAndMarkReadRespectOwner(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}

	threadID := MakeThreadID("did:owner-1", "did:peer", "")
	insertQueryTestMessage(t, ctx, db, MessageRecord{
		MsgID:          "shared-msg",
		OwnerDID:       "did:owner-1",
		ThreadID:       threadID,
		Direction:      0,
		SenderDID:      "did:peer",
		ReceiverDID:    "did:owner-1",
		Content:        "owner1",
		CredentialName: "default",
	})
	insertQueryTestMessage(t, ctx, db, MessageRecord{
		MsgID:          "shared-msg",
		OwnerDID:       "did:owner-2",
		ThreadID:       MakeThreadID("did:owner-2", "did:peer", ""),
		Direction:      0,
		SenderDID:      "did:peer",
		ReceiverDID:    "did:owner-2",
		Content:        "owner2",
		CredentialName: "default",
	})

	rows, err := ListMessagesByIDs(ctx, db, "did:owner-1", []string{"shared-msg", "missing"})
	if err != nil {
		t.Fatalf("ListMessagesByIDs() error = %v", err)
	}
	if len(rows) != 1 || rows[0]["content"] != "owner1" {
		t.Fatalf("rows = %#v, want only owner1 row", rows)
	}
	if empty, err := ListMessagesByIDs(ctx, db, "did:owner-1", nil); err != nil || empty != nil {
		t.Fatalf("ListMessagesByIDs(nil) = (%#v, %v), want (nil, nil)", empty, err)
	}

	affected, err := MarkMessagesRead(ctx, db, "did:owner-1", []string{"shared-msg", "missing"})
	if err != nil {
		t.Fatalf("MarkMessagesRead() error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("MarkMessagesRead() affected = %d, want 1", affected)
	}
	ownerOneRow, err := GetMessageByID(ctx, db, "shared-msg", "did:owner-1", "")
	if err != nil {
		t.Fatalf("GetMessageByID(owner1) error = %v", err)
	}
	ownerTwoRow, err := GetMessageByID(ctx, db, "shared-msg", "did:owner-2", "")
	if err != nil {
		t.Fatalf("GetMessageByID(owner2) error = %v", err)
	}
	if ownerOneRow["is_read"] != int64(1) || ownerTwoRow["is_read"] != int64(0) {
		t.Fatalf("owner rows read flags = owner1:%#v owner2:%#v, want 1/0", ownerOneRow["is_read"], ownerTwoRow["is_read"])
	}
	if _, err := ListThreadMessages(ctx, db, "did:owner-1", "", 0); err == nil {
		t.Fatal("ListThreadMessages(empty threadID) error = nil, want error")
	}
	threadRows, err := ListThreadMessages(ctx, db, "did:owner-1", threadID, 0)
	if err != nil {
		t.Fatalf("ListThreadMessages() error = %v", err)
	}
	if len(threadRows) != 1 || threadRows[0]["msg_id"] != "shared-msg" {
		t.Fatalf("threadRows = %#v, want shared-msg", threadRows)
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}

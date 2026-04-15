package message

import (
	"context"
	"errors"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/store"
)

func TestLeaveGroupRejectsActiveOwnerFromCachedSnapshot(t *testing.T) {
	t.Parallel()

	resolved := testResolvedConfig(t)
	manager := identity.NewManager(resolved.Paths)
	createTestIdentity(t, manager, identity.SaveInput{
		IdentityName: "alice",
		UserID:       "user-123",
		DisplayName:  "Alice",
		Handle:       "alice",
	})
	resolved.ActiveIdentity = "alice"

	service := &Service{
		resolved: resolved,
		manager:  manager,
	}
	record, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	writeCachedGroupState(t, resolved, record, store.GroupRecord{
		OwnerDID:         record.DID,
		GroupID:          "did:wba:awiki.ai:groups:test-owner",
		GroupDID:         "did:wba:awiki.ai:groups:test-owner",
		MyRole:           "owner",
		MembershipStatus: "active",
		CredentialName:   record.IdentityName,
	}, nil)

	_, err = service.LeaveGroup(context.Background(), GroupLeaveRequest{
		IdentityName: "alice",
		Group:        "did:wba:awiki.ai:groups:test-owner",
	})
	if !errors.Is(err, ErrGroupOwnerCannotLeave) {
		t.Fatalf("LeaveGroup() error = %v, want %v", err, ErrGroupOwnerCannotLeave)
	}
}

func TestMarkCachedGroupLeftClearsMembersAndResetsRole(t *testing.T) {
	t.Parallel()

	resolved := testResolvedConfig(t)
	manager := identity.NewManager(resolved.Paths)
	createTestIdentity(t, manager, identity.SaveInput{
		IdentityName: "alice",
		UserID:       "user-123",
		DisplayName:  "Alice",
		Handle:       "alice",
	})

	service := &Service{
		resolved: resolved,
		manager:  manager,
	}
	record, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	groupDID := "did:wba:awiki.ai:groups:test-left"
	writeCachedGroupState(t, resolved, record, store.GroupRecord{
		OwnerDID:         record.DID,
		GroupID:          groupDID,
		GroupDID:         groupDID,
		MyRole:           "owner",
		MembershipStatus: "active",
		CredentialName:   record.IdentityName,
	}, []store.GroupMemberRecord{
		{
			OwnerDID:       record.DID,
			GroupID:        groupDID,
			UserID:         record.DID,
			MemberDID:      record.DID,
			Role:           "owner",
			Status:         "active",
			CredentialName: record.IdentityName,
		},
	})

	warnings := service.markCachedGroupLeft(context.Background(), record, groupDID)
	if len(warnings) != 0 {
		t.Fatalf("markCachedGroupLeft() warnings = %#v, want none", warnings)
	}

	snapshot, err := service.readCachedGroupSnapshot(context.Background(), record, groupDID)
	if err != nil {
		t.Fatalf("readCachedGroupSnapshot() error = %v", err)
	}
	if got := snapshot["membership_status"]; got != "left" {
		t.Fatalf("membership_status = %#v, want %q", got, "left")
	}
	if got := snapshot["my_role"]; got != nil && got != "" {
		t.Fatalf("my_role = %#v, want empty", got)
	}

	members, err := service.readCachedGroupMembers(context.Background(), record, groupDID, 100)
	if err != nil {
		t.Fatalf("readCachedGroupMembers() error = %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("cached members = %#v, want empty", members)
	}
}

func TestShouldUseCachedGroupFallbackRejectsInactiveViewerErrors(t *testing.T) {
	t.Parallel()

	if shouldUseCachedGroupFallback(&ServiceError{RPCCode: 2501, Message: "viewer is not an active member"}) {
		t.Fatal("shouldUseCachedGroupFallback() = true, want false for inactive viewer rpc error")
	}
	if shouldUseCachedGroupFallback(errors.New("message service rpc error 2501: viewer is not an active member")) {
		t.Fatal("shouldUseCachedGroupFallback() = true, want false for inactive viewer text error")
	}
	if !shouldUseCachedGroupFallback(ErrTransportUnavailable) {
		t.Fatal("shouldUseCachedGroupFallback() = false, want true for transport fallback")
	}
}

func writeCachedGroupState(t *testing.T, resolved *appconfig.Resolved, record *identity.StoredIdentity, group store.GroupRecord, members []store.GroupMemberRecord) {
	t.Helper()

	db, err := store.Open(resolved.Paths)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	defer db.Close()

	if err := store.EnsureSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}

	if group.OwnerDID == "" {
		group.OwnerDID = record.DID
	}
	if group.GroupID == "" {
		group.GroupID = group.GroupDID
	}
	if group.CredentialName == "" {
		group.CredentialName = record.IdentityName
	}
	if err := store.UpsertGroup(context.Background(), db, group); err != nil {
		t.Fatalf("UpsertGroup() error = %v", err)
	}

	if len(members) == 0 {
		return
	}
	if err := store.ReplaceGroupMembers(context.Background(), db, record.DID, group.GroupID, members, record.IdentityName); err != nil {
		t.Fatalf("ReplaceGroupMembers() error = %v", err)
	}
}

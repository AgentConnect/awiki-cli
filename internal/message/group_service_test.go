package message

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestLeaveGroupE2EECreatesLeaveRequestWithoutLocalMLSLeave(t *testing.T) {
	t.Parallel()

	var captured rpcRequestEnvelope
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = decodeRPCRequest(t, r)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      captured.ID,
			"result": map[string]any{
				"accepted":         true,
				"leave_request_id": "lr-bob-1",
				"delivery_state":   "pending_owner_action",
			},
		})
	}))
	defer server.Close()

	service, _, record := newMessageServiceForTest(t, server.URL)
	provider := MLSExecProvider{BinaryPath: "anp-mls", Runner: &groupLeaveSafetyMLSRunner{}}
	service.mlsProvider = &provider

	result, warnings, err := service.leaveGroupE2EE(context.Background(), record, GroupLeaveRequest{
		Group:      "did:wba:awiki.ai:groups:test-e2ee-leave",
		ReasonText: "done",
	})
	if err != nil {
		t.Fatalf("leaveGroupE2EE() error = %v", err)
	}
	if captured.Method != "group.e2ee.leave_request" {
		t.Fatalf("captured.Method = %q, want leave_request", captured.Method)
	}
	body := mustMapValue(t, captured.Params["body"], "params.body")
	if got := stringFromAny(body["subject_status"]); got != "leave_requested" {
		t.Fatalf("subject_status = %q, want leave_requested", got)
	}
	if got := stringFromAny(body["reason_text"]); got != "done" {
		t.Fatalf("reason_text = %q, want done", got)
	}
	if got := stringFromAny(result["leave_request_id"]); got != "lr-bob-1" {
		t.Fatalf("leave_request_id = %q, want lr-bob-1", got)
	}
	if got := strings.Join(warnings, "\n"); !strings.Contains(got, "owner/admin") {
		t.Fatalf("warnings = %#v, want owner/admin processing guidance", warnings)
	}
}

type groupLeaveSafetyMLSRunner struct {
	calls []string
}

func (r *groupLeaveSafetyMLSRunner) Run(_ context.Context, _ string, args []string, _ []byte) ([]byte, []byte, error) {
	r.calls = append(r.calls, strings.Join(args, " "))
	return []byte(`{"ok":true,"api_version":"anp-mls/v1","request_id":"req-other","result":{}}`), nil, nil
}

func TestUnsupportedGroupE2EESelfLeaveReasonDetectsNonAdvancingEpoch(t *testing.T) {
	t.Parallel()

	if reason := unsupportedGroupE2EESelfLeaveReason(map[string]any{"from_epoch": "7", "to_epoch": "7"}); !strings.Contains(reason, "non-advancing") {
		t.Fatalf("same epoch reason = %q, want non-advancing", reason)
	}
	if reason := unsupportedGroupE2EESelfLeaveReason(map[string]any{"from_epoch": "7", "epoch": "6"}); !strings.Contains(reason, "non-advancing") {
		t.Fatalf("regressed epoch reason = %q, want non-advancing", reason)
	}
	if reason := unsupportedGroupE2EESelfLeaveReason(map[string]any{"artifact_type": "local-terminal-leave", "from_epoch": "7", "to_epoch": "8"}); !strings.Contains(reason, "local-terminal") {
		t.Fatalf("local terminal reason = %q, want local-terminal", reason)
	}
	if reason := unsupportedGroupE2EESelfLeaveReason(map[string]any{"from_epoch": "7", "to_epoch": "8"}); reason != "" {
		t.Fatalf("advancing epoch reason = %q, want empty", reason)
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

func TestGroupMemberMutationUsesPreMutationE2EESnapshot(t *testing.T) {
	t.Parallel()

	preMutation := map[string]any{
		"metadata": `{"message_security_profile":"group-e2ee","group_e2ee":{"epoch":"1"}}`,
	}
	postMutation := map[string]any{
		"metadata": `{"group_state_version":"2"}`,
	}
	if !groupMemberMutationUsesE2EE(GroupMemberRequest{}, preMutation, postMutation) {
		t.Fatal("groupMemberMutationUsesE2EE() = false, want true from pre-mutation E2EE summary")
	}
	if !groupMemberMutationUsesE2EE(GroupMemberRequest{E2EE: true}, nil, nil) {
		t.Fatal("groupMemberMutationUsesE2EE() = false, want true from explicit E2EE request")
	}
	if groupMemberMutationUsesE2EE(GroupMemberRequest{}, nil, postMutation) {
		t.Fatal("groupMemberMutationUsesE2EE() = true, want false for non-E2EE snapshots")
	}
}

func TestShouldAbortGroupE2EEPendingCommitOnlyForDeterministicServiceRejection(t *testing.T) {
	t.Parallel()

	if !shouldAbortGroupE2EEPendingCommit(&ServiceError{StatusCode: http.StatusForbidden, Message: "forbidden"}) {
		t.Fatal("403 service rejection should abort pending commit")
	}
	if !shouldAbortGroupE2EEPendingCommit(&ServiceError{RPCCode: 2501, Message: "inactive member"}) {
		t.Fatal("2xxx RPC rejection should abort pending commit")
	}
	if shouldAbortGroupE2EEPendingCommit(&ServiceError{StatusCode: http.StatusServiceUnavailable, Message: "retry later"}) {
		t.Fatal("5xx service failure should leave pending commit intact")
	}
	if shouldAbortGroupE2EEPendingCommit(&ServiceError{RPCCode: 1503, Message: "temporarily unavailable"}) {
		t.Fatal("1xxx retryable RPC failure should leave pending commit intact")
	}
	if shouldAbortGroupE2EEPendingCommit(fmt.Errorf("connection reset")) {
		t.Fatal("transport/network errors should leave pending commit intact")
	}
}

func TestGroupStateRefFromSnapshotUsesServerStateVersionNotMLSEpoch(t *testing.T) {
	t.Parallel()

	ref := groupStateRefFromSnapshot("did:wba:example.com:groups:demo:e1", map[string]any{
		"metadata": `{"group_state_version":"2","group_e2ee":{"epoch":"1","crypto_group_id_b64u":"Y3J5cHRv"}}`,
	})
	if got := stringFromAny(ref["group_state_version"]); got != "2" {
		t.Fatalf("group_state_version = %q, want server version 2", got)
	}
	if got := stringFromAny(ref["crypto_group_id_b64u"]); got != "Y3J5cHRv" {
		t.Fatalf("crypto_group_id_b64u = %q, want cached crypto id", got)
	}

	ref = groupStateRefFromSnapshot("did:wba:example.com:groups:demo:e1", map[string]any{
		"metadata": `{"group_e2ee":{"epoch":"7","crypto_group_id_b64u":"Y3J5cHRv"}}`,
	})
	if _, ok := ref["group_state_version"]; ok {
		t.Fatalf("group_state_version = %#v, want absent when only MLS epoch is cached", ref["group_state_version"])
	}
}

func TestLocalIdentityByDIDFindsStoredMemberForWelcomeProcessing(t *testing.T) {
	t.Parallel()

	resolved := testResolvedConfig(t)
	manager := identity.NewManager(resolved.Paths)
	createTestIdentity(t, manager, identity.SaveInput{
		IdentityName: "alice",
		UserID:       "user-alice",
		DisplayName:  "Alice",
		Handle:       "alice",
	})
	createTestIdentity(t, manager, identity.SaveInput{
		IdentityName: "bob",
		UserID:       "user-bob",
		DisplayName:  "Bob",
		Handle:       "bob",
	})
	bob, err := manager.Load("bob")
	if err != nil {
		t.Fatalf("Load(bob) error = %v", err)
	}
	service := &Service{resolved: resolved, manager: manager}

	found, err := service.localIdentityByDID(bob.DID)
	if err != nil {
		t.Fatalf("localIdentityByDID() error = %v", err)
	}
	if found.IdentityName != "bob" {
		t.Fatalf("localIdentityByDID() identity = %q, want bob", found.IdentityName)
	}
}

func TestGroupE2EEWelcomeDeviceIDUsesPublicKeyPackageDevice(t *testing.T) {
	t.Parallel()

	leasedPackage := map[string]any{
		"group_key_package": map[string]any{
			"device_id": "bob-main",
		},
	}
	if got := groupE2EEWelcomeDeviceID(leasedPackage); got != "bob-main" {
		t.Fatalf("groupE2EEWelcomeDeviceID() = %q, want bob-main", got)
	}
	if got := groupE2EEWelcomeDeviceID(nil); got != "default" {
		t.Fatalf("groupE2EEWelcomeDeviceID(nil) = %q, want default", got)
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

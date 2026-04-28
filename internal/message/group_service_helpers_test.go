package message

import "testing"

func TestGroupServiceHelperContracts(t *testing.T) {
	t.Parallel()

	snapshot := normalizeGroupSnapshot(map[string]any{
		"group_did":           "did:group",
		"group_state_version": "9",
		"member_count":        3,
		"group_profile": map[string]any{
			"display_name":    "Demo",
			"description":     "A demo group",
			"discoverability": "private",
		},
		"group_policy": map[string]any{"admission_mode": "open-join"},
	})
	if snapshot == nil || snapshot["group_did"] != "did:group" || snapshot["name"] != "Demo" {
		t.Fatalf("normalizeGroupSnapshot() = %#v, want normalized snapshot", snapshot)
	}
	if got := normalizeGroupSnapshot(map[string]any{"group_snapshot": map[string]any{"group_did": "did:inner"}}); got["group_did"] != "did:inner" {
		t.Fatalf("normalizeGroupSnapshot(group_snapshot) = %#v", got)
	}
	if got := normalizeGroupSnapshot(nil); got != nil {
		t.Fatalf("normalizeGroupSnapshot(nil) = %#v, want nil", got)
	}

	if !isActiveGroupOwner(map[string]any{"my_role": "owner", "membership_status": "active"}) {
		t.Fatal("isActiveGroupOwner() = false, want true")
	}
	if isActiveGroupOwner(map[string]any{"my_role": "member", "membership_status": "active"}) {
		t.Fatal("isActiveGroupOwner(member) = true, want false")
	}

	if !isInactiveGroupViewerError(&ServiceError{RPCCode: 2501, Message: "viewer inactive"}) {
		t.Fatal("isInactiveGroupViewerError(RPC 2501) = false, want true")
	}
	if !isInactiveGroupViewerError(&ServiceError{Message: "viewer is not an active member"}) {
		t.Fatal("isInactiveGroupViewerError(text) = false, want true")
	}
	if shouldUseCachedGroupFallback(&ServiceError{RPCCode: 2501, Message: "viewer inactive"}) {
		t.Fatal("shouldUseCachedGroupFallback(inactive viewer) = true, want false")
	}
}

func TestGroupMessageContentAndValueHelpers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		method string
		want   string
	}{
		{"group.join", "group_system_member_joined"},
		{"group.add", "group_system_member_joined"},
		{"group.leave", "group_system_member_left"},
		{"group.remove", "group_system_member_kicked"},
		{"group.unknown", "application/json"},
	}
	for _, tc := range cases {
		got := inferGroupMessageContentType(map[string]any{"system_event": map[string]any{"subject_method": tc.method}})
		if got != tc.want {
			t.Fatalf("inferGroupMessageContentType(%s) = %q, want %q", tc.method, got, tc.want)
		}
	}
	if got := inferGroupMessageContentType(map[string]any{"text": "hello"}); got != "text/plain" {
		t.Fatalf("inferGroupMessageContentType(text) = %q, want text/plain", got)
	}

	if got := parseInt64Ptr("42"); got == nil || *got != 42 {
		t.Fatalf("parseInt64Ptr(42) = %#v, want 42", got)
	}
	if got := parseInt64Ptr("not-a-number"); got != nil {
		t.Fatalf("parseInt64Ptr(invalid) = %#v, want nil", got)
	}
	if got := boolPtrFromAny("1"); got == nil || !*got {
		t.Fatalf("boolPtrFromAny(1) = %#v, want true", got)
	}
	if got := boolPtrFromAny(float64(0)); got == nil || *got {
		t.Fatalf("boolPtrFromAny(0) = %#v, want false", got)
	}
	if got := compactWarnings([]string{" a ", "", "a", "b"}); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("compactWarnings() = %#v, want [a b]", got)
	}
	if got := encodeAnyString(map[string]any{"ok": true}); got != `{"ok":true}` {
		t.Fatalf("encodeAnyString() = %q, want compact JSON", got)
	}
}

func TestMergeInboxMessagesSortsAndLimits(t *testing.T) {
	t.Parallel()

	merged := mergeInboxMessages(
		2,
		[]map[string]any{{"id": "old", "sent_at": "2026-04-18T10:00:00Z"}},
		[]map[string]any{
			{"id": "new", "sent_at": "2026-04-18T12:00:00Z"},
			{"id": "middle", "stored_at": "2026-04-18T11:00:00Z"},
		},
	)
	if len(merged) != 2 || merged[0]["id"] != "new" || merged[1]["id"] != "middle" {
		t.Fatalf("mergeInboxMessages() = %#v, want newest two messages", merged)
	}
}

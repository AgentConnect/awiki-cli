package cli

import "testing"

func TestNormalizeDebugHandleTrimsPrefixesAndDomains(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		" Alice.AWiki.ai ":      "alice",
		"wba://Bob.example.com": "bob",
		"carol":                 "carol",
		"":                      "",
	}
	for input, want := range tests {
		if got := normalizeDebugHandle(input); got != want {
			t.Fatalf("normalizeDebugHandle(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildHandleHistoryOwnersAggregatesByOwner(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"owner_did": "did:owner-a", "did": "did:peer-current"},
		{"owner_did": "did:owner-a", "did": "did:peer-old"},
		{"owner_did": "did:owner-b", "did": "did:peer-b"},
	}
	owners := buildHandleHistoryOwners(
		rows,
		map[string]string{"did:owner-a": "did:peer-current", "did:owner-b": "did:peer-b"},
		map[string][]string{
			"did:owner-a": {"did:peer-current", "did:peer-old"},
			"did:owner-b": {"did:peer-b"},
		},
	)
	if len(owners) != 2 {
		t.Fatalf("len(owners) = %d, want 2", len(owners))
	}
	if owners[0]["owner_did"] != "did:owner-a" || owners[0]["historical_count"] != 2 {
		t.Fatalf("owners[0] = %#v, want owner-a aggregate", owners[0])
	}
	if owners[1]["owner_did"] != "did:owner-b" || owners[1]["current_did"] != "did:peer-b" {
		t.Fatalf("owners[1] = %#v, want owner-b aggregate", owners[1])
	}
}

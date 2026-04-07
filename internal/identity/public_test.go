package identity

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicDataStripsInternalUserIDFields(t *testing.T) {
	t.Parallel()

	data := map[string]any{
		"identity": IdentitySummary{
			IdentityName: "alice",
			DID:          "did:wba:awiki.ai:user:alice",
			UserID:       "user-123",
			Handle:       "alice",
		},
		"legacy_scan": LegacyScan{
			IndexedEntries: map[string]IndexEntry{
				"alice": {
					CredentialName: "alice",
					DID:            "did:wba:awiki.ai:user:alice",
					UserID:         "user-123",
					Handle:         "alice",
				},
			},
			HasLegacy: true,
		},
		"result": map[string]any{
			"user_id": "user-123",
			"nested": map[string]any{
				"userId": "user-456",
				"handle": "alice",
			},
		},
	}

	sanitized := PublicData(data)
	raw, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	output := string(raw)
	for _, forbidden := range []string{"user_id", "userId", "UserID", "user-123", "user-456"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("sanitized output %q still contains %q", output, forbidden)
		}
	}
	if !strings.Contains(output, "alice") {
		t.Fatalf("sanitized output %q lost public handle fields", output)
	}
}

func TestEvaluateUserStateUsesPublicFriendlyMissingFields(t *testing.T) {
	t.Parallel()

	state := EvaluateUserState("", "")
	for _, item := range state.Missing {
		if item == "user_id" {
			t.Fatalf("unexpected internal field in Missing: %#v", state.Missing)
		}
	}
	expected := []string{"registration", "handle"}
	if len(state.Missing) != len(expected) {
		t.Fatalf("state.Missing = %#v, want %#v", state.Missing, expected)
	}
	for index, want := range expected {
		if state.Missing[index] != want {
			t.Fatalf("state.Missing[%d] = %q, want %q", index, state.Missing[index], want)
		}
	}
}

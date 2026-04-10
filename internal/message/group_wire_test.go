package message

import (
	"testing"

	"github.com/agentconnect/awiki-cli/internal/identity"
)

func TestBuildGroupCreateRPCParamsUsesOriginProofAndServiceTarget(t *testing.T) {
	t.Parallel()

	generated, err := identity.GenerateIdentity(identity.GenerateOptions{
		Hostname:    "awiki.ai",
		PathPrefix:  []string{"user"},
		ProofDomain: "awiki.ai",
	})
	if err != nil {
		t.Fatalf("GenerateIdentity() error = %v", err)
	}
	record := &identity.StoredIdentity{
		IdentityName:   "alice",
		DID:            generated.DID,
		DIDDocument:    generated.DIDDocument,
		Key1PrivatePEM: generated.Key1PrivatePEM,
	}

	params, err := BuildGroupCreateRPCParams(record, nil, "did:wba:awiki.ai:services:message:e1_local", GroupCreateRequest{Name: "Protocol Review"})
	if err != nil {
		t.Fatalf("BuildGroupCreateRPCParams() error = %v", err)
	}
	auth, ok := params["auth"].(map[string]any)
	if !ok {
		t.Fatalf("params[auth] = %#v, want map", params["auth"])
	}
	if got := stringFromAny(auth["scheme"]); got != OriginProofScheme {
		t.Fatalf("auth.scheme = %q, want %q", got, OriginProofScheme)
	}
	if _, ok := auth["origin_proof"]; !ok {
		t.Fatalf("auth.origin_proof missing: %#v", auth)
	}
	if _, ok := auth["actor_proof"]; ok {
		t.Fatalf("auth.actor_proof should be absent: %#v", auth)
	}
	meta, ok := params["meta"].(map[string]any)
	if !ok {
		t.Fatalf("params[meta] = %#v, want map", params["meta"])
	}
	target, ok := meta["target"].(map[string]any)
	if !ok {
		t.Fatalf("meta[target] = %#v, want map", meta["target"])
	}
	if got := stringFromAny(target["kind"]); got != "service" {
		t.Fatalf("meta.target.kind = %q, want service", got)
	}
}

func TestBuildGroupMessagesRPCParamsUsesLocalProfile(t *testing.T) {
	t.Parallel()

	record := &identity.StoredIdentity{DID: "did:wba:awiki.ai:user:alice:e1_alice"}
	params, err := BuildGroupMessagesRPCParams(record, GroupMessagesRequest{
		Group:  "did:wba:awiki.ai:groups:demo:e1_group",
		Limit:  25,
		Cursor: "12",
		Skip:   50,
	})
	if err != nil {
		t.Fatalf("BuildGroupMessagesRPCParams() error = %v", err)
	}
	meta, _ := params["meta"].(map[string]any)
	if got := stringFromAny(meta["profile"]); got != "anp.group.local.v1" {
		t.Fatalf("meta.profile = %q, want anp.group.local.v1", got)
	}
	body, _ := params["body"].(map[string]any)
	if got := stringFromAny(body["since_seq"]); got != "12" {
		t.Fatalf("body.since_seq = %q, want 12", got)
	}
	if got := intValueFromAny(body["skip"], 0); got != 50 {
		t.Fatalf("body.skip = %d, want 50", got)
	}
}

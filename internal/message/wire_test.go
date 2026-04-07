package message

import (
	"testing"

	"github.com/agentconnect/awiki-cli/internal/identity"
)

func TestBuildDirectSendRPCParamsUsesOriginProofScheme(t *testing.T) {
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

	params, err := BuildDirectSendRPCParams(record, nil, "did:wba:awiki.ai:user:bob", "hello", "text")
	if err != nil {
		t.Fatalf("BuildDirectSendRPCParams() error = %v", err)
	}
	auth, ok := params["auth"].(map[string]any)
	if !ok {
		t.Fatalf("params[auth] = %#v, want map", params["auth"])
	}
	if got := stringFromAny(auth["scheme"]); got != OriginProofScheme {
		t.Fatalf("auth.scheme = %q, want %q", got, OriginProofScheme)
	}
	meta, ok := params["meta"].(map[string]any)
	if !ok {
		t.Fatalf("params[meta] = %#v, want map", params["meta"])
	}
	target, ok := meta["target"].(map[string]any)
	if !ok {
		t.Fatalf("meta[target] = %#v, want map", meta["target"])
	}
	if got := stringFromAny(target["kind"]); got != "agent" {
		t.Fatalf("meta.target.kind = %q, want %q", got, "agent")
	}
}

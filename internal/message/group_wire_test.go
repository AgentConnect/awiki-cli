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

func TestBuildGroupCreateRPCParamsAppliesDefaultPolicyContract(t *testing.T) {
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

	params, err := BuildGroupCreateRPCParams(
		record,
		nil,
		"did:wba:awiki.ai:services:message:e1_service",
		GroupCreateRequest{Name: "Protocol Review"},
	)
	if err != nil {
		t.Fatalf("BuildGroupCreateRPCParams() error = %v", err)
	}

	body := mustMapValue(t, params["body"], "params.body")
	profile := mustMapValue(t, body["group_profile"], "body.group_profile")
	if got := stringFromAny(profile["display_name"]); got != "Protocol Review" {
		t.Fatalf("group_profile.display_name = %q, want Protocol Review", got)
	}
	policy := mustMapValue(t, body["group_policy"], "body.group_policy")
	if got := stringFromAny(policy["admission_mode"]); got != "open-join" {
		t.Fatalf("group_policy.admission_mode = %q, want open-join", got)
	}
	if got := boolFromAny(policy["attachments_allowed"]); !got {
		t.Fatalf("group_policy.attachments_allowed = %v, want true", got)
	}
	if got := stringFromAny(policy["max_members"]); got != "500" {
		t.Fatalf("group_policy.max_members = %q, want 500", got)
	}
	if got := stringFromAny(policy["message_security_profile"]); got != "transport-protected" {
		t.Fatalf("group_policy.message_security_profile = %q, want transport-protected", got)
	}
	if got := stringFromAny(policy["bootstrap_security_profile"]); got != "transport-protected" {
		t.Fatalf("group_policy.bootstrap_security_profile = %q, want transport-protected", got)
	}
	permissions := mustMapValue(t, policy["permissions"], "group_policy.permissions")
	if got := stringFromAny(permissions["send"]); got != "member" {
		t.Fatalf("group_policy.permissions.send = %q, want member", got)
	}
	if got := stringFromAny(permissions["update_policy"]); got != "owner" {
		t.Fatalf("group_policy.permissions.update_policy = %q, want owner", got)
	}
}

func TestBuildGroupMembersRPCParamsDefaultsLimitToHundred(t *testing.T) {
	t.Parallel()

	record := &identity.StoredIdentity{DID: "did:wba:awiki.ai:user:alice:e1_alice"}
	params, err := BuildGroupMembersRPCParams(record, GroupMembersRequest{
		Group: "did:wba:awiki.ai:groups:demo:e1_group",
	})
	if err != nil {
		t.Fatalf("BuildGroupMembersRPCParams() error = %v", err)
	}

	meta := mustMapValue(t, params["meta"], "params.meta")
	if got := stringFromAny(meta["profile"]); got != "anp.group.local.v1" {
		t.Fatalf("meta.profile = %q, want anp.group.local.v1", got)
	}
	body := mustMapValue(t, params["body"], "params.body")
	if got := intValueFromAny(body["limit"], 0); got != 100 {
		t.Fatalf("body.limit = %d, want 100", got)
	}
	if got := stringFromAny(body["group_did"]); got != "did:wba:awiki.ai:groups:demo:e1_group" {
		t.Fatalf("body.group_did = %q, want group did", got)
	}
}

func TestBuildGroupMessagesRPCParamsDefaultsLimitToFifty(t *testing.T) {
	t.Parallel()

	record := &identity.StoredIdentity{DID: "did:wba:awiki.ai:user:alice:e1_alice"}
	params, err := BuildGroupMessagesRPCParams(record, GroupMessagesRequest{
		Group: "did:wba:awiki.ai:groups:demo:e1_group",
	})
	if err != nil {
		t.Fatalf("BuildGroupMessagesRPCParams() error = %v", err)
	}

	body := mustMapValue(t, params["body"], "params.body")
	if got := intValueFromAny(body["limit"], 0); got != 50 {
		t.Fatalf("body.limit = %d, want 50", got)
	}
	if _, ok := body["since_seq"]; ok {
		t.Fatalf("body.since_seq should be absent when cursor is empty: %#v", body)
	}
	if _, ok := body["skip"]; ok {
		t.Fatalf("body.skip should be absent when skip is zero: %#v", body)
	}
}

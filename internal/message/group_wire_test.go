package message

import (
	"testing"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
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

func TestBuildGroupCreateRPCParamsAppliesGroupE2EEProfile(t *testing.T) {
	t.Parallel()

	record := testStoredIdentity(t)
	params, err := BuildGroupCreateRPCParams(
		record,
		nil,
		"did:wba:awiki.ai:services:message:e1_service",
		GroupCreateRequest{Name: "Encrypted Group", E2EE: true},
	)
	if err != nil {
		t.Fatalf("BuildGroupCreateRPCParams() error = %v", err)
	}
	body := mustMapValue(t, params["body"], "params.body")
	policy := mustMapValue(t, body["group_policy"], "body.group_policy")
	if got := stringFromAny(policy["message_security_profile"]); got != GroupE2EESecurityProfile {
		t.Fatalf("message_security_profile = %q, want %q", got, GroupE2EESecurityProfile)
	}
	if got := stringFromAny(policy["bootstrap_security_profile"]); got != GroupE2EESecurityProfile {
		t.Fatalf("bootstrap_security_profile = %q, want %q", got, GroupE2EESecurityProfile)
	}
}

func TestBuildGroupE2EESendRPCParamsSendsOnlyOpaqueCipherObject(t *testing.T) {
	t.Parallel()

	record := testStoredIdentity(t)
	params, err := BuildGroupE2EESendRPCParams(record, nil, "did:wba:awiki.ai:groups:demo:e1_group", map[string]any{
		"crypto_group_id_b64u":  "Y3J5cHRv",
		"openmls_group_id_b64u": "provider-local",
		"epoch":                 "1",
		"private_message_b64u":  "Y2lwaGVy",
		"epoch_authenticator":   "YXV0aA",
		"group_state_ref":       map[string]any{"group_did": "did:wba:awiki.ai:groups:demo:e1_group", "group_state_version": "7"},
		"non_cryptographic":     true,
		"artifact_mode":         "contract-test",
		"application_plaintext": map[string]any{"text": "secret"},
	}, "op-e2ee-send", "msg-e2ee-send")
	if err != nil {
		t.Fatalf("BuildGroupE2EESendRPCParams() error = %v", err)
	}
	meta := mustMapValue(t, params["meta"], "params.meta")
	if got := stringFromAny(meta["profile"]); got != GroupE2EEProfile {
		t.Fatalf("meta.profile = %q, want %q", got, GroupE2EEProfile)
	}
	if got := stringFromAny(meta["security_profile"]); got != GroupE2EESecurityProfile {
		t.Fatalf("meta.security_profile = %q, want %q", got, GroupE2EESecurityProfile)
	}
	if got := stringFromAny(meta["content_type"]); got != "application/anp-group-cipher+json" {
		t.Fatalf("meta.content_type = %q, want group cipher", got)
	}
	if got := stringFromAny(meta["operation_id"]); got != "op-e2ee-send" {
		t.Fatalf("meta.operation_id = %q, want op-e2ee-send", got)
	}
	if got := stringFromAny(meta["message_id"]); got != "msg-e2ee-send" {
		t.Fatalf("meta.message_id = %q, want msg-e2ee-send", got)
	}
	body := mustMapValue(t, params["body"], "params.body")
	if _, ok := body["application_plaintext"]; ok {
		t.Fatalf("plaintext leaked into E2EE send body: %#v", body)
	}
	groupCipher := body
	if _, ok := groupCipher["openmls_group_id_b64u"]; ok {
		t.Fatalf("provider-local OpenMLS group id leaked into service body: %#v", groupCipher)
	}
	if _, ok := groupCipher["application_plaintext"]; ok {
		t.Fatalf("plaintext leaked into service cipher object: %#v", groupCipher)
	}
	if _, ok := groupCipher["non_cryptographic"]; ok {
		t.Fatalf("legacy contract marker leaked into service cipher object: %#v", groupCipher)
	}
	if _, ok := groupCipher["artifact_mode"]; ok {
		t.Fatalf("legacy artifact marker leaked into service cipher object: %#v", groupCipher)
	}
	if got := stringFromAny(groupCipher["crypto_group_id_b64u"]); got != "Y3J5cHRv" {
		t.Fatalf("crypto_group_id_b64u = %q, want Y3J5cHRv", got)
	}
	ref := mustMapValue(t, groupCipher["group_state_ref"], "group_cipher.group_state_ref")
	if got := stringFromAny(ref["group_state_version"]); got != "7" {
		t.Fatalf("group_state_ref.group_state_version = %q, want server state version 7", got)
	}
}

func TestBuildGroupE2EEAddRPCParamsIncludesConsumedKeyPackageID(t *testing.T) {
	t.Parallel()

	record := testStoredIdentity(t)
	params, err := BuildGroupE2EEAddRPCParams(record, nil, "did:wba:awiki.ai:groups:demo:e1_group", "did:wba:awiki.ai:user:bob:e1_bob", map[string]any{
		"crypto_group_id_b64u": "Y3J5cHRv",
		"epoch":                "2",
		"epoch_authenticator":  "YXV0aDI",
		"welcome_b64u":         "d2VsY29tZQ",
		"commit_b64u":          "Y29tbWl0",
		"ratchet_tree_b64u":    "cmF0Y2hldA",
		"key_package_id":       "kp-bob-1",
		"group_state_ref":      map[string]any{"group_did": "did:wba:awiki.ai:groups:demo:e1_group", "group_state_version": "12"},
		"group_key_package":    map[string]any{"owner_did": "did:wba:awiki.ai:user:bob:e1_bob", "key_package_id": "kp-bob-1", "device_id": "phone"},
	})
	if err != nil {
		t.Fatalf("BuildGroupE2EEAddRPCParams() error = %v", err)
	}
	body := mustMapValue(t, params["body"], "params.body")
	if got := stringFromAny(body["subject_did"]); got != "did:wba:awiki.ai:user:bob:e1_bob" {
		t.Fatalf("subject_did = %q, want bob", got)
	}
	if got := stringFromAny(body["member_did"]); got != "did:wba:awiki.ai:user:bob:e1_bob" {
		t.Fatalf("member_did = %q, want bob", got)
	}
	if got := stringFromAny(body["key_package_id"]); got != "kp-bob-1" {
		t.Fatalf("key_package_id = %q, want leased id", got)
	}
	groupKeyPackage := mustMapValue(t, body["group_key_package"], "body.group_key_package")
	if got := stringFromAny(groupKeyPackage["device_id"]); got != "phone" {
		t.Fatalf("group_key_package.device_id = %q, want phone", got)
	}
	if got := stringFromAny(body["subject_key_package_id"]); got != "kp-bob-1" {
		t.Fatalf("subject_key_package_id = %q, want leased id", got)
	}
	if got := stringFromAny(body["ratchet_tree_b64u"]); got != "cmF0Y2hldA" {
		t.Fatalf("ratchet_tree_b64u = %q, want ratchet tree", got)
	}
	ref := mustMapValue(t, body["group_state_ref"], "body.group_state_ref")
	if got := stringFromAny(ref["group_state_version"]); got != "12" {
		t.Fatalf("group_state_ref.group_state_version = %q, want P4 state version", got)
	}
}

func TestBuildGroupE2EERemoveRPCParamsUsesHiddenCompositeMutation(t *testing.T) {
	t.Parallel()

	record := testStoredIdentity(t)
	params, err := BuildGroupE2EERemoveRPCParams(record, nil, "did:wba:awiki.ai:groups:demo:e1_group", "did:wba:awiki.ai:user:bob:e1_bob", map[string]any{
		"pending_commit_id":         "pc-remove-1",
		"operation_id":              "op-remove-1",
		"crypto_group_id_b64u":      "Y3J5cHRv",
		"from_epoch":                "4",
		"to_epoch":                  "5",
		"commit_b64u":               "Y29tbWl0",
		"ratchet_tree_b64u":         "cmF0Y2hldA",
		"group_info_b64u":           "Z3JvdXBpbmZv",
		"epoch_authenticator_b64u":  "YXV0aDU",
		"application_plaintext":     "must-not-leak",
		"provider_private_material": "must-not-leak",
		"group_state_ref":           map[string]any{"group_did": "did:wba:awiki.ai:groups:demo:e1_group", "group_state_version": "8", "epoch": "4"},
	}, "cleanup", "leave-req-1")
	if err != nil {
		t.Fatalf("BuildGroupE2EERemoveRPCParams() error = %v", err)
	}
	meta := mustMapValue(t, params["meta"], "params.meta")
	if got := stringFromAny(meta["profile"]); got != GroupE2EEProfile {
		t.Fatalf("meta.profile = %q, want %q", got, GroupE2EEProfile)
	}
	if got := stringFromAny(meta["security_profile"]); got != GroupE2EESecurityProfile {
		t.Fatalf("meta.security_profile = %q, want %q", got, GroupE2EESecurityProfile)
	}
	if got := stringFromAny(meta["operation_id"]); got != "op-remove-1" {
		t.Fatalf("meta.operation_id = %q, want prepared operation id", got)
	}
	target := mustMapValue(t, meta["target"], "meta.target")
	if got := stringFromAny(target["kind"]); got != "group" {
		t.Fatalf("target.kind = %q, want group", got)
	}
	body := mustMapValue(t, params["body"], "params.body")
	if got := stringFromAny(body["subject_did"]); got != "did:wba:awiki.ai:user:bob:e1_bob" {
		t.Fatalf("subject_did = %q, want bob", got)
	}
	if got := stringFromAny(body["subject_status"]); got != "removed" {
		t.Fatalf("subject_status = %q, want removed", got)
	}
	if got := stringFromAny(body["commit_b64u"]); got != "Y29tbWl0" {
		t.Fatalf("commit_b64u = %q, want opaque commit", got)
	}
	if got := stringFromAny(body["leave_request_id"]); got != "leave-req-1" {
		t.Fatalf("leave_request_id = %q, want leave-req-1", got)
	}
	if _, ok := body["application_plaintext"]; ok {
		t.Fatalf("plaintext leaked into remove body: %#v", body)
	}
	if _, ok := body["provider_private_material"]; ok {
		t.Fatalf("provider private material leaked into remove body: %#v", body)
	}
	ref := mustMapValue(t, body["group_state_ref"], "body.group_state_ref")
	if got := stringFromAny(ref["epoch"]); got != "4" {
		t.Fatalf("group_state_ref.epoch = %q, want from_epoch/pre-remove epoch", got)
	}
	if got := stringFromAny(ref["group_state_version"]); got != "8" {
		t.Fatalf("group_state_ref.group_state_version = %q, want P4 state version", got)
	}
}

func TestBuildGroupE2EELeaveRequestRPCParamsUsesHiddenTransportProtectedControlPlane(t *testing.T) {
	t.Parallel()

	record := testStoredIdentity(t)
	params, err := BuildGroupE2EELeaveRequestRPCParams(record, nil, "did:wba:awiki.ai:groups:demo:e1_group", "done for now")
	if err != nil {
		t.Fatalf("BuildGroupE2EELeaveRequestRPCParams() error = %v", err)
	}
	meta := mustMapValue(t, params["meta"], "params.meta")
	if got := stringFromAny(meta["profile"]); got != GroupE2EEProfile {
		t.Fatalf("meta.profile = %q, want %q", got, GroupE2EEProfile)
	}
	if got := stringFromAny(meta["security_profile"]); got != GroupE2EETransportProfile {
		t.Fatalf("meta.security_profile = %q, want transport protected", got)
	}
	if _, ok := meta["message_id"]; ok {
		t.Fatalf("leave_request meta must not look like a public message: %#v", meta)
	}
	body := mustMapValue(t, params["body"], "params.body")
	if got := stringFromAny(body["subject_did"]); got != record.DID {
		t.Fatalf("subject_did = %q, want actor DID", got)
	}
	if got := stringFromAny(body["subject_status"]); got != "leave_requested" {
		t.Fatalf("subject_status = %q, want leave_requested", got)
	}
	if got := stringFromAny(body["reason_text"]); got != "done for now" {
		t.Fatalf("reason_text = %q, want reason", got)
	}
}

func TestBuildGroupE2EELeaveRPCParamsUsesActorAsSubject(t *testing.T) {
	t.Parallel()

	record := testStoredIdentity(t)
	params, err := BuildGroupE2EELeaveRPCParams(record, nil, "did:wba:awiki.ai:groups:demo:e1_group", map[string]any{
		"operation_id":         "op-leave-1",
		"pending_commit_id":    "pc-leave-1",
		"crypto_group_id_b64u": "Y3J5cHRv",
		"from_epoch":           "5",
		"to_epoch":             "6",
		"commit_b64u":          "Y29tbWl0LWxlYXZl",
	})
	if err != nil {
		t.Fatalf("BuildGroupE2EELeaveRPCParams() error = %v", err)
	}
	body := mustMapValue(t, params["body"], "params.body")
	if got := stringFromAny(body["subject_did"]); got != record.DID {
		t.Fatalf("subject_did = %q, want actor DID", got)
	}
	if got := stringFromAny(body["member_did"]); got != record.DID {
		t.Fatalf("member_did = %q, want actor DID", got)
	}
	if got := stringFromAny(body["subject_status"]); got != "left" {
		t.Fatalf("subject_status = %q, want left", got)
	}
	if got := stringFromAny(body["epoch"]); got != "6" {
		t.Fatalf("epoch = %q, want to_epoch", got)
	}
}

func TestBuildGroupE2EEPublishKeyPackageRPCParamsStripsProviderOnlyFields(t *testing.T) {
	t.Parallel()

	record := testStoredIdentity(t)
	params, err := BuildGroupE2EEPublishKeyPackageRPCParams(record, nil, "did:wba:awiki.ai:services:message:e1_service", map[string]any{
		"group_key_package": map[string]any{
			"owner_did":                record.DID,
			"key_package_id":           "kp-bob-main",
			"suite":                    "MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519",
			"mls_key_package_b64u":     "a3A",
			"did_wba_binding":          map[string]any{"agent_did": record.DID},
			"device_id":                "bob-main",
			"purpose":                  "recovery",
			"group_did":                "did:wba:awiki.ai:groups:demo:e1_group",
			"private_key_package_b64u": "must-not-leak",
		},
	})
	if err != nil {
		t.Fatalf("BuildGroupE2EEPublishKeyPackageRPCParams() error = %v", err)
	}
	body := mustMapValue(t, params["body"], "params.body")
	groupKeyPackage := mustMapValue(t, body["group_key_package"], "body.group_key_package")
	if got := stringFromAny(groupKeyPackage["device_id"]); got != "bob-main" {
		t.Fatalf("device_id = %q, want public device binding", got)
	}
	if _, ok := groupKeyPackage["private_key_package_b64u"]; ok {
		t.Fatalf("private provider field leaked into service KeyPackage payload: %#v", groupKeyPackage)
	}
	if got := stringFromAny(groupKeyPackage["key_package_id"]); got != "kp-bob-main" {
		t.Fatalf("key_package_id = %q, want kp-bob-main", got)
	}
	if got := stringFromAny(groupKeyPackage["purpose"]); got != "recovery" {
		t.Fatalf("purpose = %q, want recovery", got)
	}
	if got := stringFromAny(groupKeyPackage["group_did"]); got != "did:wba:awiki.ai:groups:demo:e1_group" {
		t.Fatalf("group_did = %q, want recovery group DID", got)
	}
	if _, ok := params["auth"]; !ok {
		t.Fatalf("auth missing from publish params: %#v", params)
	}
	meta := mustMapValue(t, params["meta"], "params.meta")
	if got := stringFromAny(meta["security_profile"]); got != "transport-protected" {
		t.Fatalf("publish security_profile = %q, want transport-protected", got)
	}
}

func TestSanitizeGroupKeyPackageForServiceOmitsEmptyOptionalRecoveryFields(t *testing.T) {
	t.Parallel()

	got := sanitizeGroupKeyPackageForService(map[string]any{
		"owner_did":            "did:wba:awiki.ai:user:bob:e1_bob",
		"device_id":            "bob-main",
		"key_package_id":       "kp-bob-main",
		"suite":                "MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519",
		"mls_key_package_b64u": "a3A",
		"did_wba_binding":      map[string]any{"agent_did": "did:wba:awiki.ai:user:bob:e1_bob"},
		"purpose":              "",
		"group_did":            "",
	})
	if _, ok := got["purpose"]; ok {
		t.Fatalf("empty purpose must not be sent to service: %#v", got)
	}
	if _, ok := got["group_did"]; ok {
		t.Fatalf("empty group_did must not be sent to service: %#v", got)
	}
}

func TestSignGroupKeyPackageDIDWBABindingAddsStrictObjectProof(t *testing.T) {
	t.Parallel()

	record := testStoredIdentity(t)
	providerResult := map[string]any{
		"group_key_package": map[string]any{
			"owner_did":            record.DID,
			"device_id":            "alice-main",
			"key_package_id":       "kp-alice-main",
			"suite":                "MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519",
			"mls_key_package_b64u": "a3A",
			"did_wba_binding": map[string]any{
				"agent_did":               record.DID,
				"verification_method":     record.DID + "#provider-placeholder",
				"leaf_signature_key_b64u": "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY",
				"issued_at":               "2026-01-01T00:00:00Z",
				"expires_at":              "2099-01-01T00:00:00Z",
			},
		},
	}

	signedResult, err := signGroupKeyPackageDIDWBABinding(record, providerResult)
	if err != nil {
		t.Fatalf("signGroupKeyPackageDIDWBABinding() error = %v", err)
	}
	groupKeyPackage := mustMapValue(t, signedResult["group_key_package"], "signed.group_key_package")
	binding := mustMapValue(t, groupKeyPackage["did_wba_binding"], "signed.did_wba_binding")
	if got := stringFromAny(binding["verification_method"]); got != verificationMethodID(record.DIDDocument) {
		t.Fatalf("verification_method = %q, want active identity method", got)
	}
	proof := mustMapValue(t, binding["proof"], "did_wba_binding.proof")
	if got := stringFromAny(proof["verificationMethod"]); got != verificationMethodID(record.DIDDocument) {
		t.Fatalf("proof.verificationMethod = %q, want active identity method", got)
	}
	if got := stringFromAny(proof["proofValue"]); got == "" || got[0] != 'z' {
		t.Fatalf("proofValue = %q, want multibase z proof", got)
	}
	if err := anpsdk.VerifyDidWbaBinding(binding, record.DIDDocument, anpsdk.DidWbaBindingVerificationOptions{
		Now:                        "2026-01-02T00:00:00Z",
		ExpectedLeafSignatureKey:   "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY",
		ExpectedCredentialIdentity: record.DID,
	}); err != nil {
		t.Fatalf("signed did_wba_binding did not verify: %v", err)
	}
	originalGroupKeyPackage := mustMapValue(t, providerResult["group_key_package"], "provider.group_key_package")
	originalBinding := mustMapValue(t, originalGroupKeyPackage["did_wba_binding"], "provider.did_wba_binding")
	if _, ok := originalBinding["proof"]; ok {
		t.Fatalf("signing should not mutate provider result: %#v", originalBinding)
	}
}

func TestBuildGroupE2EECreateRPCParamsUsesServiceTarget(t *testing.T) {
	t.Parallel()

	record := testStoredIdentity(t)
	params, err := BuildGroupE2EECreateRPCParams(record, nil, "did:wba:awiki.ai:services:message:e1_service", "did:wba:awiki.ai:groups:demo:e1_group", map[string]any{
		"crypto_group_id_b64u": "Y3J5cHRv",
		"epoch":                "0",
		"epoch_authenticator":  "YXV0aA",
		"group_state_ref":      map[string]any{"group_did": "did:wba:awiki.ai:groups:demo:e1_group", "group_state_version": "3"},
	})
	if err != nil {
		t.Fatalf("BuildGroupE2EECreateRPCParams() error = %v", err)
	}
	meta := mustMapValue(t, params["meta"], "params.meta")
	target := mustMapValue(t, meta["target"], "meta.target")
	if got := stringFromAny(target["kind"]); got != "service" {
		t.Fatalf("create target.kind = %q, want service", got)
	}
	if got := stringFromAny(target["did"]); got != "did:wba:awiki.ai:services:message:e1_service" {
		t.Fatalf("create target.did = %q, want service DID", got)
	}
	body := mustMapValue(t, params["body"], "params.body")
	if got := stringFromAny(body["group_did"]); got != "did:wba:awiki.ai:groups:demo:e1_group" {
		t.Fatalf("body.group_did = %q, want group DID", got)
	}
	ref := mustMapValue(t, body["group_state_ref"], "body.group_state_ref")
	if got := stringFromAny(ref["group_did"]); got != "did:wba:awiki.ai:groups:demo:e1_group" {
		t.Fatalf("group_state_ref.group_did = %q, want group DID", got)
	}
	if got := stringFromAny(ref["group_state_version"]); got != "3" {
		t.Fatalf("group_state_ref.group_state_version = %q, want P4 state version", got)
	}
}

func TestBuildGroupE2EENoticeRPCParamsUsesTransportProtectedAgentTarget(t *testing.T) {
	t.Parallel()

	record := testStoredIdentity(t)
	params, err := BuildGroupE2EENoticeRPCParams(record, nil, "did:wba:awiki.ai:groups:demo:e1_group", 500, true, []string{"notice-1"})
	if err != nil {
		t.Fatalf("BuildGroupE2EENoticeRPCParams() error = %v", err)
	}
	meta := mustMapValue(t, params["meta"], "params.meta")
	if got := stringFromAny(meta["security_profile"]); got != GroupE2EETransportProfile {
		t.Fatalf("notice security_profile = %q, want transport-protected", got)
	}
	target := mustMapValue(t, meta["target"], "meta.target")
	if got := stringFromAny(target["kind"]); got != "agent" {
		t.Fatalf("notice target.kind = %q, want agent", got)
	}
	if got := stringFromAny(target["did"]); got != record.DID {
		t.Fatalf("notice target.did = %q, want identity DID", got)
	}
	body := mustMapValue(t, params["body"], "params.body")
	if got := intValueFromAny(body["limit"], 0); got != 100 {
		t.Fatalf("notice limit = %d, want capped 100", got)
	}
	if got := stringFromAny(body["group_did"]); got != "did:wba:awiki.ai:groups:demo:e1_group" {
		t.Fatalf("notice group_did = %q, want group DID", got)
	}
	ids, ok := body["notice_ids"].([]string)
	if !ok || len(ids) != 1 || ids[0] != "notice-1" {
		t.Fatalf("notice_ids = %#v, want [notice-1]", body["notice_ids"])
	}
	if got := boolFromAny(body["mark_delivered"]); !got {
		t.Fatalf("mark_delivered = %v, want true", got)
	}
}

func TestBuildGroupE2EEHeadRPCParamsUsesTransportProtectedGroupTarget(t *testing.T) {
	t.Parallel()

	record := testStoredIdentity(t)
	groupDID := "did:wba:awiki.ai:groups:demo:e1_group"
	params, err := BuildGroupE2EEHeadRPCParams(record, nil, groupDID)
	if err != nil {
		t.Fatalf("BuildGroupE2EEHeadRPCParams() error = %v", err)
	}
	meta := mustMapValue(t, params["meta"], "params.meta")
	if got := stringFromAny(meta["profile"]); got != GroupE2EEProfile {
		t.Fatalf("head profile = %q, want group E2EE profile", got)
	}
	if got := stringFromAny(meta["security_profile"]); got != GroupE2EETransportProfile {
		t.Fatalf("head security_profile = %q, want transport-protected", got)
	}
	target := mustMapValue(t, meta["target"], "meta.target")
	if got := stringFromAny(target["kind"]); got != "group" {
		t.Fatalf("head target.kind = %q, want group", got)
	}
	if got := stringFromAny(target["did"]); got != groupDID {
		t.Fatalf("head target.did = %q, want group DID", got)
	}
	body := mustMapValue(t, params["body"], "params.body")
	if got := stringFromAny(body["group_did"]); got != groupDID {
		t.Fatalf("body.group_did = %q, want group DID", got)
	}
	ref := mustMapValue(t, body["group_state_ref"], "body.group_state_ref")
	if got := stringFromAny(ref["group_did"]); got != groupDID {
		t.Fatalf("group_state_ref.group_did = %q, want group DID", got)
	}
}

func TestBuildGroupE2EEGetKeyPackageUsesTransportProtectedServiceTarget(t *testing.T) {
	t.Parallel()

	record := testStoredIdentity(t)
	params, err := BuildGroupE2EEGetKeyPackageRPCParams(record, nil, "did:wba:awiki.ai:services:message:e1_service", "did:wba:awiki.ai:groups:demo:e1_group", "did:wba:awiki.ai:users:bob:e1_bob")
	if err != nil {
		t.Fatalf("BuildGroupE2EEGetKeyPackageRPCParams() error = %v", err)
	}
	meta := mustMapValue(t, params["meta"], "params.meta")
	if got := stringFromAny(meta["security_profile"]); got != "transport-protected" {
		t.Fatalf("get security_profile = %q, want transport-protected", got)
	}
	target := mustMapValue(t, meta["target"], "meta.target")
	if got := stringFromAny(target["kind"]); got != "service" {
		t.Fatalf("get target.kind = %q, want service", got)
	}
	body := mustMapValue(t, params["body"], "params.body")
	if got := stringFromAny(body["group_did"]); got != "did:wba:awiki.ai:groups:demo:e1_group" {
		t.Fatalf("body.group_did = %q, want group DID", got)
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

func testStoredIdentity(t *testing.T) *identity.StoredIdentity {
	t.Helper()
	generated, err := identity.GenerateIdentity(identity.GenerateOptions{
		Hostname:    "awiki.ai",
		PathPrefix:  []string{"user"},
		ProofDomain: "awiki.ai",
	})
	if err != nil {
		t.Fatalf("GenerateIdentity() error = %v", err)
	}
	return &identity.StoredIdentity{
		IdentityName:   "alice",
		DID:            generated.DID,
		DIDDocument:    generated.DIDDocument,
		Key1PrivatePEM: generated.Key1PrivatePEM,
	}
}

func TestBuildGroupE2EERecoverMemberRPCParamsAvoidsP4MembershipFields(t *testing.T) {
	t.Parallel()

	record := testStoredIdentity(t)
	params, err := BuildGroupE2EERecoverMemberRPCParams(record, nil, "did:wba:awiki.ai:groups:demo:e1_group", "did:wba:awiki.ai:user:bob:e1_bob", "bob-main", map[string]any{
		"operation_id":             "op-recover-1",
		"pending_commit_id":        "pc-recover-1",
		"crypto_group_id_b64u":     "Y3J5cHRv",
		"from_epoch":               "5",
		"to_epoch":                 "6",
		"commit_b64u":              "Y29tbWl0",
		"welcome_b64u":             "d2VsY29tZQ",
		"ratchet_tree_b64u":        "cmF0Y2hldA",
		"epoch_authenticator_b64u": "YXV0aDY",
		"group_state_ref":          map[string]any{"group_did": "did:wba:awiki.ai:groups:demo:e1_group", "group_state_version": "9"},
		"application_plaintext":    "must-not-leak",
		"member_did":               "must-not-be-forwarded",
	}, map[string]any{
		"key_package_id": "kp-recovery-1",
		"group_key_package": map[string]any{
			"owner_did":                "did:wba:awiki.ai:user:bob:e1_bob",
			"purpose":                  "recovery",
			"device_id":                "bob-main",
			"private_key_package_b64u": "must-not-leak",
		},
	})
	if err != nil {
		t.Fatalf("BuildGroupE2EERecoverMemberRPCParams() error = %v", err)
	}
	meta := mustMapValue(t, params["meta"], "params.meta")
	if got := stringFromAny(meta["operation_id"]); got != "op-recover-1" {
		t.Fatalf("operation_id = %q, want prepared operation", got)
	}
	body := mustMapValue(t, params["body"], "params.body")
	if _, ok := body["member_did"]; ok {
		t.Fatalf("recover_member must not carry P4 member_did: %#v", body)
	}
	if _, ok := body["role"]; ok {
		t.Fatalf("recover_member must not carry P4 role: %#v", body)
	}
	target := mustMapValue(t, body["target"], "body.target")
	if got := stringFromAny(target["agent_did"]); got != "did:wba:awiki.ai:user:bob:e1_bob" {
		t.Fatalf("target.agent_did = %q, want bob", got)
	}
	if got := stringFromAny(target["device_id"]); got != "bob-main" {
		t.Fatalf("target.device_id = %q, want bob-main", got)
	}
	if got := stringFromAny(body["recovery_key_package_id"]); got != "kp-recovery-1" {
		t.Fatalf("recovery_key_package_id = %q, want kp", got)
	}
	if _, ok := body["application_plaintext"]; ok {
		t.Fatalf("plaintext leaked into recovery body: %#v", body)
	}
	recoveryPackage := mustMapValue(t, body["group_key_package"], "body.group_key_package")
	if _, ok := recoveryPackage["private_key_package_b64u"]; ok {
		t.Fatalf("private KeyPackage material leaked: %#v", recoveryPackage)
	}
	if got := stringFromAny(recoveryPackage["purpose"]); got != "recovery" {
		t.Fatalf("recovery group_key_package.purpose = %q, want recovery", got)
	}
	if got := stringFromAny(recoveryPackage["device_id"]); got != "bob-main" {
		t.Fatalf("recovery group_key_package.device_id = %q, want bob-main", got)
	}
	ref := mustMapValue(t, body["group_state_ref"], "body.group_state_ref")
	if got := stringFromAny(ref["group_state_version"]); got != "9" {
		t.Fatalf("group_state_ref.group_state_version = %q, want P4 state version", got)
	}
}

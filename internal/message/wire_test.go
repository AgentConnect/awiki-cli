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
	if _, ok := auth["origin_proof"]; !ok {
		t.Fatalf("auth.origin_proof missing: %#v", auth)
	}
	if _, ok := auth["sender_proof"]; ok {
		t.Fatalf("auth.sender_proof should be absent: %#v", auth)
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

func TestBuildAttachmentCreateSlotRPCParamsUsesAttachmentProfile(t *testing.T) {
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

	params, err := BuildAttachmentCreateSlotRPCParams(record, nil, "did:wba:awiki.ai:services:message:e1", "agent", "did:wba:awiki.ai:user:bob", &preparedAttachment{
		Filename:   "hello.txt",
		MIMEType:   "text/plain",
		SizeString: "5",
		DigestB64U: "digest",
	})
	if err != nil {
		t.Fatalf("BuildAttachmentCreateSlotRPCParams() error = %v", err)
	}
	meta, ok := params["meta"].(map[string]any)
	if !ok {
		t.Fatalf("params[meta] = %#v, want map", params["meta"])
	}
	if got := stringFromAny(meta["profile"]); got != "anp.attachment.v1" {
		t.Fatalf("meta.profile = %q, want %q", got, "anp.attachment.v1")
	}
	target, ok := meta["target"].(map[string]any)
	if !ok {
		t.Fatalf("meta[target] = %#v, want map", meta["target"])
	}
	if got := stringFromAny(target["kind"]); got != "service" {
		t.Fatalf("meta.target.kind = %q, want %q", got, "service")
	}
	body, ok := params["body"].(map[string]any)
	if !ok {
		t.Fatalf("params[body] = %#v, want map", params["body"])
	}
	intendedTarget, ok := body["intended_target"].(map[string]any)
	if !ok {
		t.Fatalf("body[intended_target] = %#v, want map", body["intended_target"])
	}
	if got := stringFromAny(intendedTarget["kind"]); got != "agent" {
		t.Fatalf("body.intended_target.kind = %q, want %q", got, "agent")
	}
	if _, ok := params["auth"]; ok {
		t.Fatalf("attachment control params should not include auth: %#v", params["auth"])
	}
}

func TestFindAttachmentSelectionMatchesVisibleOrRawMessageID(t *testing.T) {
	t.Parallel()

	messages := []map[string]any{
		{
			"id":         "did:wba:awiki.ai:groups:test:e1_group:7",
			"message_id": "msg-raw-1",
			"sender_did": "did:wba:awiki.ai:user:alice:e1",
			"content": map[string]any{
				"attachments": []any{
					map[string]any{
						"attachment_id": "att-1",
						"filename":      "hello.txt",
						"mime_type":     "text/plain",
						"size":          "5",
						"digest":        map[string]any{"alg": "sha-256", "value_b64u": "digest"},
						"access_info": map[string]any{
							"object_uri": "https://awiki.test/objects/obj-1",
						},
					},
				},
				"primary_attachment_id": "att-1",
				"caption":               "hello",
			},
		},
	}

	selection, err := findAttachmentSelection(messages, "did:wba:awiki.ai:groups:test:e1_group:7", "")
	if err != nil {
		t.Fatalf("findAttachmentSelection() error = %v", err)
	}
	if selection.MessageID != "msg-raw-1" {
		t.Fatalf("selection.MessageID = %q, want %q", selection.MessageID, "msg-raw-1")
	}
	if selection.AttachmentID != "att-1" {
		t.Fatalf("selection.AttachmentID = %q, want %q", selection.AttachmentID, "att-1")
	}
	if selection.SenderDID != "did:wba:awiki.ai:user:alice:e1" {
		t.Fatalf("selection.SenderDID = %q, want sender DID", selection.SenderDID)
	}
}

func TestBuildAttachmentDownloadTicketRPCParamsIncludesSenderDID(t *testing.T) {
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
		IdentityName:   "bob",
		DID:            generated.DID,
		DIDDocument:    generated.DIDDocument,
		Key1PrivatePEM: generated.Key1PrivatePEM,
	}
	params, err := BuildAttachmentDownloadTicketRPCParams(
		record,
		nil,
		"did:wba:awiki.ai",
		"did:wba:awiki.ai:user:alice:e1",
		"msg-1",
		"",
		&attachmentSelection{
			AttachmentID: "att-1",
			ObjectURI:    "https://awiki.test/objects/obj-1",
		},
	)
	if err != nil {
		t.Fatalf("BuildAttachmentDownloadTicketRPCParams() error = %v", err)
	}
	body, ok := params["body"].(map[string]any)
	if !ok {
		t.Fatalf("params[body] = %#v, want map", params["body"])
	}
	if got := stringFromAny(body["sender_did"]); got != "did:wba:awiki.ai:user:alice:e1" {
		t.Fatalf("body.sender_did = %q, want sender DID", got)
	}
	if _, ok := params["auth"]; ok {
		t.Fatalf("download ticket params should not include auth: %#v", params["auth"])
	}
}

package message

import (
	"fmt"
	"strings"
	"time"

	"github.com/agentconnect/awiki-cli/internal/identity"
)

func BuildAttachmentCreateSlotRPCParams(
	record *identity.StoredIdentity,
	manager *identity.Manager,
	serviceDID string,
	targetKind string,
	targetDID string,
	prepared *preparedAttachment,
) (map[string]any, error) {
	if prepared == nil {
		return nil, ErrFilePathRequired
	}
	if strings.TrimSpace(serviceDID) == "" {
		return nil, fmt.Errorf("message service did is required")
	}
	if strings.TrimSpace(targetKind) == "" || strings.TrimSpace(targetDID) == "" {
		return nil, ErrTargetRequired
	}
	meta := map[string]any{
		"anp_version":      "1.0",
		"profile":          "anp.attachment.v1",
		"security_profile": "transport-protected",
		"sender_did":       record.DID,
		"target": map[string]any{
			"kind": "service",
			"did":  serviceDID,
		},
		"operation_id": "op-" + generateOperationID(),
		"created_at":   nowRFC3339(),
	}
	body := map[string]any{
		"expected_size": prepared.SizeString,
		"expected_digest": map[string]any{
			"alg":        "sha-256",
			"value_b64u": prepared.DigestB64U,
		},
		"mime_type":                         prepared.MIMEType,
		"filename":                          prepared.Filename,
		"intended_message_security_profile": "transport-protected",
		"intended_target": map[string]any{
			"kind": targetKind,
			"did":  targetDID,
		},
		"object_encryption_mode": "none",
	}
	return map[string]any{
		"meta": meta,
		"body": body,
	}, nil
}

func BuildAttachmentCommitObjectRPCParams(
	record *identity.StoredIdentity,
	manager *identity.Manager,
	serviceDID string,
	prepared *preparedAttachment,
	slot *attachmentCreateSlotResult,
) (map[string]any, error) {
	if prepared == nil || slot == nil {
		return nil, ErrFilePathRequired
	}
	meta := map[string]any{
		"anp_version":      "1.0",
		"profile":          "anp.attachment.v1",
		"security_profile": "transport-protected",
		"sender_did":       record.DID,
		"target": map[string]any{
			"kind": "service",
			"did":  serviceDID,
		},
		"operation_id": "op-" + generateOperationID(),
		"created_at":   nowRFC3339(),
	}
	body := map[string]any{
		"attachment_id":          slot.AttachmentID,
		"slot_id":                slot.SlotID,
		"commit_token":           slot.CommitToken,
		"size":                   prepared.SizeString,
		"object_encryption_mode": "none",
		"digest": map[string]any{
			"alg":        "sha-256",
			"value_b64u": prepared.DigestB64U,
		},
	}
	return map[string]any{
		"meta": meta,
		"body": body,
	}, nil
}

func BuildAttachmentDownloadTicketRPCParams(
	record *identity.StoredIdentity,
	manager *identity.Manager,
	serviceDID string,
	senderDID string,
	messageID string,
	groupDID string,
	selection *attachmentSelection,
) (map[string]any, error) {
	if selection == nil {
		return nil, ErrAttachmentNotFound
	}
	if strings.TrimSpace(serviceDID) == "" {
		return nil, fmt.Errorf("attachment service did is required")
	}
	if strings.TrimSpace(senderDID) == "" {
		return nil, ErrAttachmentSenderRequired
	}
	meta := map[string]any{
		"anp_version":      "1.0",
		"profile":          "anp.attachment.v1",
		"security_profile": "transport-protected",
		"sender_did":       record.DID,
		"target": map[string]any{
			"kind": "service",
			"did":  serviceDID,
		},
		"operation_id": "op-" + generateOperationID(),
		"created_at":   nowRFC3339(),
	}
	body := map[string]any{
		"attachment_id":            selection.AttachmentID,
		"object_uri":               selection.ObjectURI,
		"sender_did":               senderDID,
		"requester_did":            record.DID,
		"message_security_profile": "transport-protected",
		"message_id":               messageID,
		"one_time":                 true,
	}
	if strings.TrimSpace(groupDID) != "" {
		body["group_did"] = groupDID
	} else {
		body["message_target_did"] = record.DID
	}
	return map[string]any{
		"meta": meta,
		"body": body,
	}, nil
}

func BuildDirectAttachmentSendRPCParams(
	record *identity.StoredIdentity,
	manager *identity.Manager,
	targetDID string,
	manifest map[string]any,
) (map[string]any, error) {
	if strings.TrimSpace(targetDID) == "" {
		return nil, ErrTargetRequired
	}
	auth, err := newAuthContext(record, manager)
	if err != nil {
		return nil, err
	}
	meta := map[string]any{
		"anp_version":      "1.0",
		"profile":          "anp.direct.base.v1",
		"security_profile": "transport-protected",
		"sender_did":       record.DID,
		"target": map[string]any{
			"kind": "agent",
			"did":  targetDID,
		},
		"operation_id": "op-" + generateOperationID(),
		"message_id":   "msg-" + generateOperationID(),
		"created_at":   time.Now().UTC().Format(time.RFC3339),
		"content_type": attachmentManifestContentType,
	}
	body := map[string]any{"payload": manifest}
	payload := signedPayload{Method: "direct.send", Meta: meta, Body: body}
	originProof, err := buildOriginProof(auth, payload)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"meta": meta,
		"auth": map[string]any{"scheme": OriginProofScheme, "origin_proof": originProof},
		"body": body,
	}, nil
}

func BuildGroupAttachmentSendRPCParams(
	record *identity.StoredIdentity,
	manager *identity.Manager,
	groupDID string,
	manifest map[string]any,
) (map[string]any, error) {
	if strings.TrimSpace(groupDID) == "" {
		return nil, ErrGroupRequired
	}
	auth, err := newAuthContext(record, manager)
	if err != nil {
		return nil, err
	}
	meta := map[string]any{
		"anp_version":      "1.0",
		"profile":          "anp.group.base.v1",
		"security_profile": "transport-protected",
		"sender_did":       record.DID,
		"target": map[string]any{
			"kind": "group",
			"did":  groupDID,
		},
		"operation_id": "op-" + generateOperationID(),
		"message_id":   "msg-" + generateOperationID(),
		"created_at":   time.Now().UTC().Format(time.RFC3339),
		"content_type": attachmentManifestContentType,
	}
	body := map[string]any{"payload": manifest}
	payload := signedPayload{Method: "group.send", Meta: meta, Body: body}
	originProof, err := buildOriginProof(auth, payload)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"meta": meta,
		"auth": map[string]any{"scheme": OriginProofScheme, "origin_proof": originProof},
		"body": body,
	}, nil
}

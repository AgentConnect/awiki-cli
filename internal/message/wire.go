package message

import (
	"fmt"

	"github.com/agentconnect/awiki-cli/internal/identity"
)

func BuildDirectSendRPCParams(record *identity.StoredIdentity, manager *identity.Manager, targetDID string, text string, messageType string) (map[string]any, error) {
	auth, err := newAuthContext(record, manager)
	if err != nil {
		return nil, err
	}
	payload, err := buildDirectTextPayload(record.DID, targetDID, text, contentTypeForMessageType(messageType))
	if err != nil {
		return nil, err
	}
	originProof, err := buildOriginProof(auth, payload)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"meta": payload.Meta,
		"auth": map[string]any{
			"scheme":       OriginProofScheme,
			"origin_proof": originProof,
		},
		"body": payload.Body,
	}, nil
}

func BuildInboxRPCParams(record *identity.StoredIdentity, request InboxRequest) map[string]any {
	limit := request.Limit
	if limit <= 0 {
		limit = 20
	}
	return map[string]any{
		"meta": map[string]any{
			"anp_version":      "1.0",
			"profile":          "anp.inbox.local.v1",
			"security_profile": "transport-protected",
			"sender_did":       record.DID,
			"operation_id":     "op-" + generateOperationID(),
			"created_at":       nowRFC3339(),
		},
		"body": map[string]any{
			"user_did": record.DID,
			"limit":    limit,
		},
	}
}

func BuildHistoryRPCParams(record *identity.StoredIdentity, request HistoryRequest) (map[string]any, error) {
	if request.With == "" {
		return nil, ErrTargetRequired
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 50
	}
	body := map[string]any{
		"user_did": record.DID,
		"peer_did": request.With,
		"limit":    limit,
	}
	if request.Cursor != "" {
		body["since_seq"] = request.Cursor
	}
	if request.Skip > 0 {
		body["skip"] = request.Skip
	}
	return map[string]any{
		"meta": map[string]any{
			"anp_version":      "1.0",
			"profile":          "anp.direct.local.v1",
			"security_profile": "transport-protected",
			"sender_did":       record.DID,
			"operation_id":     "op-" + generateOperationID(),
			"created_at":       nowRFC3339(),
		},
		"body": body,
	}, nil
}

func BuildMarkReadRPCParams(record *identity.StoredIdentity, request MarkReadRequest) (map[string]any, error) {
	if len(request.MessageIDs) == 0 {
		return nil, fmt.Errorf("%w: message_ids are required", ErrMessageNotFound)
	}
	return map[string]any{
		"meta": map[string]any{
			"anp_version":      "1.0",
			"profile":          "anp.inbox.local.v1",
			"security_profile": "transport-protected",
			"sender_did":       record.DID,
			"operation_id":     "op-" + generateOperationID(),
			"created_at":       nowRFC3339(),
		},
		"body": map[string]any{
			"user_did":    record.DID,
			"message_ids": request.MessageIDs,
		},
	}, nil
}

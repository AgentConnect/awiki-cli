package message

import (
	"fmt"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
)

type directPayload struct {
	Method string         `json:"method"`
	Meta   map[string]any `json:"meta"`
	Body   map[string]any `json:"body"`
}

type signedPayload = directPayload

const OriginProofScheme = "anp-rfc9421-origin-proof-v1"

func buildDirectTextPayload(senderDID string, targetDID string, text string, contentType string) (directPayload, error) {
	if senderDID == "" || targetDID == "" {
		return directPayload{}, fmt.Errorf("sender and target did are required")
	}
	if text == "" {
		return directPayload{}, ErrTextRequired
	}
	if contentType == "" {
		contentType = "text/plain"
	}
	operationID := "op-" + generateOperationID()
	messageID := "msg-" + generateOperationID()
	meta := map[string]any{
		"anp_version":      "1.0",
		"profile":          "anp.direct.base.v1",
		"security_profile": "transport-protected",
		"sender_did":       senderDID,
		"target": map[string]any{
			"kind": "agent",
			"did":  targetDID,
		},
		"operation_id": operationID,
		"message_id":   messageID,
		"created_at":   nowRFC3339(),
		"content_type": contentType,
	}
	body := map[string]any{"text": text}
	return directPayload{Method: "direct.send", Meta: meta, Body: body}, nil
}

func buildOriginProof(auth *authContext, payload signedPayload) (map[string]any, error) {
	if auth == nil {
		return nil, fmt.Errorf("auth context is required")
	}
	keyID := verificationMethodID(auth.record.DIDDocument)
	if keyID == "" {
		return nil, fmt.Errorf(
			"identity %s is missing an authentication verification method",
			auth.record.IdentityName,
		)
	}
	proofValue, err := anpsdk.GenerateRFC9421OriginProof(
		payload.Method,
		payload.Meta,
		payload.Body,
		auth.privateKey,
		keyID,
		anpsdk.RFC9421OriginProofGenerationOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("generate origin proof: %w", err)
	}
	return map[string]any{
		"contentDigest":  proofValue.ContentDigest,
		"signatureInput": proofValue.SignatureInput,
		"signature":      proofValue.Signature,
	}, nil
}

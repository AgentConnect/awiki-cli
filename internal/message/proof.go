package message

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/google/uuid"
)

type directPayload struct {
	Method string         `json:"method"`
	Meta   map[string]any `json:"meta"`
	Body   map[string]any `json:"body"`
}

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
	operationID := "op-" + uuid.NewString()
	messageID := "msg-" + uuid.NewString()
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
		"created_at":   time.Now().UTC().Format(time.RFC3339),
		"content_type": contentType,
	}
	body := map[string]any{"text": text}
	return directPayload{Method: "direct.send", Meta: meta, Body: body}, nil
}

func buildSenderProof(auth *authContext, payload directPayload, targetDID string) (map[string]any, error) {
	keyID := verificationMethodID(auth.record.DIDDocument)
	if keyID == "" {
		return nil, fmt.Errorf("identity %s is missing an authentication verification method", auth.record.IdentityName)
	}
	payloadMap := map[string]any{
		"method": payload.Method,
		"meta":   payload.Meta,
		"body":   payload.Body,
	}
	canonicalPayload, err := canonicalJSON(payloadMap)
	if err != nil {
		return nil, fmt.Errorf("canonicalize direct payload: %w", err)
	}
	signatureInput, err := anpsdk.BuildIMSignatureInput(keyID, anpsdk.IMGenerationOptions{})
	if err != nil {
		return nil, fmt.Errorf("build sender proof signatureInput: %w", err)
	}
	parsed, err := anpsdk.ParseIMSignatureInput(signatureInput)
	if err != nil {
		return nil, fmt.Errorf("parse sender proof signatureInput: %w", err)
	}
	contentDigest := anpsdk.BuildIMContentDigest(canonicalPayload)
	signatureBase, err := buildBusinessSignatureBase(payload.Method, "anp://agent/"+strictPercentEncode(targetDID), contentDigest, parsed)
	if err != nil {
		return nil, err
	}
	signatureBytes, err := signBusinessProof(auth.privateKey, []byte(signatureBase))
	if err != nil {
		return nil, fmt.Errorf("sign sender proof: %w", err)
	}
	return map[string]any{
		"contentDigest":  contentDigest,
		"signatureInput": signatureInput,
		"signature":      anpsdk.EncodeIMSignature(signatureBytes, parsed.Label),
	}, nil
}

func strictPercentEncode(value string) string {
	var builder strings.Builder
	builder.Grow(len(value) * 3)
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if (ch >= 'A' && ch <= 'Z') ||
			(ch >= 'a' && ch <= 'z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '.' || ch == '_' || ch == '~' {
			builder.WriteByte(ch)
			continue
		}
		builder.WriteString(fmt.Sprintf("%%%02X", ch))
	}
	return builder.String()
}

func signBusinessProof(privateKey anpsdk.PrivateKeyMaterial, message []byte) ([]byte, error) {
	if privateKey.Type != anpsdk.KeyTypeSecp256k1 {
		return nil, fmt.Errorf("unsupported business proof key type: %s", privateKey.Type)
	}
	secpPrivateKey := secp256k1.PrivKeyFromBytes(privateKey.Bytes)
	ecdsaPrivateKey := secpPrivateKey.ToECDSA()
	digest := sha256.Sum256(message)
	r, s, err := ecdsa.Sign(rand.Reader, ecdsaPrivateKey, digest[:])
	if err != nil {
		return nil, err
	}
	curveOrder := secp256k1.S256().Params().N
	halfOrder := new(big.Int).Rsh(new(big.Int).Set(curveOrder), 1)
	if s.Cmp(halfOrder) > 0 {
		s.Sub(curveOrder, s)
	}
	signature := make([]byte, 64)
	copy(signature[32-len(r.Bytes()):32], r.Bytes())
	copy(signature[64-len(s.Bytes()):], s.Bytes())
	return signature, nil
}

func buildBusinessSignatureBase(method string, logicalTargetURI string, contentDigest string, parsed anpsdk.ParsedIMSignatureInput) (string, error) {
	lines := make([]string, 0, len(parsed.Components)+1)
	for _, component := range parsed.Components {
		var value string
		switch component {
		case "@method":
			value = method
		case "@target-uri":
			value = logicalTargetURI
		case "content-digest":
			value = contentDigest
		default:
			return "", fmt.Errorf("unsupported business proof component: %s", component)
		}
		lines = append(lines, fmt.Sprintf("\"%s\": %s", component, value))
	}
	lines = append(lines, fmt.Sprintf("\"@signature-params\": %s", parsed.SignatureParams))
	return strings.Join(lines, "\n"), nil
}

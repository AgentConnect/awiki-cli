package message

import (
	"testing"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
	"github.com/agentconnect/awiki-cli/internal/identity"
)

func TestBuildSenderProofRoundTrip(t *testing.T) {
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
		IdentityName:   "default",
		DID:            generated.DID,
		DIDDocument:    generated.DIDDocument,
		Key1PrivatePEM: generated.Key1PrivatePEM,
	}
	auth, err := newAuthContext(record, nil)
	if err != nil {
		t.Fatalf("newAuthContext() error = %v", err)
	}
	payload, err := buildDirectTextPayload(generated.DID, "did:wba:awiki.ai:user:bob", "hello", "text/plain")
	if err != nil {
		t.Fatalf("buildDirectTextPayload() error = %v", err)
	}
	proofMap, err := buildSenderProof(auth, payload, "did:wba:awiki.ai:user:bob")
	if err != nil {
		t.Fatalf("buildSenderProof() error = %v", err)
	}
	signatureInput, _ := proofMap["signatureInput"].(string)
	parsed, err := anpsdk.ParseIMSignatureInput(signatureInput)
	if err != nil {
		t.Fatalf("ParseIMSignatureInput() error = %v", err)
	}
	payloadMap := map[string]any{"method": payload.Method, "meta": payload.Meta, "body": payload.Body}
	canonicalPayload, err := canonicalJSON(payloadMap)
	if err != nil {
		t.Fatalf("canonicalJSON() error = %v", err)
	}
	signatureBase, err := buildBusinessSignatureBase(payload.Method, "anp://agent/"+strictPercentEncode("did:wba:awiki.ai:user:bob"), proofMap["contentDigest"].(string), parsed)
	if err != nil {
		t.Fatalf("buildBusinessSignatureBase() error = %v", err)
	}
	proof := anpsdk.IMProof{
		ContentDigest:  proofMap["contentDigest"].(string),
		SignatureInput: signatureInput,
		Signature:      proofMap["signature"].(string),
	}
	if _, err := anpsdk.VerifyIMProofWithDocument(proof, canonicalPayload, []byte(signatureBase), generated.DIDDocument, generated.DID); err != nil {
		t.Fatalf("VerifyIMProofWithDocument() error = %v", err)
	}
}

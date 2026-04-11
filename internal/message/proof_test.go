package message

import (
	"strings"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
	"github.com/agentconnect/awiki-cli/internal/identity"
)

func TestBuildOriginProofProducesRFC9421Fields(t *testing.T) {
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
	payload, err := buildDirectTextPayload(
		generated.DID,
		"did:wba:awiki.ai:user:bob",
		"hello",
		"text/plain",
	)
	if err != nil {
		t.Fatalf("buildDirectTextPayload() error = %v", err)
	}
	proofMap, err := buildOriginProof(auth, payload)
	if err != nil {
		t.Fatalf("buildOriginProof() error = %v", err)
	}
	signedRequestObject, err := anpsdk.BuildSignedRequestObject(
		payload.Method,
		payload.Meta,
		payload.Body,
	)
	if err != nil {
		t.Fatalf("BuildSignedRequestObject() error = %v", err)
	}
	canonicalRequest, err := anpsdk.CanonicalizeSignedRequestObject(signedRequestObject)
	if err != nil {
		t.Fatalf("CanonicalizeSignedRequestObject() error = %v", err)
	}
	contentDigest, _ := proofMap["contentDigest"].(string)
	signatureInput, _ := proofMap["signatureInput"].(string)
	signature, _ := proofMap["signature"].(string)
	if contentDigest == "" || signatureInput == "" || signature == "" {
		t.Fatalf("proof map is incomplete: %#v", proofMap)
	}
	if got, want := contentDigest, anpsdk.BuildIMContentDigest(canonicalRequest); got != want {
		t.Fatalf("contentDigest = %q, want %q", got, want)
	}
	if !strings.Contains(signatureInput, "\"@method\"") ||
		!strings.Contains(signatureInput, "\"@target-uri\"") ||
		!strings.Contains(signatureInput, "\"content-digest\"") {
		t.Fatalf("signatureInput = %q, want RFC9421 covered components", signatureInput)
	}
}

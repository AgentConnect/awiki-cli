package identity

import (
	"testing"

	anp "github.com/agent-network-protocol/anp/golang"
	anpauth "github.com/agent-network-protocol/anp/golang/authentication"
	anpproof "github.com/agent-network-protocol/anp/golang/proof"
)

func TestGenerateIdentity(t *testing.T) {
	t.Parallel()

	generated, err := GenerateIdentity(GenerateOptions{
		Hostname:    "awiki.ai",
		PathPrefix:  []string{"user"},
		ProofDomain: "awiki.ai",
	})
	if err != nil {
		t.Fatalf("GenerateIdentity() error = %v", err)
	}
	if generated.DID == "" || generated.UniqueID == "" {
		t.Fatalf("generated identity is missing did/unique_id: %+v", generated)
	}
	if generated.DIDDocument == nil {
		t.Fatal("generated identity is missing did_document")
	}
	if got := stringValue(generated.DIDDocument["id"], ""); got != generated.DID {
		t.Fatalf("did document id mismatch: got %q want %q", got, generated.DID)
	}
	if !anpauth.ValidateDIDDocumentBinding(generated.DIDDocument, true) {
		t.Fatal("generated did document failed did:wba binding validation")
	}
	publicKey, err := anp.PublicKeyFromPEM(generated.Key1PublicPEM)
	if err != nil {
		t.Fatalf("PublicKeyFromPEM() error = %v", err)
	}
	if !anpproof.VerifyW3CProof(generated.DIDDocument, publicKey, anpproof.VerificationOptions{
		ExpectedPurpose: "assertionMethod",
		ExpectedDomain:  "awiki.ai",
	}) {
		t.Fatal("generated did proof verification failed")
	}
	if generated.E2EESigningPrivatePEM == "" {
		t.Fatal("generated identity is missing e2ee signing private key")
	}
	if generated.E2EEAgreementPrivatePEM == "" {
		t.Fatal("generated identity is missing e2ee agreement private key")
	}
}

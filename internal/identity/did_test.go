package identity

import (
	"strings"
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
	if got := generated.DID; !strings.Contains(got, ":e1_") {
		t.Fatalf("generated DID = %q, want e1 profile suffix", got)
	}
	services, ok := generated.DIDDocument["service"].([]any)
	if !ok || len(services) != 1 {
		t.Fatalf("generated did document service = %#v, want exactly one ANP service", generated.DIDDocument["service"])
	}
	service, ok := services[0].(map[string]any)
	if !ok {
		t.Fatalf("generated did document service[0] = %#v, want map", services[0])
	}
	if got := stringValue(service["type"], ""); got != "ANPMessageService" {
		t.Fatalf("service type = %q, want %q", got, "ANPMessageService")
	}
	if got := stringValue(service["serviceEndpoint"], ""); got != "https://awiki.ai/message/rpc" {
		t.Fatalf("service endpoint = %q, want %q", got, "https://awiki.ai/message/rpc")
	}
	if got := stringValue(service["serviceDid"], ""); got != "did:wba:awiki.ai" {
		t.Fatalf("service DID = %q, want %q", got, "did:wba:awiki.ai")
	}
	profiles, ok := service["profiles"].([]any)
	if !ok {
		t.Fatalf("service profiles = %#v, want []any", service["profiles"])
	}
	expectedProfiles := []string{
		"anp.core.binding.v1",
		"anp.direct.base.v1",
		"anp.attachment.v1",
	}
	if len(profiles) != len(expectedProfiles) {
		t.Fatalf("service profiles len = %d, want %d: %#v", len(profiles), len(expectedProfiles), profiles)
	}
	for index, expected := range expectedProfiles {
		if got := stringValue(profiles[index], ""); got != expected {
			t.Fatalf("service profiles[%d] = %q, want %q", index, got, expected)
		}
	}
	for _, profile := range profiles {
		if got := stringValue(profile, ""); got == "anp.direct.e2ee.v1" {
			t.Fatalf("service profiles unexpectedly contains %q", got)
		}
	}
	securityProfiles, ok := service["securityProfiles"].([]any)
	if !ok {
		t.Fatalf("service securityProfiles = %#v, want []any", service["securityProfiles"])
	}
	if len(securityProfiles) != 1 {
		t.Fatalf("service securityProfiles len = %d, want 1: %#v", len(securityProfiles), securityProfiles)
	}
	if got := stringValue(securityProfiles[0], ""); got != "transport-protected" {
		t.Fatalf("service securityProfiles[0] = %q, want %q", got, "transport-protected")
	}
	for _, profile := range securityProfiles {
		if got := stringValue(profile, ""); got == "direct-e2ee" {
			t.Fatalf("service securityProfiles unexpectedly contains %q", got)
		}
	}
	if !anpauth.ValidateDIDDocumentBinding(generated.DIDDocument, true) {
		t.Fatal("generated did document failed did:wba binding validation")
	}
	publicKey, err := anp.PublicKeyFromPEM(generated.Key1PublicPEM)
	if err != nil {
		t.Fatalf("PublicKeyFromPEM() error = %v", err)
	}
	if publicKey.Type != anp.KeyTypeEd25519 {
		t.Fatalf("key-1 type = %v, want %v", publicKey.Type, anp.KeyTypeEd25519)
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

func TestGenerateIdentityRejectsLoopbackANPServiceEndpoint(t *testing.T) {
	t.Parallel()

	_, err := GenerateIdentity(GenerateOptions{
		Hostname:           "awiki.ai",
		PathPrefix:         []string{"user"},
		ProofDomain:        "awiki.ai",
		ANPServiceEndpoint: "http://127.0.0.1:9898/message/rpc",
	})
	if err == nil {
		t.Fatal("GenerateIdentity() error = nil, want loopback endpoint validation error")
	}
}

func TestGenerateIdentityRejectsNonBareANPServiceDID(t *testing.T) {
	t.Parallel()

	_, err := GenerateIdentity(GenerateOptions{
		Hostname:      "awiki.ai",
		PathPrefix:    []string{"user"},
		ProofDomain:   "awiki.ai",
		ANPServiceDID: "did:wba:awiki.ai:services:message:e1_local",
	})
	if err == nil {
		t.Fatal("GenerateIdentity() error = nil, want bare-domain DID validation error")
	}
}

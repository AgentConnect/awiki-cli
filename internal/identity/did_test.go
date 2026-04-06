package identity

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/pem"
	"testing"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	secp256k1ecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
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
	proof, ok := generated.DIDDocument["proof"].(map[string]any)
	if !ok {
		t.Fatalf("proof missing or invalid: %#v", generated.DIDDocument["proof"])
	}
	proofValue, ok := proof["proofValue"].(string)
	if !ok || proofValue == "" {
		t.Fatalf("proofValue missing: %#v", proof)
	}
	documentWithoutProof := cloneMap(generated.DIDDocument)
	delete(documentWithoutProof, "proof")
	proofWithoutValue := cloneMap(proof)
	delete(proofWithoutValue, "proofValue")
	documentCanonical, err := canonicalJSON(documentWithoutProof)
	if err != nil {
		t.Fatalf("canonicalJSON(document) error = %v", err)
	}
	proofCanonical, err := canonicalJSON(proofWithoutValue)
	if err != nil {
		t.Fatalf("canonicalJSON(proof) error = %v", err)
	}
	hasher := sha256.New()
	hasher.Write(documentCanonical)
	hasher.Write(proofCanonical)
	digest := hasher.Sum(nil)

	signatureBytes, err := base64.RawURLEncoding.DecodeString(proofValue)
	if err != nil {
		t.Fatalf("DecodeString(proofValue) error = %v", err)
	}
	signature, err := secp256k1ecdsa.ParseDERSignature(signatureBytes)
	if err != nil {
		t.Fatalf("ParseDERSignature() error = %v", err)
	}
	block, _ := pem.Decode([]byte(generated.Key1PublicPEM))
	if block == nil {
		t.Fatal("failed to decode key-1 public pem")
	}
	publicKey, err := secp256k1.ParsePubKey(block.Bytes)
	if err != nil {
		t.Fatalf("ParsePubKey() error = %v", err)
	}
	if !signature.Verify(digest, publicKey) {
		t.Fatal("generated proof signature does not verify")
	}
}

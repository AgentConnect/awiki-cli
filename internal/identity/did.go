package identity

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	secp256k1ecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

func GenerateIdentity(options GenerateOptions) (*GeneratedIdentity, error) {
	if strings.TrimSpace(options.Hostname) == "" {
		return nil, fmt.Errorf("%w: hostname is required", ErrInvalidInput)
	}
	if len(options.PathPrefix) == 0 {
		options.PathPrefix = []string{"user"}
	}
	if strings.TrimSpace(options.ProofDomain) == "" {
		options.ProofDomain = options.Hostname
	}

	key1Private, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		return nil, fmt.Errorf("generate secp256k1 key: %w", err)
	}
	key1Public := key1Private.PubKey()
	keyDigest := sha256.Sum256(key1Public.SerializeCompressed())
	kid := base64.RawURLEncoding.EncodeToString(keyDigest[:16])
	uniqueID := "k1_" + kid
	didParts := append([]string{"did", "wba", options.Hostname}, options.PathPrefix...)
	didParts = append(didParts, uniqueID)
	did := strings.Join(didParts, ":")
	verificationMethodID := did + "#key-1"

	didDocument := map[string]any{
		"@context": []string{"https://www.w3.org/ns/did/v1"},
		"id":       did,
		"verificationMethod": []any{
			map[string]any{
				"id":         verificationMethodID,
				"type":       "EcdsaSecp256k1VerificationKey2019",
				"controller": did,
				"publicKeyJwk": map[string]any{
					"kty": "EC",
					"crv": "secp256k1",
					"x":   encodeCoordinate(key1Public.X()),
					"y":   encodeCoordinate(key1Public.Y()),
					"kid": kid,
				},
			},
		},
		"authentication": []string{verificationMethodID},
	}

	proof := map[string]any{
		"type":               "EcdsaSecp256k1Signature2019",
		"created":            time.Now().UTC().Format(time.RFC3339),
		"verificationMethod": verificationMethodID,
		"proofPurpose":       "assertionMethod",
		"domain":             options.ProofDomain,
		"challenge":          randomHex(16),
	}
	signature, err := signProof(key1Private, didDocument, proof)
	if err != nil {
		return nil, err
	}
	proof["proofValue"] = base64.RawURLEncoding.EncodeToString(signature)
	didDocument["proof"] = proof

	p256Private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate e2ee signing key: %w", err)
	}
	key2Bytes, err := x509.MarshalECPrivateKey(p256Private)
	if err != nil {
		return nil, fmt.Errorf("marshal e2ee signing key: %w", err)
	}
	x25519Private, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate x25519 key: %w", err)
	}

	return &GeneratedIdentity{
		DID:                     did,
		UniqueID:                uniqueID,
		DIDDocument:             didDocument,
		Key1PrivatePEM:          encodePEM("SECP256K1 PRIVATE KEY", key1Private.Serialize()),
		Key1PublicPEM:           encodePEM("SECP256K1 PUBLIC KEY", key1Public.SerializeCompressed()),
		E2EESigningPrivatePEM:   encodePEM("EC PRIVATE KEY", key2Bytes),
		E2EEAgreementPrivatePEM: encodePEM("X25519 PRIVATE KEY", x25519Private.Bytes()),
	}, nil
}

func signProof(privateKey *secp256k1.PrivateKey, didDocument map[string]any, proof map[string]any) ([]byte, error) {
	documentWithoutProof := cloneMap(didDocument)
	delete(documentWithoutProof, "proof")
	proofWithoutValue := cloneMap(proof)
	delete(proofWithoutValue, "proofValue")
	documentCanonical, err := canonicalJSON(documentWithoutProof)
	if err != nil {
		return nil, fmt.Errorf("canonicalize did document: %w", err)
	}
	proofCanonical, err := canonicalJSON(proofWithoutValue)
	if err != nil {
		return nil, fmt.Errorf("canonicalize proof: %w", err)
	}
	hasher := sha256.New()
	hasher.Write(documentCanonical)
	hasher.Write(proofCanonical)
	digest := hasher.Sum(nil)
	signature := secp256k1ecdsa.Sign(privateKey, digest)
	return signature.Serialize(), nil
}

func canonicalJSON(value any) ([]byte, error) {
	buffer := bytes.NewBuffer(nil)
	if err := writeCanonicalJSON(buffer, value); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func writeCanonicalJSON(buffer *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case nil:
		buffer.WriteString("null")
	case string:
		raw, err := json.Marshal(typed)
		if err != nil {
			return err
		}
		buffer.Write(raw)
	case bool:
		if typed {
			buffer.WriteString("true")
		} else {
			buffer.WriteString("false")
		}
	case []string:
		buffer.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				buffer.WriteByte(',')
			}
			if err := writeCanonicalJSON(buffer, item); err != nil {
				return err
			}
		}
		buffer.WriteByte(']')
	case []any:
		buffer.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				buffer.WriteByte(',')
			}
			if err := writeCanonicalJSON(buffer, item); err != nil {
				return err
			}
		}
		buffer.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		buffer.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				buffer.WriteByte(',')
			}
			if err := writeCanonicalJSON(buffer, key); err != nil {
				return err
			}
			buffer.WriteByte(':')
			if err := writeCanonicalJSON(buffer, typed[key]); err != nil {
				return err
			}
		}
		buffer.WriteByte('}')
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return err
		}
		var normalized any
		if err := json.Unmarshal(raw, &normalized); err != nil {
			return err
		}
		return writeCanonicalJSON(buffer, normalized)
	}
	return nil
}

func encodeCoordinate(value *big.Int) string {
	if value == nil {
		return ""
	}
	bytes := value.Bytes()
	if len(bytes) < 32 {
		padding := make([]byte, 32-len(bytes))
		bytes = append(padding, bytes...)
	}
	return base64.RawURLEncoding.EncodeToString(bytes)
}

func encodePEM(blockType string, raw []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: raw}))
}

func randomHex(numBytes int) string {
	buffer := make([]byte, numBytes)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

func cloneMap(input map[string]any) map[string]any {
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

package identity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
)

func GenerateIdentity(options GenerateOptions) (*GeneratedIdentity, error) {
	hostname := strings.TrimSpace(options.Hostname)
	if hostname == "" {
		return nil, fmt.Errorf("%w: hostname is required", ErrInvalidInput)
	}
	pathSegments := clonePathPrefix(options.PathPrefix)
	if len(pathSegments) == 0 {
		pathSegments = []string{"user"}
	}
	proofDomain := strings.TrimSpace(options.ProofDomain)
	if proofDomain == "" {
		proofDomain = hostname
	}

	proofPurpose := strings.TrimSpace(options.ProofPurpose)
	if proofPurpose == "" {
		proofPurpose = "assertionMethod"
	}

	bundle, err := anpsdk.CreateDidWBADocumentWithKeyBinding(hostname, anpsdk.DidDocumentOptions{
		PathSegments: pathSegments,
		ProofPurpose: proofPurpose,
		Domain:       proofDomain,
		Challenge:    randomHex(16),
	})
	if err != nil {
		return nil, fmt.Errorf("generate did document: %w", err)
	}

	did := stringValue(bundle.DidDocument["id"], "")
	if did == "" {
		return nil, fmt.Errorf("generated did document is missing id")
	}
	key1, ok := bundle.Keys["key-1"]
	if !ok {
		return nil, fmt.Errorf("generated did document is missing key-1")
	}

	generated := &GeneratedIdentity{
		DID:            did,
		UniqueID:       didSuffix(did),
		DIDDocument:    bundle.DidDocument,
		Key1PrivatePEM: key1.PrivateKeyPEM,
		Key1PublicPEM:  key1.PublicKeyPEM,
	}
	if key2, ok := bundle.Keys["key-2"]; ok {
		generated.E2EESigningPrivatePEM = key2.PrivateKeyPEM
	}
	if key3, ok := bundle.Keys["key-3"]; ok {
		generated.E2EEAgreementPrivatePEM = key3.PrivateKeyPEM
	}
	return generated, nil
}

func didSuffix(did string) string {
	index := strings.LastIndex(did, ":")
	if index == -1 || index == len(did)-1 {
		return did
	}
	return did[index+1:]
}

func clonePathPrefix(pathPrefix []string) []string {
	cloned := make([]string, 0, len(pathPrefix))
	for _, segment := range pathPrefix {
		trimmed := strings.TrimSpace(segment)
		if trimmed != "" {
			cloned = append(cloned, trimmed)
		}
	}
	return cloned
}

func randomHex(numBytes int) string {
	buffer := make([]byte, numBytes)
	if _, err := rand.Read(buffer); err != nil {
		return ""
	}
	return hex.EncodeToString(buffer)
}

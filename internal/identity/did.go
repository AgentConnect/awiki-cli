package identity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
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
	serviceEndpoint := strings.TrimSpace(options.ANPServiceEndpoint)
	if serviceEndpoint == "" {
		serviceEndpoint = DefaultANPServiceEndpoint(hostname)
	}
	serviceDID := strings.TrimSpace(options.ANPServiceDID)
	if serviceDID == "" {
		serviceDID = DefaultANPServiceDID(hostname)
	}
	service, err := BuildAgentANPMessageService(serviceEndpoint, serviceDID)
	if err != nil {
		return nil, err
	}

	bundle, err := anpsdk.CreateDidWBADocument(hostname, anpsdk.DidDocumentOptions{
		PathSegments: pathSegments,
		Domain:       proofDomain,
		Challenge:    randomHex(16),
		Services:     []map[string]any{service},
		DidProfile:   anpsdk.DidProfileE1,
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

func IsK1DID(did string) bool {
	return strings.HasPrefix(didSuffix(strings.TrimSpace(did)), "k1_")
}

func IsE1DID(did string) bool {
	return strings.HasPrefix(didSuffix(strings.TrimSpace(did)), "e1_")
}

func parseDIDPath(did string) (string, []string, error) {
	trimmed := strings.TrimSpace(did)
	if !strings.HasPrefix(trimmed, "did:wba:") {
		return "", nil, fmt.Errorf("%w: invalid did %q", ErrInvalidInput, did)
	}
	parts := strings.Split(trimmed, ":")
	if len(parts) < 5 {
		return "", nil, fmt.Errorf("%w: invalid did %q", ErrInvalidInput, did)
	}
	domain, err := url.PathUnescape(parts[2])
	if err != nil {
		return "", nil, fmt.Errorf("%w: invalid did domain %q", ErrInvalidInput, parts[2])
	}
	pathSegments := append([]string(nil), parts[3:len(parts)-1]...)
	if len(pathSegments) == 0 {
		return "", nil, fmt.Errorf("%w: missing did path segments", ErrInvalidInput)
	}
	return domain, pathSegments, nil
}

func HandlePathPrefixFromDID(did string) (string, []string, error) {
	domain, pathSegments, err := parseDIDPath(did)
	if err != nil {
		return "", nil, err
	}
	if len(pathSegments) == 0 || strings.EqualFold(pathSegments[0], "user") {
		return "", nil, fmt.Errorf("%w: current did is not a handle did", ErrInvalidInput)
	}
	return domain, pathSegments, nil
}

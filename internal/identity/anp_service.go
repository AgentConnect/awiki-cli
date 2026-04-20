package identity

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
)

const defaultANPServicePath = "/anp-im/rpc"

var (
	agentMessageServiceProfiles = []string{
		"anp.core.binding.v1",
		"anp.direct.base.v1",
		"anp.group.base.v1",
		"anp.attachment.v1",
	}
	agentMessageServiceSecurityProfiles = []string{"transport-protected"}
)

func DefaultANPServiceEndpoint(hostname string) string {
	return "https://" + strings.TrimSpace(hostname) + defaultANPServicePath
}

func DefaultANPServiceDID(hostname string) string {
	return "did:wba:" + strings.TrimSpace(hostname)
}

func ValidateANPServiceEndpoint(serviceEndpoint string) error {
	trimmedEndpoint := strings.TrimSpace(serviceEndpoint)
	if trimmedEndpoint == "" {
		return fmt.Errorf("%w: anp_service_endpoint is required", ErrInvalidInput)
	}
	parsed, err := url.Parse(trimmedEndpoint)
	if err != nil {
		return fmt.Errorf("%w: anp_service_endpoint is invalid: %v", ErrInvalidInput, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: anp_service_endpoint must use http or https", ErrInvalidInput)
	}
	if parsed.Hostname() == "" {
		return fmt.Errorf("%w: anp_service_endpoint must include a hostname", ErrInvalidInput)
	}
	hostname := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if hostname == "localhost" {
		return fmt.Errorf("%w: anp_service_endpoint must not use localhost", ErrInvalidInput)
	}
	if ip := net.ParseIP(hostname); ip != nil && ip.IsLoopback() {
		return fmt.Errorf("%w: anp_service_endpoint must not use a loopback address", ErrInvalidInput)
	}
	return nil
}

func ValidateANPServiceDID(serviceDID string) error {
	trimmedDID := strings.TrimSpace(serviceDID)
	if trimmedDID == "" {
		return fmt.Errorf("%w: anp_service_did is required", ErrInvalidInput)
	}
	if !strings.HasPrefix(trimmedDID, "did:wba:") {
		return fmt.Errorf("%w: anp_service_did must use did:wba", ErrInvalidInput)
	}
	if strings.Contains(trimmedDID, "#") {
		return fmt.Errorf("%w: anp_service_did must not include a fragment", ErrInvalidInput)
	}
	remainder := strings.TrimPrefix(trimmedDID, "did:wba:")
	if remainder == "" {
		return fmt.Errorf("%w: anp_service_did must include a domain", ErrInvalidInput)
	}
	if strings.Contains(remainder, ":") || strings.Contains(remainder, "/") || strings.Contains(remainder, "?") {
		return fmt.Errorf("%w: anp_service_did must be a bare-domain did:wba DID", ErrInvalidInput)
	}
	return nil
}

func BuildAgentANPMessageService(serviceEndpoint string, serviceDID string) (map[string]any, error) {
	if err := ValidateANPServiceEndpoint(serviceEndpoint); err != nil {
		return nil, err
	}
	if err := ValidateANPServiceDID(serviceDID); err != nil {
		return nil, err
	}
	return anpsdk.BuildANPMessageService(
		"#message",
		strings.TrimSpace(serviceEndpoint),
		anpsdk.AnpMessageServiceOptions{
			ServiceDID:       strings.TrimSpace(serviceDID),
			Profiles:         append([]string(nil), agentMessageServiceProfiles...),
			SecurityProfiles: append([]string(nil), agentMessageServiceSecurityProfiles...),
		},
	), nil
}

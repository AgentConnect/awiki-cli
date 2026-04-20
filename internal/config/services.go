package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

const (
	ServiceSeverityError = "error"
	ServiceSeverityWarn  = "warn"
)

type ServicesConfig struct {
	ServiceBaseURL     string `json:"service_base_url"`
	DIDDomain          string `json:"did_domain"`
	ANPServiceEndpoint string `json:"anp_service_endpoint"`
	ANPServiceDID      string `json:"anp_service_did"`
}

type ServiceDiagnostic struct {
	Field    string `json:"field"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`
}

func NormalizeDomain(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimRight(trimmed, ".")
	if trimmed == "" {
		return "", fmt.Errorf("domain is required")
	}
	lowered := strings.ToLower(trimmed)
	if strings.Contains(lowered, "://") || strings.ContainsAny(lowered, "/\\?#@") || strings.Contains(lowered, ":") || strings.ContainsFunc(lowered, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }) {
		return "", fmt.Errorf("domain must be a plain host name, not a URL or host:port")
	}
	return lowered, nil
}

func DefaultServicesForDomain(domain string) (ServicesConfig, error) {
	normalizedDomain, err := NormalizeDomain(domain)
	if err != nil {
		return ServicesConfig{}, err
	}
	return ServicesConfig{
		ServiceBaseURL:     "https://" + normalizedDomain,
		DIDDomain:          normalizedDomain,
		ANPServiceEndpoint: "https://" + normalizedDomain + defaultANPPath,
		ANPServiceDID:      "did:wba:" + normalizedDomain,
	}, nil
}

func ValidateServices(services ServicesConfig) []ServiceDiagnostic {
	var diagnostics []ServiceDiagnostic
	plannedDomain, domainErr := NormalizeDomain(services.DIDDomain)
	if domainErr != nil {
		diagnostics = append(diagnostics, ServiceDiagnostic{
			Field:    "services.did_domain",
			Severity: ServiceSeverityError,
			Message:  domainErr.Error(),
			Hint:     "Use a bare DNS host such as a.example.com.",
		})
	}
	if err := validateHTTPURL(services.ServiceBaseURL, "service_base_url"); err != nil {
		diagnostics = append(diagnostics, ServiceDiagnostic{
			Field:    "services.service_base_url",
			Severity: ServiceSeverityError,
			Message:  err.Error(),
			Hint:     "Use an http or https URL for the API gateway.",
		})
	}
	endpointURL, endpointErr := parseHTTPURL(services.ANPServiceEndpoint, "anp_service_endpoint")
	if endpointErr != nil {
		diagnostics = append(diagnostics, ServiceDiagnostic{
			Field:    "services.anp_service_endpoint",
			Severity: ServiceSeverityError,
			Message:  endpointErr.Error(),
			Hint:     "Use a public http or https URL such as https://a.example.com/anp-im/rpc.",
		})
	} else {
		hostname := strings.ToLower(strings.TrimSpace(endpointURL.Hostname()))
		if hostname == "localhost" || isLoopbackHost(hostname) {
			diagnostics = append(diagnostics, ServiceDiagnostic{
				Field:    "services.anp_service_endpoint",
				Severity: ServiceSeverityError,
				Message:  "anp_service_endpoint must not use localhost or a loopback address",
				Hint:     "Publish a reachable hosted-domain endpoint in DID documents.",
			})
		}
		if domainErr == nil && hostname != "" && hostname != plannedDomain {
			diagnostics = append(diagnostics, ServiceDiagnostic{
				Field:    "services.anp_service_endpoint",
				Severity: ServiceSeverityWarn,
				Message:  "anp_service_endpoint host differs from services.did_domain",
				Hint:     "This is allowed for advanced gateway deployments; verify the public endpoint is intentional.",
			})
		}
	}
	serviceDIDDomain, serviceDIDErr := bareWbaServiceDIDDomain(services.ANPServiceDID)
	if serviceDIDErr != nil {
		diagnostics = append(diagnostics, ServiceDiagnostic{
			Field:    "services.anp_service_did",
			Severity: ServiceSeverityError,
			Message:  serviceDIDErr.Error(),
			Hint:     "Use a bare-domain DID such as did:wba:a.example.com.",
		})
	} else if domainErr == nil && serviceDIDDomain != plannedDomain {
		diagnostics = append(diagnostics, ServiceDiagnostic{
			Field:    "services.anp_service_did",
			Severity: ServiceSeverityWarn,
			Message:  "anp_service_did differs from did:wba:<did_domain>",
			Hint:     "This is allowed only for advanced deployments; verify the service DID owns the advertised domain.",
		})
	}
	return diagnostics
}

func validateHTTPURL(raw string, fieldName string) error {
	_, err := parseHTTPURL(raw, fieldName)
	return err
}

func parseHTTPURL(raw string, fieldName string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("%s is required", fieldName)
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("%s is invalid: %w", fieldName, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%s must use http or https", fieldName)
	}
	if parsed.Hostname() == "" {
		return nil, fmt.Errorf("%s must include a hostname", fieldName)
	}
	return parsed, nil
}

func bareWbaServiceDIDDomain(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("anp_service_did is required")
	}
	if !strings.HasPrefix(trimmed, "did:wba:") {
		return "", fmt.Errorf("anp_service_did must use did:wba")
	}
	if strings.Contains(trimmed, "#") {
		return "", fmt.Errorf("anp_service_did must not include a fragment")
	}
	remainder := strings.TrimPrefix(trimmed, "did:wba:")
	if remainder == "" {
		return "", fmt.Errorf("anp_service_did must include a domain")
	}
	if strings.ContainsAny(remainder, ":/?") {
		return "", fmt.Errorf("anp_service_did must be a bare-domain did:wba DID")
	}
	return NormalizeDomain(remainder)
}

func isLoopbackHost(hostname string) bool {
	ip := net.ParseIP(hostname)
	return ip != nil && ip.IsLoopback()
}

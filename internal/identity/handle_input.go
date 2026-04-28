package identity

import (
	"fmt"
	"strings"
)

type NormalizedHandle struct {
	LocalPart       string
	FullHandle      string
	EffectiveDomain string
	ExplicitDomain  bool
}

// NormalizeHandleInput normalizes bare/full/wba handle input into a canonical
// full handle. DID inputs are rejected because callers should route those
// through explicit DID-aware command flags instead.
func NormalizeHandleInput(raw string, didDomain string) (*NormalizedHandle, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: handle is required", ErrInvalidInput)
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "did:") {
		return nil, fmt.Errorf("%w: did values are not supported in handle input %q", ErrInvalidInput, raw)
	}

	handle := strings.TrimPrefix(lower, "wba://")
	if handle == "" {
		return nil, fmt.Errorf("%w: handle is required", ErrInvalidInput)
	}
	if dot := strings.Index(handle, "."); dot >= 0 {
		localPart := strings.TrimSpace(handle[:dot])
		domain := normalizeHandleDomain(handle[dot+1:])
		if localPart == "" || domain == "" {
			return nil, fmt.Errorf("%w: invalid handle %q", ErrInvalidInput, raw)
		}
		return &NormalizedHandle{
			LocalPart:       localPart,
			FullHandle:      localPart + "." + domain,
			EffectiveDomain: domain,
			ExplicitDomain:  true,
		}, nil
	}

	domain := normalizeHandleDomain(didDomain)
	if domain == "" {
		return nil, fmt.Errorf("%w: did_domain is required to complete bare handle %q", ErrInvalidInput, raw)
	}
	return &NormalizedHandle{
		LocalPart:       handle,
		FullHandle:      handle + "." + domain,
		EffectiveDomain: domain,
		ExplicitDomain:  false,
	}, nil
}

// CompleteBareHandle expands bare and wba://bare handles to a canonical full
// handle. Explicit full handles and DID values pass through with their original
// trimmed spelling.
func CompleteBareHandle(target string, didDomain string) string {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "did:") {
		return trimmed
	}
	normalized, err := NormalizeHandleInput(trimmed, didDomain)
	if err != nil {
		return trimmed
	}
	if normalized.ExplicitDomain {
		return trimmed
	}
	return normalized.FullHandle
}

func normalizeHandleDomain(domain string) string {
	return strings.TrimSuffix(strings.TrimSpace(strings.ToLower(domain)), ".")
}

func storedHandleFields(handle string, fullHandle string, did string) (string, string) {
	localPart := strings.TrimSpace(strings.ToLower(handle))
	if strings.HasPrefix(localPart, "wba://") {
		localPart = strings.TrimPrefix(localPart, "wba://")
	}
	if dot := strings.Index(localPart, "."); dot >= 0 {
		localPart = localPart[:dot]
	}

	if normalizedFull, ok := normalizeStoredFullHandle(fullHandle, did); ok {
		if localPart == "" {
			localPart = normalizedFull.LocalPart
		}
		return localPart, normalizedFull.FullHandle
	}
	if localPart == "" {
		return "", ""
	}
	return localPart, deriveFullHandleFromDID(localPart, did)
}

func deriveFullHandleFromDID(handle string, did string) string {
	localPart := strings.TrimSpace(strings.ToLower(handle))
	if localPart == "" {
		return ""
	}
	domain, _, err := HandlePathPrefixFromDID(did)
	if err != nil {
		return ""
	}
	domain = normalizeHandleDomain(domain)
	if domain == "" {
		return ""
	}
	return localPart + "." + domain
}

func normalizeStoredFullHandle(fullHandle string, did string) (*NormalizedHandle, bool) {
	trimmed := strings.TrimSpace(fullHandle)
	if trimmed == "" {
		return nil, false
	}
	if normalized, err := NormalizeHandleInput(trimmed, ""); err == nil {
		return normalized, true
	}
	domain, _, err := HandlePathPrefixFromDID(did)
	if err != nil {
		return nil, false
	}
	normalized, err := NormalizeHandleInput(trimmed, domain)
	if err != nil {
		return nil, false
	}
	return normalized, true
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

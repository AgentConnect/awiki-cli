package testenv

import (
	"strings"
)

const defaultDomain = "awiki.test"

// Domain returns the domain used by domain-sensitive tests.
func Domain() string {
	return defaultDomain
}

func BaseURL() string {
	return "https://" + Domain()
}

func Subdomain(prefix string) string {
	return strings.TrimSuffix(prefix, ".") + "." + Domain()
}

func SubdomainURL(prefix string) string {
	return "https://" + Subdomain(prefix)
}

func ServiceDID() string {
	return "did:wba:" + Domain()
}

func DID(path ...string) string {
	parts := append([]string{"did:wba:" + Domain()}, path...)
	return strings.Join(parts, ":")
}

func FullHandle(handle string) string {
	return handle + "." + Domain()
}

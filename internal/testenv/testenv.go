package testenv

import (
	"os"
	"path/filepath"
	"strings"
)

const defaultDomain = "awiki.test"

// Domain returns the domain used by domain-sensitive tests.
func Domain() string {
	return value("AWIKI_CLI_TEST_DOMAIN", defaultDomain)
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

func value(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return strings.TrimSuffix(value, "/")
	}
	env := loadDotEnv()
	if value := strings.TrimSpace(env[key]); value != "" {
		return strings.TrimSuffix(value, "/")
	}
	if key == "AWIKI_CLI_TEST_DOMAIN" {
		if value := strings.TrimSpace(os.Getenv("AWIKI_LOCAL_DOMAIN")); value != "" {
			return strings.TrimSuffix(value, "/")
		}
		if value := strings.TrimSpace(env["AWIKI_LOCAL_DOMAIN"]); value != "" {
			return strings.TrimSuffix(value, "/")
		}
	}
	return fallback
}

func loadDotEnv() map[string]string {
	root, ok := repoRoot()
	if !ok {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		return nil
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		key, value, _ := strings.Cut(line, "=")
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return values
}

func repoRoot() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if raw, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil &&
			strings.Contains(string(raw), "module github.com/agentconnect/awiki-cli") {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

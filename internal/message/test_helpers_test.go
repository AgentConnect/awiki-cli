package message

import (
	"strings"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
)

func mustMapValue(t *testing.T, value any, label string) map[string]any {
	t.Helper()

	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want map[string]any", label, value)
	}
	return result
}

func warningContains(warnings []string, want string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, want) {
			return true
		}
	}
	return false
}

func loadTestIdentityRecord(
	t *testing.T,
	resolved *appconfig.Resolved,
	identityName string,
) (*identity.Manager, *identity.StoredIdentity) {
	t.Helper()

	manager := identity.NewManager(resolved.Paths)
	createTestIdentity(t, manager, identity.SaveInput{
		IdentityName: identityName,
		UserID:       "user-" + identityName,
		DisplayName:  identityName,
		Handle:       identityName,
	})
	record, err := manager.Load(identityName)
	if err != nil {
		t.Fatalf("manager.Load(%q) error = %v", identityName, err)
	}
	return manager, record
}

func newHTTPTransportForTest(
	t *testing.T,
	baseURL string,
) (*HTTPTransport, *appconfig.Resolved, *identity.StoredIdentity) {
	t.Helper()

	resolved := testResolvedConfig(t)
	resolved.ServiceBaseURL = baseURL
	manager, record := loadTestIdentityRecord(t, resolved, "alice")
	auth, err := newAuthContext(record, manager)
	if err != nil {
		t.Fatalf("newAuthContext() error = %v", err)
	}
	return NewHTTPTransport(resolved, auth, nil), resolved, record
}

func newMessageServiceForTest(
	t *testing.T,
	baseURL string,
) (*Service, *appconfig.Resolved, *identity.StoredIdentity) {
	t.Helper()

	resolved := testResolvedConfig(t)
	resolved.ServiceBaseURL = baseURL
	resolved.ActiveIdentity = "alice"
	_, record := loadTestIdentityRecord(t, resolved, "alice")
	service, err := NewService(resolved)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service, resolved, record
}

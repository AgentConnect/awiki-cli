package message

import (
	"errors"
	"path/filepath"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
)

func TestRequireActiveIdentityRejectsLocalOnlyIdentity(t *testing.T) {
	t.Parallel()

	resolved := testResolvedConfig(t)
	manager := identity.NewManager(resolved.Paths)
	createTestIdentity(t, manager, identity.SaveInput{
		IdentityName: "default",
		DisplayName:  "Alice",
	})

	service, err := NewService(resolved)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.requireActiveIdentity("")
	if !errors.Is(err, identity.ErrUserRegistrationRequired) {
		t.Fatalf("requireActiveIdentity() error = %v, want %v", err, identity.ErrUserRegistrationRequired)
	}
}

func TestRequireActiveIdentityAcceptsRegisteredUser(t *testing.T) {
	t.Parallel()

	resolved := testResolvedConfig(t)
	manager := identity.NewManager(resolved.Paths)
	createTestIdentity(t, manager, identity.SaveInput{
		IdentityName: "alice",
		UserID:       "user-123",
		DisplayName:  "Alice",
		Handle:       "alice",
	})
	resolved.ActiveIdentity = "alice"

	service, err := NewService(resolved)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	record, err := service.requireActiveIdentity("")
	if err != nil {
		t.Fatalf("requireActiveIdentity() error = %v", err)
	}
	if record.Handle != "alice" || record.UserID != "user-123" {
		t.Fatalf("unexpected record = %#v", record)
	}
}

func testResolvedConfig(t *testing.T) *appconfig.Resolved {
	t.Helper()
	root := t.TempDir()
	return &appconfig.Resolved{
		Paths: appconfig.Paths{
			IdentityDir:          filepath.Join(root, "identities"),
			LegacyCredentialsDir: filepath.Join(root, "legacy"),
			DataDir:              filepath.Join(root, "data"),
			StateDir:             filepath.Join(root, "state"),
			DatabaseFile:         filepath.Join(root, "data", "awiki-cli.db"),
		},
		ServiceBaseURL: "https://awiki.test",
		DIDDomain:      "awiki.ai",
	}
}

func createTestIdentity(t *testing.T, manager *identity.Manager, input identity.SaveInput) {
	t.Helper()
	generated, err := identity.GenerateIdentity(identity.GenerateOptions{
		Hostname:    "awiki.ai",
		PathPrefix:  []string{"user"},
		ProofDomain: "awiki.ai",
	})
	if err != nil {
		t.Fatalf("GenerateIdentity() error = %v", err)
	}
	input.DID = generated.DID
	input.UniqueID = generated.UniqueID
	input.DIDDocument = generated.DIDDocument
	input.Key1PrivatePEM = generated.Key1PrivatePEM
	input.Key1PublicPEM = generated.Key1PublicPEM
	input.E2EESigningPrivatePEM = generated.E2EESigningPrivatePEM
	input.E2EEAgreementPrivatePEM = generated.E2EEAgreementPrivatePEM
	if _, err := manager.Save(input); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

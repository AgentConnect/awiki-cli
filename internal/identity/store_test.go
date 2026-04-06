package identity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func TestManagerSaveLoadAndCurrent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manager := NewManager(appconfig.Paths{
		IdentityDir:          filepath.Join(root, "identities"),
		LegacyCredentialsDir: filepath.Join(root, "legacy"),
	})

	generated, err := GenerateIdentity(GenerateOptions{
		Hostname:    "awiki.ai",
		PathPrefix:  []string{"user"},
		ProofDomain: "awiki.ai",
	})
	if err != nil {
		t.Fatalf("GenerateIdentity() error = %v", err)
	}
	record, err := manager.save(SaveInput{
		IdentityName:            "default",
		DID:                     generated.DID,
		UniqueID:                generated.UniqueID,
		DisplayName:             "Alice",
		DIDDocument:             generated.DIDDocument,
		Key1PrivatePEM:          generated.Key1PrivatePEM,
		Key1PublicPEM:           generated.Key1PublicPEM,
		E2EESigningPrivatePEM:   generated.E2EESigningPrivatePEM,
		E2EEAgreementPrivatePEM: generated.E2EEAgreementPrivatePEM,
	})
	if err != nil {
		t.Fatalf("save() error = %v", err)
	}
	if record.IdentityName != "default" {
		t.Fatalf("unexpected identity name: %+v", record)
	}

	loaded, err := manager.Load("default")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.DID != generated.DID {
		t.Fatalf("loaded DID mismatch: got=%s want=%s", loaded.DID, generated.DID)
	}

	listed, err := manager.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 1 || listed[0].IdentityName != "default" {
		t.Fatalf("unexpected list result: %#v", listed)
	}

	current, err := manager.Current()
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if current.IdentityName != "default" || !current.IsDefault {
		t.Fatalf("unexpected current identity: %#v", current)
	}
}

func TestImportLegacyFlatIdentity(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	legacyRoot := filepath.Join(root, "legacy")
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	manager := NewManager(appconfig.Paths{
		IdentityDir:          filepath.Join(root, "identities"),
		LegacyCredentialsDir: legacyRoot,
	})
	generated, err := GenerateIdentity(GenerateOptions{
		Hostname:    "awiki.ai",
		PathPrefix:  []string{"user"},
		ProofDomain: "awiki.ai",
	})
	if err != nil {
		t.Fatalf("GenerateIdentity() error = %v", err)
	}
	legacyPayload := map[string]any{
		"did":                        generated.DID,
		"unique_id":                  generated.UniqueID,
		"name":                       "Legacy Alice",
		"handle":                     "legacy-alice",
		"jwt_token":                  "legacy-token",
		"private_key_pem":            generated.Key1PrivatePEM,
		"public_key_pem":             generated.Key1PublicPEM,
		"e2ee_signing_private_pem":   generated.E2EESigningPrivatePEM,
		"e2ee_agreement_private_pem": generated.E2EEAgreementPrivatePEM,
		"did_document":               generated.DIDDocument,
	}
	raw, err := json.MarshalIndent(legacyPayload, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "default.json"), raw, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	scan, err := manager.ScanLegacy()
	if err != nil {
		t.Fatalf("ScanLegacy() error = %v", err)
	}
	if !scan.HasLegacy || len(scan.LegacyCredentials) != 1 {
		t.Fatalf("unexpected legacy scan: %#v", scan)
	}

	imported, err := manager.ImportLegacy("default")
	if err != nil {
		t.Fatalf("ImportLegacy() error = %v", err)
	}
	if len(imported.Imported) != 1 {
		t.Fatalf("unexpected import result: %#v", imported)
	}
	current, err := manager.Current()
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if current.IdentityName != "default" || current.DisplayName != "Legacy Alice" {
		t.Fatalf("unexpected current identity after import: %#v", current)
	}
}

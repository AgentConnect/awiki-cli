package identity

import (
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
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
	if current.UserState.RegistrationState != "local_identity" || current.UserState.ReadyForMessaging {
		t.Fatalf("unexpected user state for local identity: %#v", current.UserState)
	}
}

func TestManagerLoadMigratesLegacyANPPrivateKeysToPKCS8(t *testing.T) {
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
	if _, err := manager.save(SaveInput{
		IdentityName:            "default",
		DID:                     generated.DID,
		UniqueID:                generated.UniqueID,
		DisplayName:             "Alice",
		DIDDocument:             generated.DIDDocument,
		Key1PrivatePEM:          legacyANPPrivatePEM(t, generated.Key1PrivatePEM, anpEd25519PrivateKeyLabel),
		Key1PublicPEM:           generated.Key1PublicPEM,
		E2EESigningPrivatePEM:   legacyANPPrivatePEM(t, generated.E2EESigningPrivatePEM, anpSecp256r1PrivateKeyLabel),
		E2EEAgreementPrivatePEM: legacyANPPrivatePEM(t, generated.E2EEAgreementPrivatePEM, anpX25519PrivateKeyLabel),
	}); err != nil {
		t.Fatalf("save() error = %v", err)
	}

	loaded, err := manager.Load("default")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for name, value := range map[string]string{
		"key-1":          loaded.Key1PrivatePEM,
		"e2ee signing":   loaded.E2EESigningPrivatePEM,
		"e2ee agreement": loaded.E2EEAgreementPrivatePEM,
	} {
		if strings.Contains(value, "BEGIN ANP ") {
			t.Fatalf("%s private key still uses legacy ANP PEM label", name)
		}
		if !strings.HasPrefix(value, "-----BEGIN PRIVATE KEY-----") {
			t.Fatalf("%s private key = %q, want standard PKCS#8 PEM", name, value[:32])
		}
		if _, err := anpsdk.PrivateKeyFromPEM(value); err != nil {
			t.Fatalf("PrivateKeyFromPEM(%s) error = %v", name, err)
		}
	}

	paths := manager.BuildPaths(generated.UniqueID)
	for _, path := range []string{
		paths.Key1PrivatePath,
		paths.E2EESigningPrivatePath,
		paths.E2EEAgreementPrivatePath,
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		if strings.Contains(string(raw), "BEGIN ANP ") {
			t.Fatalf("%s still uses legacy ANP PEM label", path)
		}
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

func TestManagerSummaryShowsRegisteredUserState(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manager := NewManager(appconfig.Paths{
		IdentityDir:          filepath.Join(root, "identities"),
		LegacyCredentialsDir: filepath.Join(root, "legacy"),
	})

	generated, err := GenerateIdentity(GenerateOptions{
		Hostname:    "awiki.ai",
		PathPrefix:  []string{"alice"},
		ProofDomain: "awiki.ai",
	})
	if err != nil {
		t.Fatalf("GenerateIdentity() error = %v", err)
	}

	if _, err := manager.save(SaveInput{
		IdentityName:            "alice",
		DID:                     generated.DID,
		UniqueID:                generated.UniqueID,
		UserID:                  "user-123",
		DisplayName:             "Alice",
		Handle:                  "alice",
		DIDDocument:             generated.DIDDocument,
		Key1PrivatePEM:          generated.Key1PrivatePEM,
		Key1PublicPEM:           generated.Key1PublicPEM,
		E2EESigningPrivatePEM:   generated.E2EESigningPrivatePEM,
		E2EEAgreementPrivatePEM: generated.E2EEAgreementPrivatePEM,
	}); err != nil {
		t.Fatalf("save() error = %v", err)
	}

	current, err := manager.Current()
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if current.UserState.RegistrationState != "registered_user" || !current.UserState.ReadyForMessaging {
		t.Fatalf("unexpected registered user state: %#v", current.UserState)
	}
}

func TestManagerLoadBackfillsFullHandleFromDID(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manager := NewManager(appconfig.Paths{
		IdentityDir:          filepath.Join(root, "identities"),
		LegacyCredentialsDir: filepath.Join(root, "legacy"),
	})

	generated, err := GenerateIdentity(GenerateOptions{
		Hostname:    "tenant.example",
		PathPrefix:  []string{"alice"},
		ProofDomain: "tenant.example",
	})
	if err != nil {
		t.Fatalf("GenerateIdentity() error = %v", err)
	}
	record, err := manager.save(SaveInput{
		IdentityName: "alice",
		DID:          generated.DID,
		UniqueID:     generated.UniqueID,
		Handle:       "alice",
	})
	if err != nil {
		t.Fatalf("save() error = %v", err)
	}

	paths := manager.BuildPaths(record.DirName)
	payload, err := readJSONMap(paths.IdentityPath)
	if err != nil {
		t.Fatalf("readJSONMap(identity) error = %v", err)
	}
	delete(payload, "full_handle")
	if err := writeSecureJSON(paths.IdentityPath, payload); err != nil {
		t.Fatalf("writeSecureJSON(identity) error = %v", err)
	}
	index, err := manager.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() error = %v", err)
	}
	entry := index.Credentials["alice"]
	entry.FullHandle = ""
	index.Credentials["alice"] = entry
	if err := manager.SaveIndex(index); err != nil {
		t.Fatalf("SaveIndex() error = %v", err)
	}

	loaded, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.FullHandle != "alice.tenant.example" {
		t.Fatalf("loaded.FullHandle = %q, want alice.tenant.example", loaded.FullHandle)
	}

	payload, err = readJSONMap(paths.IdentityPath)
	if err != nil {
		t.Fatalf("readJSONMap(identity after backfill) error = %v", err)
	}
	if got, _ := payload["full_handle"].(string); got != "alice.tenant.example" {
		t.Fatalf("identity payload full_handle = %q, want alice.tenant.example", got)
	}
	index, err = manager.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex(after backfill) error = %v", err)
	}
	if got := index.Credentials["alice"].FullHandle; got != "alice.tenant.example" {
		t.Fatalf("index full_handle = %q, want alice.tenant.example", got)
	}
}

func TestManagerLoadDoesNotBackfillFullHandleForNonHandleDID(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manager := NewManager(appconfig.Paths{
		IdentityDir:          filepath.Join(root, "identities"),
		LegacyCredentialsDir: filepath.Join(root, "legacy"),
	})

	generated, err := GenerateIdentity(GenerateOptions{
		Hostname:    "tenant.example",
		PathPrefix:  []string{"user"},
		ProofDomain: "tenant.example",
	})
	if err != nil {
		t.Fatalf("GenerateIdentity() error = %v", err)
	}
	record, err := manager.save(SaveInput{
		IdentityName: "alice",
		DID:          generated.DID,
		UniqueID:     generated.UniqueID,
		Handle:       "alice",
	})
	if err != nil {
		t.Fatalf("save() error = %v", err)
	}

	paths := manager.BuildPaths(record.DirName)
	payload, err := readJSONMap(paths.IdentityPath)
	if err != nil {
		t.Fatalf("readJSONMap(identity) error = %v", err)
	}
	delete(payload, "full_handle")
	if err := writeSecureJSON(paths.IdentityPath, payload); err != nil {
		t.Fatalf("writeSecureJSON(identity) error = %v", err)
	}
	index, err := manager.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex() error = %v", err)
	}
	entry := index.Credentials["alice"]
	entry.FullHandle = ""
	index.Credentials["alice"] = entry
	if err := manager.SaveIndex(index); err != nil {
		t.Fatalf("SaveIndex() error = %v", err)
	}

	loaded, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.FullHandle != "" {
		t.Fatalf("loaded.FullHandle = %q, want empty for non-handle did", loaded.FullHandle)
	}
}

func legacyANPPrivatePEM(t *testing.T, standardPEM string, label string) string {
	t.Helper()
	privateKey, err := anpsdk.PrivateKeyFromPEM(standardPEM)
	if err != nil {
		t.Fatalf("PrivateKeyFromPEM() fixture error = %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: label, Bytes: privateKey.Bytes}))
}

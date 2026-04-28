package identity

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func legacyTestManager(root string) *Manager {
	return NewManager(appconfig.Paths{
		IdentityDir:          filepath.Join(root, "identities"),
		LegacyCredentialsDir: filepath.Join(root, "legacy"),
	})
}

func writeLegacyFlatCredential(t *testing.T, legacyRoot string, credentialName string, handle string) *GeneratedIdentity {
	t.Helper()
	generated, err := GenerateIdentity(GenerateOptions{
		Hostname:           "awiki.test",
		PathPrefix:         []string{handle},
		ProofDomain:        "awiki.test",
		ANPServiceEndpoint: "https://awiki.test/anp-im/rpc",
		ANPServiceDID:      "did:wba:awiki.test",
	})
	if err != nil {
		t.Fatalf("GenerateIdentity() error = %v", err)
	}
	payload := map[string]any{
		"did":                        generated.DID,
		"unique_id":                  generated.UniqueID,
		"name":                       handle,
		"handle":                     handle,
		"jwt_token":                  "legacy-token-" + credentialName,
		"private_key_pem":            generated.Key1PrivatePEM,
		"public_key_pem":             generated.Key1PublicPEM,
		"e2ee_signing_private_pem":   generated.E2EESigningPrivatePEM,
		"e2ee_agreement_private_pem": generated.E2EEAgreementPrivatePEM,
		"did_document":               generated.DIDDocument,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, credentialName+".json"), raw, 0o600); err != nil {
		t.Fatalf("os.WriteFile(%s) error = %v", credentialName, err)
	}
	return generated
}

func writeLegacyIndexedCredential(t *testing.T, legacyRoot string, credentialName string, dirName string, handle string) IndexEntry {
	t.Helper()
	generated, err := GenerateIdentity(GenerateOptions{
		Hostname:           "awiki.test",
		PathPrefix:         []string{handle},
		ProofDomain:        "awiki.test",
		ANPServiceEndpoint: "https://awiki.test/anp-im/rpc",
		ANPServiceDID:      "did:wba:awiki.test",
	})
	if err != nil {
		t.Fatalf("GenerateIdentity() error = %v", err)
	}
	identityDir := filepath.Join(legacyRoot, dirName)
	if err := os.MkdirAll(identityDir, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	identityPaths := Paths{
		IdentityDir:              identityDir,
		IdentityPath:             filepath.Join(identityDir, IdentityFileName),
		AuthPath:                 filepath.Join(identityDir, AuthFileName),
		DIDDocumentPath:          filepath.Join(identityDir, DIDDocumentFileName),
		Key1PrivatePath:          filepath.Join(identityDir, Key1PrivateFileName),
		Key1PublicPath:           filepath.Join(identityDir, Key1PublicFileName),
		E2EESigningPrivatePath:   filepath.Join(identityDir, E2EESigningPrivateFileName),
		E2EEAgreementPrivatePath: filepath.Join(identityDir, E2EEAgreementPrivateFileName),
		E2EEStatePath:            filepath.Join(identityDir, E2EEStateFileName),
	}
	if err := writeSecureJSON(identityPaths.IdentityPath, map[string]any{
		"did":        generated.DID,
		"unique_id":  generated.UniqueID,
		"created_at": "2026-01-01T00:00:00Z",
		"name":       "Indexed " + handle,
		"handle":     handle,
	}); err != nil {
		t.Fatalf("writeSecureJSON(identity) error = %v", err)
	}
	if err := writeSecureJSON(identityPaths.AuthPath, map[string]any{"jwt_token": "jwt-" + credentialName}); err != nil {
		t.Fatalf("writeSecureJSON(auth) error = %v", err)
	}
	if err := writeSecureJSON(identityPaths.DIDDocumentPath, generated.DIDDocument); err != nil {
		t.Fatalf("writeSecureJSON(did_document) error = %v", err)
	}
	if err := writeSecureText(identityPaths.Key1PrivatePath, generated.Key1PrivatePEM); err != nil {
		t.Fatalf("writeSecureText(key1_private) error = %v", err)
	}
	if err := writeSecureText(identityPaths.Key1PublicPath, generated.Key1PublicPEM); err != nil {
		t.Fatalf("writeSecureText(key1_public) error = %v", err)
	}
	if err := writeSecureText(identityPaths.E2EESigningPrivatePath, generated.E2EESigningPrivatePEM); err != nil {
		t.Fatalf("writeSecureText(e2ee_signing) error = %v", err)
	}
	if err := writeSecureText(identityPaths.E2EEAgreementPrivatePath, generated.E2EEAgreementPrivatePEM); err != nil {
		t.Fatalf("writeSecureText(e2ee_agreement) error = %v", err)
	}
	return IndexEntry{
		CredentialName: credentialName,
		DirName:        dirName,
		DID:            generated.DID,
		UniqueID:       generated.UniqueID,
		UserID:         "user-" + credentialName,
		Name:           "Indexed " + handle,
		Handle:         handle,
		CreatedAt:      "2026-01-01T00:00:00Z",
	}
}

func TestScanLegacyDetectsIndexedFlatInvalidAndOrphanArtifacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manager := legacyTestManager(root)
	legacyRoot := manager.LegacyRootDir()
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	indexedEntry := writeLegacyIndexedCredential(t, legacyRoot, "indexed", "legacy-indexed", "indexed")
	if err := saveIndexTo(filepath.Join(legacyRoot, IndexFileName), IndexPayload{
		SchemaVersion:         IndexSchemaVersion,
		DefaultCredentialName: "indexed",
		Credentials: map[string]IndexEntry{
			"indexed": indexedEntry,
		},
	}); err != nil {
		t.Fatalf("saveIndexTo() error = %v", err)
	}
	writeLegacyFlatCredential(t, legacyRoot, "flat", "flat")
	if err := os.WriteFile(filepath.Join(legacyRoot, "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(broken.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "note.json"), []byte(`{"note":"not a credential"}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile(note.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, LegacyE2EEPrefix+"orphan.json"), []byte(`{"state":"orphan"}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile(orphan e2ee) error = %v", err)
	}

	scan, err := manager.ScanLegacy()
	if err != nil {
		t.Fatalf("manager.ScanLegacy() error = %v", err)
	}
	if !scan.HasLegacy || !scan.IndexedLayout {
		t.Fatalf("scan = %#v, want legacy indexed layout detected", scan)
	}
	if len(scan.IndexedEntries) != 1 || scan.IndexedEntries["indexed"].DirName != "legacy-indexed" {
		t.Fatalf("scan.IndexedEntries = %#v, want indexed entry", scan.IndexedEntries)
	}
	if len(scan.LegacyCredentials) != 1 || scan.LegacyCredentials[0].CredentialName != "flat" {
		t.Fatalf("scan.LegacyCredentials = %#v, want flat credential", scan.LegacyCredentials)
	}
	invalidFiles := []string{}
	for _, item := range scan.InvalidJSONFiles {
		invalidFiles = append(invalidFiles, item["file"])
	}
	sort.Strings(invalidFiles)
	if len(invalidFiles) != 2 || invalidFiles[0] != "broken.json" || invalidFiles[1] != "note.json" {
		t.Fatalf("invalidFiles = %#v, want [broken.json note.json]", invalidFiles)
	}
	if len(scan.OrphanE2EEFiles) != 1 || scan.OrphanE2EEFiles[0]["credential_name"] != "orphan" {
		t.Fatalf("scan.OrphanE2EEFiles = %#v, want orphan e2ee entry", scan.OrphanE2EEFiles)
	}
}

func TestImportLegacyRequiresNameWhenMultipleFlatCredentialsExist(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manager := legacyTestManager(root)
	legacyRoot := manager.LegacyRootDir()
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	writeLegacyFlatCredential(t, legacyRoot, "alice", "alice")
	writeLegacyFlatCredential(t, legacyRoot, "bob", "bob")

	result, err := manager.ImportLegacy("")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("manager.ImportLegacy(\"\") error = %v, want ErrInvalidInput", err)
	}
	if result != nil {
		t.Fatalf("manager.ImportLegacy(\"\") result = %#v, want nil", result)
	}
}

func TestImportAllLegacyImportsIndexedDefaultAndCopiesFlatE2EEState(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manager := legacyTestManager(root)
	legacyRoot := manager.LegacyRootDir()
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	indexedEntry := writeLegacyIndexedCredential(t, legacyRoot, "indexed", "legacy-indexed", "indexed")
	if err := saveIndexTo(filepath.Join(legacyRoot, IndexFileName), IndexPayload{
		SchemaVersion:         IndexSchemaVersion,
		DefaultCredentialName: "indexed",
		Credentials: map[string]IndexEntry{
			"indexed": indexedEntry,
		},
	}); err != nil {
		t.Fatalf("saveIndexTo() error = %v", err)
	}
	writeLegacyFlatCredential(t, legacyRoot, "flat", "flat")
	if err := os.WriteFile(filepath.Join(legacyRoot, LegacyE2EEPrefix+"flat.json"), []byte(`{"state":"copied"}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile(flat e2ee) error = %v", err)
	}

	result, err := manager.ImportAllLegacy()
	if err != nil {
		t.Fatalf("manager.ImportAllLegacy() error = %v", err)
	}
	if len(result.Imported) != 2 {
		t.Fatalf("len(result.Imported) = %d, want 2: %#v", len(result.Imported), result)
	}
	current, err := manager.Current()
	if err != nil {
		t.Fatalf("manager.Current() error = %v", err)
	}
	if current.IdentityName != "indexed" {
		t.Fatalf("current identity = %#v, want indexed as imported default", current)
	}
	flatPaths, err := manager.PathsForIdentity("flat")
	if err != nil {
		t.Fatalf("manager.PathsForIdentity(flat) error = %v", err)
	}
	raw, err := os.ReadFile(flatPaths.E2EEStatePath)
	if err != nil {
		t.Fatalf("os.ReadFile(flat e2ee state) error = %v", err)
	}
	if string(raw) != `{"state":"copied"}` {
		t.Fatalf("flat e2ee state = %q, want copied payload", string(raw))
	}
}

func TestImportAllLegacySkipsConflictingFlatCredentials(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manager := legacyTestManager(root)
	legacyRoot := manager.LegacyRootDir()
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	existing := writeLegacyFlatCredential(t, legacyRoot, "conflict", "conflict")
	if _, err := manager.Save(SaveInput{
		IdentityName:   "conflict",
		DID:            "did:wba:awiki.test:user:conflict:e1_other",
		UniqueID:       "e1_other",
		DisplayName:    "Conflict",
		DIDDocument:    existing.DIDDocument,
		Key1PrivatePEM: existing.Key1PrivatePEM,
		Key1PublicPEM:  existing.Key1PublicPEM,
	}); err != nil {
		t.Fatalf("manager.Save(existing conflict) error = %v", err)
	}
	writeLegacyFlatCredential(t, legacyRoot, "ok", "ok")

	result, err := manager.ImportAllLegacy()
	if err != nil {
		t.Fatalf("manager.ImportAllLegacy() error = %v", err)
	}
	if len(result.Imported) != 1 || result.Imported[0].IdentityName != "ok" {
		t.Fatalf("result.Imported = %#v, want only ok imported", result.Imported)
	}
	if len(result.Skipped) != 1 || result.Skipped[0] != "conflict" {
		t.Fatalf("result.Skipped = %#v, want [conflict]", result.Skipped)
	}
}

package upgrade

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	runtimecfg "github.com/agentconnect/awiki-cli/internal/runtime"
	"github.com/agentconnect/awiki-cli/internal/store"
)

func TestUpgradeIfNeededSkipsEmptyWorkspace(t *testing.T) {
	t.Parallel()

	resolved := testResolvedConfig(t)
	if err := UpgradeIfNeeded(context.Background(), resolved, "1.0.0"); err != nil {
		t.Fatalf("UpgradeIfNeeded() error = %v", err)
	}
	paths := ResolvePaths(resolved)
	if fileExists(paths.MetaPath) {
		t.Fatalf("meta file should not exist for an empty workspace")
	}
	if fileExists(paths.JournalPath) {
		t.Fatalf("journal file should not exist for an empty workspace")
	}
}

func TestUpgradeIfNeededStampsCurrentWorkspaceMetadata(t *testing.T) {
	t.Parallel()

	resolved := testResolvedConfig(t)
	if err := os.MkdirAll(filepath.Dir(resolved.Paths.ConfigFile), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(resolved.Paths.ConfigFile, []byte("{\"runtime\":{\"mode\":\"http\"}}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := UpgradeIfNeeded(context.Background(), resolved, "1.2.3"); err != nil {
		t.Fatalf("UpgradeIfNeeded() error = %v", err)
	}
	meta, err := LoadMeta(ResolvePaths(resolved).MetaPath)
	if err != nil {
		t.Fatalf("LoadMeta() error = %v", err)
	}
	if meta == nil || meta.WorkspaceSchemaVersion != LatestWorkspaceSchemaVersion {
		t.Fatalf("unexpected workspace meta: %#v", meta)
	}
	fileConfig, exists, err := appconfig.ReadFileConfig(resolved.Paths.ConfigFile)
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if !exists {
		t.Fatalf("expected config file to exist after upgrade")
	}
	if fileConfig.SchemaVersion != appconfig.ConfigSchemaVersion {
		t.Fatalf("config schema version = %d, want %d", fileConfig.SchemaVersion, appconfig.ConfigSchemaVersion)
	}
}

func TestUpgradeIfNeededImportsLegacyWorkspace(t *testing.T) {
	t.Parallel()

	resolved := testResolvedConfig(t)
	legacyIdentity := writeLegacyIdentity(t, resolved.Paths.LegacyCredentialsDir)
	writeLegacySettings(t, resolved.Paths.LegacyDataDir)
	writeLegacyDatabase(t, resolved.Paths.LegacyDataDir, legacyIdentity.DID)

	if err := UpgradeIfNeeded(context.Background(), resolved, "2.0.0"); err != nil {
		t.Fatalf("UpgradeIfNeeded() error = %v", err)
	}

	meta, err := LoadMeta(ResolvePaths(resolved).MetaPath)
	if err != nil {
		t.Fatalf("LoadMeta() error = %v", err)
	}
	if meta == nil || meta.WorkspaceSchemaVersion != LatestWorkspaceSchemaVersion {
		t.Fatalf("unexpected workspace meta: %#v", meta)
	}

	fileConfig, exists, err := appconfig.ReadFileConfig(resolved.Paths.ConfigFile)
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if !exists {
		t.Fatalf("expected config file to be imported from legacy settings")
	}
	if fileConfig.SchemaVersion != appconfig.ConfigSchemaVersion {
		t.Fatalf("config schema version = %d, want %d", fileConfig.SchemaVersion, appconfig.ConfigSchemaVersion)
	}
	if fileConfig.Runtime.Mode != runtimecfg.ModeWebSocket {
		t.Fatalf("runtime mode = %q, want %q", fileConfig.Runtime.Mode, runtimecfg.ModeWebSocket)
	}
	if fileConfig.Services.ServiceBaseURL != "https://legacy.awiki.test" {
		t.Fatalf("service base url = %q", fileConfig.Services.ServiceBaseURL)
	}

	manager := identity.NewManager(resolved.Paths)
	identities, err := manager.List()
	if err != nil {
		t.Fatalf("manager.List() error = %v", err)
	}
	if len(identities) != 1 || identities[0].IdentityName != "default" {
		t.Fatalf("unexpected imported identities: %#v", identities)
	}

	db, err := store.OpenReadOnly(resolved.Paths.DatabaseFile)
	if err != nil {
		t.Fatalf("store.OpenReadOnly() error = %v", err)
	}
	defer db.Close()
	row, err := store.GetMessageByID(context.Background(), db, "legacy-msg", legacyIdentity.DID, "")
	if err != nil {
		t.Fatalf("store.GetMessageByID() error = %v", err)
	}
	if row["content"] != "legacy hello" {
		t.Fatalf("unexpected imported message: %#v", row)
	}
	if fileExists(ResolvePaths(resolved).JournalPath) {
		t.Fatalf("journal should be cleared after a successful upgrade")
	}
}

func testResolvedConfig(t *testing.T) *appconfig.Resolved {
	t.Helper()
	root := t.TempDir()
	return &appconfig.Resolved{
		Paths: appconfig.Paths{
			WorkspaceHomeDir:     filepath.Join(root, ".awiki-cli"),
			ConfigDir:            filepath.Join(root, ".awiki-cli"),
			DataDir:              filepath.Join(root, "data"),
			StateDir:             filepath.Join(root, "state"),
			CacheDir:             filepath.Join(root, "cache"),
			ConfigFile:           filepath.Join(root, ".awiki-cli", "config.json"),
			IdentityDir:          filepath.Join(root, ".awiki-cli", "identities"),
			DatabaseFile:         filepath.Join(root, "data", "awiki-cli.db"),
			LegacyCredentialsDir: filepath.Join(root, "legacy-credentials"),
			LegacyDataDir:        filepath.Join(root, "legacy-data"),
		},
		RuntimeMode:  runtimecfg.ModeHTTP,
		OutputFormat: "json",
	}
}

func writeLegacyIdentity(t *testing.T, legacyRoot string) *identity.GeneratedIdentity {
	t.Helper()
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	generated, err := identity.GenerateIdentity(identity.GenerateOptions{
		Hostname:    "awiki.ai",
		PathPrefix:  []string{"user"},
		ProofDomain: "awiki.ai",
	})
	if err != nil {
		t.Fatalf("GenerateIdentity() error = %v", err)
	}
	payload := map[string]any{
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
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "default.json"), raw, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return generated
}

func writeLegacySettings(t *testing.T, legacyDataDir string) {
	t.Helper()
	settingsPath := filepath.Join(legacyDataDir, "config", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	payload := map[string]any{
		"user_service_url": "https://legacy.awiki.test",
		"molt_message_url": "https://legacy.awiki.test",
		"did_domain":       "legacy.awiki.test",
		"message_transport": map[string]any{
			"receive_mode": "websocket",
		},
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	if err := os.WriteFile(settingsPath, raw, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func writeLegacyDatabase(t *testing.T, legacyDataDir string, ownerDID string) {
	t.Helper()
	legacyDBPath := filepath.Join(legacyDataDir, "database", "awiki.db")
	if err := os.MkdirAll(filepath.Dir(legacyDBPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	db, err := store.Open(appconfig.Paths{DatabaseFile: legacyDBPath})
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	defer db.Close()
	if err := store.EnsureSchema(context.Background(), db); err != nil {
		t.Fatalf("store.EnsureSchema() error = %v", err)
	}
	if err := store.StoreMessage(context.Background(), db, store.MessageRecord{
		MsgID:          "legacy-msg",
		OwnerDID:       ownerDID,
		ThreadID:       store.MakeThreadID(ownerDID, "did:wba:awiki.ai:user:e1_peer", ""),
		Direction:      0,
		SenderDID:      "did:wba:awiki.ai:user:e1_peer",
		ReceiverDID:    ownerDID,
		ContentType:    "text",
		Content:        "legacy hello",
		CredentialName: "default",
	}); err != nil {
		t.Fatalf("store.StoreMessage() error = %v", err)
	}
}

package upgrade

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
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

func TestUpgradeIfNeededMigratesLegacyConfigJSON(t *testing.T) {
	t.Parallel()

	resolved := testResolvedConfig(t)
	legacyConfigPath := appconfig.LegacyConfigPath(resolved.Paths)
	if err := os.MkdirAll(filepath.Dir(legacyConfigPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	legacyConfig := `{"schema_version":1,"services":{"service_base_url":"https://legacy.awiki.test","did_domain":"legacy.awiki.test"},"runtime":{"mode":"http"}}`
	if err := os.WriteFile(legacyConfigPath, []byte(legacyConfig+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := UpgradeIfNeeded(context.Background(), resolved, "1.2.3"); err != nil {
		t.Fatalf("UpgradeIfNeeded() error = %v", err)
	}
	if _, err := os.Stat(legacyConfigPath); !os.IsNotExist(err) {
		t.Fatalf("legacy config should be removed after migration, stat err = %v", err)
	}
	fileConfig, exists, err := appconfig.ReadFileConfig(resolved.Paths.ConfigFile)
	if err != nil {
		t.Fatalf("ReadFileConfig() error = %v", err)
	}
	if !exists {
		t.Fatalf("expected canonical config file to exist after migration")
	}
	if fileConfig.Services.ServiceBaseURL != "https://legacy.awiki.test" {
		t.Fatalf("service base url = %q", fileConfig.Services.ServiceBaseURL)
	}
	if fileConfig.Services.DIDDomain != "legacy.awiki.test" {
		t.Fatalf("did domain = %q", fileConfig.Services.DIDDomain)
	}
}

func TestLoadLegacySettingsRejectsSplitServiceURLs(t *testing.T) {
	t.Parallel()

	legacyDataDir := t.TempDir()
	writeLegacySettingsSplit(t, legacyDataDir, "https://auth.awiki.test", "https://msg.awiki.test", "awiki.test")
	_, err := loadLegacySettings(filepath.Join(legacyDataDir, "config", "settings.json"))
	if err == nil {
		t.Fatal("loadLegacySettings() error = nil, want split-endpoint error")
	}
	if got := err.Error(); !strings.Contains(got, "different user_service_url") {
		t.Fatalf("loadLegacySettings() error = %q, want split endpoint message", got)
	}
}

func TestUpgradeIfNeededImportsLegacyWorkspace(t *testing.T) {
	t.Parallel()

	resolved := testResolvedConfig(t)
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user-service/did-auth/rpc" {
			t.Fatalf("r.URL.Path = %q, want %q", r.URL.Path, "/user-service/did-auth/rpc")
		}
		gotAuth = r.Header.Get("Authorization")
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if got, _ := payload["method"].(string); got != "replace_did" {
			t.Fatalf("rpc method = %q, want replace_did", got)
		}
		params, _ := payload["params"].(map[string]any)
		newDocument, _ := params["new_did_document"].(map[string]any)
		newDID, _ := newDocument["id"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"old_did":"legacy-old","did":"` + newDID + `","user_id":"user-legacy","handle":"legacy-alice","full_handle":"legacy-alice.awiki.test","access_token":"new-token","message":"DID replaced successfully"},"id":"req-1"}`))
	}))
	defer server.Close()

	legacyIdentity := writeLegacyIdentity(t, resolved.Paths.LegacyCredentialsDir)
	writeLegacySettings(t, resolved.Paths.LegacyDataDir, server.URL, "awiki.test")
	writeLegacyDatabase(t, resolved.Paths.LegacyDataDir, legacyIdentity.DID)

	if err := UpgradeIfNeeded(context.Background(), resolved, "2.0.0"); err != nil {
		t.Fatalf("UpgradeIfNeeded() error = %v", err)
	}
	if gotAuth != "Bearer legacy-token" {
		t.Fatalf("Authorization = %q, want Bearer legacy-token", gotAuth)
	}

	meta, err := LoadMeta(ResolvePaths(resolved).MetaPath)
	if err != nil {
		t.Fatalf("LoadMeta() error = %v", err)
	}
	if meta == nil || meta.WorkspaceSchemaVersion != LatestWorkspaceSchemaVersion {
		t.Fatalf("unexpected workspace meta: %#v", meta)
	}
	if len(meta.Warnings) != 0 {
		t.Fatalf("meta.Warnings = %#v, want none", meta.Warnings)
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
	if fileConfig.Services.ServiceBaseURL != server.URL {
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
	if !identity.IsE1DID(identities[0].DID) {
		t.Fatalf("imported identity DID = %q, want e1 DID", identities[0].DID)
	}
	record, err := manager.Load("default")
	if err != nil {
		t.Fatalf("manager.Load() error = %v", err)
	}
	if record.JWTToken != "new-token" {
		t.Fatalf("record.JWTToken = %q, want new-token", record.JWTToken)
	}

	db, err := store.OpenReadOnly(resolved.Paths.DatabaseFile)
	if err != nil {
		t.Fatalf("store.OpenReadOnly() error = %v", err)
	}
	defer db.Close()
	row, err := store.GetMessageByID(context.Background(), db, "legacy-msg", record.DID, "")
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
			ConfigFile:           filepath.Join(root, ".awiki-cli", "config.yaml"),
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
	service, err := identity.BuildAgentANPMessageService("https://awiki.test/anp-im/rpc", "did:wba:awiki.test")
	if err != nil {
		t.Fatalf("BuildAgentANPMessageService() error = %v", err)
	}
	bundle, err := anpsdk.CreateDidWBADocument("awiki.test", anpsdk.DidDocumentOptions{
		PathSegments: []string{"legacy-alice"},
		Domain:       "awiki.test",
		Challenge:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Services:     []map[string]any{service},
		DidProfile:   anpsdk.DidProfileK1,
	})
	if err != nil {
		t.Fatalf("CreateDidWBADocument() error = %v", err)
	}
	key1 := bundle.Keys["key-1"]
	generated := &identity.GeneratedIdentity{
		DID:            stringValue(bundle.DidDocument["id"]),
		UniqueID:       didSuffixForTest(stringValue(bundle.DidDocument["id"])),
		DIDDocument:    bundle.DidDocument,
		Key1PrivatePEM: key1.PrivateKeyPEM,
		Key1PublicPEM:  key1.PublicKeyPEM,
	}
	if key2, ok := bundle.Keys["key-2"]; ok {
		generated.E2EESigningPrivatePEM = key2.PrivateKeyPEM
	}
	if key3, ok := bundle.Keys["key-3"]; ok {
		generated.E2EEAgreementPrivatePEM = key3.PrivateKeyPEM
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

func writeLegacySettingsSplit(t *testing.T, legacyDataDir string, userServiceURL string, moltMessageURL string, didDomain string) {
	t.Helper()
	settingsPath := filepath.Join(legacyDataDir, "config", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	payload := map[string]any{
		"user_service_url": userServiceURL,
		"molt_message_url": moltMessageURL,
		"did_domain":       didDomain,
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

func writeLegacySettings(t *testing.T, legacyDataDir string, serviceBaseURL string, didDomain string) {
	t.Helper()
	settingsPath := filepath.Join(legacyDataDir, "config", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	payload := map[string]any{
		"user_service_url": serviceBaseURL,
		"molt_message_url": serviceBaseURL,
		"did_domain":       didDomain,
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

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func didSuffixForTest(did string) string {
	lastIndex := len(did) - 1
	for idx := len(did) - 1; idx >= 0; idx-- {
		if did[idx] == ':' {
			lastIndex = idx
			break
		}
	}
	if lastIndex < len(did)-1 {
		return did[lastIndex+1:]
	}
	return did
}

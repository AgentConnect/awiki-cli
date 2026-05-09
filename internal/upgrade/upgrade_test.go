package upgrade

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	runtimecfg "github.com/agentconnect/awiki-cli/internal/runtime"
	"github.com/agentconnect/awiki-cli/internal/store"
	"github.com/agentconnect/awiki-cli/internal/testenv"
)

type legacyIdentityFixture struct {
	name        string
	handle      string
	displayName string
	token       string
}

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
	legacyConfig := `{"schema_version":1,"services":{"service_base_url":"` + testenv.SubdomainURL("legacy") + `","did_domain":"` + testenv.Subdomain("legacy") + `"},"runtime":{"mode":"http"}}`
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
	if fileConfig.Services.ServiceBaseURL != testenv.SubdomainURL("legacy") {
		t.Fatalf("service base url = %q", fileConfig.Services.ServiceBaseURL)
	}
	if fileConfig.Services.DIDDomain != testenv.Subdomain("legacy") {
		t.Fatalf("did domain = %q", fileConfig.Services.DIDDomain)
	}
}

func TestLoadLegacySettingsRejectsSplitServiceURLs(t *testing.T) {
	t.Parallel()

	legacyDataDir := t.TempDir()
	writeLegacySettingsSplit(t, legacyDataDir, testenv.SubdomainURL("auth"), testenv.SubdomainURL("msg"), testenv.Domain())
	_, err := loadLegacySettings(filepath.Join(legacyDataDir, "config", "settings.json"))
	if err == nil {
		t.Fatal("loadLegacySettings() error = nil, want split-endpoint error")
	}
	if got := err.Error(); !strings.Contains(got, "different user_service_url") {
		t.Fatalf("loadLegacySettings() error = %q, want split endpoint message", got)
	}
}

func TestUpgradeIfNeededImportsLegacyWorkspace(t *testing.T) {
	resolved := testResolvedConfig(t)
	homeDir := filepath.Dir(resolved.Paths.WorkspaceHomeDir)
	commandCalls := installLegacyCleanupTestHooks(t, homeDir)
	writeLegacySkillArtifacts(t, homeDir)

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
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"old_did":"legacy-old","did":"` + newDID + `","user_id":"user-legacy","handle":"legacy-alice","full_handle":"` + testenv.FullHandle("legacy-alice") + `","access_token":"new-token","message":"DID replaced successfully"},"id":"req-1"}`))
	}))
	defer server.Close()

	legacyIdentity := writeLegacyIdentity(t, resolved.Paths.LegacyCredentialsDir)
	writeLegacySettings(t, resolved.Paths.LegacyDataDir, server.URL, testenv.Domain())
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
	assertLegacySkillArtifactsRemoved(t, homeDir)
	if len(*commandCalls) == 0 {
		t.Fatal("legacy cleanup did not invoke any listener stop/uninstall command")
	}
}

func TestUpgradeIfNeededReplacesAllImportedLegacyK1Handles(t *testing.T) {
	resolved := testResolvedConfig(t)
	homeDir := filepath.Dir(resolved.Paths.WorkspaceHomeDir)
	installLegacyCleanupTestHooks(t, homeDir)

	legacyIdentities := []legacyIdentityFixture{
		{name: "default", handle: "legacy-default", displayName: "Legacy Default", token: "legacy-token-default"},
		{name: "alice", handle: "legacy-alice", displayName: "Legacy Alice", token: "legacy-token-alice"},
		{name: "bob", handle: "legacy-bob", displayName: "Legacy Bob", token: "legacy-token-bob"},
	}

	replaceServer := startReplaceDIDTestServer(t, writeLegacyIdentityFixtures(t, resolved, legacyIdentities))

	writeLegacySettings(t, resolved.Paths.LegacyDataDir, replaceServer.URL(), testenv.Domain())

	if err := UpgradeIfNeeded(context.Background(), resolved, "2.0.0"); err != nil {
		t.Fatalf("UpgradeIfNeeded() error = %v", err)
	}

	meta, err := LoadMeta(ResolvePaths(resolved).MetaPath)
	if err != nil {
		t.Fatalf("LoadMeta() error = %v", err)
	}
	if len(meta.Warnings) != 0 {
		t.Fatalf("meta.Warnings = %#v, want none", meta.Warnings)
	}
	replaceServer.assertCalls(t, legacyIdentities, "imported")

	manager := identity.NewManager(resolved.Paths)
	identities, err := manager.List()
	if err != nil {
		t.Fatalf("manager.List() error = %v", err)
	}
	if len(identities) != len(legacyIdentities) {
		t.Fatalf("len(identities) = %d, want %d: %#v", len(identities), len(legacyIdentities), identities)
	}
	assertStoredIdentitiesReplaced(t, manager, legacyIdentities)
	assertReplaceDIDBackupsForFixtures(t, resolved, legacyIdentities)
}

func TestUpgradeIfNeededReplacesExistingWorkspaceK1Handles(t *testing.T) {
	resolved := testResolvedConfig(t)

	legacyIdentities := []legacyIdentityFixture{
		{name: "default", handle: "existing-default", displayName: "Existing Default", token: "existing-token-default"},
		{name: "alice", handle: "existing-alice", displayName: "Existing Alice", token: "existing-token-alice"},
		{name: "bob", handle: "existing-bob", displayName: "Existing Bob", token: "existing-token-bob"},
	}

	expectedHandleByAuth := writeLegacyIdentityFixtures(t, resolved, legacyIdentities)
	manager := identity.NewManager(resolved.Paths)
	imported, err := manager.ImportAllLegacy()
	if err != nil {
		t.Fatalf("manager.ImportAllLegacy() error = %v", err)
	}
	if len(imported.Imported) != len(legacyIdentities) {
		t.Fatalf("len(imported.Imported) = %d, want %d", len(imported.Imported), len(legacyIdentities))
	}

	replaceServer := startReplaceDIDTestServer(t, expectedHandleByAuth)

	resolved.ServiceBaseURL = replaceServer.URL()
	resolved.DIDDomain = testenv.Domain()
	resolved.ANPServiceEndpoint = identity.DefaultANPServiceEndpoint(testenv.Domain())
	resolved.ANPServiceDID = identity.DefaultANPServiceDID(testenv.Domain())
	fileConfig := appconfig.FileConfig{}
	fileConfig.Runtime.Mode = runtimecfg.ModeWebSocket
	fileConfig.Services.ServiceBaseURL = replaceServer.URL()
	fileConfig.Services.DIDDomain = testenv.Domain()
	if err := appconfig.WriteFileConfig(resolved.Paths.ConfigFile, fileConfig); err != nil {
		t.Fatalf("WriteFileConfig() error = %v", err)
	}
	if err := SaveMeta(ResolvePaths(resolved).MetaPath, Meta{
		WorkspaceSchemaVersion: 2,
		AppVersion:             "2.0.0",
		UpdatedAt:              "2026-04-17T00:00:00Z",
		LastUpgradeID:          "20260417T000000Z",
	}); err != nil {
		t.Fatalf("SaveMeta() error = %v", err)
	}

	if err := UpgradeIfNeeded(context.Background(), resolved, "2.1.0"); err != nil {
		t.Fatalf("UpgradeIfNeeded() error = %v", err)
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
	replaceServer.assertCalls(t, legacyIdentities, "existing")

	assertStoredIdentitiesReplaced(t, manager, legacyIdentities)
	assertReplaceDIDBackupsForFixtures(t, resolved, legacyIdentities)
}

func writeLegacyIdentityFixtures(t *testing.T, resolved *appconfig.Resolved, fixtures []legacyIdentityFixture) map[string]string {
	t.Helper()
	expectedHandleByAuth := make(map[string]string, len(fixtures))
	for _, legacy := range fixtures {
		writeLegacyIdentityNamed(t, resolved.Paths.LegacyCredentialsDir, legacy.name, legacy.handle, legacy.displayName, legacy.token)
		expectedHandleByAuth["Bearer "+legacy.token] = legacy.handle
	}
	return expectedHandleByAuth
}

type replaceDIDTestServer struct {
	server        *httptest.Server
	mu            sync.Mutex
	callsByHandle map[string]int
}

func startReplaceDIDTestServer(t *testing.T, expectedHandleByAuth map[string]string) *replaceDIDTestServer {
	t.Helper()
	server := &replaceDIDTestServer{
		callsByHandle: make(map[string]int, len(expectedHandleByAuth)),
	}
	server.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user-service/did-auth/rpc" {
			t.Fatalf("r.URL.Path = %q, want %q", r.URL.Path, "/user-service/did-auth/rpc")
		}
		handle, ok := expectedHandleByAuth[r.Header.Get("Authorization")]
		if !ok {
			t.Fatalf("Authorization = %q, want one of %#v", r.Header.Get("Authorization"), expectedHandleByAuth)
		}
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
		if !identity.IsE1DID(newDID) {
			t.Fatalf("new DID = %q, want e1 DID", newDID)
		}

		server.mu.Lock()
		server.callsByHandle[handle]++
		server.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"result": map[string]any{
				"old_did":      "legacy-old",
				"did":          newDID,
				"user_id":      "user-" + handle,
				"handle":       handle,
				"full_handle":  testenv.FullHandle(handle),
				"access_token": "new-token-" + handle,
				"message":      "DID replaced successfully",
			},
			"id": "req-1",
		}); err != nil {
			t.Fatalf("Encode() error = %v", err)
		}
	}))
	t.Cleanup(server.server.Close)
	return server
}

func (s *replaceDIDTestServer) URL() string {
	return s.server.URL
}

func (s *replaceDIDTestServer) assertCalls(t *testing.T, fixtures []legacyIdentityFixture, label string) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.callsByHandle) != len(fixtures) {
		t.Fatalf("replace calls = %#v, want one call per %s handle", s.callsByHandle, label)
	}
	for _, legacy := range fixtures {
		if s.callsByHandle[legacy.handle] != 1 {
			t.Fatalf("replace calls for handle %s = %d, want 1; all calls = %#v", legacy.handle, s.callsByHandle[legacy.handle], s.callsByHandle)
		}
	}
}

func assertStoredIdentitiesReplaced(t *testing.T, manager *identity.Manager, fixtures []legacyIdentityFixture) {
	t.Helper()
	for _, legacy := range fixtures {
		record, err := manager.Load(legacy.name)
		if err != nil {
			t.Fatalf("manager.Load(%q) error = %v", legacy.name, err)
		}
		if !identity.IsE1DID(record.DID) {
			t.Fatalf("identity %s DID = %q, want e1 DID", legacy.name, record.DID)
		}
		if record.Handle != legacy.handle {
			t.Fatalf("identity %s handle = %q, want %q", legacy.name, record.Handle, legacy.handle)
		}
		if record.JWTToken != "new-token-"+legacy.handle {
			t.Fatalf("identity %s JWTToken = %q, want %q", legacy.name, record.JWTToken, "new-token-"+legacy.handle)
		}
	}
}

func assertReplaceDIDBackupsForFixtures(t *testing.T, resolved *appconfig.Resolved, fixtures []legacyIdentityFixture) {
	t.Helper()
	backupRoot := filepath.Join(resolved.Paths.IdentityDir, identity.LegacyBackupDirName, "replace-did")
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", backupRoot, err)
	}
	if len(entries) != len(fixtures) {
		t.Fatalf("backup dir count = %d, want %d", len(entries), len(fixtures))
	}
	expected := make(map[string]struct{}, len(fixtures))
	for _, fixture := range fixtures {
		expected[fixture.name] = struct{}{}
	}
	seen := map[string]struct{}{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		backupPath := filepath.Join(backupRoot, entry.Name())
		manifest := readUpgradeTestJSONMap(t, filepath.Join(backupPath, "backup_manifest.json"))
		identityName, _ := manifest["identity_name"].(string)
		if _, ok := expected[identityName]; !ok {
			t.Fatalf("unexpected backup identity_name %q in %#v", identityName, manifest)
		}
		seen[identityName] = struct{}{}
		oldDID, _ := manifest["old_did"].(string)
		if !identity.IsK1DID(oldDID) {
			t.Fatalf("backup old_did = %q, want k1 DID", oldDID)
		}
		plannedNewDID, _ := manifest["planned_new_did"].(string)
		if !identity.IsE1DID(plannedNewDID) {
			t.Fatalf("backup planned_new_did = %q, want e1 DID", plannedNewDID)
		}
		if _, err := os.Stat(filepath.Join(backupPath, identity.DIDDocumentFileName)); err != nil {
			t.Fatalf("backup %s missing DID document: %v", backupPath, err)
		}
		if _, err := os.Stat(filepath.Join(backupPath, identity.Key1PrivateFileName)); err != nil {
			t.Fatalf("backup %s missing private key: %v", backupPath, err)
		}
	}
	for _, fixture := range fixtures {
		if _, ok := seen[fixture.name]; !ok {
			t.Fatalf("backup for identity %s not found; seen = %#v", fixture.name, seen)
		}
	}
}

func readUpgradeTestJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", path, err)
	}
	return payload
}

func TestUpgradeIfNeededCleansLegacySkillArtifactsForExistingWorkspace(t *testing.T) {
	resolved := testResolvedConfig(t)
	homeDir := filepath.Dir(resolved.Paths.WorkspaceHomeDir)
	commandCalls := installLegacyCleanupTestHooks(t, homeDir)

	if err := os.MkdirAll(filepath.Dir(resolved.Paths.ConfigFile), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(resolved.Paths.ConfigFile, []byte("{\"runtime\":{\"mode\":\"websocket\"}}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	paths := ResolvePaths(resolved)
	if err := os.MkdirAll(filepath.Dir(paths.MetaPath), 0o700); err != nil {
		t.Fatalf("MkdirAll(meta) error = %v", err)
	}
	if err := SaveMeta(paths.MetaPath, Meta{
		WorkspaceSchemaVersion: 1,
		AppVersion:             "1.9.0",
		UpdatedAt:              "2026-04-16T00:00:00Z",
		LastUpgradeID:          "20260416T000000Z",
	}); err != nil {
		t.Fatalf("SaveMeta() error = %v", err)
	}

	writeLegacySkillArtifacts(t, homeDir)

	if err := UpgradeIfNeeded(context.Background(), resolved, "2.1.0"); err != nil {
		t.Fatalf("UpgradeIfNeeded() error = %v", err)
	}

	meta, err := LoadMeta(paths.MetaPath)
	if err != nil {
		t.Fatalf("LoadMeta() error = %v", err)
	}
	if meta == nil || meta.WorkspaceSchemaVersion != LatestWorkspaceSchemaVersion {
		t.Fatalf("unexpected workspace meta: %#v", meta)
	}
	assertLegacySkillArtifactsRemoved(t, homeDir)
	if len(*commandCalls) == 0 {
		t.Fatal("legacy cleanup did not invoke any listener stop/uninstall command")
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
		RuntimeMode:         runtimecfg.ModeHTTP,
		OutputFormat:        "json",
		ServiceBaseURL:      "https://awiki.ai",
		DIDDomain:           "awiki.ai",
		ANPServiceEndpoint:  appconfig.DeriveANPServiceEndpoint("https://awiki.ai"),
		ANPServiceDID:       appconfig.DeriveANPServiceDID("https://awiki.ai"),
		ConfigSchemaVersion: appconfig.ConfigSchemaVersion,
	}
}

func TestRefreshResolvedConfigSyncsMailServiceURLFromConfig(t *testing.T) {
	t.Parallel()

	resolved := testResolvedConfig(t)
	if err := os.MkdirAll(filepath.Dir(resolved.Paths.ConfigFile), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	configYAML := []byte(
		"services:\n" +
			"  service_base_url: " + testenv.SubdomainURL("api") + "/\n" +
			"  mail_service_url: " + testenv.SubdomainURL("mail") + "/\n",
	)
	if err := os.WriteFile(resolved.Paths.ConfigFile, configYAML, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	refreshed, err := refreshResolvedConfig(resolved)
	if err != nil {
		t.Fatalf("refreshResolvedConfig() error = %v", err)
	}
	if refreshed.ServiceBaseURL != testenv.SubdomainURL("api") {
		t.Fatalf("refreshed.ServiceBaseURL = %q, want %q", refreshed.ServiceBaseURL, testenv.SubdomainURL("api"))
	}
	if refreshed.MailServiceURL != testenv.SubdomainURL("mail") {
		t.Fatalf("refreshed.MailServiceURL = %q, want %q", refreshed.MailServiceURL, testenv.SubdomainURL("mail"))
	}
}

func TestRefreshResolvedConfigDerivesMailServiceURLFromServiceBaseURL(t *testing.T) {
	t.Parallel()

	resolved := testResolvedConfig(t)
	if err := os.MkdirAll(filepath.Dir(resolved.Paths.ConfigFile), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	configYAML := []byte(
		"services:\n" +
			"  service_base_url: " + testenv.BaseURL() + "/\n",
	)
	if err := os.WriteFile(resolved.Paths.ConfigFile, configYAML, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	refreshed, err := refreshResolvedConfig(resolved)
	if err != nil {
		t.Fatalf("refreshResolvedConfig() error = %v", err)
	}
	if refreshed.ServiceBaseURL != testenv.BaseURL() {
		t.Fatalf("refreshed.ServiceBaseURL = %q, want %q", refreshed.ServiceBaseURL, testenv.BaseURL())
	}
	if refreshed.MailServiceURL != testenv.BaseURL() {
		t.Fatalf("refreshed.MailServiceURL = %q, want %q", refreshed.MailServiceURL, testenv.BaseURL())
	}
}

func writeLegacyIdentity(t *testing.T, legacyRoot string) *identity.GeneratedIdentity {
	return writeLegacyIdentityNamed(t, legacyRoot, "default", "legacy-alice", "Legacy Alice", "legacy-token")
}

func writeLegacyIdentityNamed(t *testing.T, legacyRoot string, credentialName string, handle string, displayName string, jwtToken string) *identity.GeneratedIdentity {
	t.Helper()
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	service, err := identity.BuildAgentANPMessageService(testenv.BaseURL()+"/anp-im/rpc", testenv.ServiceDID())
	if err != nil {
		t.Fatalf("BuildAgentANPMessageService() error = %v", err)
	}
	bundle, err := anpsdk.CreateDidWBADocument(testenv.Domain(), anpsdk.DidDocumentOptions{
		PathSegments: []string{handle},
		Domain:       testenv.Domain(),
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
		"name":                       displayName,
		"handle":                     handle,
		"jwt_token":                  jwtToken,
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
	if err := os.WriteFile(filepath.Join(legacyRoot, credentialName+".json"), raw, 0o600); err != nil {
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

func installLegacyCleanupTestHooks(t *testing.T, homeDir string) *[]string {
	t.Helper()

	originalHome := legacyCleanupUserHome
	originalRun := legacyCleanupRunCommand

	commandCalls := make([]string, 0)
	legacyCleanupUserHome = func() (string, error) {
		return homeDir, nil
	}
	legacyCleanupRunCommand = func(_ context.Context, name string, args ...string) error {
		commandCalls = append(commandCalls, strings.Join(append([]string{name}, args...), " "))
		return nil
	}
	t.Cleanup(func() {
		legacyCleanupUserHome = originalHome
		legacyCleanupRunCommand = originalRun
	})
	return &commandCalls
}

func writeLegacySkillArtifacts(t *testing.T, homeDir string) {
	t.Helper()

	for _, path := range legacySkillInstallDirs(homeDir) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", path, err)
		}
		if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("# legacy\n"), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}

	servicePath := legacyServiceArtifactPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(servicePath), 0o700); err != nil {
		t.Fatalf("MkdirAll(service) error = %v", err)
	}
	if err := os.WriteFile(servicePath, []byte("legacy service\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(service) error = %v", err)
	}

	heartbeatPath := filepath.Join(homeDir, ".openclaw", "workspace", "HEARTBEAT.md")
	if err := os.MkdirAll(filepath.Dir(heartbeatPath), 0o700); err != nil {
		t.Fatalf("MkdirAll(heartbeat) error = %v", err)
	}
	legacySkillDir := filepath.Join(homeDir, ".openclaw", "skills", legacySkillInstallDirName)
	heartbeatContent := "# Heartbeat checklist\n\n" +
		legacyHeartbeatSectionStart + "\n" +
		"## awiki — DID messaging (every heartbeat)\n\n" +
		"- Run: `cd " + legacySkillDir + " && python scripts/check_status.py`\n" +
		legacyHeartbeatSectionEnd + "\n\n" +
		"## Other checks\n\n- Keep this section.\n"
	if err := os.WriteFile(heartbeatPath, []byte(heartbeatContent), 0o600); err != nil {
		t.Fatalf("WriteFile(HEARTBEAT.md) error = %v", err)
	}
}

func assertLegacySkillArtifactsRemoved(t *testing.T, homeDir string) {
	t.Helper()

	for _, path := range legacySkillInstallDirs(homeDir) {
		if pathExists(path) {
			t.Fatalf("legacy skill path still exists: %s", path)
		}
	}

	if pathExists(legacyServiceArtifactPath(homeDir)) {
		t.Fatalf("legacy listener service artifact still exists: %s", legacyServiceArtifactPath(homeDir))
	}

	heartbeatPath := filepath.Join(homeDir, ".openclaw", "workspace", "HEARTBEAT.md")
	raw, err := os.ReadFile(heartbeatPath)
	if err != nil {
		t.Fatalf("ReadFile(HEARTBEAT.md) error = %v", err)
	}
	text := string(raw)
	if strings.Contains(text, legacyHeartbeatSectionStart) || strings.Contains(text, legacySkillInstallDirName) {
		t.Fatalf("legacy heartbeat section still present: %s", text)
	}
	if !strings.Contains(text, "## Other checks") {
		t.Fatalf("heartbeat cleanup removed unrelated content: %s", text)
	}
}

func legacyServiceArtifactPath(homeDir string) string {
	switch legacyCleanupGOOS {
	case "darwin":
		return filepath.Join(homeDir, "Library", "LaunchAgents", legacyMacOSListenerLabel+".plist")
	case "linux":
		return filepath.Join(legacyXDGConfigHome(homeDir), "systemd", "user", legacyLinuxListenerUnit)
	case "windows":
		return filepath.Join(legacyLocalAppData(homeDir), legacyWindowsListenerTaskName, "run-listener.bat")
	default:
		return filepath.Join(homeDir, "legacy-listener-service")
	}
}

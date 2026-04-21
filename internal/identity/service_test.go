package identity_test

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/store"
)

func TestReplaceDIDUpdatesIdentityAndLocalStore(t *testing.T) {
	t.Parallel()

	legacy := generateK1IdentityForTest(t, "awiki.test", []string{"alice"})
	var (
		gotAuth   string
		gotMethod string
		gotNewDID string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user-service/did-auth/rpc" {
			t.Fatalf("r.URL.Path = %q, want %q", r.URL.Path, "/user-service/did-auth/rpc")
		}
		gotAuth = r.Header.Get("Authorization")
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		gotMethod, _ = payload["method"].(string)
		params, _ := payload["params"].(map[string]any)
		newDocument, _ := params["new_did_document"].(map[string]any)
		gotNewDID, _ = newDocument["id"].(string)
		if !identity.IsE1DID(gotNewDID) {
			t.Fatalf("new did = %q, want e1 did", gotNewDID)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"old_did":"` + legacy.DID + `","did":"` + gotNewDID + `","user_id":"user-1","handle":"alice","full_handle":"alice.awiki.test","access_token":"new-token","message":"DID replaced successfully"},"id":"req-1"}`))
	}))
	defer server.Close()

	resolved, manager := newReplaceTestWorkspace(t, server.URL)
	if _, err := manager.Save(identity.SaveInput{
		IdentityName:            "alice",
		DID:                     legacy.DID,
		UniqueID:                legacy.UniqueID,
		UserID:                  "user-1",
		DisplayName:             "Alice",
		Handle:                  "alice",
		JWTToken:                "legacy-token",
		DIDDocument:             legacy.DIDDocument,
		Key1PrivatePEM:          legacy.Key1PrivatePEM,
		Key1PublicPEM:           legacy.Key1PublicPEM,
		E2EESigningPrivatePEM:   legacy.E2EESigningPrivatePEM,
		E2EEAgreementPrivatePEM: legacy.E2EEAgreementPrivatePEM,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	seedReplaceTestStore(t, resolved.Paths, legacy.DID)

	service, err := identity.NewService(resolved)
	if err != nil {
		t.Fatalf("identity.NewService() error = %v", err)
	}
	result, err := service.ReplaceDID(context.Background(), identity.ReplaceDIDParams{})
	if err != nil {
		t.Fatalf("ReplaceDID() error = %v", err)
	}
	if gotAuth != "Bearer legacy-token" {
		t.Fatalf("Authorization = %q, want Bearer legacy-token", gotAuth)
	}
	if gotMethod != "replace_did" {
		t.Fatalf("rpc method = %q, want replace_did", gotMethod)
	}

	updated, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if updated.DID != gotNewDID {
		t.Fatalf("updated DID = %q, want %q", updated.DID, gotNewDID)
	}
	if updated.JWTToken != "new-token" {
		t.Fatalf("updated JWTToken = %q, want new-token", updated.JWTToken)
	}
	if updated.DirName == legacy.UniqueID {
		t.Fatalf("updated dir name = %q, want a new e1 dir", updated.DirName)
	}
	if !identity.IsE1DID(updated.DID) {
		t.Fatalf("updated DID = %q, want e1 did", updated.DID)
	}
	if strings.Contains(updated.Key1PrivatePEM, "BEGIN ANP ") {
		t.Fatalf("new e1 key-1 private key still uses legacy ANP PEM label")
	}
	if !strings.HasPrefix(updated.Key1PrivatePEM, "-----BEGIN PRIVATE KEY-----") {
		t.Fatalf("new e1 key-1 private key = %q, want standard PKCS#8 PEM", updated.Key1PrivatePEM[:32])
	}

	identityData, _ := result.Data["identity"].(*identity.IdentitySummary)
	if identityData == nil || identityData.DID != gotNewDID {
		t.Fatalf("result identity = %#v, want new did %q", identityData, gotNewDID)
	}
	backupPath, _ := result.Data["backup_path"].(string)
	assertReplaceDIDBackup(t, manager, backupPath, legacy, gotNewDID)

	storeRebind, e2eeCleanup, err := store.RebindLocalIdentityState(context.Background(), resolved.Paths, legacy.DID, gotNewDID)
	if err != nil {
		t.Fatalf("store.RebindLocalIdentityState() error = %v", err)
	}
	if storeRebind["messages"] != 1 {
		t.Fatalf("store_rebind = %#v, want messages=1", storeRebind)
	}
	if e2eeCleanup["e2ee_outbox"] != 1 || e2eeCleanup["e2ee_sessions"] != 1 {
		t.Fatalf("e2ee_cleanup = %#v, want both cleanup counts = 1", e2eeCleanup)
	}

	db, err := store.OpenReadOnly(resolved.Paths.DatabaseFile)
	if err != nil {
		t.Fatalf("store.OpenReadOnly() error = %v", err)
	}
	defer db.Close()
	row, err := store.GetMessageByID(context.Background(), db, "msg-1", gotNewDID, "")
	if err != nil {
		t.Fatalf("GetMessageByID() error = %v", err)
	}
	if got, _ := row["content"].(string); got != "legacy hello" {
		t.Fatalf("message content = %q, want legacy hello", got)
	}
	var oldOutboxCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM e2ee_outbox WHERE owner_did = ?`, legacy.DID).Scan(&oldOutboxCount); err != nil {
		t.Fatalf("QueryRowContext(outbox) error = %v", err)
	}
	if oldOutboxCount != 0 {
		t.Fatalf("old outbox count = %d, want 0", oldOutboxCount)
	}
	var oldSessionCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM e2ee_sessions WHERE owner_did = ?`, legacy.DID).Scan(&oldSessionCount); err != nil {
		t.Fatalf("QueryRowContext(session) error = %v", err)
	}
	if oldSessionCount != 0 {
		t.Fatalf("old session count = %d, want 0", oldSessionCount)
	}
}

func TestReplaceDIDConvertsLegacyANPK1KeyWhenJWTMissing(t *testing.T) {
	t.Parallel()

	legacy := generateK1IdentityForTest(t, "awiki.test", []string{"alice"})
	legacy.Key1PrivatePEM = legacyANPPrivatePEMFromStandard(t, legacy.Key1PrivatePEM, "ANP SECP256K1 PRIVATE KEY")

	var methods []string
	var replaceAuth string
	var gotNewDID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user-service/did-auth/rpc" {
			t.Fatalf("r.URL.Path = %q, want %q", r.URL.Path, "/user-service/did-auth/rpc")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		method, _ := payload["method"].(string)
		methods = append(methods, method)
		w.Header().Set("Content-Type", "application/json")
		switch method {
		case "get_me":
			if r.Header.Get("Signature-Input") == "" || r.Header.Get("Signature") == "" {
				t.Fatalf("get_me auth headers missing Signature-Input/Signature: %#v", r.Header)
			}
			w.Header().Set("Authentication-Info", `access_token="fresh-token"`)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"did":"` + legacy.DID + `","user_id":"user-1"},"id":"req-1"}`))
		case "replace_did":
			replaceAuth = r.Header.Get("Authorization")
			params, _ := payload["params"].(map[string]any)
			newDocument, _ := params["new_did_document"].(map[string]any)
			gotNewDID, _ = newDocument["id"].(string)
			if !identity.IsE1DID(gotNewDID) {
				t.Fatalf("new did = %q, want e1 did", gotNewDID)
			}
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"old_did":"` + legacy.DID + `","did":"` + gotNewDID + `","user_id":"user-1","handle":"alice","full_handle":"alice.awiki.test","access_token":"new-token","message":"DID replaced successfully"},"id":"req-1"}`))
		default:
			t.Fatalf("unexpected rpc method %q", method)
		}
	}))
	defer server.Close()

	resolved, manager := newReplaceTestWorkspace(t, server.URL)
	if _, err := manager.Save(identity.SaveInput{
		IdentityName:            "alice",
		DID:                     legacy.DID,
		UniqueID:                legacy.UniqueID,
		UserID:                  "user-1",
		DisplayName:             "Alice",
		Handle:                  "alice",
		JWTToken:                "",
		DIDDocument:             legacy.DIDDocument,
		Key1PrivatePEM:          legacy.Key1PrivatePEM,
		Key1PublicPEM:           legacy.Key1PublicPEM,
		E2EESigningPrivatePEM:   legacy.E2EESigningPrivatePEM,
		E2EEAgreementPrivatePEM: legacy.E2EEAgreementPrivatePEM,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	migratedLegacy, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("Load() migrated legacy identity error = %v", err)
	}
	if strings.Contains(migratedLegacy.Key1PrivatePEM, "BEGIN ANP ") {
		t.Fatalf("migrated key-1 private key still uses legacy ANP PEM label")
	}
	if !strings.HasPrefix(migratedLegacy.Key1PrivatePEM, "-----BEGIN PRIVATE KEY-----") {
		t.Fatalf("migrated key-1 private key = %q, want standard PKCS#8 PEM", migratedLegacy.Key1PrivatePEM[:32])
	}

	service, err := identity.NewService(resolved)
	if err != nil {
		t.Fatalf("identity.NewService() error = %v", err)
	}
	result, err := service.ReplaceDID(context.Background(), identity.ReplaceDIDParams{})
	if err != nil {
		t.Fatalf("ReplaceDID() error = %v", err)
	}
	if got := strings.Join(methods, ","); got != "get_me,replace_did" {
		t.Fatalf("rpc methods = %q, want get_me,replace_did", got)
	}
	if replaceAuth != "Bearer fresh-token" {
		t.Fatalf("replace Authorization = %q, want Bearer fresh-token", replaceAuth)
	}
	backupPath, _ := result.Data["backup_path"].(string)
	expectedBackup := *legacy
	expectedBackup.Key1PrivatePEM = migratedLegacy.Key1PrivatePEM
	assertReplaceDIDBackup(t, manager, backupPath, &expectedBackup, gotNewDID)
}

func TestReplaceDIDStopsBeforeRemoteWhenBackupFails(t *testing.T) {
	t.Parallel()

	legacy := generateK1IdentityForTest(t, "awiki.test", []string{"alice"})
	var remoteCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remoteCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	resolved, manager := newReplaceTestWorkspace(t, server.URL)
	if _, err := manager.Save(identity.SaveInput{
		IdentityName:            "alice",
		DID:                     legacy.DID,
		UniqueID:                legacy.UniqueID,
		UserID:                  "user-1",
		DisplayName:             "Alice",
		Handle:                  "alice",
		JWTToken:                "legacy-token",
		DIDDocument:             legacy.DIDDocument,
		Key1PrivatePEM:          legacy.Key1PrivatePEM,
		Key1PublicPEM:           legacy.Key1PublicPEM,
		E2EESigningPrivatePEM:   legacy.E2EESigningPrivatePEM,
		E2EEAgreementPrivatePEM: legacy.E2EEAgreementPrivatePEM,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(resolved.Paths.IdentityDir, identity.LegacyBackupDirName), []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("WriteFile(.legacy-backup) error = %v", err)
	}

	service, err := identity.NewService(resolved)
	if err != nil {
		t.Fatalf("identity.NewService() error = %v", err)
	}
	if _, err := service.ReplaceDID(context.Background(), identity.ReplaceDIDParams{}); err == nil {
		t.Fatal("ReplaceDID() error = nil, want backup failure")
	} else if !strings.Contains(err.Error(), "backup identity directory") {
		t.Fatalf("ReplaceDID() error = %v, want backup failure", err)
	}
	if remoteCalls.Load() != 0 {
		t.Fatalf("remoteCalls = %d, want 0", remoteCalls.Load())
	}
	stillOld, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if stillOld.DID != legacy.DID {
		t.Fatalf("identity DID = %q, want original DID %q", stillOld.DID, legacy.DID)
	}
}

func TestRecoverStagesAndFinalizesSameHandleLiveIdentities(t *testing.T) {
	t.Parallel()

	first := generateK1IdentityForTest(t, "awiki.test", []string{"zhuocheng"})
	second := generateK1IdentityForTest(t, "awiki.test", []string{"zhuocheng", "archive"})
	other := generateK1IdentityForTest(t, "awiki.test", []string{"lzc"})

	var gotHandle string
	var gotMethod string
	var gotRecoveredDID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user-service/did-auth/rpc" {
			t.Fatalf("r.URL.Path = %q, want %q", r.URL.Path, "/user-service/did-auth/rpc")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		gotMethod, _ = payload["method"].(string)
		params, _ := payload["params"].(map[string]any)
		gotHandle, _ = params["handle"].(string)
		document, _ := params["did_document"].(map[string]any)
		gotRecoveredDID, _ = document["id"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"did":"` + gotRecoveredDID + `","user_id":"user-z","handle":"zhuocheng","full_handle":"zhuocheng.awiki.test","access_token":"recover-token"},"id":"req-1"}`))
	}))
	defer server.Close()

	resolved, manager := newReplaceTestWorkspace(t, server.URL)
	if err := appconfig.WriteFileConfig(resolved.Paths.ConfigFile, appconfig.FileConfig{
		Identity: struct {
			Active string `json:"active" yaml:"active"`
		}{Active: "zhuocheng-2"},
	}); err != nil {
		t.Fatalf("WriteFileConfig() error = %v", err)
	}
	if _, err := manager.Save(identity.SaveInput{
		IdentityName:            "zhuocheng",
		DID:                     first.DID,
		UniqueID:                first.UniqueID,
		UserID:                  "user-z",
		DisplayName:             "zhuocheng",
		Handle:                  "zhuocheng",
		JWTToken:                "token-1",
		DIDDocument:             first.DIDDocument,
		Key1PrivatePEM:          first.Key1PrivatePEM,
		Key1PublicPEM:           first.Key1PublicPEM,
		E2EESigningPrivatePEM:   first.E2EESigningPrivatePEM,
		E2EEAgreementPrivatePEM: first.E2EEAgreementPrivatePEM,
	}); err != nil {
		t.Fatalf("Save(zhuocheng) error = %v", err)
	}
	if _, err := manager.Save(identity.SaveInput{
		IdentityName:            "zhuocheng-2",
		DID:                     second.DID,
		UniqueID:                second.UniqueID,
		UserID:                  "user-z",
		DisplayName:             "zhuocheng",
		Handle:                  "zhuocheng",
		JWTToken:                "token-2",
		DIDDocument:             second.DIDDocument,
		Key1PrivatePEM:          second.Key1PrivatePEM,
		Key1PublicPEM:           second.Key1PublicPEM,
		E2EESigningPrivatePEM:   second.E2EESigningPrivatePEM,
		E2EEAgreementPrivatePEM: second.E2EEAgreementPrivatePEM,
	}); err != nil {
		t.Fatalf("Save(zhuocheng-2) error = %v", err)
	}
	if _, err := manager.Save(identity.SaveInput{
		IdentityName:            "lzc",
		DID:                     other.DID,
		UniqueID:                other.UniqueID,
		UserID:                  "user-lzc",
		DisplayName:             "lzc",
		Handle:                  "lzc",
		JWTToken:                "token-lzc",
		DIDDocument:             other.DIDDocument,
		Key1PrivatePEM:          other.Key1PrivatePEM,
		Key1PublicPEM:           other.Key1PublicPEM,
		E2EESigningPrivatePEM:   other.E2EESigningPrivatePEM,
		E2EEAgreementPrivatePEM: other.E2EEAgreementPrivatePEM,
	}); err != nil {
		t.Fatalf("Save(lzc) error = %v", err)
	}

	service, err := identity.NewService(resolved)
	if err != nil {
		t.Fatalf("identity.NewService() error = %v", err)
	}
	result, err := service.Recover(context.Background(), identity.RecoverParams{
		IdentityName: "ignored-by-recover",
		Handle:       "zhuocheng",
		Phone:        "13800138000",
		OTP:          "123456",
	})
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if gotMethod != "recover_handle" {
		t.Fatalf("rpc method = %q, want recover_handle", gotMethod)
	}
	if gotHandle != "zhuocheng" {
		t.Fatalf("recover handle = %q, want zhuocheng", gotHandle)
	}
	if gotRecoveredDID == "" {
		t.Fatal("recovered DID is empty")
	}

	tempName, _ := result.Data["temp_identity_name"].(string)
	finalName, _ := result.Data["final_identity_name"].(string)
	backupPath, _ := result.Data["backup_path"].(string)
	activeBefore, _ := result.Data["active_before"].(string)
	archived, _ := result.Data["archived_identities"].([]string)
	stagedSummary, _ := result.Data["identity"].(*identity.IdentitySummary)
	if finalName != "zhuocheng" {
		t.Fatalf("final_identity_name = %q, want zhuocheng", finalName)
	}
	if tempName == "" || tempName == "ignored-by-recover" {
		t.Fatalf("temp_identity_name = %q, want generated temporary identity name", tempName)
	}
	if stagedSummary == nil || stagedSummary.DID != gotRecoveredDID {
		t.Fatalf("staged identity = %#v, want recovered did %q", stagedSummary, gotRecoveredDID)
	}
	if activeBefore != "zhuocheng-2" {
		t.Fatalf("active_before = %q, want zhuocheng-2", activeBefore)
	}
	stagedList, err := manager.List()
	if err != nil {
		t.Fatalf("List() staged error = %v", err)
	}
	if len(stagedList) != 4 {
		t.Fatalf("staged live identities = %d, want 4", len(stagedList))
	}

	promoted, err := service.FinalizeRecoveredHandle(finalName, tempName, archived, activeBefore, backupPath, stagedSummary.DID)
	if err != nil {
		t.Fatalf("FinalizeRecoveredHandle() error = %v", err)
	}
	if promoted.IdentityName != "zhuocheng" {
		t.Fatalf("promoted identity name = %q, want zhuocheng", promoted.IdentityName)
	}
	if promoted.DID != gotRecoveredDID {
		t.Fatalf("promoted DID = %q, want %q", promoted.DID, gotRecoveredDID)
	}

	finalList, err := manager.List()
	if err != nil {
		t.Fatalf("List() final error = %v", err)
	}
	if len(finalList) != 2 {
		t.Fatalf("final live identities = %d, want 2", len(finalList))
	}
	if findIdentitySummary(finalList, "zhuocheng-2") != nil {
		t.Fatalf("zhuocheng-2 still present in live index: %#v", finalList)
	}
	if findIdentitySummary(finalList, "lzc") == nil {
		t.Fatalf("lzc missing from final live identities: %#v", finalList)
	}

	for _, dirName := range []string{first.UniqueID, second.UniqueID} {
		if _, err := os.Stat(filepath.Join(resolved.Paths.IdentityDir, dirName)); err != nil {
			t.Fatalf("old identity dir %s missing after finalize: %v", dirName, err)
		}
	}
	if _, err := os.Stat(filepath.Join(backupPath, "backup_manifest.json")); err != nil {
		t.Fatalf("backup manifest missing: %v", err)
	}
	configAfter, _, err := appconfig.ReadFileConfig(resolved.Paths.ConfigFile)
	if err != nil {
		t.Fatalf("ReadFileConfig() after finalize error = %v", err)
	}
	if configAfter.Identity.Active != "zhuocheng" {
		t.Fatalf("config identity.active = %q, want zhuocheng", configAfter.Identity.Active)
	}
}

func assertReplaceDIDBackup(t *testing.T, manager *identity.Manager, backupPath string, legacy *identity.GeneratedIdentity, plannedNewDID string) {
	t.Helper()
	if strings.TrimSpace(backupPath) == "" {
		t.Fatal("backup_path is empty")
	}
	if _, err := os.Stat(filepath.Join(manager.RootDir(), legacy.UniqueID)); !os.IsNotExist(err) {
		t.Fatalf("old live identity dir still exists or stat failed unexpectedly: %v", err)
	}
	identityPayload := readTestJSONMap(t, filepath.Join(backupPath, identity.IdentityFileName))
	if got, _ := identityPayload["did"].(string); got != legacy.DID {
		t.Fatalf("backup identity did = %q, want %q", got, legacy.DID)
	}
	didDocument := readTestJSONMap(t, filepath.Join(backupPath, identity.DIDDocumentFileName))
	if got, _ := didDocument["id"].(string); got != legacy.DID {
		t.Fatalf("backup DID document id = %q, want %q", got, legacy.DID)
	}
	privateKey, err := os.ReadFile(filepath.Join(backupPath, identity.Key1PrivateFileName))
	if err != nil {
		t.Fatalf("ReadFile(backup key-1-private.pem) error = %v", err)
	}
	if string(privateKey) != legacy.Key1PrivatePEM {
		t.Fatalf("backup private key mismatch")
	}
	manifest := readTestJSONMap(t, filepath.Join(backupPath, "backup_manifest.json"))
	if got, _ := manifest["reason"].(string); got != "replace_did" {
		t.Fatalf("manifest reason = %q, want replace_did", got)
	}
	if got, _ := manifest["old_did"].(string); got != legacy.DID {
		t.Fatalf("manifest old_did = %q, want %q", got, legacy.DID)
	}
	if got, _ := manifest["planned_new_did"].(string); got != plannedNewDID {
		t.Fatalf("manifest planned_new_did = %q, want %q", got, plannedNewDID)
	}
}

func readTestJSONMap(t *testing.T, path string) map[string]any {
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

func findIdentitySummary(items []identity.IdentitySummary, identityName string) *identity.IdentitySummary {
	for idx := range items {
		if items[idx].IdentityName == identityName {
			summary := items[idx]
			return &summary
		}
	}
	return nil
}

func newReplaceTestWorkspace(t *testing.T, serviceBaseURL string) (*appconfig.Resolved, *identity.Manager) {
	t.Helper()
	root := t.TempDir()
	resolved := &appconfig.Resolved{
		Paths: appconfig.Paths{
			WorkspaceHomeDir:     filepath.Join(root, ".awiki-cli"),
			ConfigFile:           filepath.Join(root, ".awiki-cli", "config.yaml"),
			ConfigDir:            filepath.Join(root, ".awiki-cli"),
			IdentityDir:          filepath.Join(root, ".awiki-cli", "identities"),
			DataDir:              filepath.Join(root, ".awiki-cli", "data"),
			StateDir:             filepath.Join(root, ".awiki-cli", "runtime"),
			CacheDir:             filepath.Join(root, ".awiki-cli", "cache"),
			DatabaseFile:         filepath.Join(root, ".awiki-cli", "data", "awiki-cli.db"),
			LegacyCredentialsDir: filepath.Join(root, "legacy"),
			LegacyDataDir:        filepath.Join(root, "legacy-data"),
		},
		ServiceBaseURL:     serviceBaseURL,
		DIDDomain:          "awiki.test",
		ANPServiceEndpoint: "https://awiki.test/anp-im/rpc",
		ANPServiceDID:      "did:wba:awiki.test",
		ActiveIdentity:     "alice",
		OutputFormat:       "json",
	}
	return resolved, identity.NewManager(resolved.Paths)
}

func seedReplaceTestStore(t *testing.T, paths appconfig.Paths, ownerDID string) {
	t.Helper()
	db, err := store.Open(paths)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	defer db.Close()
	if err := store.EnsureSchema(context.Background(), db); err != nil {
		t.Fatalf("store.EnsureSchema() error = %v", err)
	}
	peerDID := "did:wba:awiki.test:bob:e1_peer"
	if err := store.StoreMessage(context.Background(), db, store.MessageRecord{
		MsgID:          "msg-1",
		OwnerDID:       ownerDID,
		ThreadID:       store.MakeThreadID(ownerDID, peerDID, ""),
		Direction:      0,
		SenderDID:      peerDID,
		ReceiverDID:    ownerDID,
		ContentType:    "text",
		Content:        "legacy hello",
		CredentialName: "alice",
	}); err != nil {
		t.Fatalf("store.StoreMessage() error = %v", err)
	}
	if _, err := store.QueueE2EEOutbox(context.Background(), db, store.E2EEOutboxRecord{
		OutboxID:       "outbox-1",
		OwnerDID:       ownerDID,
		PeerDID:        peerDID,
		OriginalType:   "text",
		Plaintext:      "secret",
		LocalStatus:    "queued",
		CredentialName: "alice",
	}); err != nil {
		t.Fatalf("store.QueueE2EEOutbox() error = %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
INSERT INTO e2ee_sessions
    (owner_did, peer_did, session_id, is_initiator, send_chain_key, recv_chain_key,
     send_seq, recv_seq, expires_at, created_at, active_at, peer_confirmed, credential_name, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ownerDID,
		peerDID,
		"session-1",
		1,
		"send-key",
		"recv-key",
		1,
		1,
		nil,
		"2026-04-11T00:00:00Z",
		"2026-04-11T00:00:00Z",
		1,
		"alice",
		"2026-04-11T00:00:00Z",
	); err != nil {
		t.Fatalf("ExecContext(e2ee_sessions) error = %v", err)
	}
}

func generateK1IdentityForTest(t *testing.T, hostname string, pathPrefix []string) *identity.GeneratedIdentity {
	t.Helper()
	service, err := identity.BuildAgentANPMessageService("https://"+hostname+"/anp-im/rpc", "did:wba:"+hostname)
	if err != nil {
		t.Fatalf("BuildAgentANPMessageService() error = %v", err)
	}
	bundle, err := anpsdk.CreateDidWBADocument(hostname, anpsdk.DidDocumentOptions{
		PathSegments: pathPrefix,
		Domain:       hostname,
		Challenge:    strings.Repeat("a", 32),
		Services:     []map[string]any{service},
		DidProfile:   anpsdk.DidProfileK1,
	})
	if err != nil {
		t.Fatalf("CreateDidWBADocument() error = %v", err)
	}
	key1, ok := bundle.Keys["key-1"]
	if !ok {
		t.Fatal("generated k1 identity is missing key-1")
	}
	generated := &identity.GeneratedIdentity{
		DID:            testStringValue(bundle.DidDocument["id"]),
		UniqueID:       testDidSuffix(testStringValue(bundle.DidDocument["id"])),
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
	return generated
}

func testStringValue(value any) string {
	text, _ := value.(string)
	return text
}

func testDidSuffix(did string) string {
	lastIndex := strings.LastIndex(did, ":")
	if lastIndex >= 0 && lastIndex < len(did)-1 {
		return did[lastIndex+1:]
	}
	return did
}

func legacyANPPrivatePEMFromStandard(t *testing.T, standardPEM string, label string) string {
	t.Helper()
	privateKey, err := anpsdk.PrivateKeyFromPEM(standardPEM)
	if err != nil {
		t.Fatalf("PrivateKeyFromPEM() fixture error = %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: label, Bytes: privateKey.Bytes}))
}

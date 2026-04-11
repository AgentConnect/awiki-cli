package identity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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

	identityData, _ := result.Data["identity"].(*identity.IdentitySummary)
	if identityData == nil || identityData.DID != gotNewDID {
		t.Fatalf("result identity = %#v, want new did %q", identityData, gotNewDID)
	}
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

func newReplaceTestWorkspace(t *testing.T, serviceBaseURL string) (*appconfig.Resolved, *identity.Manager) {
	t.Helper()
	root := t.TempDir()
	resolved := &appconfig.Resolved{
		Paths: appconfig.Paths{
			WorkspaceHomeDir:     filepath.Join(root, ".awiki-cli"),
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

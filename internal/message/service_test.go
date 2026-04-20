package message

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/store"
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

func TestGroupControlSourceDefaultsToRemoteHTTP(t *testing.T) {
	t.Parallel()

	if got := groupControlSource(nil); got != "remote_http" {
		t.Fatalf("groupControlSource(nil) = %q, want %q", got, "remote_http")
	}
	if got := groupControlSource(map[string]any{"source": "custom"}); got != "custom" {
		t.Fatalf("groupControlSource(custom) = %q, want %q", got, "custom")
	}
}

func TestTransportSourceMatchesActualMode(t *testing.T) {
	t.Parallel()

	if got := transportSource("http"); got != "remote_http" {
		t.Fatalf("transportSource(http) = %q, want %q", got, "remote_http")
	}
	if got := transportSource("websocket"); got != "local_ws_cache" {
		t.Fatalf("transportSource(websocket) = %q, want %q", got, "local_ws_cache")
	}
}

func TestHTTPTransportPersistsAuthenticationInfoTokenFromFirstSignedRequest(t *testing.T) {
	t.Parallel()

	var authHeaders []string
	var signatureInputs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != MessageRPCEndpoint {
			http.NotFound(w, r)
			return
		}
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		signatureInputs = append(signatureInputs, r.Header.Get("Signature-Input"))
		switch len(authHeaders) {
		case 1:
			if authHeaders[0] != "" {
				t.Fatalf("first request Authorization = %q, want signed request without bearer", authHeaders[0])
			}
			if signatureInputs[0] == "" || r.Header.Get("Signature") == "" {
				t.Fatalf("first request missing HTTP signature headers: Signature-Input=%q Signature=%q", signatureInputs[0], r.Header.Get("Signature"))
			}
			w.Header().Set("Authentication-Info", `access_token="fresh-token", token_type="Bearer", expires_in=3600`)
		case 2:
			if authHeaders[1] != "Bearer fresh-token" {
				t.Fatalf("second request Authorization = %q, want Bearer fresh-token", authHeaders[1])
			}
		default:
			t.Fatalf("unexpected request count %d", len(authHeaders))
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"messages":[],"total":0},"id":"req-1"}`))
	}))
	defer server.Close()

	resolved := testResolvedConfig(t)
	resolved.ServiceBaseURL = server.URL
	resolved.ActiveIdentity = "alice"
	manager := identity.NewManager(resolved.Paths)
	createTestIdentity(t, manager, identity.SaveInput{
		IdentityName: "alice",
		UserID:       "user-123",
		DisplayName:  "Alice",
		Handle:       "alice",
	})

	service, err := NewService(resolved)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	record, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	transport, _, err := service.httpTransport(record)
	if err != nil {
		t.Fatalf("httpTransport() error = %v", err)
	}
	if _, err := transport.GetInbox(context.Background(), InboxRequest{Limit: 1}); err != nil {
		t.Fatalf("first GetInbox() error = %v", err)
	}
	stored, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("Load(after first request) error = %v", err)
	}
	if stored.JWTToken != "fresh-token" {
		t.Fatalf("stored JWTToken = %q, want fresh-token", stored.JWTToken)
	}

	transport, _, err = service.httpTransport(stored)
	if err != nil {
		t.Fatalf("httpTransport(after refresh) error = %v", err)
	}
	if _, err := transport.GetInbox(context.Background(), InboxRequest{Limit: 1}); err != nil {
		t.Fatalf("second GetInbox() error = %v", err)
	}
	if len(authHeaders) != 2 {
		t.Fatalf("request count = %d, want 2", len(authHeaders))
	}
}

func TestHTTPTransportRefreshesExpiredBearerAfterHTTP401(t *testing.T) {
	t.Parallel()

	var authHeaders []string
	var signatureInputs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != MessageRPCEndpoint {
			http.NotFound(w, r)
			return
		}
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		signatureInputs = append(signatureInputs, r.Header.Get("Signature-Input"))
		switch len(authHeaders) {
		case 1:
			if authHeaders[0] != "Bearer expired-token" {
				t.Fatalf("first request Authorization = %q, want Bearer expired-token", authHeaders[0])
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"expired"}`))
			return
		case 2:
			if authHeaders[1] != "" {
				t.Fatalf("retry request Authorization = %q, want signed request without bearer", authHeaders[1])
			}
			if signatureInputs[1] == "" || r.Header.Get("Signature") == "" {
				t.Fatalf("retry request missing HTTP signature headers: Signature-Input=%q Signature=%q", signatureInputs[1], r.Header.Get("Signature"))
			}
			w.Header().Set("Authentication-Info", `access_token="refreshed-token", token_type="Bearer", expires_in=3600`)
		case 3:
			if authHeaders[2] != "Bearer refreshed-token" {
				t.Fatalf("third request Authorization = %q, want Bearer refreshed-token", authHeaders[2])
			}
		default:
			t.Fatalf("unexpected request count %d", len(authHeaders))
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"messages":[],"total":0},"id":"req-1"}`))
	}))
	defer server.Close()

	resolved := testResolvedConfig(t)
	resolved.ServiceBaseURL = server.URL
	resolved.ActiveIdentity = "alice"
	manager := identity.NewManager(resolved.Paths)
	createTestIdentity(t, manager, identity.SaveInput{
		IdentityName: "alice",
		UserID:       "user-123",
		DisplayName:  "Alice",
		Handle:       "alice",
		JWTToken:     "expired-token",
	})

	service, err := NewService(resolved)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	record, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	transport, _, err := service.httpTransport(record)
	if err != nil {
		t.Fatalf("httpTransport() error = %v", err)
	}
	if _, err := transport.GetInbox(context.Background(), InboxRequest{Limit: 1}); err != nil {
		t.Fatalf("GetInbox() error = %v", err)
	}
	stored, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("Load(after refresh) error = %v", err)
	}
	if stored.JWTToken != "refreshed-token" {
		t.Fatalf("stored JWTToken = %q, want refreshed-token", stored.JWTToken)
	}

	transport, _, err = service.httpTransport(stored)
	if err != nil {
		t.Fatalf("httpTransport(after refresh) error = %v", err)
	}
	if _, err := transport.GetInbox(context.Background(), InboxRequest{Limit: 1}); err != nil {
		t.Fatalf("second GetInbox() error = %v", err)
	}
	if len(authHeaders) != 3 {
		t.Fatalf("request count = %d, want 3", len(authHeaders))
	}
}

func TestSyncPeerHandleRebindsCurrentContactAndPreservesHistory(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user-service/handle/rpc" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"handle":"bob","did":"did:peer-new","domain":"awiki.ai","status":"active","full_handle":"bob.awiki.ai"},"id":"req-1"}`))
	}))
	defer server.Close()

	resolved := testResolvedConfig(t)
	resolved.ServiceBaseURL = server.URL
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

	db, err := store.Open(resolved.Paths)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	defer db.Close()
	if err := store.EnsureSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	if err := store.UpsertContact(context.Background(), db, store.ContactRecord{
		OwnerDID:       record.DID,
		DID:            "did:peer-old",
		Handle:         "bob",
		CredentialName: record.IdentityName,
	}); err != nil {
		t.Fatalf("UpsertContact(old) error = %v", err)
	}

	handle, err := service.syncPeerHandle(context.Background(), db, record.DID, "did:peer-new", "", "msg.inbox", "")
	if err != nil {
		t.Fatalf("syncPeerHandle() error = %v", err)
	}
	if handle != "bob" {
		t.Fatalf("syncPeerHandle() handle = %q, want bob", handle)
	}
	current, err := store.GetCurrentContactByHandle(context.Background(), db, record.DID, "bob")
	if err != nil {
		t.Fatalf("GetCurrentContactByHandle() error = %v", err)
	}
	if current["did"] != "did:peer-new" {
		t.Fatalf("current contact did = %#v, want did:peer-new", current["did"])
	}
	dids, err := store.ListDIDsByHandle(context.Background(), db, record.DID, "bob")
	if err != nil {
		t.Fatalf("ListDIDsByHandle() error = %v", err)
	}
	if len(dids) != 2 {
		t.Fatalf("len(dids) = %d, want 2", len(dids))
	}
}

func TestReadHistoryFromCacheByPeerDIDsAggregatesHistoricalBindings(t *testing.T) {
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

	db, err := store.Open(resolved.Paths)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	defer db.Close()
	if err := store.EnsureSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	if err := store.UpsertContact(context.Background(), db, store.ContactRecord{
		OwnerDID:       record.DID,
		DID:            "did:peer-old",
		Handle:         "bob",
		CredentialName: record.IdentityName,
	}); err != nil {
		t.Fatalf("UpsertContact(old) error = %v", err)
	}
	if err := store.UpsertContact(context.Background(), db, store.ContactRecord{
		OwnerDID:       record.DID,
		DID:            "did:peer-new",
		Handle:         "bob",
		CredentialName: record.IdentityName,
	}); err != nil {
		t.Fatalf("UpsertContact(new) error = %v", err)
	}
	if err := store.StoreMessage(context.Background(), db, store.MessageRecord{
		MsgID:          "msg-old",
		OwnerDID:       record.DID,
		ThreadID:       store.MakeThreadID(record.DID, "did:peer-old", ""),
		Direction:      0,
		SenderDID:      "did:peer-old",
		ReceiverDID:    record.DID,
		ContentType:    "text/plain",
		Content:        "hello from old",
		IsRead:         false,
		CredentialName: record.IdentityName,
	}); err != nil {
		t.Fatalf("StoreMessage(old) error = %v", err)
	}
	if err := store.StoreMessage(context.Background(), db, store.MessageRecord{
		MsgID:          "msg-new",
		OwnerDID:       record.DID,
		ThreadID:       store.MakeThreadID(record.DID, "did:peer-new", ""),
		Direction:      0,
		SenderDID:      "did:peer-new",
		ReceiverDID:    record.DID,
		ContentType:    "text/plain",
		Content:        "hello from new",
		IsRead:         false,
		CredentialName: record.IdentityName,
	}); err != nil {
		t.Fatalf("StoreMessage(new) error = %v", err)
	}

	peerDIDs, err := service.peerDIDsForHandleFromStore(context.Background(), record.DID, "bob", "did:peer-new")
	if err != nil {
		t.Fatalf("peerDIDsForHandleFromStore() error = %v", err)
	}
	if len(peerDIDs) != 2 {
		t.Fatalf("len(peerDIDs) = %d, want 2", len(peerDIDs))
	}
	rows, err := service.readHistoryFromCacheByPeerDIDs(context.Background(), record, peerDIDs, 10)
	if err != nil {
		t.Fatalf("readHistoryFromCacheByPeerDIDs() error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
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

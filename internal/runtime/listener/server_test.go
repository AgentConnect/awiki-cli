package listener

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/store"
	"github.com/coder/websocket"
)

func TestMessageRecordFromDirectIncomingUsesProtocolFieldsOnly(t *testing.T) {
	t.Parallel()

	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  "direct.incoming",
		"params": map[string]any{
			"meta": map[string]any{
				"sender_did":   "did:wba:example.com:user:bob:e1_yyy",
				"message_id":   "msg-001",
				"created_at":   "2026-04-07T00:00:00Z",
				"content_type": "text/plain",
				"target": map[string]any{
					"kind": "agent",
					"did":  "did:wba:example.com:user:alice:e1_xxx",
				},
			},
			"auth": map[string]any{
				"scheme": "anp-rfc9421-origin-proof-v1",
				"origin_proof": map[string]any{
					"contentDigest":  "sha-256=:digest:",
					"signatureInput": "sig1=(\"@method\");created=1;keyid=\"did:wba:example.com:user:bob:e1_yyy#key-1\"",
					"signature":      "sig1=:signature:",
				},
			},
			"body": map[string]any{
				"text": "hello back",
			},
		},
	}

	record, ok := messageRecordFromDirectIncoming(notification, "alice")
	if !ok {
		t.Fatalf("messageRecordFromDirectIncoming() ok = false, want true")
	}
	if record.OwnerDID != "did:wba:example.com:user:alice:e1_xxx" {
		t.Fatalf("record.OwnerDID = %q", record.OwnerDID)
	}
	if record.SenderDID != "did:wba:example.com:user:bob:e1_yyy" {
		t.Fatalf("record.SenderDID = %q", record.SenderDID)
	}
	if record.SentAt != "2026-04-07T00:00:00Z" {
		t.Fatalf("record.SentAt = %q", record.SentAt)
	}
	if !strings.Contains(record.Metadata, "anp-rfc9421-origin-proof-v1") {
		t.Fatalf("record.Metadata = %q, want auth payload", record.Metadata)
	}
	if strings.Contains(record.Metadata, "\"server\"") {
		t.Fatalf("record.Metadata = %q, should not depend on server wrapper", record.Metadata)
	}
}

func TestMessageRecordFromDirectIncomingRejectsNonDirectNotification(t *testing.T) {
	t.Parallel()

	_, ok := messageRecordFromDirectIncoming(map[string]any{"method": "group.incoming"}, "alice")
	if ok {
		t.Fatalf("messageRecordFromDirectIncoming() ok = true, want false")
	}
}

func TestMessageRecordFromMailNotificationBuildsSystemMessage(t *testing.T) {
	t.Parallel()

	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  "mail.notification",
		"params": map[string]any{
			"mailbox_did":     "did:wba:example.com:user:alice:e1_alice",
			"mailbox_address": "alice@example.com",
			"from_addr":       "sender@example.com",
			"subject":         "Mail Subject",
			"preview":         "First 200 chars of the body...",
			"has_attachments": true,
			"message_id":      "mail-msg-001",
		},
	}

	record, ok := messageRecordFromMailNotification(notification, "alice")
	if !ok {
		t.Fatalf("messageRecordFromMailNotification() ok = false, want true")
	}
	if record.OwnerDID != "did:wba:example.com:user:alice:e1_alice" {
		t.Fatalf("record.OwnerDID = %q", record.OwnerDID)
	}
	if record.ThreadID != "mail:alice@example.com" {
		t.Fatalf("record.ThreadID = %q", record.ThreadID)
	}
	if record.Direction != 0 {
		t.Fatalf("record.Direction = %d, want 0 (inbound)", record.Direction)
	}
	if record.ContentType != "mail.notification" {
		t.Fatalf("record.ContentType = %q", record.ContentType)
	}
	if record.Title != "Mail Subject" {
		t.Fatalf("record.Title = %q", record.Title)
	}
	if !strings.Contains(record.Content, "alice@example.com") || !strings.Contains(record.Content, "Mail Subject") {
		t.Fatalf("record.Content = %q, want summary with mailbox and subject", record.Content)
	}
	if record.CredentialName != "alice" {
		t.Fatalf("record.CredentialName = %q", record.CredentialName)
	}
}

func TestMessageRecordFromGroupIncomingUsesProtocolFieldsOnly(t *testing.T) {
	t.Parallel()

	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  "group.incoming",
		"params": map[string]any{
			"meta": map[string]any{
				"sender_did":   "did:wba:example.com:user:bob:e1_bob",
				"message_id":   "msg-group-001",
				"content_type": "text/plain",
				"target": map[string]any{
					"kind": "agent",
					"did":  "did:wba:example.com:user:alice:e1_alice",
				},
			},
			"body": map[string]any{
				"text":            "hello group",
				"group_did":       "did:wba:example.com:groups:demo:e1_group",
				"group_event_seq": "5",
				"accepted_at":     "2026-04-07T09:11:01Z",
			},
		},
	}

	record, ok := messageRecordFromGroupIncoming(notification, "alice")
	if !ok {
		t.Fatalf("messageRecordFromGroupIncoming() ok = false, want true")
	}
	if record.GroupDID != "did:wba:example.com:groups:demo:e1_group" {
		t.Fatalf("record.GroupDID = %q", record.GroupDID)
	}
	if record.ThreadID != "group:did:wba:example.com:groups:demo:e1_group" {
		t.Fatalf("record.ThreadID = %q", record.ThreadID)
	}
	if record.Content != "hello group" {
		t.Fatalf("record.Content = %q", record.Content)
	}
}

func TestRecordsFromGroupStateChangedBuildsMemberAndSystemMessage(t *testing.T) {
	t.Parallel()

	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  "group.state_changed",
		"params": map[string]any{
			"meta": map[string]any{
				"target": map[string]any{
					"kind": "agent",
					"did":  "did:wba:example.com:user:alice:e1_alice",
				},
			},
			"body": map[string]any{
				"event_id":          "evt-3",
				"group_did":         "did:wba:example.com:groups:demo:e1_group",
				"group_event_seq":   "3",
				"subject_method":    "group.remove",
				"subject_did":       "did:wba:example.com:user:carol:e1_carol",
				"actor_did":         "did:wba:example.com:user:alice:e1_alice",
				"membership_status": "removed",
				"changed_at":        "2026-04-07T09:06:01Z",
			},
		},
	}

	groupRecord, memberRecord, messageRecord, ok := recordsFromGroupStateChanged(notification, "alice")
	if !ok {
		t.Fatalf("recordsFromGroupStateChanged() ok = false, want true")
	}
	if groupRecord == nil || groupRecord.GroupDID != "did:wba:example.com:groups:demo:e1_group" {
		t.Fatalf("groupRecord = %#v", groupRecord)
	}
	if memberRecord == nil || memberRecord.Status != "removed" {
		t.Fatalf("memberRecord = %#v", memberRecord)
	}
	if messageRecord == nil || messageRecord.ContentType != "group_system_member_kicked" {
		t.Fatalf("messageRecord = %#v", messageRecord)
	}
}

func TestSessionLoopReconnectsAndStoresNotifications(t *testing.T) {
	t.Parallel()

	var connectionCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/im/ws" {
			http.NotFound(w, r)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("websocket.Accept() error = %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")

		index := connectionCount.Add(1)
		payload := map[string]any{
			"jsonrpc": "2.0",
			"method":  "direct.incoming",
			"params": map[string]any{
				"meta": map[string]any{
					"sender_did":   "did:wba:example.com:user:bob:e1_bob",
					"message_id":   "msg-" + string(rune('0'+index)),
					"created_at":   "2026-04-07T00:00:00Z",
					"content_type": "text/plain",
					"target": map[string]any{
						"kind": "agent",
						"did":  "did:wba:awiki.ai:user:alice:e1_alice",
					},
				},
				"body": map[string]any{
					"text": "hello-" + string(rune('0'+index)),
				},
			},
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Errorf("json.Marshal() error = %v", err)
			return
		}
		writeCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := conn.Write(writeCtx, websocket.MessageText, raw); err != nil {
			t.Errorf("conn.Write() error = %v", err)
			return
		}
		if index == 1 {
			return
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	resolved := testResolvedConfig(t, server.URL)
	supervisor, err := NewSupervisor(resolved)
	if err != nil {
		t.Fatalf("NewSupervisor() error = %v", err)
	}
	defer supervisor.Close()

	session, err := supervisor.ensureSession("alice")
	if err != nil {
		t.Fatalf("ensureSession() error = %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if connectionCount.Load() >= 2 && session.currentClient() != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf(
				"reconnect state not ready: connectionCount=%d currentClientNil=%t",
				connectionCount.Load(),
				session.currentClient() == nil,
			)
		}
		time.Sleep(100 * time.Millisecond)
	}

	db, err := store.Open(resolved.Paths)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	defer db.Close()
	deadline = time.Now().Add(10 * time.Second)
	for {
		var count int
		row := db.QueryRow(`SELECT COUNT(*) FROM messages WHERE owner_did = ?`, "did:wba:awiki.ai:user:alice:e1_alice")
		if scanErr := row.Scan(&count); scanErr != nil {
			t.Fatalf("Scan() error = %v", scanErr)
		}
		if count >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stored message count = %d, want at least 2", count)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestNewSupervisorMarksInstalledWhenRunningAsService(t *testing.T) {
	t.Setenv(listenerServiceModeEnv, "1")

	resolved := testResolvedConfig(t, "https://awiki.test")
	supervisor, err := NewSupervisor(resolved)
	if err != nil {
		t.Fatalf("NewSupervisor() error = %v", err)
	}
	defer supervisor.Close()

	if !supervisor.status.Installed {
		t.Fatal("supervisor.status.Installed = false, want true in service mode")
	}
}

func TestStartSocketPersistsBridgeAvailability(t *testing.T) {
	t.Parallel()

	resolved := testResolvedConfig(t, "https://awiki.test")
	supervisor, err := NewSupervisor(resolved)
	if err != nil {
		t.Fatalf("NewSupervisor() error = %v", err)
	}
	defer supervisor.Close()
	defer cleanupRuntimeArtifacts(resolved)

	if err := supervisor.startSocket(); err != nil {
		t.Fatalf("startSocket() error = %v", err)
	}

	status, err := readStatus(supervisor.status.StatusFile)
	if err != nil {
		t.Fatalf("readStatus() error = %v", err)
	}
	if !status.BridgeAvailable {
		t.Fatalf("status.BridgeAvailable = false, want true")
	}
}

func testResolvedConfig(t *testing.T, messageServiceURL string) *appconfig.Resolved {
	t.Helper()

	root := t.TempDir()
	manager := identity.NewManager(appconfig.Paths{
		WorkspaceHomeDir:     filepath.Join(root, ".awiki-cli"),
		IdentityDir:          filepath.Join(root, "identities"),
		LegacyCredentialsDir: filepath.Join(root, "legacy"),
		DataDir:              filepath.Join(root, "data"),
		StateDir:             filepath.Join(root, "state"),
		DatabaseFile:         filepath.Join(root, "data", "awiki-cli.db"),
	})
	createTestIdentity(t, manager, identity.SaveInput{
		IdentityName: "alice",
		DisplayName:  "Alice",
		Handle:       "alice",
		UserID:       "user-alice-123",
		JWTToken:     "token-123",
	})
	return &appconfig.Resolved{
		Paths: appconfig.Paths{
			WorkspaceHomeDir:     filepath.Join(root, ".awiki-cli"),
			IdentityDir:          filepath.Join(root, "identities"),
			LegacyCredentialsDir: filepath.Join(root, "legacy"),
			DataDir:              filepath.Join(root, "data"),
			StateDir:             filepath.Join(root, "state"),
			DatabaseFile:         filepath.Join(root, "data", "awiki-cli.db"),
		},
		ServiceBaseURL: messageServiceURL,
		DIDDomain:      "awiki.ai",
		RuntimeMode:    "websocket",
		ActiveIdentity: "alice",
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
	input.DID = "did:wba:awiki.ai:user:alice:e1_alice"
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

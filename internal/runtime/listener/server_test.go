package listener

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/store"
	"github.com/agentconnect/awiki-cli/internal/upgrader"
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
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		if connectionCount.Load() >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("connectionCount = %d, want at least 2", connectionCount.Load())
		}
		time.Sleep(100 * time.Millisecond)
	}
	if session.currentClient() == nil {
		t.Fatalf("session.currentClient() = nil, want active client after reconnect")
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

func TestUpgradeNotificationApplyInstallsManagedBinary(t *testing.T) {
	t.Parallel()

	originalVersion := buildinfo.Version
	defer func() { buildinfo.Version = originalVersion }()
	buildinfo.Version = "1.8.0"

	archivePath := filepath.Join(t.TempDir(), "awiki-cli.tar.gz")
	if err := writeServerTestTarGZ(archivePath, filepath.Join("release", "awiki-cli"), []byte("#!/bin/sh\necho upgraded\n")); err != nil {
		t.Fatalf("writeServerTestTarGZ() error = %v", err)
	}
	archiveSHA := checksumFile(t, archivePath)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/im/ws":
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				t.Errorf("websocket.Accept() error = %v", err)
				return
			}
			defer conn.Close(websocket.StatusNormalClosure, "done")
			payload := map[string]any{
				"jsonrpc": "2.0",
				"method":  upgradeMethodAvailable,
				"params": map[string]any{
					"latest_version":        "1.8.1",
					"min_supported_version": "1.8.0",
					"channel":               "stable",
					"artifact": map[string]any{
						"url":    server.URL + "/artifacts/awiki-cli.tar.gz",
						"sha256": archiveSHA,
					},
					"skill_bundle": map[string]any{
						"bundle_version":    "2026.04.17",
						"bundle_sha256":     "bundle-sha",
						"root_skill_sha256": "root-sha",
					},
				},
			}
			raw, _ := json.Marshal(payload)
			if err := conn.Write(r.Context(), websocket.MessageText, raw); err != nil {
				t.Errorf("conn.Write() error = %v", err)
			}
		case "/artifacts/awiki-cli.tar.gz":
			http.ServeFile(w, r, archivePath)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolved := testResolvedConfig(t, server.URL)
	resolved.UpdateAutoUpgradeEnabled = true
	resolved.UpdateAutoUpgradeMode = "apply"
	resolved.UpdateAllowWSPushTrigger = true

	supervisor, err := NewSupervisor(resolved)
	if err != nil {
		t.Fatalf("NewSupervisor() error = %v", err)
	}
	defer supervisor.Close()

	if _, err := supervisor.ensureSession("alice"); err != nil {
		t.Fatalf("ensureSession() error = %v", err)
	}
	manager, err := upgrader.NewManager(resolved)
	if err != nil {
		t.Fatalf("upgrader.NewManager() error = %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		state, loadErr := manager.LoadReleaseState()
		if loadErr != nil {
			t.Fatalf("LoadReleaseState() error = %v", loadErr)
		}
		if state != nil && state.CurrentVersion == "1.8.1" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("release state did not record applied upgrade before timeout")
		}
		time.Sleep(100 * time.Millisecond)
	}
	target := upgrader.ResolveCurrentTarget(manager.Paths().CurrentBinaryPath)
	if !strings.HasSuffix(target, filepath.Join("versions", "1.8.1", "awiki-cli")) {
		t.Fatalf("managed binary target = %q, want versions/1.8.1/awiki-cli suffix", target)
	}
}

func TestUpgradeNotificationNotifyOnlyRecordsPendingUpdate(t *testing.T) {
	t.Parallel()

	originalVersion := buildinfo.Version
	defer func() { buildinfo.Version = originalVersion }()
	buildinfo.Version = "1.8.0"

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
		payload := map[string]any{
			"jsonrpc": "2.0",
			"method":  upgradeMethodAvailable,
			"params": map[string]any{
				"latest_version":        "1.8.1",
				"min_supported_version": "1.8.0",
				"channel":               "stable",
				"artifact": map[string]any{
					"url":    "https://downloads.example.com/awiki-cli.tar.gz",
					"sha256": "abc123",
				},
			},
		}
		raw, _ := json.Marshal(payload)
		if err := conn.Write(r.Context(), websocket.MessageText, raw); err != nil {
			t.Errorf("conn.Write() error = %v", err)
		}
	}))
	defer server.Close()

	resolved := testResolvedConfig(t, server.URL)
	resolved.UpdateAutoUpgradeEnabled = false
	resolved.UpdateAutoUpgradeMode = "notify"
	resolved.UpdateAllowWSPushTrigger = true

	supervisor, err := NewSupervisor(resolved)
	if err != nil {
		t.Fatalf("NewSupervisor() error = %v", err)
	}
	defer supervisor.Close()

	if _, err := supervisor.ensureSession("alice"); err != nil {
		t.Fatalf("ensureSession() error = %v", err)
	}
	manager, err := upgrader.NewManager(resolved)
	if err != nil {
		t.Fatalf("upgrader.NewManager() error = %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		state, loadErr := manager.LoadReleaseState()
		if loadErr != nil {
			t.Fatalf("LoadReleaseState() error = %v", loadErr)
		}
		if state != nil && state.PendingUpdate != nil && state.LastWSUpgradeEventAt != "" {
			if state.CurrentVersion != "" {
				t.Fatalf("state.CurrentVersion = %q, want empty string in notify mode", state.CurrentVersion)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("release state did not record pending update before timeout")
		}
		time.Sleep(100 * time.Millisecond)
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

func writeServerTestTarGZ(archivePath string, binaryPath string, content []byte) error {
	file, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()
	header := &tar.Header{
		Name: binaryPath,
		Mode: 0o755,
		Size: int64(len(content)),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}
	_, err = tarWriter.Write(content)
	return err
}

func checksumFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

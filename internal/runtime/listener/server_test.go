package listener

import (
	"context"
	"crypto/ecdh"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	anp "github.com/agent-network-protocol/anp/golang"
	directe2ee "github.com/agent-network-protocol/anp/golang/direct_e2ee"
	"github.com/agentconnect/awiki-cli/internal/anpsdk"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/message"
	"github.com/agentconnect/awiki-cli/internal/runtime"
	"github.com/agentconnect/awiki-cli/internal/store"
	"github.com/agentconnect/awiki-cli/internal/testenv"
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
	if record.ContentType != "text/plain" {
		t.Fatalf("record.ContentType = %q", record.ContentType)
	}
	if !strings.Contains(record.Metadata, `"source_kind":"mail"`) {
		t.Fatalf("record.Metadata = %q, want source_kind marker", record.Metadata)
	}
	if record.Title != "[邮件] Mail Subject" {
		t.Fatalf("record.Title = %q", record.Title)
	}
	if !strings.Contains(record.Content, "[邮件] 收件邮箱: alice@example.com") || !strings.Contains(record.Content, "主题: Mail Subject") {
		t.Fatalf("record.Content = %q, want mail summary with mailbox and subject", record.Content)
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

func TestHandleNotificationDecryptsSecureDirectIncomingAndStoresPlaintext(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := appconfig.Paths{
		WorkspaceHomeDir:     filepath.Join(root, ".awiki-cli"),
		IdentityDir:          filepath.Join(root, "identities"),
		LegacyCredentialsDir: filepath.Join(root, "legacy"),
		DataDir:              filepath.Join(root, "data"),
		StateDir:             filepath.Join(root, "state"),
		DatabaseFile:         filepath.Join(root, "data", "awiki-cli.db"),
	}
	manager := identity.NewManager(paths)
	aliceGenerated, err := identity.GenerateIdentity(identity.GenerateOptions{
		Hostname:    "awiki.ai",
		PathPrefix:  []string{"user"},
		ProofDomain: "awiki.ai",
	})
	if err != nil {
		t.Fatalf("GenerateIdentity(alice) error = %v", err)
	}
	if _, err := manager.Save(identity.SaveInput{
		IdentityName:            "alice",
		DID:                     aliceGenerated.DID,
		UniqueID:                aliceGenerated.UniqueID,
		UserID:                  "user-alice-123",
		DisplayName:             "Alice",
		Handle:                  "alice",
		JWTToken:                "token-123",
		DIDDocument:             aliceGenerated.DIDDocument,
		Key1PrivatePEM:          aliceGenerated.Key1PrivatePEM,
		Key1PublicPEM:           aliceGenerated.Key1PublicPEM,
		E2EESigningPrivatePEM:   aliceGenerated.E2EESigningPrivatePEM,
		E2EEAgreementPrivatePEM: aliceGenerated.E2EEAgreementPrivatePEM,
	}); err != nil {
		t.Fatalf("manager.Save(alice) error = %v", err)
	}
	resolved := &appconfig.Resolved{
		Paths:          paths,
		ServiceBaseURL: "https://awiki.test",
		DIDDomain:      "awiki.ai",
		RuntimeMode:    "websocket",
		ActiveIdentity: "alice",
	}
	supervisor, err := NewSupervisor(resolved)
	if err != nil {
		t.Fatalf("NewSupervisor() error = %v", err)
	}
	defer supervisor.Close()

	record, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("manager.Load(alice) error = %v", err)
	}
	aliceClient, err := message.NewSecureE2EEClientForRecord(context.Background(), manager, record, func(string, map[string]any) (map[string]any, error) {
		return map[string]any{}, nil
	})
	if err != nil {
		t.Fatalf("NewSecureE2EEClientForRecord(alice) error = %v", err)
	}
	aliceBundle, err := aliceClient.EnsureFreshPrekeyBundle()
	if err != nil {
		t.Fatalf("EnsureFreshPrekeyBundle(alice) error = %v", err)
	}
	alicePaths, err := manager.PathsForIdentity("alice")
	if err != nil {
		t.Fatalf("PathsForIdentity(alice) error = %v", err)
	}
	aliceOPKStore, err := anpsdk.NewFileOneTimePrekeyStore(filepath.Join(alicePaths.IdentityDir, "p5-one-time-prekeys"))
	if err != nil {
		t.Fatalf("NewFileOneTimePrekeyStore(alice) error = %v", err)
	}
	aliceOPKs, err := aliceOPKStore.ListOneTimePrekeys()
	if err != nil {
		t.Fatalf("ListOneTimePrekeys(alice) error = %v", err)
	}
	if len(aliceOPKs) == 0 {
		t.Fatal("expected at least one Alice OPK")
	}

	bobGenerated, err := identity.GenerateIdentity(identity.GenerateOptions{
		Hostname:    "awiki.ai",
		PathPrefix:  []string{"user"},
		ProofDomain: "awiki.ai",
	})
	if err != nil {
		t.Fatalf("GenerateIdentity(bob) error = %v", err)
	}
	if _, err := manager.Save(identity.SaveInput{
		IdentityName:            "bob",
		DID:                     bobGenerated.DID,
		UniqueID:                bobGenerated.UniqueID,
		UserID:                  "user-bob-123",
		DisplayName:             "Bob",
		Handle:                  "bob",
		JWTToken:                "token-bob",
		DIDDocument:             bobGenerated.DIDDocument,
		Key1PrivatePEM:          bobGenerated.Key1PrivatePEM,
		Key1PublicPEM:           bobGenerated.Key1PublicPEM,
		E2EESigningPrivatePEM:   bobGenerated.E2EESigningPrivatePEM,
		E2EEAgreementPrivatePEM: bobGenerated.E2EEAgreementPrivatePEM,
	}); err != nil {
		t.Fatalf("manager.Save(bob) error = %v", err)
	}
	bobPrivate, err := privateKeyFromPEM(t, bobGenerated.E2EEAgreementPrivatePEM)
	if err != nil {
		t.Fatalf("privateKeyFromPEM(bob) error = %v", err)
	}
	recipientStaticPublic, err := directe2ee.ExtractX25519PublicKey(record.DIDDocument, aliceBundle.StaticKeyAgreementID)
	if err != nil {
		t.Fatalf("ExtractX25519PublicKey(alice) error = %v", err)
	}
	recipientSignedPrekey := decodeFixed32Value(t, aliceBundle.SignedPrekey.PublicKeyB64U)
	recipientOneTimePrekey := decodeFixed32Value(t, aliceOPKs[0].PublicKeyB64U)
	metadata := directe2ee.DirectEnvelopeMetadata{
		SenderDID:       bobGenerated.DID,
		RecipientDID:    record.DID,
		MessageID:       "msg-secure-init-001",
		Profile:         "anp.direct.e2ee.v1",
		SecurityProfile: "direct-e2ee",
	}
	sessionBuilder := directe2ee.DirectE2eeSession{}
	_, _, initBody, err := sessionBuilder.InitiateSessionWithOPK(
		metadata,
		"msg-secure-init-001",
		bobGenerated.DID+"#key-3",
		bobPrivate,
		aliceBundle,
		recipientStaticPublic,
		recipientSignedPrekey,
		&recipientOneTimePrekey,
		aliceOPKs[0].KeyID,
		directe2ee.NewTextPlaintext("text/plain", "hello secure listener"),
	)
	if err != nil {
		t.Fatalf("InitiateSessionWithOPK() error = %v", err)
	}
	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  "direct.incoming",
		"params": map[string]any{
			"meta": map[string]any{
				"sender_did":       bobGenerated.DID,
				"message_id":       metadata.MessageID,
				"created_at":       "2026-04-07T00:00:00Z",
				"profile":          metadata.Profile,
				"security_profile": metadata.SecurityProfile,
				"content_type":     "application/anp-direct-init+json",
				"target": map[string]any{
					"kind": "agent",
					"did":  record.DID,
				},
			},
			"body": mustJSONMap(t, initBody),
		},
	}

	var ackSent atomic.Int32
	supervisor.handleNotification(context.Background(), &session{
		identityName: "alice",
		record:       record,
		secureRPCCall: func(_ context.Context, method string, params map[string]any) (map[string]any, error) {
			if method != "direct.send" {
				return nil, fmt.Errorf("unexpected rpc method: %s", method)
			}
			ackSent.Add(1)
			meta := params["meta"].(map[string]any)
			return map[string]any{
				"accepted":     true,
				"message_id":   meta["message_id"],
				"operation_id": meta["operation_id"],
				"target_did":   meta["target"].(map[string]any)["did"],
				"accepted_at":  "2026-04-07T00:00:01Z",
			}, nil
		},
	}, notification)

	row, err := store.GetMessageByID(context.Background(), supervisor.db, metadata.MessageID, record.DID, record.IdentityName)
	if err != nil {
		t.Fatalf("GetMessageByID() error = %v", err)
	}
	if got := row["content"]; got != "hello secure listener" {
		t.Fatalf("stored content = %#v, want plaintext", got)
	}
	if got := row["content_type"]; got != "text/plain" {
		t.Fatalf("stored content_type = %#v, want text/plain", got)
	}
	if got := row["is_e2ee"]; got != int64(1) {
		t.Fatalf("stored is_e2ee = %#v, want 1", got)
	}
	sessionEntries, err := os.ReadDir(filepath.Join(alicePaths.IdentityDir, "p5-e2ee-sessions"))
	if err != nil {
		t.Fatalf("ReadDir(session store) error = %v", err)
	}
	if len(sessionEntries) != 1 {
		t.Fatalf("len(sessionEntries) = %d, want 1", len(sessionEntries))
	}
	sessionBytes, err := os.ReadFile(filepath.Join(alicePaths.IdentityDir, "p5-e2ee-sessions", sessionEntries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile(session) error = %v", err)
	}
	var savedSession map[string]any
	if err := json.Unmarshal(sessionBytes, &savedSession); err != nil {
		t.Fatalf("json.Unmarshal(session) error = %v", err)
	}
	if got := savedSession["status"]; got != "established" {
		t.Fatalf("saved session status = %#v, want established", got)
	}
	if _, _, err := aliceOPKStore.LoadOneTimePrekey(aliceOPKs[0].KeyID); err == nil {
		t.Fatalf("one-time prekey %s should be consumed by secure listener decrypt", aliceOPKs[0].KeyID)
	}
	if ackSent.Load() != 1 {
		t.Fatalf("ackSent = %d, want 1 auto secure ack", ackSent.Load())
	}
}

func TestDeliverLocalSecureAckInProcessPromotesPendingInitiatorSession(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := appconfig.Paths{
		WorkspaceHomeDir:     filepath.Join(root, ".awiki-cli"),
		IdentityDir:          filepath.Join(root, "identities"),
		LegacyCredentialsDir: filepath.Join(root, "legacy"),
		DataDir:              filepath.Join(root, "data"),
		StateDir:             filepath.Join(root, "state"),
		DatabaseFile:         filepath.Join(root, "data", "awiki-cli.db"),
	}
	manager := identity.NewManager(paths)
	resolved := &appconfig.Resolved{
		Paths:          paths,
		ServiceBaseURL: "https://awiki.test",
		DIDDomain:      "awiki.ai",
		RuntimeMode:    "websocket",
		ActiveIdentity: "alice",
	}
	supervisor, err := NewSupervisor(resolved)
	if err != nil {
		t.Fatalf("NewSupervisor() error = %v", err)
	}
	defer supervisor.Close()

	aliceGenerated, err := identity.GenerateIdentity(identity.GenerateOptions{Hostname: "awiki.ai", PathPrefix: []string{"user"}, ProofDomain: "awiki.ai"})
	if err != nil {
		t.Fatalf("GenerateIdentity(alice) error = %v", err)
	}
	if _, err := manager.Save(identity.SaveInput{
		IdentityName:            "alice",
		DID:                     aliceGenerated.DID,
		UniqueID:                aliceGenerated.UniqueID,
		UserID:                  "user-alice",
		DisplayName:             "Alice",
		Handle:                  "alice",
		JWTToken:                "token-alice",
		DIDDocument:             aliceGenerated.DIDDocument,
		Key1PrivatePEM:          aliceGenerated.Key1PrivatePEM,
		Key1PublicPEM:           aliceGenerated.Key1PublicPEM,
		E2EESigningPrivatePEM:   aliceGenerated.E2EESigningPrivatePEM,
		E2EEAgreementPrivatePEM: aliceGenerated.E2EEAgreementPrivatePEM,
	}); err != nil {
		t.Fatalf("manager.Save(alice) error = %v", err)
	}
	bobGenerated, err := identity.GenerateIdentity(identity.GenerateOptions{Hostname: "awiki.ai", PathPrefix: []string{"user"}, ProofDomain: "awiki.ai"})
	if err != nil {
		t.Fatalf("GenerateIdentity(bob) error = %v", err)
	}
	if _, err := manager.Save(identity.SaveInput{
		IdentityName:            "bob",
		DID:                     bobGenerated.DID,
		UniqueID:                bobGenerated.UniqueID,
		UserID:                  "user-bob",
		DisplayName:             "Bob",
		Handle:                  "bob",
		JWTToken:                "token-bob",
		DIDDocument:             bobGenerated.DIDDocument,
		Key1PrivatePEM:          bobGenerated.Key1PrivatePEM,
		Key1PublicPEM:           bobGenerated.Key1PublicPEM,
		E2EESigningPrivatePEM:   bobGenerated.E2EESigningPrivatePEM,
		E2EEAgreementPrivatePEM: bobGenerated.E2EEAgreementPrivatePEM,
	}); err != nil {
		t.Fatalf("manager.Save(bob) error = %v", err)
	}

	aliceRecord, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("manager.Load(alice) error = %v", err)
	}
	bobRecord, err := manager.Load("bob")
	if err != nil {
		t.Fatalf("manager.Load(bob) error = %v", err)
	}

	bobClient, err := message.NewSecureE2EEClientForRecord(context.Background(), manager, bobRecord, func(string, map[string]any) (map[string]any, error) {
		return map[string]any{}, nil
	})
	if err != nil {
		t.Fatalf("NewSecureE2EEClientForRecord(bob) error = %v", err)
	}
	bobBundle, err := bobClient.EnsureFreshPrekeyBundle()
	if err != nil {
		t.Fatalf("EnsureFreshPrekeyBundle(bob) error = %v", err)
	}
	bobPaths, err := manager.PathsForIdentity("bob")
	if err != nil {
		t.Fatalf("PathsForIdentity(bob) error = %v", err)
	}
	bobOPKStore, err := anpsdk.NewFileOneTimePrekeyStore(filepath.Join(bobPaths.IdentityDir, "p5-one-time-prekeys"))
	if err != nil {
		t.Fatalf("NewFileOneTimePrekeyStore(bob) error = %v", err)
	}
	bobOPKs, err := bobOPKStore.ListOneTimePrekeys()
	if err != nil {
		t.Fatalf("ListOneTimePrekeys(bob) error = %v", err)
	}
	if len(bobOPKs) == 0 {
		t.Fatal("expected at least one Bob OPK")
	}

	alicePrivate, err := privateKeyFromPEM(t, aliceGenerated.E2EEAgreementPrivatePEM)
	if err != nil {
		t.Fatalf("privateKeyFromPEM(alice) error = %v", err)
	}
	recipientStaticPublic, err := directe2ee.ExtractX25519PublicKey(bobRecord.DIDDocument, bobBundle.StaticKeyAgreementID)
	if err != nil {
		t.Fatalf("ExtractX25519PublicKey(bob) error = %v", err)
	}
	recipientSignedPrekey := decodeFixed32Value(t, bobBundle.SignedPrekey.PublicKeyB64U)
	recipientOneTimePrekey := decodeFixed32Value(t, bobOPKs[0].PublicKeyB64U)
	initMetadata := directe2ee.DirectEnvelopeMetadata{
		SenderDID:       aliceRecord.DID,
		RecipientDID:    bobRecord.DID,
		MessageID:       "msg-pending-init-001",
		Profile:         "anp.direct.e2ee.v1",
		SecurityProfile: "direct-e2ee",
	}
	builder := directe2ee.DirectE2eeSession{}
	aliceSession, _, initBody, err := builder.InitiateSessionWithOPK(
		initMetadata,
		"msg-pending-init-001",
		aliceRecord.DID+"#key-3",
		alicePrivate,
		bobBundle,
		recipientStaticPublic,
		recipientSignedPrekey,
		&recipientOneTimePrekey,
		bobOPKs[0].KeyID,
		directe2ee.NewTextPlaintext("text/plain", "hello pending"),
	)
	if err != nil {
		t.Fatalf("InitiateSessionWithOPK() error = %v", err)
	}
	alicePaths, err := manager.PathsForIdentity("alice")
	if err != nil {
		t.Fatalf("PathsForIdentity(alice) error = %v", err)
	}
	aliceSessionStore, err := anpsdk.NewFileSessionStore(filepath.Join(alicePaths.IdentityDir, "p5-e2ee-sessions"))
	if err != nil {
		t.Fatalf("NewFileSessionStore(alice) error = %v", err)
	}
	if err := aliceSessionStore.SaveSession(aliceSession); err != nil {
		t.Fatalf("SaveSession(alice) error = %v", err)
	}

	bobStatic, err := privateKeyFromPEM(t, bobGenerated.E2EEAgreementPrivatePEM)
	if err != nil {
		t.Fatalf("privateKeyFromPEM(bob) error = %v", err)
	}
	bobSPKStore, err := anpsdk.NewFileSignedPrekeyStore(filepath.Join(bobPaths.IdentityDir, "p5-signed-prekeys"))
	if err != nil {
		t.Fatalf("NewFileSignedPrekeyStore(bob) error = %v", err)
	}
	bobSPKMaterial, _, err := bobSPKStore.LoadSignedPrekey(initBody.RecipientSignedPrekeyID)
	if err != nil {
		t.Fatalf("LoadSignedPrekey(bob) error = %v", err)
	}
	bobSPKPrivate, err := ecdh.X25519().NewPrivateKey(bobSPKMaterial.Bytes)
	if err != nil {
		t.Fatalf("NewPrivateKey(bob spk) error = %v", err)
	}
	bobOPKMaterial, _, err := bobOPKStore.LoadOneTimePrekey(initBody.RecipientOneTimePrekeyID)
	if err != nil {
		t.Fatalf("LoadOneTimePrekey(bob) error = %v", err)
	}
	bobOPKPrivate, err := ecdh.X25519().NewPrivateKey(bobOPKMaterial.Bytes)
	if err != nil {
		t.Fatalf("NewPrivateKey(bob opk) error = %v", err)
	}
	senderStaticPublic, err := directe2ee.ExtractX25519PublicKey(aliceRecord.DIDDocument, initBody.SenderStaticKeyAgreementID)
	if err != nil {
		t.Fatalf("ExtractX25519PublicKey(alice) error = %v", err)
	}
	bobSession, _, err := builder.AcceptIncomingInitWithOPK(initMetadata, bobRecord.DID+"#key-3", bobStatic, bobSPKPrivate, bobOPKPrivate, senderStaticPublic, initBody)
	if err != nil {
		t.Fatalf("AcceptIncomingInitWithOPK() error = %v", err)
	}
	bobSessionStore, err := anpsdk.NewFileSessionStore(filepath.Join(bobPaths.IdentityDir, "p5-e2ee-sessions"))
	if err != nil {
		t.Fatalf("NewFileSessionStore(bob) error = %v", err)
	}
	if err := bobSessionStore.SaveSession(bobSession); err != nil {
		t.Fatalf("SaveSession(bob) error = %v", err)
	}

	supervisor.sessions["alice"] = &session{identityName: "alice", record: aliceRecord}
	supervisor.sessions["bob"] = &session{identityName: "bob", record: bobRecord}

	ackID := "ack-" + aliceSession.SessionID
	if ok := supervisor.deliverLocalSecureAckInProcess(context.Background(), bobRecord, aliceRecord.DID, aliceSession.SessionID, initMetadata.MessageID, ackID); !ok {
		t.Fatal("deliverLocalSecureAckInProcess() = false, want true")
	}
	updatedAliceSession, ok, err := aliceSessionStore.FindByPeerDID(bobRecord.DID)
	if err != nil {
		t.Fatalf("FindByPeerDID(alice) error = %v", err)
	}
	if !ok {
		t.Fatal("expected Alice session to remain present")
	}
	if updatedAliceSession.Status != directe2ee.SessionStatusEstablished {
		t.Fatalf("Alice session status = %q, want established", updatedAliceSession.Status)
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
		var writeMu sync.Mutex
		go func() {
			for {
				_, raw, err := conn.Read(r.Context())
				if err != nil {
					return
				}
				var request map[string]any
				if err := json.Unmarshal(raw, &request); err != nil {
					continue
				}
				if request["method"] != "inbox.get" {
					continue
				}
				response, err := json.Marshal(map[string]any{
					"jsonrpc": "2.0",
					"id":      request["id"],
					"result": map[string]any{
						"messages": []any{},
					},
				})
				if err != nil {
					return
				}
				writeMu.Lock()
				err = conn.Write(r.Context(), websocket.MessageText, response)
				writeMu.Unlock()
				if err != nil {
					return
				}
			}
		}()

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
		writeMu.Lock()
		err = conn.Write(writeCtx, websocket.MessageText, raw)
		writeMu.Unlock()
		if err != nil {
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

	resolved := testResolvedConfig(t, testenv.BaseURL())
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

	resolved := testResolvedConfig(t, testenv.BaseURL())
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

func TestHandleBridgeRequestPreservesSkipForHistoryAndGroupMessages(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		request    runtime.BridgeRequest
		wantMethod string
		verifyBody func(t *testing.T, body map[string]any)
	}{
		{
			name: "direct history",
			request: runtime.BridgeRequest{
				Method:       "direct.get_history",
				IdentityName: "alice",
				Params: map[string]any{
					"with":   "did:wba:awiki.ai:user:bob:e1_bob",
					"limit":  5,
					"cursor": "42",
					"skip":   3,
				},
			},
			wantMethod: "direct.get_history",
			verifyBody: func(t *testing.T, body map[string]any) {
				t.Helper()
				if body["peer_did"] != "did:wba:awiki.ai:user:bob:e1_bob" {
					t.Fatalf("body.peer_did = %#v, want did:wba:awiki.ai:user:bob:e1_bob", body["peer_did"])
				}
				if body["since_seq"] != "42" {
					t.Fatalf("body.since_seq = %#v, want 42", body["since_seq"])
				}
				if body["skip"] != float64(3) {
					t.Fatalf("body.skip = %#v, want 3", body["skip"])
				}
			},
		},
		{
			name: "group messages",
			request: runtime.BridgeRequest{
				Method:       "group.list_messages",
				IdentityName: "alice",
				Params: map[string]any{
					"group":  "did:wba:awiki.ai:groups:demo:e1_group",
					"limit":  8,
					"cursor": "7",
					"skip":   2,
				},
			},
			wantMethod: "group.list_messages",
			verifyBody: func(t *testing.T, body map[string]any) {
				t.Helper()
				if body["group_did"] != "did:wba:awiki.ai:groups:demo:e1_group" {
					t.Fatalf("body.group_did = %#v, want did:wba:awiki.ai:groups:demo:e1_group", body["group_did"])
				}
				if body["since_seq"] != "7" {
					t.Fatalf("body.since_seq = %#v, want 7", body["since_seq"])
				}
				if body["skip"] != float64(2) {
					t.Fatalf("body.skip = %#v, want 2", body["skip"])
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan map[string]any, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					t.Errorf("websocket.Accept() error = %v", err)
					return
				}
				defer conn.Close(websocket.StatusNormalClosure, "done")

				readCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
				defer cancel()
				_, raw, err := conn.Read(readCtx)
				if err != nil {
					t.Errorf("conn.Read() error = %v", err)
					return
				}

				var request map[string]any
				if err := json.Unmarshal(raw, &request); err != nil {
					t.Errorf("json.Unmarshal() error = %v", err)
					return
				}
				requests <- request

				response, err := json.Marshal(map[string]any{
					"jsonrpc": "2.0",
					"id":      request["id"],
					"result":  map[string]any{"ok": true},
				})
				if err != nil {
					t.Errorf("json.Marshal() error = %v", err)
					return
				}
				writeCtx, writeCancel := context.WithTimeout(r.Context(), 5*time.Second)
				defer writeCancel()
				if err := conn.Write(writeCtx, websocket.MessageText, response); err != nil {
					t.Errorf("conn.Write() error = %v", err)
				}
			}))
			defer server.Close()

			wsURL := strings.Replace(server.URL, "http", "ws", 1)
			dialCtx, dialCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer dialCancel()
			conn, _, err := websocket.Dial(dialCtx, wsURL, nil)
			if err != nil {
				t.Fatalf("websocket.Dial() error = %v", err)
			}

			client := &WSClient{
				pending:       map[string]chan map[string]any{},
				notifications: make(chan map[string]any, 1),
			}
			client.attach(conn, nil)
			defer client.Close()

			supervisor := &Supervisor{
				sessions: map[string]*session{
					"alice": {
						identityName: "alice",
						record:       &identity.StoredIdentity{IdentityName: "alice", DID: "did:wba:awiki.ai:user:alice:e1_alice"},
						client:       client,
						connected:    true,
					},
				},
			}

			if _, err := supervisor.handleBridgeRequest(tc.request); err != nil {
				t.Fatalf("handleBridgeRequest() error = %v", err)
			}

			captured := <-requests
			if captured["method"] != tc.wantMethod {
				t.Fatalf("request.method = %#v, want %q", captured["method"], tc.wantMethod)
			}
			params, ok := captured["params"].(map[string]any)
			if !ok {
				t.Fatalf("request.params = %#v, want map[string]any", captured["params"])
			}
			body, ok := params["body"].(map[string]any)
			if !ok {
				t.Fatalf("request.params.body = %#v, want map[string]any", params["body"])
			}
			tc.verifyBody(t, body)
		})
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

func privateKeyFromPEM(t *testing.T, pemValue string) (*ecdh.PrivateKey, error) {
	t.Helper()
	privateKey, err := anp.PrivateKeyFromPEM(pemValue)
	if err != nil {
		return nil, err
	}
	return ecdh.X25519().NewPrivateKey(privateKey.Bytes)
}

func decodeFixed32Value(t *testing.T, value string) [32]byte {
	t.Helper()
	decoded, err := anp.DecodeBase64URL(value)
	if err != nil {
		t.Fatalf("DecodeBase64URL(%q) error = %v", value, err)
	}
	if len(decoded) != 32 {
		t.Fatalf("DecodeBase64URL(%q) len = %d, want 32", value, len(decoded))
	}
	var result [32]byte
	copy(result[:], decoded)
	return result
}

func mustJSONMap(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return result
}

package message

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	anp "github.com/agent-network-protocol/anp/golang"
	directe2ee "github.com/agent-network-protocol/anp/golang/direct_e2ee"
	"github.com/agentconnect/awiki-cli/internal/anpsdk"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/store"
)

func TestServiceSendSecureDirectUsesP5KeyServiceTargetAndPersistsPendingSession(t *testing.T) {
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

	record, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("manager.Load() error = %v", err)
	}
	serviceDID := testMessageServiceDID(t, record.DIDDocument)
	prekeyBundle, oneTimePrekey := buildTestSecurePrekeyMaterial(t, resolved, record)

	var sawGetPrekey bool
	var sawPublishPrekey bool
	var sawDirectSend bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != MessageRPCEndpoint {
			http.NotFound(w, r)
			return
		}
		envelope := decodeRPCRequest(t, r)
		switch envelope.Method {
		case "direct.e2ee.publish_prekey_bundle":
			sawPublishPrekey = true
			meta := mustMapValue(t, envelope.Params["meta"], "params.meta")
			target := mustMapValue(t, meta["target"], "meta.target")
			if got := stringFromAny(target["kind"]); got != "service" {
				t.Fatalf("publish meta.target.kind = %q, want service", got)
			}
			if got := stringFromAny(target["did"]); got != serviceDID {
				t.Fatalf("publish meta.target.did = %q, want %q", got, serviceDID)
			}
			body := mustMapValue(t, envelope.Params["body"], "params.body")
			if _, ok := body["prekey_bundle"]; !ok {
				t.Fatalf("publish body.prekey_bundle missing: %#v", body)
			}
			if _, ok := body["one_time_prekeys"]; !ok {
				t.Fatalf("publish body.one_time_prekeys missing: %#v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      envelope.ID,
				"result": map[string]any{
					"published":           true,
					"owner_did":           record.DID,
					"bundle_id":           mustMapValue(t, body["prekey_bundle"], "body.prekey_bundle")["bundle_id"],
					"published_opk_count": len(body["one_time_prekeys"].([]any)),
					"published_at":        "2026-04-23T09:25:00Z",
				},
			})
		case "direct.e2ee.get_prekey_bundle":
			sawGetPrekey = true
			if _, ok := envelope.Params["auth"]; ok {
				t.Fatalf("direct.e2ee.get_prekey_bundle params.auth should be omitted: %#v", envelope.Params)
			}
			meta := mustMapValue(t, envelope.Params["meta"], "params.meta")
			target := mustMapValue(t, meta["target"], "meta.target")
			if got := stringFromAny(meta["profile"]); got != "anp.direct.e2ee.v1" {
				t.Fatalf("meta.profile = %q, want anp.direct.e2ee.v1", got)
			}
			if got := stringFromAny(meta["security_profile"]); got != "transport-protected" {
				t.Fatalf("meta.security_profile = %q, want transport-protected", got)
			}
			if got := stringFromAny(target["kind"]); got != "service" {
				t.Fatalf("meta.target.kind = %q, want service", got)
			}
			if got := stringFromAny(target["did"]); got != serviceDID {
				t.Fatalf("meta.target.did = %q, want %q", got, serviceDID)
			}
			body := mustMapValue(t, envelope.Params["body"], "params.body")
			if got := stringFromAny(body["target_did"]); got != record.DID {
				t.Fatalf("body.target_did = %q, want %q", got, record.DID)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      envelope.ID,
				"result": map[string]any{
					"target_did":      record.DID,
					"prekey_bundle":   prekeyBundle,
					"one_time_prekey": map[string]any{"key_id": oneTimePrekey.KeyID, "public_key_b64u": oneTimePrekey.PublicKeyB64U},
				},
			})
		case "direct.send":
			sawDirectSend = true
			if _, ok := envelope.Params["auth"]; ok {
				t.Fatalf("direct.send params.auth should be omitted for P5 direct-e2ee: %#v", envelope.Params)
			}
			meta := mustMapValue(t, envelope.Params["meta"], "params.meta")
			target := mustMapValue(t, meta["target"], "meta.target")
			if got := stringFromAny(meta["content_type"]); got != "application/anp-direct-init+json" {
				t.Fatalf("meta.content_type = %q, want application/anp-direct-init+json", got)
			}
			if got := stringFromAny(meta["operation_id"]); got != stringFromAny(meta["message_id"]) {
				t.Fatalf("meta.operation_id = %q, want same as message_id %q", got, meta["message_id"])
			}
			if got := stringFromAny(target["kind"]); got != "agent" {
				t.Fatalf("meta.target.kind = %q, want agent", got)
			}
			if got := stringFromAny(target["did"]); got != record.DID {
				t.Fatalf("meta.target.did = %q, want %q", got, record.DID)
			}
			body := mustMapValue(t, envelope.Params["body"], "params.body")
			if _, ok := body["recipient_static_key_agreement_id"]; ok {
				t.Fatalf("body should not contain recipient_static_key_agreement_id: %#v", body)
			}
			if got := stringFromAny(body["recipient_one_time_prekey_id"]); got != oneTimePrekey.KeyID {
				t.Fatalf("body.recipient_one_time_prekey_id = %q, want %q", got, oneTimePrekey.KeyID)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      envelope.ID,
				"result": map[string]any{
					"accepted":     true,
					"message_id":   meta["message_id"],
					"operation_id": meta["operation_id"],
					"target_did":   record.DID,
					"accepted_at":  "2026-04-23T09:30:00Z",
				},
			})
		default:
			t.Fatalf("unexpected RPC method: %s", envelope.Method)
		}
	}))
	defer server.Close()

	resolved.ServiceBaseURL = server.URL
	service, err := NewService(resolved)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Send(context.Background(), SendRequest{
		IdentityName: "alice",
		Target:       record.DID,
		Text:         "hello secure world",
		SecureMode:   "on",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !sawPublishPrekey || !sawGetPrekey || !sawDirectSend {
		t.Fatalf("expected secure send to publish/get/send, sawPublishPrekey=%v sawGetPrekey=%v sawDirectSend=%v", sawPublishPrekey, sawGetPrekey, sawDirectSend)
	}

	message := mustMapValue(t, result.Data["message"], "result.data.message")
	if got := stringFromAny(message["type"]); got != "text" {
		t.Fatalf("result.data.message.type = %q, want text", got)
	}
	if got, ok := message["secure"].(bool); !ok || !got {
		t.Fatalf("result.data.message.secure = %#v, want true", message["secure"])
	}
	db, err := store.Open(resolved.Paths)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	defer db.Close()
	var isE2EE int
	if err := db.QueryRowContext(context.Background(), "SELECT is_e2ee FROM messages WHERE msg_id = ?", stringFromAny(message["id"])).Scan(&isE2EE); err != nil {
		t.Fatalf("query stored secure message is_e2ee error = %v", err)
	}
	if isE2EE != 1 {
		t.Fatalf("stored secure message is_e2ee = %d, want 1", isE2EE)
	}

	paths, err := manager.PathsForIdentity("alice")
	if err != nil {
		t.Fatalf("PathsForIdentity() error = %v", err)
	}
	sessionEntries, err := os.ReadDir(filepath.Join(paths.IdentityDir, "p5-e2ee-sessions"))
	if err != nil {
		t.Fatalf("ReadDir(session store) error = %v", err)
	}
	if len(sessionEntries) != 1 {
		t.Fatalf("len(sessionEntries) = %d, want 1", len(sessionEntries))
	}
	sessionBytes, err := os.ReadFile(filepath.Join(paths.IdentityDir, "p5-e2ee-sessions", sessionEntries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile(session) error = %v", err)
	}
	var session map[string]any
	if err := json.Unmarshal(sessionBytes, &session); err != nil {
		t.Fatalf("Unmarshal(session) error = %v", err)
	}
	if got := stringFromAny(session["status"]); got != "pending-confirmation" {
		t.Fatalf("session.status = %q, want pending-confirmation", got)
	}
}

func TestServiceSendSecureDirectQueuesFollowUpWhilePendingConfirmation(t *testing.T) {
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

	record, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("manager.Load() error = %v", err)
	}
	prekeyBundle, oneTimePrekey := buildTestSecurePrekeyMaterial(t, resolved, record)

	var directSendCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelope := decodeRPCRequest(t, r)
		switch envelope.Method {
		case "direct.e2ee.publish_prekey_bundle":
			body := mustMapValue(t, envelope.Params["body"], "params.body")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      envelope.ID,
				"result": map[string]any{
					"published":           true,
					"owner_did":           record.DID,
					"bundle_id":           mustMapValue(t, body["prekey_bundle"], "body.prekey_bundle")["bundle_id"],
					"published_opk_count": len(body["one_time_prekeys"].([]any)),
					"published_at":        "2026-04-23T09:25:00Z",
				},
			})
		case "direct.e2ee.get_prekey_bundle":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      envelope.ID,
				"result": map[string]any{
					"target_did":      record.DID,
					"prekey_bundle":   prekeyBundle,
					"one_time_prekey": map[string]any{"key_id": oneTimePrekey.KeyID, "public_key_b64u": oneTimePrekey.PublicKeyB64U},
				},
			})
		case "direct.send":
			directSendCalls++
			meta := mustMapValue(t, envelope.Params["meta"], "params.meta")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      envelope.ID,
				"result": map[string]any{
					"accepted":     true,
					"message_id":   meta["message_id"],
					"operation_id": meta["operation_id"],
					"target_did":   mustMapValue(t, meta["target"], "meta.target")["did"],
					"accepted_at":  "2026-04-23T09:30:00Z",
				},
			})
		default:
			t.Fatalf("unexpected RPC method: %s", envelope.Method)
		}
	}))
	defer server.Close()

	resolved.ServiceBaseURL = server.URL
	service, err := NewService(resolved)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	first, err := service.Send(context.Background(), SendRequest{
		IdentityName: "alice",
		Target:       record.DID,
		Text:         "first secure",
		SecureMode:   "on",
	})
	if err != nil {
		t.Fatalf("first Send() error = %v", err)
	}
	if first.Data["action"] != "send_message" {
		t.Fatalf("first action = %#v, want send_message", first.Data["action"])
	}
	second, err := service.Send(context.Background(), SendRequest{
		IdentityName: "alice",
		Target:       record.DID,
		Text:         "queued secure",
		SecureMode:   "on",
	})
	if err != nil {
		t.Fatalf("second Send() error = %v", err)
	}
	if second.Data["action"] != "queue_secure_message" {
		t.Fatalf("second action = %#v, want queue_secure_message", second.Data["action"])
	}
	delivery := mustMapValue(t, second.Data["delivery"], "second.data.delivery")
	if got := stringFromAny(delivery["delivery_state"]); got != "queued" {
		t.Fatalf("delivery_state = %q, want queued", got)
	}
	if directSendCalls != 1 {
		t.Fatalf("directSendCalls = %d, want 1", directSendCalls)
	}
	db, err := store.Open(resolved.Paths)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	defer db.Close()
	rows, err := store.ListE2EEOutbox(context.Background(), db, record.DID, record.IdentityName, "queued")
	if err != nil {
		t.Fatalf("ListE2EEOutbox() error = %v", err)
	}
	if len(rows) != 1 || rows[0]["plaintext"] != "queued secure" {
		t.Fatalf("queued outbox rows = %#v, want one queued secure row", rows)
	}
}

func TestPollingInboxDecryptsDirectInitAndSendsSecureAck(t *testing.T) {
	t.Parallel()

	resolved := testResolvedConfig(t)
	manager := identity.NewManager(resolved.Paths)
	createTestIdentity(t, manager, identity.SaveInput{
		IdentityName: "alice",
		UserID:       "user-alice-123",
		DisplayName:  "Alice",
		Handle:       "alice",
		JWTToken:     "token-alice",
	})
	createTestIdentity(t, manager, identity.SaveInput{
		IdentityName: "bob",
		UserID:       "user-bob-123",
		DisplayName:  "Bob",
		Handle:       "bob",
		JWTToken:     "token-bob",
	})
	aliceRecord, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("manager.Load(alice) error = %v", err)
	}
	bobRecord, err := manager.Load("bob")
	if err != nil {
		t.Fatalf("manager.Load(bob) error = %v", err)
	}
	prekeyBundle, oneTimePrekey := buildTestSecurePrekeyMaterial(t, resolved, bobRecord)

	var capturedInit map[string]any
	aliceClient, err := NewSecureE2EEClientForRecord(context.Background(), manager, aliceRecord, func(method string, params map[string]any) (map[string]any, error) {
		switch method {
		case "direct.e2ee.get_prekey_bundle":
			return map[string]any{
				"target_did":      bobRecord.DID,
				"prekey_bundle":   prekeyBundle,
				"one_time_prekey": map[string]any{"key_id": oneTimePrekey.KeyID, "public_key_b64u": oneTimePrekey.PublicKeyB64U},
			}, nil
		case "direct.send":
			capturedInit = params
			meta := mustMapValue(t, params["meta"], "init.meta")
			return map[string]any{
				"accepted":     true,
				"message_id":   meta["message_id"],
				"operation_id": meta["operation_id"],
				"target_did":   bobRecord.DID,
				"accepted_at":  "2026-04-24T08:00:00Z",
			}, nil
		default:
			return nil, fmt.Errorf("unexpected alice RPC method: %s", method)
		}
	})
	if err != nil {
		t.Fatalf("NewSecureE2EEClientForRecord(alice) error = %v", err)
	}
	if _, err := aliceClient.SendText(context.Background(), bobRecord.DID, "hello via polling", "msg-init-poll-001", "msg-init-poll-001"); err != nil {
		t.Fatalf("alice SendText() error = %v", err)
	}
	if capturedInit == nil {
		t.Fatal("alice direct init was not captured")
	}

	var ackDirectSendCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelope := decodeRPCRequest(t, r)
		if envelope.Method != "direct.send" {
			t.Fatalf("unexpected bob RPC method: %s", envelope.Method)
		}
		ackDirectSendCalls++
		meta := mustMapValue(t, envelope.Params["meta"], "ack.meta")
		if got := stringFromAny(meta["content_type"]); got != "application/anp-direct-cipher+json" {
			t.Fatalf("ack meta.content_type = %q, want direct cipher", got)
		}
		if got := stringFromAny(meta["operation_id"]); got == "" || got != stringFromAny(meta["message_id"]) {
			t.Fatalf("ack operation/message mismatch: %#v", meta)
		}
		if got := stringFromAny(meta["sender_did"]); got != bobRecord.DID {
			t.Fatalf("ack sender_did = %q, want %q", got, bobRecord.DID)
		}
		target := mustMapValue(t, meta["target"], "ack.meta.target")
		if got := stringFromAny(target["did"]); got != aliceRecord.DID {
			t.Fatalf("ack target.did = %q, want %q", got, aliceRecord.DID)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      envelope.ID,
			"result": map[string]any{
				"accepted":     true,
				"message_id":   meta["message_id"],
				"operation_id": meta["operation_id"],
				"target_did":   aliceRecord.DID,
				"accepted_at":  "2026-04-24T08:00:01Z",
			},
		})
	}))
	defer server.Close()
	resolved.ServiceBaseURL = server.URL
	resolved.ActiveIdentity = "bob"
	service, err := NewService(resolved)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	initMeta := mustMapValue(t, capturedInit["meta"], "captured init meta")
	initBody := mustMapValue(t, capturedInit["body"], "captured init body")
	messages, _, warnings := service.persistInboxMessages(context.Background(), bobRecord, map[string]any{
		"messages": []any{
			map[string]any{
				"id":           stringFromAny(initMeta["message_id"]),
				"sender_did":   aliceRecord.DID,
				"receiver_did": bobRecord.DID,
				"content_type": stringFromAny(initMeta["content_type"]),
				"content":      initBody,
				"server_seq":   float64(1),
				"sent_at":      "2026-04-24T08:00:00Z",
			},
		},
		"total": 1,
	}, "alice")
	if len(warnings) != 0 {
		t.Fatalf("persistInboxMessages warnings = %#v, want none", warnings)
	}
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	if got := stringFromAny(messages[0]["content"]); got != "hello via polling" {
		t.Fatalf("decrypted content = %q, want hello via polling", got)
	}
	if ackDirectSendCalls != 1 {
		t.Fatalf("ackDirectSendCalls = %d, want 1", ackDirectSendCalls)
	}
}

func TestServiceSecureInitCreatesPendingSession(t *testing.T) {
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

	record, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("manager.Load() error = %v", err)
	}
	serviceDID := testMessageServiceDID(t, record.DIDDocument)
	prekeyBundle, oneTimePrekey := buildTestSecurePrekeyMaterial(t, resolved, record)

	var directSendCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelope := decodeRPCRequest(t, r)
		switch envelope.Method {
		case "direct.e2ee.publish_prekey_bundle":
			body := mustMapValue(t, envelope.Params["body"], "params.body")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      envelope.ID,
				"result": map[string]any{
					"published":           true,
					"owner_did":           record.DID,
					"bundle_id":           mustMapValue(t, body["prekey_bundle"], "body.prekey_bundle")["bundle_id"],
					"published_opk_count": len(body["one_time_prekeys"].([]any)),
					"published_at":        "2026-04-23T09:25:00Z",
				},
			})
		case "direct.e2ee.get_prekey_bundle":
			meta := mustMapValue(t, envelope.Params["meta"], "params.meta")
			target := mustMapValue(t, meta["target"], "meta.target")
			if stringFromAny(target["did"]) != serviceDID {
				t.Fatalf("get target.did = %q, want %q", target["did"], serviceDID)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      envelope.ID,
				"result": map[string]any{
					"target_did":      record.DID,
					"prekey_bundle":   prekeyBundle,
					"one_time_prekey": map[string]any{"key_id": oneTimePrekey.KeyID, "public_key_b64u": oneTimePrekey.PublicKeyB64U},
				},
			})
		case "direct.send":
			directSendCalls++
			meta := mustMapValue(t, envelope.Params["meta"], "params.meta")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      envelope.ID,
				"result": map[string]any{
					"accepted":     true,
					"message_id":   meta["message_id"],
					"operation_id": meta["operation_id"],
					"target_did":   mustMapValue(t, meta["target"], "meta.target")["did"],
					"accepted_at":  "2026-04-23T09:30:00Z",
				},
			})
		default:
			t.Fatalf("unexpected RPC method: %s", envelope.Method)
		}
	}))
	defer server.Close()

	resolved.ServiceBaseURL = server.URL
	service, err := NewService(resolved)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	result, err := service.SecureInit(context.Background(), SecurePeerRequest{
		IdentityName: "alice",
		With:         record.DID,
	})
	if err != nil {
		t.Fatalf("SecureInit() error = %v", err)
	}
	if result.Data["initialized"] != true {
		t.Fatalf("initialized = %#v, want true", result.Data["initialized"])
	}
	if directSendCalls != 1 {
		t.Fatalf("directSendCalls = %d, want 1", directSendCalls)
	}
	paths, err := manager.PathsForIdentity("alice")
	if err != nil {
		t.Fatalf("PathsForIdentity() error = %v", err)
	}
	sessionEntries, err := os.ReadDir(filepath.Join(paths.IdentityDir, "p5-e2ee-sessions"))
	if err != nil {
		t.Fatalf("ReadDir(session store) error = %v", err)
	}
	if len(sessionEntries) != 1 {
		t.Fatalf("len(sessionEntries) = %d, want 1", len(sessionEntries))
	}
}

func TestFlushQueuedSecureOutboxSendsCipherAfterConfirmation(t *testing.T) {
	t.Parallel()

	resolved := testResolvedConfig(t)
	manager := identity.NewManager(resolved.Paths)
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
		JWTToken:                "token-alice",
		DIDDocument:             aliceGenerated.DIDDocument,
		Key1PrivatePEM:          aliceGenerated.Key1PrivatePEM,
		Key1PublicPEM:           aliceGenerated.Key1PublicPEM,
		E2EESigningPrivatePEM:   aliceGenerated.E2EESigningPrivatePEM,
		E2EEAgreementPrivatePEM: aliceGenerated.E2EEAgreementPrivatePEM,
	}); err != nil {
		t.Fatalf("manager.Save(alice) error = %v", err)
	}
	aliceRecord, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("manager.Load(alice) error = %v", err)
	}

	bobGenerated, err := identity.GenerateIdentity(identity.GenerateOptions{
		Hostname:    "awiki.ai",
		PathPrefix:  []string{"user"},
		ProofDomain: "awiki.ai",
	})
	if err != nil {
		t.Fatalf("GenerateIdentity(bob) error = %v", err)
	}
	aliceStatic, err := secureECDHPrivateKey(aliceGenerated.E2EEAgreementPrivatePEM)
	if err != nil {
		t.Fatalf("secureECDHPrivateKey(alice) error = %v", err)
	}
	bobStatic, err := secureECDHPrivateKey(bobGenerated.E2EEAgreementPrivatePEM)
	if err != nil {
		t.Fatalf("secureECDHPrivateKey(bob) error = %v", err)
	}
	bobSPK, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey(bobSPK) error = %v", err)
	}
	bobSigning, err := anp.PrivateKeyFromPEM(bobGenerated.Key1PrivatePEM)
	if err != nil {
		t.Fatalf("PrivateKeyFromPEM(bob signing) error = %v", err)
	}
	bundle, err := directe2ee.BuildPrekeyBundle(
		"bundle-bob-001",
		bobGenerated.DID,
		bobGenerated.DID+"#key-3",
		directe2ee.SignedPrekeyFromPrivateKey("spk-bob-001", bobSPK, "2026-04-07T00:00:00Z"),
		bobSigning,
		bobGenerated.DID+"#key-1",
		"2026-03-31T09:58:58Z",
	)
	if err != nil {
		t.Fatalf("BuildPrekeyBundle() error = %v", err)
	}
	recipientStaticPublic, err := directe2ee.ExtractX25519PublicKey(bobGenerated.DIDDocument, bundle.StaticKeyAgreementID)
	if err != nil {
		t.Fatalf("ExtractX25519PublicKey(bob) error = %v", err)
	}
	recipientSignedPrekey := fixed32(t, bobSPK.PublicKey().Bytes())
	sessionBuilder := directe2ee.DirectE2eeSession{}
	initMetadata := directe2ee.DirectEnvelopeMetadata{
		SenderDID:       aliceGenerated.DID,
		RecipientDID:    bobGenerated.DID,
		MessageID:       "msg-init-001",
		Profile:         "anp.direct.e2ee.v1",
		SecurityProfile: "direct-e2ee",
	}
	aliceSession, _, initBody, err := sessionBuilder.InitiateSession(
		initMetadata,
		"msg-init-001",
		aliceGenerated.DID+"#key-3",
		aliceStatic,
		bundle,
		recipientStaticPublic,
		recipientSignedPrekey,
		directe2ee.NewTextPlaintext("text/plain", "init"),
	)
	if err != nil {
		t.Fatalf("InitiateSession() error = %v", err)
	}
	bobSession, _, err := sessionBuilder.AcceptIncomingInit(
		initMetadata,
		bobGenerated.DID+"#key-3",
		bobStatic,
		bobSPK,
		fixed32(t, aliceStatic.PublicKey().Bytes()),
		initBody,
	)
	if err != nil {
		t.Fatalf("AcceptIncomingInit() error = %v", err)
	}
	replyMetadata := directe2ee.DirectEnvelopeMetadata{
		SenderDID:       bobGenerated.DID,
		RecipientDID:    aliceGenerated.DID,
		MessageID:       "msg-reply-001",
		Profile:         "anp.direct.e2ee.v1",
		SecurityProfile: "direct-e2ee",
	}
	_, replyBody, err := sessionBuilder.EncryptFollowUp(&bobSession, replyMetadata, "msg-reply-001", directe2ee.NewJSONPlaintext("application/json", BuildSecureAckPayload(aliceSession.SessionID, initMetadata.MessageID)))
	if err != nil {
		t.Fatalf("EncryptFollowUp(reply) error = %v", err)
	}
	if _, err := sessionBuilder.DecryptFollowUp(&aliceSession, replyMetadata, replyBody); err != nil {
		t.Fatalf("DecryptFollowUp(reply) error = %v", err)
	}
	paths, err := manager.PathsForIdentity("alice")
	if err != nil {
		t.Fatalf("PathsForIdentity(alice) error = %v", err)
	}
	sessionStore, err := anpsdk.NewFileSessionStore(filepath.Join(paths.IdentityDir, "p5-e2ee-sessions"))
	if err != nil {
		t.Fatalf("NewFileSessionStore() error = %v", err)
	}
	if err := sessionStore.SaveSession(aliceSession); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	db, err := store.Open(resolved.Paths)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	defer db.Close()
	if err := store.EnsureSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	outboxID, err := store.QueueE2EEOutbox(context.Background(), db, store.E2EEOutboxRecord{
		OwnerDID:       aliceRecord.DID,
		PeerDID:        bobGenerated.DID,
		SessionID:      aliceSession.SessionID,
		OriginalType:   "text",
		Plaintext:      "queued follow-up",
		LocalStatus:    "queued",
		CredentialName: aliceRecord.IdentityName,
	})
	if err != nil {
		t.Fatalf("QueueE2EEOutbox() error = %v", err)
	}

	var sendCalls int
	warnings := FlushQueuedSecureOutbox(context.Background(), resolved, manager, aliceRecord, bobGenerated.DID, func(method string, params map[string]any) (map[string]any, error) {
		if method != "direct.send" {
			t.Fatalf("unexpected method: %s", method)
		}
		sendCalls++
		meta := mustMapValue(t, params["meta"], "params.meta")
		if got := stringFromAny(meta["content_type"]); got != "application/anp-direct-cipher+json" {
			t.Fatalf("meta.content_type = %q, want application/anp-direct-cipher+json", got)
		}
		return map[string]any{
			"accepted":     true,
			"message_id":   meta["message_id"],
			"operation_id": meta["operation_id"],
			"target_did":   mustMapValue(t, meta["target"], "meta.target")["did"],
			"accepted_at":  "2026-04-23T10:00:00Z",
		}, nil
	})
	if len(warnings) != 0 {
		t.Fatalf("FlushQueuedSecureOutbox() warnings = %#v, want none", warnings)
	}
	if sendCalls != 1 {
		t.Fatalf("sendCalls = %d, want 1", sendCalls)
	}
	outboxRow, err := store.GetE2EEOutbox(context.Background(), db, outboxID, aliceRecord.DID, aliceRecord.IdentityName)
	if err != nil {
		t.Fatalf("GetE2EEOutbox() error = %v", err)
	}
	if got := outboxRow["local_status"]; got != "sent" {
		t.Fatalf("outbox local_status = %#v, want sent", got)
	}
	messageRow, err := store.GetMessageByID(context.Background(), db, outboxID, aliceRecord.DID, aliceRecord.IdentityName)
	if err != nil {
		t.Fatalf("GetMessageByID() error = %v", err)
	}
	if got := messageRow["content"]; got != "queued follow-up" {
		t.Fatalf("stored content = %#v, want queued follow-up", got)
	}
}

func TestServiceSecureStatusReturnsSessionAndOutboxSummary(t *testing.T) {
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

	record, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("manager.Load() error = %v", err)
	}
	paths, err := manager.PathsForIdentity("alice")
	if err != nil {
		t.Fatalf("PathsForIdentity() error = %v", err)
	}
	sessionStore, err := anpsdk.NewFileSessionStore(filepath.Join(paths.IdentityDir, "p5-e2ee-sessions"))
	if err != nil {
		t.Fatalf("NewFileSessionStore() error = %v", err)
	}
	if err := sessionStore.SaveSession(anpsdk.DirectSessionState{
		SessionID:             "session-001",
		Suite:                 directe2ee.MTIDirectE2EESuite,
		PeerDID:               "did:wba:awiki.ai:user:bob:e1_bob",
		LocalKeyAgreementID:   record.DID + "#key-3",
		PeerKeyAgreementID:    "did:wba:awiki.ai:user:bob:e1_bob#key-3",
		RootKeyB64U:           "root",
		SendChainKeyB64U:      "send",
		RecvChainKeyB64U:      "recv",
		RatchetPrivateKeyB64U: "priv",
		RatchetPublicKeyB64U:  "pub",
		SendN:                 1,
		RecvN:                 1,
		SkippedMessageKeys: []directe2ee.SkippedMessageKey{
			{
				DHPubB64U:      "skipped-dh",
				N:              7,
				MessageKeyB64U: "skipped-message-key",
				NonceB64U:      "skipped-nonce",
			},
		},
		IsInitiator: true,
		Status:      "established",
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	db, err := store.Open(resolved.Paths)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	defer db.Close()
	if err := store.EnsureSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	if _, err := store.QueueE2EEOutbox(context.Background(), db, store.E2EEOutboxRecord{
		OwnerDID:       record.DID,
		PeerDID:        "did:wba:awiki.ai:user:bob:e1_bob",
		SessionID:      "session-001",
		OriginalType:   "text",
		Plaintext:      "secret queued body",
		LocalStatus:    "failed",
		CredentialName: record.IdentityName,
	}); err != nil {
		t.Fatalf("QueueE2EEOutbox() error = %v", err)
	}

	service, err := NewService(resolved)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	result, err := service.SecureStatus(context.Background(), SecureStatusRequest{
		IdentityName: "alice",
		With:         "did:wba:awiki.ai:user:bob:e1_bob",
	})
	if err != nil {
		t.Fatalf("SecureStatus() error = %v", err)
	}
	sessions, ok := result.Data["sessions"].([]map[string]any)
	if !ok || len(sessions) != 1 {
		t.Fatalf("sessions = %#v, want 1 session", result.Data["sessions"])
	}
	if got := sessions[0]["skipped_key_count"]; got != 1 {
		t.Fatalf("session.skipped_key_count = %#v, want 1", got)
	}
	outbox := mustMapValue(t, result.Data["outbox"], "data.outbox")
	if got := outbox["total"]; got != 1 {
		t.Fatalf("outbox.total = %#v, want 1", got)
	}
	byStatus, ok := outbox["by_status"].(map[string]int)
	if !ok {
		t.Fatalf("outbox.by_status = %#v, want map[string]int", outbox["by_status"])
	}
	if byStatus["failed"] != 1 {
		t.Fatalf("by_status = %#v, want failed=1", byStatus)
	}
	records, ok := outbox["records"].([]map[string]any)
	if !ok || len(records) != 1 {
		t.Fatalf("outbox.records = %#v, want one redacted record", outbox["records"])
	}
	if _, ok := records[0]["plaintext"]; ok {
		t.Fatalf("status outbox record leaked plaintext: %#v", records[0])
	}
	encoded, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatalf("Marshal(status data) error = %v", err)
	}
	forbidden := []string{
		"root_key_b64u",
		"send_chain_key_b64u",
		"recv_chain_key_b64u",
		"ratchet_private_key_b64u",
		"message_key_b64u",
		"nonce_b64u",
		"secret queued body",
	}
	for _, value := range forbidden {
		if strings.Contains(string(encoded), value) {
			t.Fatalf("SecureStatus leaked %q in %s", value, encoded)
		}
	}
}

func TestServiceSecureFailedAndDropOperateOnOutbox(t *testing.T) {
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

	record, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("manager.Load() error = %v", err)
	}
	db, err := store.Open(resolved.Paths)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	defer db.Close()
	if err := store.EnsureSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	outboxID, err := store.QueueE2EEOutbox(context.Background(), db, store.E2EEOutboxRecord{
		OwnerDID:       record.DID,
		PeerDID:        "did:wba:awiki.ai:user:bob:e1_bob",
		SessionID:      "session-001",
		OriginalType:   "text",
		Plaintext:      "failed",
		LocalStatus:    "failed",
		CredentialName: record.IdentityName,
	})
	if err != nil {
		t.Fatalf("QueueE2EEOutbox() error = %v", err)
	}
	service, err := NewService(resolved)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	failedResult, err := service.SecureFailed(context.Background(), SecureStatusRequest{IdentityName: "alice"})
	if err != nil {
		t.Fatalf("SecureFailed() error = %v", err)
	}
	if failedResult.Data["total"] != 1 {
		t.Fatalf("failed total = %#v, want 1", failedResult.Data["total"])
	}
	dropResult, err := service.SecureDrop(context.Background(), SecureOutboxActionRequest{
		IdentityName: "alice",
		OutboxID:     outboxID,
	})
	if err != nil {
		t.Fatalf("SecureDrop() error = %v", err)
	}
	if dropResult.Data["status"] != "dropped" {
		t.Fatalf("drop status = %#v, want dropped", dropResult.Data["status"])
	}
	row, err := store.GetE2EEOutbox(context.Background(), db, outboxID, record.DID, record.IdentityName)
	if err != nil {
		t.Fatalf("GetE2EEOutbox() error = %v", err)
	}
	if got := row["local_status"]; got != "dropped" {
		t.Fatalf("local_status = %#v, want dropped", got)
	}
}

func TestServiceSecureRetryMarksQueuedRecordSent(t *testing.T) {
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

	record, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("manager.Load() error = %v", err)
	}
	paths, err := manager.PathsForIdentity("alice")
	if err != nil {
		t.Fatalf("PathsForIdentity() error = %v", err)
	}
	sessionStore, err := anpsdk.NewFileSessionStore(filepath.Join(paths.IdentityDir, "p5-e2ee-sessions"))
	if err != nil {
		t.Fatalf("NewFileSessionStore() error = %v", err)
	}
	bobGenerated, err := identity.GenerateIdentity(identity.GenerateOptions{
		Hostname:    "awiki.ai",
		PathPrefix:  []string{"user"},
		ProofDomain: "awiki.ai",
	})
	if err != nil {
		t.Fatalf("GenerateIdentity(bob) error = %v", err)
	}
	aliceStatic, err := secureECDHPrivateKey(record.E2EEAgreementPrivatePEM)
	if err != nil {
		t.Fatalf("secureECDHPrivateKey(alice) error = %v", err)
	}
	bobStatic, err := secureECDHPrivateKey(bobGenerated.E2EEAgreementPrivatePEM)
	if err != nil {
		t.Fatalf("secureECDHPrivateKey(bob) error = %v", err)
	}
	bobSPK, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey(bobSPK) error = %v", err)
	}
	bobSigning, err := anp.PrivateKeyFromPEM(bobGenerated.Key1PrivatePEM)
	if err != nil {
		t.Fatalf("PrivateKeyFromPEM(bob signing) error = %v", err)
	}
	bundle, err := directe2ee.BuildPrekeyBundle(
		"bundle-bob-001",
		bobGenerated.DID,
		bobGenerated.DID+"#key-3",
		directe2ee.SignedPrekeyFromPrivateKey("spk-bob-001", bobSPK, "2026-04-07T00:00:00Z"),
		bobSigning,
		bobGenerated.DID+"#key-1",
		"2026-03-31T09:58:58Z",
	)
	if err != nil {
		t.Fatalf("BuildPrekeyBundle() error = %v", err)
	}
	recipientStaticPublic, err := directe2ee.ExtractX25519PublicKey(bobGenerated.DIDDocument, bundle.StaticKeyAgreementID)
	if err != nil {
		t.Fatalf("ExtractX25519PublicKey(bob) error = %v", err)
	}
	recipientSignedPrekey := fixed32(t, bobSPK.PublicKey().Bytes())
	sessionBuilder := directe2ee.DirectE2eeSession{}
	initMetadata := directe2ee.DirectEnvelopeMetadata{
		SenderDID:       record.DID,
		RecipientDID:    bobGenerated.DID,
		MessageID:       "msg-init-retry-001",
		Profile:         "anp.direct.e2ee.v1",
		SecurityProfile: "direct-e2ee",
	}
	aliceSession, _, initBody, err := sessionBuilder.InitiateSession(
		initMetadata,
		"msg-init-retry-001",
		record.DID+"#key-3",
		aliceStatic,
		bundle,
		recipientStaticPublic,
		recipientSignedPrekey,
		directe2ee.NewTextPlaintext("text/plain", "init"),
	)
	if err != nil {
		t.Fatalf("InitiateSession() error = %v", err)
	}
	bobSession, _, err := sessionBuilder.AcceptIncomingInit(
		initMetadata,
		bobGenerated.DID+"#key-3",
		bobStatic,
		bobSPK,
		fixed32(t, aliceStatic.PublicKey().Bytes()),
		initBody,
	)
	if err != nil {
		t.Fatalf("AcceptIncomingInit() error = %v", err)
	}
	replyMetadata := directe2ee.DirectEnvelopeMetadata{
		SenderDID:       bobGenerated.DID,
		RecipientDID:    record.DID,
		MessageID:       "msg-reply-retry-001",
		Profile:         "anp.direct.e2ee.v1",
		SecurityProfile: "direct-e2ee",
	}
	_, replyBody, err := sessionBuilder.EncryptFollowUp(&bobSession, replyMetadata, "msg-reply-retry-001", directe2ee.NewJSONPlaintext("application/json", BuildSecureAckPayload(aliceSession.SessionID, initMetadata.MessageID)))
	if err != nil {
		t.Fatalf("EncryptFollowUp(reply) error = %v", err)
	}
	if _, err := sessionBuilder.DecryptFollowUp(&aliceSession, replyMetadata, replyBody); err != nil {
		t.Fatalf("DecryptFollowUp(reply) error = %v", err)
	}
	if err := sessionStore.SaveSession(aliceSession); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	db, err := store.Open(resolved.Paths)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	defer db.Close()
	if err := store.EnsureSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	outboxID, err := store.QueueE2EEOutbox(context.Background(), db, store.E2EEOutboxRecord{
		OwnerDID:       record.DID,
		PeerDID:        bobGenerated.DID,
		SessionID:      aliceSession.SessionID,
		OriginalType:   "text",
		Plaintext:      "retry me",
		LocalStatus:    "failed",
		CredentialName: record.IdentityName,
	})
	if err != nil {
		t.Fatalf("QueueE2EEOutbox() error = %v", err)
	}

	var sendCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelope := decodeRPCRequest(t, r)
		switch envelope.Method {
		case "direct.e2ee.publish_prekey_bundle":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      envelope.ID,
				"result":  map[string]any{"published": true, "owner_did": record.DID, "bundle_id": "bundle-001", "published_at": "2026-04-23T09:25:00Z"},
			})
		case "direct.send":
			sendCalls++
			meta := mustMapValue(t, envelope.Params["meta"], "params.meta")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      envelope.ID,
				"result": map[string]any{
					"accepted":     true,
					"message_id":   meta["message_id"],
					"operation_id": meta["operation_id"],
					"target_did":   mustMapValue(t, meta["target"], "meta.target")["did"],
					"accepted_at":  "2026-04-23T09:30:00Z",
				},
			})
		default:
			t.Fatalf("unexpected RPC method: %s", envelope.Method)
		}
	}))
	defer server.Close()
	resolved.ServiceBaseURL = server.URL
	service, err := NewService(resolved)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	result, err := service.SecureRetry(context.Background(), SecureOutboxActionRequest{
		IdentityName: "alice",
		OutboxID:     outboxID,
	})
	if err != nil {
		t.Fatalf("SecureRetry() error = %v", err)
	}
	if sendCalls != 1 {
		t.Fatalf("sendCalls = %d, want 1", sendCalls)
	}
	recordMap := mustMapValue(t, result.Data["record"], "data.record")
	if got := recordMap["local_status"]; got != "sent" {
		t.Fatalf("record.local_status = %#v, want sent", got)
	}
}

func TestServiceSecureRepairResetsFailedOutboxAndStartsNewInit(t *testing.T) {
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

	record, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("manager.Load() error = %v", err)
	}
	serviceDID := testMessageServiceDID(t, record.DIDDocument)
	prekeyBundle, oneTimePrekey := buildTestSecurePrekeyMaterial(t, resolved, record)
	paths, err := manager.PathsForIdentity("alice")
	if err != nil {
		t.Fatalf("PathsForIdentity() error = %v", err)
	}
	sessionStore, err := anpsdk.NewFileSessionStore(filepath.Join(paths.IdentityDir, "p5-e2ee-sessions"))
	if err != nil {
		t.Fatalf("NewFileSessionStore() error = %v", err)
	}
	if err := sessionStore.SaveSession(anpsdk.DirectSessionState{
		SessionID:             "old-session",
		Suite:                 directe2ee.MTIDirectE2EESuite,
		PeerDID:               record.DID,
		LocalKeyAgreementID:   record.DID + "#key-3",
		PeerKeyAgreementID:    record.DID + "#key-3",
		RootKeyB64U:           "root",
		SendChainKeyB64U:      "send",
		RecvChainKeyB64U:      "recv",
		RatchetPrivateKeyB64U: "priv",
		RatchetPublicKeyB64U:  "pub",
		SendN:                 1,
		RecvN:                 1,
		IsInitiator:           true,
		Status:                "established",
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	db, err := store.Open(resolved.Paths)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	defer db.Close()
	if err := store.EnsureSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	outboxID, err := store.QueueE2EEOutbox(context.Background(), db, store.E2EEOutboxRecord{
		OwnerDID:       record.DID,
		PeerDID:        record.DID,
		SessionID:      "old-session",
		OriginalType:   "text",
		Plaintext:      "failed message",
		LocalStatus:    "failed",
		CredentialName: record.IdentityName,
	})
	if err != nil {
		t.Fatalf("QueueE2EEOutbox() error = %v", err)
	}

	var directSendCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelope := decodeRPCRequest(t, r)
		switch envelope.Method {
		case "direct.e2ee.publish_prekey_bundle":
			body := mustMapValue(t, envelope.Params["body"], "params.body")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      envelope.ID,
				"result": map[string]any{
					"published":           true,
					"owner_did":           record.DID,
					"bundle_id":           mustMapValue(t, body["prekey_bundle"], "body.prekey_bundle")["bundle_id"],
					"published_opk_count": len(body["one_time_prekeys"].([]any)),
					"published_at":        "2026-04-23T09:25:00Z",
				},
			})
		case "direct.e2ee.get_prekey_bundle":
			meta := mustMapValue(t, envelope.Params["meta"], "params.meta")
			target := mustMapValue(t, meta["target"], "meta.target")
			if stringFromAny(target["did"]) != serviceDID {
				t.Fatalf("get target.did = %q, want %q", target["did"], serviceDID)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      envelope.ID,
				"result": map[string]any{
					"target_did":      record.DID,
					"prekey_bundle":   prekeyBundle,
					"one_time_prekey": map[string]any{"key_id": oneTimePrekey.KeyID, "public_key_b64u": oneTimePrekey.PublicKeyB64U},
				},
			})
		case "direct.send":
			directSendCalls++
			meta := mustMapValue(t, envelope.Params["meta"], "params.meta")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      envelope.ID,
				"result": map[string]any{
					"accepted":     true,
					"message_id":   meta["message_id"],
					"operation_id": meta["operation_id"],
					"target_did":   mustMapValue(t, meta["target"], "meta.target")["did"],
					"accepted_at":  "2026-04-23T09:30:00Z",
				},
			})
		default:
			t.Fatalf("unexpected RPC method: %s", envelope.Method)
		}
	}))
	defer server.Close()
	resolved.ServiceBaseURL = server.URL
	service, err := NewService(resolved)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	result, err := service.SecureRepair(context.Background(), SecurePeerRequest{
		IdentityName: "alice",
		With:         record.DID,
	})
	if err != nil {
		t.Fatalf("SecureRepair() error = %v", err)
	}
	repair := mustMapValue(t, result.Data["repair"], "data.repair")
	if repair["reset_records"] != 2 {
		t.Fatalf("repair.reset_records = %#v, want 2", repair["reset_records"])
	}
	if directSendCalls != 1 {
		t.Fatalf("directSendCalls = %d, want 1", directSendCalls)
	}
	row, err := store.GetE2EEOutbox(context.Background(), db, outboxID, record.DID, record.IdentityName)
	if err != nil {
		t.Fatalf("GetE2EEOutbox() error = %v", err)
	}
	if got := row["local_status"]; got != "queued" {
		t.Fatalf("local_status = %#v, want queued", got)
	}
}

func buildTestSecurePrekeyMaterial(
	t *testing.T,
	resolved *appconfig.Resolved,
	record *identity.StoredIdentity,
) (map[string]any, anpsdk.OneTimePrekey) {
	t.Helper()

	paths, err := identity.NewManager(resolved.Paths).PathsForIdentity(record.IdentityName)
	if err != nil {
		t.Fatalf("PathsForIdentity() error = %v", err)
	}
	signingPrivate, err := anpsdk.PrivateKeyFromPEM(record.Key1PrivatePEM)
	if err != nil {
		t.Fatalf("PrivateKeyFromPEM(signing) error = %v", err)
	}
	agreementPrivate, err := anpsdk.PrivateKeyFromPEM(record.E2EEAgreementPrivatePEM)
	if err != nil {
		t.Fatalf("PrivateKeyFromPEM(agreement) error = %v", err)
	}
	sessionStore, err := anpsdk.NewFileSessionStore(filepath.Join(paths.IdentityDir, "p5-e2ee-sessions"))
	if err != nil {
		t.Fatalf("NewFileSessionStore() error = %v", err)
	}
	signedPrekeyStore, err := anpsdk.NewFileSignedPrekeyStore(filepath.Join(paths.IdentityDir, "p5-signed-prekeys"))
	if err != nil {
		t.Fatalf("NewFileSignedPrekeyStore() error = %v", err)
	}
	oneTimePrekeyStore, err := anpsdk.NewFileOneTimePrekeyStore(filepath.Join(paths.IdentityDir, "p5-one-time-prekeys"))
	if err != nil {
		t.Fatalf("NewFileOneTimePrekeyStore() error = %v", err)
	}
	resolver := func(_ context.Context, did string) (map[string]any, error) {
		if did != record.DID {
			return nil, fmt.Errorf("unexpected did: %s", did)
		}
		return record.DIDDocument, nil
	}
	client, err := anpsdk.NewMessageServiceDirectE2eeClient(
		record.DID,
		signingPrivate,
		record.DID+"#key-1",
		agreementPrivate,
		record.DID+"#key-3",
		func(string, map[string]any) (map[string]any, error) { return map[string]any{}, nil },
		resolver,
		sessionStore,
		signedPrekeyStore,
		oneTimePrekeyStore,
	)
	if err != nil {
		t.Fatalf("NewMessageServiceDirectE2eeClient() error = %v", err)
	}
	bundle, err := client.EnsureFreshPrekeyBundle()
	if err != nil {
		t.Fatalf("EnsureFreshPrekeyBundle() error = %v", err)
	}
	oneTimePrekeys, err := oneTimePrekeyStore.ListOneTimePrekeys()
	if err != nil {
		t.Fatalf("ListOneTimePrekeys() error = %v", err)
	}
	if len(oneTimePrekeys) == 0 {
		t.Fatal("expected EnsureFreshPrekeyBundle to materialize local OPKs")
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("Marshal(prekey bundle) error = %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("Unmarshal(prekey bundle) error = %v", err)
	}
	return result, oneTimePrekeys[0]
}

func testMessageServiceDID(t *testing.T, didDocument map[string]any) string {
	t.Helper()
	services, ok := didDocument["service"].([]any)
	if !ok {
		t.Fatalf("didDocument.service = %#v, want []any", didDocument["service"])
	}
	for _, entry := range services {
		service, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if stringFromAny(service["type"]) != "ANPMessageService" {
			continue
		}
		serviceDID := stringFromAny(service["serviceDid"])
		if serviceDID != "" {
			return serviceDID
		}
	}
	t.Fatalf("ANPMessageService.serviceDid not found in %#v", didDocument["service"])
	return ""
}

func secureECDHPrivateKey(pemValue string) (*ecdh.PrivateKey, error) {
	privateKey, err := anp.PrivateKeyFromPEM(pemValue)
	if err != nil {
		return nil, err
	}
	return ecdh.X25519().NewPrivateKey(privateKey.Bytes)
}

func fixed32(t *testing.T, value []byte) [32]byte {
	t.Helper()
	if len(value) != 32 {
		t.Fatalf("len(value) = %d, want 32", len(value))
	}
	var result [32]byte
	copy(result[:], value)
	return result
}

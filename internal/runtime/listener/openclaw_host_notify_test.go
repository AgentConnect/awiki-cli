package listener

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/runtime/openclawnotify"
)

func TestBuildOpenClawHookRequestIncludesChannelDelivery(t *testing.T) {
	request, err := buildOpenClawHookRequest(HostNotificationEvent{
		Version:    "1.0",
		ID:         "msg-001",
		Topic:      "im.message.received",
		ReceivedAt: "2026-04-12T10:30:00Z",
		Data: DirectMessageNotificationData{
			Channel:        "direct",
			MessageID:      "msg-001",
			ConversationID: "conv-alice-bob",
			SenderDID:      "did:wba:example.com:user:alice:e1_alice",
			RecipientDID:   "did:wba:example.com:user:bob:e1_bob",
			ContentType:    "text/plain",
			Text:           "hello",
		},
	}, openclawnotify.FixedHookName, "telegram", "123456")
	if err != nil {
		t.Fatalf("buildOpenClawHookRequest() error = %v", err)
	}
	if !request.Deliver {
		t.Fatal("request.Deliver = false, want true")
	}
	if request.Channel != "telegram" {
		t.Fatalf("request.Channel = %q, want telegram", request.Channel)
	}
	if request.To != "123456" {
		t.Fatalf("request.To = %q, want 123456", request.To)
	}
	if request.WakeMode != "now" {
		t.Fatalf("request.WakeMode = %q, want now", request.WakeMode)
	}
	if !strings.Contains(request.Message, "You received a new im message from awiki.") {
		t.Fatalf("request.Message = %q, want Python-style prompt header", request.Message)
	}
}

func TestBuildOpenClawEventTextUsesMainAgentSessionFormat(t *testing.T) {
	text := buildOpenClawEventText(HostNotificationEvent{
		Version:    "1.0",
		ID:         "msg-001",
		Topic:      "im.message.received",
		ReceivedAt: "2026-04-12T10:30:00Z",
		Data: DirectMessageNotificationData{
			SenderHandle:    "alice",
			SenderDID:       "did:wba:example.com:user:alice:e1_alice",
			RecipientHandle: "bob",
			RecipientDID:    "did:wba:example.com:user:bob:e1_bob",
			CreatedAt:       "2026-04-07T00:00:00Z",
			Text:            "hello back",
		},
	})
	if !strings.Contains(text, "[Awiki New Direct Message]") {
		t.Fatalf("text = %q, want direct message header", text)
	}
	if !strings.Contains(text, "sender_did: did:wba:example.com:user:alice:e1_alice") {
		t.Fatalf("text = %q, want sender_did", text)
	}
	if !strings.Contains(text, "sender_handle: alice") {
		t.Fatalf("text = %q, want sender_handle", text)
	}
	if !strings.Contains(text, "recipient_handle: bob") {
		t.Fatalf("text = %q, want recipient_handle", text)
	}
	if !strings.Contains(text, "sent_at: 2026-04-07T00:00:00Z") {
		t.Fatalf("text = %q, want sent_at", text)
	}
}

func TestNewOpenClawHostNotifySinkRejectsNonLoopbackHookURL(t *testing.T) {
	root := t.TempDir()
	resolved := &appconfig.Resolved{
		Paths:                     appconfig.Paths{ConfigFile: filepath.Join(root, "config.yaml")},
		HostNotifySink:            "openclaw",
		HostNotifyOpenClawHookURL: "https://example.com/hooks/agent",
		Sources: map[string]appconfig.ValueSource{
			"host_notify_openclaw_hook_url": {Source: "config_file", Value: "https://example.com/hooks/agent"},
		},
	}
	_, err := newOpenClawHostNotifySink(resolved)
	if err == nil {
		t.Fatal("newOpenClawHostNotifySink() error = nil, want loopback validation error")
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("newOpenClawHostNotifySink() error = %q, want loopback", err.Error())
	}
}

func TestNewOpenClawHostNotifySinkAllowsEmptyToken(t *testing.T) {
	root := t.TempDir()
	resolved := &appconfig.Resolved{
		Paths:                     appconfig.Paths{ConfigFile: filepath.Join(root, "config.yaml")},
		HostNotifySink:            "openclaw",
		HostNotifyOpenClawHookURL: "http://127.0.0.1:18789/hooks/agent",
		Sources: map[string]appconfig.ValueSource{
			"host_notify_openclaw_hook_url": {Source: "config_file", Value: "http://127.0.0.1:18789/hooks/agent"},
		},
	}
	sink, err := newOpenClawHostNotifySink(resolved)
	if err != nil {
		t.Fatalf("newOpenClawHostNotifySink() error = %v", err)
	}
	if sink == nil {
		t.Fatal("sink = nil, want openclaw sink")
	}
}

func TestOpenClawHostNotifySinkNotifyUsesRouteRegistry(t *testing.T) {
	root := t.TempDir()
	paths := appconfig.Paths{
		ConfigFile:       filepath.Join(root, "config.yaml"),
		WorkspaceHomeDir: root,
		StateDir:         filepath.Join(root, "runtime"),
	}
	if err := openclawnotify.WriteRoutes(paths, []openclawnotify.Route{{Channel: "telegram", To: "123"}}); err != nil {
		t.Fatalf("WriteRoutes() error = %v", err)
	}

	var hookRequests []openclawnotify.HookRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body openclawnotify.HookRequest
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("io.ReadAll() error = %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		hookRequests = append(hookRequests, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"runId":"run-123"}`))
	}))
	defer server.Close()

	resolved := &appconfig.Resolved{
		Paths:                     paths,
		HostNotifyEnabled:         true,
		HostNotifySink:            "openclaw",
		HostNotifyOpenClawHookURL: server.URL,
		Sources: map[string]appconfig.ValueSource{
			"host_notify_openclaw_hook_url": {Source: "config_file", Value: server.URL},
		},
	}

	sink, err := newOpenClawHostNotifySink(resolved)
	if err != nil {
		t.Fatalf("newOpenClawHostNotifySink() error = %v", err)
	}
	defer sink.Close()

	err = sink.Notify(context.Background(), HostNotificationEvent{
		Version:    "1.0",
		ID:         "msg-001",
		Topic:      "im.message.received",
		ReceivedAt: time.Now().UTC().Format(time.RFC3339),
		Data: DirectMessageNotificationData{
			Channel:        "direct",
			MessageID:      "msg-001",
			ConversationID: "conv-alice-bob",
			SenderDID:      "did:wba:example.com:user:alice:e1_alice",
			RecipientDID:   "did:wba:example.com:user:bob:e1_bob",
			ContentType:    "text/plain",
			Text:           "hello",
		},
	})
	if err != nil {
		t.Fatalf("sink.Notify() error = %v", err)
	}
	if len(hookRequests) != 1 {
		t.Fatalf("len(hookRequests) = %d, want 1", len(hookRequests))
	}
	if !hookRequests[0].Deliver {
		t.Fatal("hookRequests[0].Deliver = false, want true")
	}
	if hookRequests[0].Channel != "telegram" {
		t.Fatalf("hookRequests[0].Channel = %q, want telegram", hookRequests[0].Channel)
	}
	if hookRequests[0].To != "123" {
		t.Fatalf("hookRequests[0].To = %q, want 123", hookRequests[0].To)
	}
}

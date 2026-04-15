package listener

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	runtimecfg "github.com/agentconnect/awiki-cli/internal/runtime"
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
	}, "AWiki", "telegram", "123456")
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

func TestParseOpenClawExternalChannelsFiltersHookAndMainSessions(t *testing.T) {
	nowMillis := time.Now().UnixMilli()
	channels, err := parseOpenClawExternalChannels(fmt.Sprintf(`plugin log line
{"sessions":{"recent":[{"key":"agent:main:telegram:user:123","updatedAt":%d},{"key":"agent:main:main","updatedAt":%d},{"key":"hook:awiki:dm:abcd","updatedAt":%d},{"key":"agent:main:slack:user:alice","updatedAt":%d}]}}`, nowMillis, nowMillis, nowMillis, nowMillis-1000))
	if err != nil {
		t.Fatalf("parseOpenClawExternalChannels() error = %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("len(channels) = %d, want 2", len(channels))
	}
	if channels[0].Channel != "telegram" || channels[0].Target != "123" {
		t.Fatalf("channels[0] = %#v, want telegram:123", channels[0])
	}
	if channels[1].Channel != "slack" || channels[1].Target != "alice" {
		t.Fatalf("channels[1] = %#v, want slack:alice", channels[1])
	}
}

func TestNewOpenClawHostNotifySinkRejectsNonLoopbackHookURL(t *testing.T) {
	_, err := newOpenClawHostNotifySink(&appconfig.Resolved{}, runtimecfg.OpenClawConfig{
		HookURL:  "https://example.com/hooks/agent",
		AgentID:  "main",
		HookName: "AWiki",
	})
	if err == nil {
		t.Fatal("newOpenClawHostNotifySink() error = nil, want loopback validation error")
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("newOpenClawHostNotifySink() error = %q, want loopback", err.Error())
	}
}

func TestNewOpenClawHostNotifySinkAllowsEmptyToken(t *testing.T) {
	sink, err := newOpenClawHostNotifySink(&appconfig.Resolved{}, runtimecfg.OpenClawConfig{
		HookURL:  "http://127.0.0.1:18789/hooks/agent",
		AgentID:  "main",
		HookName: "AWiki",
	})
	if err != nil {
		t.Fatalf("newOpenClawHostNotifySink() error = %v", err)
	}
	if sink == nil {
		t.Fatal("sink = nil, want openclaw sink")
	}
}

func TestOpenClawHostNotifySinkNotifyUsesChatInjectAndExternalChannels(t *testing.T) {
	var hookRequests []openClawHookRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body openClawHookRequest
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("io.ReadAll() error = %v", err)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		hookRequests = append(hookRequests, body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	originalFind := findOpenClawBinary
	originalRun := runOpenClawCommand
	defer func() {
		findOpenClawBinary = originalFind
		runOpenClawCommand = originalRun
	}()
	findOpenClawBinary = func() (string, error) { return "/usr/local/bin/openclaw", nil }
	var seenInject bool
	var seenStatus bool
	runOpenClawCommand = func(_ context.Context, _ string, args ...string) (openClawCLIResult, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "chat.inject"):
			seenInject = true
			if !strings.Contains(joined, `agent:main:main`) {
				return openClawCLIResult{}, fmt.Errorf("chat.inject args = %q, want agent:main:main", joined)
			}
			return openClawCLIResult{Stdout: `{"ok":true}`, ExitCode: 0}, nil
		case strings.Contains(joined, "status --json"):
			seenStatus = true
			nowMillis := time.Now().UnixMilli()
			return openClawCLIResult{Stdout: fmt.Sprintf(`{"sessions":{"recent":[{"key":"agent:main:telegram:user:123","updatedAt":%d}]}}`, nowMillis), ExitCode: 0}, nil
		default:
			return openClawCLIResult{}, fmt.Errorf("unexpected openclaw command: %s", joined)
		}
	}

	t.Setenv(openClawHookTokenEnv, "token-123")
	sink, err := newOpenClawHostNotifySink(&appconfig.Resolved{}, runtimecfg.OpenClawConfig{
		HookURL:  server.URL,
		AgentID:  "main",
		HookName: "AWiki",
	})
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
	if !seenInject {
		t.Fatal("chat.inject was not called")
	}
	if !seenStatus {
		t.Fatal("gateway status was not called")
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
	if strings.Contains(mustJSON(t, hookRequests[0]), "agentId") {
		t.Fatalf("hook request json = %s, should not include agentId", mustJSON(t, hookRequests[0]))
	}
	if strings.Contains(mustJSON(t, hookRequests[0]), "sessionKey") {
		t.Fatalf("hook request json = %s, should not include sessionKey", mustJSON(t, hookRequests[0]))
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return string(raw)
}

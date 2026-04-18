package listener

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	runtimecfg "github.com/agentconnect/awiki-cli/internal/runtime"
)

func TestNewHermesHostNotifySinkRejectsInvalidNotifyURL(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("runtime:\n  host_notify:\n    hermes:\n      secret: test-secret\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	_, err := newHermesHostNotifySink(&appconfig.Resolved{
		Paths: appconfig.Paths{ConfigFile: configPath},
	}, runtimecfg.HermesConfig{
		NotifyURL: "ftp://example.com/notify",
	})
	if err == nil {
		t.Fatal("newHermesHostNotifySink() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "must use http or https") {
		t.Fatalf("newHermesHostNotifySink() error = %q, want scheme validation", err.Error())
	}
}

func TestHermesHostNotifySinkNotifySignsRequest(t *testing.T) {
	t.Parallel()

	var capturedHeaders http.Header
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("runtime:\n  host_notify:\n    hermes:\n      secret: test-secret\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	sink, err := newHermesHostNotifySink(&appconfig.Resolved{
		Paths: appconfig.Paths{ConfigFile: configPath},
	}, runtimecfg.HermesConfig{
		NotifyURL: server.URL + "/notify/host-event",
	})
	if err != nil {
		t.Fatalf("newHermesHostNotifySink() error = %v", err)
	}
	defer sink.Close()

	event := HostNotificationEvent{
		Version:    "1.0",
		ID:         "msg-001",
		Topic:      "im.message.received",
		ReceivedAt: "2026-04-12T10:30:00Z",
		Data: map[string]any{
			"recipient_did": "did:wba:test:bob",
			"sender_did":    "did:wba:test:alice",
		},
	}
	if err := sink.Notify(context.Background(), event); err != nil {
		t.Fatalf("sink.Notify() error = %v", err)
	}
	if capturedHeaders.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", capturedHeaders.Get("Content-Type"))
	}
	timestamp := capturedHeaders.Get("X-Notify-Timestamp")
	if strings.TrimSpace(timestamp) == "" {
		t.Fatal("X-Notify-Timestamp is empty")
	}
	gotSignature := strings.TrimPrefix(capturedHeaders.Get("X-Notify-Signature"), "sha256=")
	if gotSignature == "" {
		t.Fatal("X-Notify-Signature is empty")
	}
	var posted HostNotificationEvent
	if err := json.Unmarshal(capturedBody, &posted); err != nil {
		t.Fatalf("json.Unmarshal(capturedBody) error = %v", err)
	}
	if posted.ID != "msg-001" {
		t.Fatalf("posted.ID = %q, want msg-001", posted.ID)
	}
	expectedMAC := hmac.New(sha256.New, []byte("test-secret"))
	expectedMAC.Write(append([]byte(timestamp+"."), capturedBody...))
	expectedSignature := hex.EncodeToString(expectedMAC.Sum(nil))
	if gotSignature != expectedSignature {
		t.Fatalf("signature = %q, want %q", gotSignature, expectedSignature)
	}
}

package openclawnotify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookClientRequiresRunID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"runId":"run-123"}`))
	}))
	defer server.Close()

	client, err := NewWebhookClient(server.URL, "")
	if err != nil {
		t.Fatalf("NewWebhookClient() error = %v", err)
	}
	response, err := client.Send(context.Background(), HookRequest{
		Message: "hello",
		Deliver: true,
		Channel: "feishu",
		To:      "ou_123",
	})
	if err != nil {
		t.Fatalf("client.Send() error = %v", err)
	}
	if response.RunID != "run-123" {
		t.Fatalf("response.RunID = %q, want run-123", response.RunID)
	}
}

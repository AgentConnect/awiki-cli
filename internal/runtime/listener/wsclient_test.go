package listener

import (
	"testing"

	"github.com/agentconnect/awiki-cli/internal/authsdk"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func TestNewWSClientDoesNotDoubleAppendWSEndpoint(t *testing.T) {
	t.Parallel()

	client, err := NewWSClient(&appconfig.Resolved{
		MessageServiceURL:   "http://127.0.0.1:18080",
		MessageServiceWSURL: "ws://127.0.0.1:18080/ws",
	}, dummyAuthSession())
	if err != nil {
		t.Fatalf("NewWSClient() error = %v", err)
	}
	if client.websocketURL != "ws://127.0.0.1:18080/ws" {
		t.Fatalf("client.websocketURL = %q, want ws://127.0.0.1:18080/ws", client.websocketURL)
	}
	if client.requestURL != "http://127.0.0.1:18080/ws" {
		t.Fatalf("client.requestURL = %q, want http://127.0.0.1:18080/ws", client.requestURL)
	}
}

func TestAppendEndpointIfMissing(t *testing.T) {
	t.Parallel()

	if got := appendEndpointIfMissing("ws://127.0.0.1:18080", "/ws"); got != "ws://127.0.0.1:18080/ws" {
		t.Fatalf("appendEndpointIfMissing() = %q", got)
	}
	if got := appendEndpointIfMissing("ws://127.0.0.1:18080/ws", "/ws"); got != "ws://127.0.0.1:18080/ws" {
		t.Fatalf("appendEndpointIfMissing() = %q", got)
	}
}

func dummyAuthSession() *authsdk.Session {
	return authsdk.NewSession("", "", "alice", "did:wba:example.com:user:alice", "token", nil)
}

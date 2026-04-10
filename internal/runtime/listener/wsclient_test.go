package listener

import (
	"testing"

	"github.com/agentconnect/awiki-cli/internal/authsdk"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func TestNewWSClientDerivesIMWebSocketEndpointFromServiceBaseURL(t *testing.T) {
	t.Parallel()

	client, err := NewWSClient(&appconfig.Resolved{
		ServiceBaseURL: "http://127.0.0.1:18080",
	}, dummyAuthSession())
	if err != nil {
		t.Fatalf("NewWSClient() error = %v", err)
	}
	if client.websocketURL != "ws://127.0.0.1:18080/im/ws" {
		t.Fatalf("client.websocketURL = %q, want ws://127.0.0.1:18080/im/ws", client.websocketURL)
	}
	if client.requestURL != "http://127.0.0.1:18080/im/ws" {
		t.Fatalf("client.requestURL = %q, want http://127.0.0.1:18080/im/ws", client.requestURL)
	}
}

func dummyAuthSession() *authsdk.Session {
	return authsdk.NewSession("", "", "alice", "did:wba:example.com:user:alice", "token", nil)
}

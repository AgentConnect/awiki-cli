package listener

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentconnect/awiki-cli/internal/authsdk"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/coder/websocket"
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

func TestWSClientConnectRefreshesExpiredBearerBeforeRetryingWebSocket(t *testing.T) {
	t.Parallel()

	var wsAttempts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/im/ws":
			wsAttempts = append(wsAttempts, r.Header.Get("Authorization"))
			if r.Header.Get("Authorization") != "Bearer refreshed-token" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":1401,"message":"expired session"}}`))
				return
			}
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				t.Fatalf("websocket.Accept() error = %v", err)
			}
			defer conn.Close(websocket.StatusNormalClosure, "done")
			<-r.Context().Done()
		case "/user-service/did-auth/rpc":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"req-1","result":{"did":"did:wba:awiki.ai:user:alice:e1_alice","access_token":"refreshed-token"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolved := testResolvedConfig(t, server.URL)
	client, authSession := testWSClientWithIdentity(t, resolved, "expired-token")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()

	if got := authSession.CurrentJWT(); got != "refreshed-token" {
		t.Fatalf("CurrentJWT() = %q, want %q", got, "refreshed-token")
	}
	if len(wsAttempts) != 2 {
		t.Fatalf("websocket attempts = %#v, want 2 attempts", wsAttempts)
	}
	if wsAttempts[0] != "Bearer expired-token" {
		t.Fatalf("first websocket attempt = %q, want expired bearer", wsAttempts[0])
	}
	if wsAttempts[1] != "Bearer refreshed-token" {
		t.Fatalf("second websocket attempt = %q, want refreshed bearer", wsAttempts[1])
	}
}

func TestWSClientConnectBootstrapsBearerBeforeOpeningWebSocket(t *testing.T) {
	t.Parallel()

	var wsAttempts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/im/ws":
			wsAttempts = append(wsAttempts, r.Header.Get("Authorization"))
			if r.Header.Get("Authorization") != "Bearer bootstrapped-token" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":1401,"message":"missing session"}}`))
				return
			}
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				t.Fatalf("websocket.Accept() error = %v", err)
			}
			defer conn.Close(websocket.StatusNormalClosure, "done")
			<-r.Context().Done()
		case "/user-service/did-auth/rpc":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"req-1","result":{"did":"did:wba:awiki.ai:user:alice:e1_alice","access_token":"bootstrapped-token"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolved := testResolvedConfig(t, server.URL)
	client, authSession := testWSClientWithIdentity(t, resolved, "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()

	if got := authSession.CurrentJWT(); got != "bootstrapped-token" {
		t.Fatalf("CurrentJWT() = %q, want %q", got, "bootstrapped-token")
	}
	if len(wsAttempts) != 1 || wsAttempts[0] != "Bearer bootstrapped-token" {
		t.Fatalf("websocket attempts = %#v, want one bootstrapped bearer attempt", wsAttempts)
	}
}

func testWSClientWithIdentity(t *testing.T, resolved *appconfig.Resolved, jwt string) (*WSClient, *authsdk.Session) {
	t.Helper()

	manager := identity.NewManager(resolved.Paths)
	record, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("manager.Load() error = %v", err)
	}
	record.JWTToken = jwt
	paths, err := manager.PathsForIdentity("alice")
	if err != nil {
		t.Fatalf("manager.PathsForIdentity() error = %v", err)
	}
	session := authsdk.NewSession(
		paths.DIDDocumentPath,
		paths.Key1PrivatePath,
		record.IdentityName,
		record.DID,
		record.JWTToken,
		nil,
	)
	if jwt != "" {
		session.SetBearer(resolved.ServiceBaseURL, jwt)
		session.SetBearer(appconfig.JoinBaseURL(resolved.ServiceBaseURL, "/user-service/did-auth/rpc"), jwt)
		session.SetBearer(appconfig.JoinBaseURL(resolved.ServiceBaseURL, "/im/ws"), jwt)
	}
	client, err := NewWSClient(resolved, session)
	if err != nil {
		t.Fatalf("NewWSClient() error = %v", err)
	}
	return client, session
}

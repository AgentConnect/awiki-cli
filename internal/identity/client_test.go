package identity_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
)

func newRemoteClientTestServer(t *testing.T, handler http.HandlerFunc) *identity.RemoteClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := identity.NewRemoteClient(&appconfig.Resolved{ServiceBaseURL: server.URL})
	if err != nil {
		t.Fatalf("identity.NewRemoteClient() error = %v", err)
	}
	return client
}

func TestNewRemoteClientRejectsNilResolvedConfig(t *testing.T) {
	t.Parallel()

	_, err := identity.NewRemoteClient(nil)
	if !errors.Is(err, identity.ErrInvalidInput) {
		t.Fatalf("identity.NewRemoteClient(nil) error = %v, want ErrInvalidInput", err)
	}
}

func TestRemoteClientTranslatesRPCAndRESTErrors(t *testing.T) {
	t.Parallel()

	client := newRemoteClientTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rpc-error":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32077,"message":"rpc exploded"},"id":"req-1"}`))
		case "/post-error":
			http.Error(w, "backend unavailable", http.StatusBadGateway)
		case "/get-success":
			if got := r.URL.Query().Get("email"); got != "alice@example.com" {
				t.Fatalf("r.URL.Query().Get(email) = %q, want alice@example.com", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"verified":true}`))
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	})

	var rpcOut map[string]any
	err := client.RPCCall(context.Background(), "/rpc-error", "lookup", map[string]any{"did": "did:peer"}, "", &rpcOut)
	var serviceErr *identity.ServiceError
	if !errors.As(err, &serviceErr) {
		t.Fatalf("client.RPCCall() error = %v, want *identity.ServiceError", err)
	}
	if serviceErr.RPCCode != -32077 || serviceErr.StatusCode != 0 {
		t.Fatalf("client.RPCCall() service error = %+v, want rpc_code=-32077 status=0", serviceErr)
	}

	err = client.RestPost(context.Background(), "/post-error", map[string]any{"email": "alice@example.com"}, "", nil)
	serviceErr = nil
	if !errors.As(err, &serviceErr) {
		t.Fatalf("client.RestPost() error = %v, want *identity.ServiceError", err)
	}
	if serviceErr.StatusCode != http.StatusBadGateway || serviceErr.RPCCode != 0 {
		t.Fatalf("client.RestPost() service error = %+v, want status=%d rpc_code=0", serviceErr, http.StatusBadGateway)
	}

	var getOut struct {
		Verified bool `json:"verified"`
	}
	if err := client.RestGet(context.Background(), "/get-success", url.Values{"email": {"alice@example.com"}}, &getOut); err != nil {
		t.Fatalf("client.RestGet() error = %v", err)
	}
	if !getOut.Verified {
		t.Fatalf("client.RestGet() output = %+v, want verified=true", getOut)
	}
}

func TestLookupHandleByDIDHandlesNotFoundEmptyAndSuccess(t *testing.T) {
	t.Parallel()

	t.Run("blank_did", func(t *testing.T) {
		t.Parallel()

		client := newRemoteClientTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			t.Fatalf("unexpected request path %q", r.URL.Path)
		})
		result, err := client.LookupHandleByDID(context.Background(), "   ")
		if !errors.Is(err, identity.ErrInvalidInput) {
			t.Fatalf("client.LookupHandleByDID(blank) error = %v, want ErrInvalidInput", err)
		}
		if result != nil {
			t.Fatalf("client.LookupHandleByDID(blank) result = %#v, want nil", result)
		}
	})

	t.Run("http_not_found", func(t *testing.T) {
		t.Parallel()

		client := newRemoteClientTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "missing", http.StatusNotFound)
		})
		result, err := client.LookupHandleByDID(context.Background(), "did:peer")
		if err != nil {
			t.Fatalf("client.LookupHandleByDID() error = %v", err)
		}
		if result != nil {
			t.Fatalf("client.LookupHandleByDID() result = %#v, want nil", result)
		}
	})

	t.Run("rpc_not_found", func(t *testing.T) {
		t.Parallel()

		client := newRemoteClientTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32002,"message":"not found"},"id":"req-1"}`))
		})
		result, err := client.LookupHandleByDID(context.Background(), "did:peer")
		if err != nil {
			t.Fatalf("client.LookupHandleByDID() error = %v", err)
		}
		if result != nil {
			t.Fatalf("client.LookupHandleByDID() result = %#v, want nil", result)
		}
	})

	t.Run("empty_payload", func(t *testing.T) {
		t.Parallel()

		client := newRemoteClientTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"handle":"","did":"did:peer"},"id":"req-1"}`))
		})
		result, err := client.LookupHandleByDID(context.Background(), "did:peer")
		if err != nil {
			t.Fatalf("client.LookupHandleByDID() error = %v", err)
		}
		if result != nil {
			t.Fatalf("client.LookupHandleByDID() result = %#v, want nil", result)
		}
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		client := newRemoteClientTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"handle":"alice","did":"did:peer","domain":"awiki.test","full_handle":"alice.awiki.test","status":"active"},"id":"req-1"}`))
		})
		result, err := client.LookupHandleByDID(context.Background(), "did:peer")
		if err != nil {
			t.Fatalf("client.LookupHandleByDID() error = %v", err)
		}
		if result == nil {
			t.Fatal("client.LookupHandleByDID() result = nil, want non-nil")
		}
		if result.Handle != "alice" || result.FullHandle != "alice.awiki.test" {
			t.Fatalf("client.LookupHandleByDID() result = %+v, want alice/alice.awiki.test", result)
		}
	})
}

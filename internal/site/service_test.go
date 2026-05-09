package site

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
)

func TestGetRootCallsSiteRPC(t *testing.T) {
	t.Parallel()

	var (
		gotMethod string
		gotParams map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != siteRPCEndpoint {
			t.Fatalf("r.URL.Path = %q, want %q", r.URL.Path, siteRPCEndpoint)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		gotMethod, _ = payload["method"].(string)
		gotParams, _ = payload["params"].(map[string]any)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"domain":"tenant.example","kind":"root","body":"Welcome"},"id":"req-1"}`))
	}))
	defer server.Close()

	service := newTestService(t, server.URL, "token-123")
	result, err := service.GetRoot(context.Background(), "Tenant.Example.")
	if err != nil {
		t.Fatalf("GetRoot() error = %v", err)
	}
	if gotMethod != "get_root" {
		t.Fatalf("rpc method = %q, want get_root", gotMethod)
	}
	if got, _ := gotParams["domain"].(string); got != "tenant.example" {
		t.Fatalf("params.domain = %q, want tenant.example", got)
	}
	root, _ := result.Data["root"].(map[string]any)
	if got, _ := root["kind"].(string); got != "root" {
		t.Fatalf("root.kind = %q, want root", got)
	}
}

func TestDeletePageMapsRPCError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32001,"message":"forbidden"},"id":"req-1"}`))
	}))
	defer server.Close()

	service := newTestService(t, server.URL, "token-123")
	_, err := service.DeletePage(context.Background(), "tenant.example", "hello")
	if err == nil {
		t.Fatal("DeletePage() error = nil, want rpc error")
	}
	serviceErr, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("DeletePage() error = %T, want *ServiceError", err)
	}
	if serviceErr.RPCCode != -32001 {
		t.Fatalf("serviceErr.RPCCode = %d, want -32001", serviceErr.RPCCode)
	}
}

func TestNormalizeDomainRejectsURLs(t *testing.T) {
	t.Parallel()

	if _, err := normalizeDomain("https://tenant.example"); err == nil {
		t.Fatal("normalizeDomain() error = nil, want invalid domain")
	}
}

func newTestService(t *testing.T, userServiceURL string, jwtToken string) *Service {
	t.Helper()

	root := t.TempDir()
	resolved := &appconfig.Resolved{
		Paths: appconfig.Paths{
			IdentityDir:          filepath.Join(root, "identities"),
			LegacyCredentialsDir: filepath.Join(root, "legacy"),
			DataDir:              filepath.Join(root, "data"),
			StateDir:             filepath.Join(root, "state"),
			DatabaseFile:         filepath.Join(root, "data", "awiki-cli.db"),
		},
		ServiceBaseURL: userServiceURL,
		DIDDomain:      "awiki.ai",
		ActiveIdentity: "alice",
	}
	manager := identity.NewManager(resolved.Paths)
	createTestIdentity(t, manager, identity.SaveInput{
		IdentityName: "alice",
		DisplayName:  "Alice",
		Handle:       "alice",
		JWTToken:     jwtToken,
	})
	service, err := NewService(resolved)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
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
	input.DID = generated.DID
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

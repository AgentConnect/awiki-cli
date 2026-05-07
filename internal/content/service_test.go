package content

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/testenv"
)

func TestCreatePageCallsContentRPC(t *testing.T) {
	t.Parallel()

	var (
		gotMethod string
		gotAuth   string
		gotParams map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != contentRPCEndpoint {
			t.Fatalf("r.URL.Path = %q, want %q", r.URL.Path, contentRPCEndpoint)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		gotMethod, _ = payload["method"].(string)
		gotParams, _ = payload["params"].(map[string]any)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"slug":"hello-world","title":"Hello","visibility":"draft"},"id":"req-1"}`))
	}))
	defer server.Close()

	service := newTestService(t, server.URL, "token-123")
	result, err := service.CreatePage(context.Background(), CreatePageParams{
		Slug:       "hello-world",
		Title:      "Hello",
		Body:       "# Hello",
		Visibility: "draft",
	})
	if err != nil {
		t.Fatalf("CreatePage() error = %v", err)
	}
	if gotMethod != "create" {
		t.Fatalf("rpc method = %q, want create", gotMethod)
	}
	if gotAuth != "Bearer token-123" {
		t.Fatalf("Authorization = %q, want Bearer token-123", gotAuth)
	}
	if got, _ := gotParams["visibility"].(string); got != "draft" {
		t.Fatalf("params.visibility = %q, want draft", got)
	}
	page, _ := result.Data["page"].(map[string]any)
	if got, _ := page["slug"].(string); got != "hello-world" {
		t.Fatalf("page.slug = %q, want hello-world", got)
	}
}

func TestUpdatePageRejectsEmptyMutation(t *testing.T) {
	t.Parallel()

	service := newTestService(t, testenv.BaseURL(), "token-123")
	_, err := service.UpdatePage(context.Background(), UpdatePageParams{Slug: "hello-world"})
	if err != ErrNoUpdateFields {
		t.Fatalf("UpdatePage() error = %v, want %v", err, ErrNoUpdateFields)
	}
}

func TestDeletePageMapsRPCError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32004,"message":"slug already exists"},"id":"req-1"}`))
	}))
	defer server.Close()

	service := newTestService(t, server.URL, "token-123")
	_, err := service.DeletePage(context.Background(), "hello-world")
	if err == nil {
		t.Fatal("DeletePage() error = nil, want rpc error")
	}
	serviceErr, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("DeletePage() error = %T, want *ServiceError", err)
	}
	if serviceErr.RPCCode != -32004 {
		t.Fatalf("serviceErr.RPCCode = %d, want -32004", serviceErr.RPCCode)
	}
}

func TestNormalizeVisibility(t *testing.T) {
	t.Parallel()

	got, err := normalizeVisibility("", false)
	if err != nil || got != "public" {
		t.Fatalf("normalizeVisibility('', false) = (%q, %v), want (public, nil)", got, err)
	}
	got, err = normalizeVisibility("UNLISTED", false)
	if err != nil || got != "unlisted" {
		t.Fatalf("normalizeVisibility('UNLISTED', false) = (%q, %v), want (unlisted, nil)", got, err)
	}
	if _, err := normalizeVisibility("private", false); err != ErrVisibilityInvalid {
		t.Fatalf("normalizeVisibility('private', false) error = %v, want %v", err, ErrVisibilityInvalid)
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

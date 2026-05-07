package identity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/testenv"
)

func newIdentityServiceWorkspace(t *testing.T, serviceBaseURL string) (*appconfig.Resolved, *identity.Manager) {
	t.Helper()
	root := t.TempDir()
	resolved := &appconfig.Resolved{
		Paths: appconfig.Paths{
			WorkspaceHomeDir:     filepath.Join(root, ".awiki-cli"),
			ConfigDir:            filepath.Join(root, ".awiki-cli"),
			IdentityDir:          filepath.Join(root, ".awiki-cli", "identities"),
			DataDir:              filepath.Join(root, ".awiki-cli", "data"),
			StateDir:             filepath.Join(root, ".awiki-cli", "runtime"),
			CacheDir:             filepath.Join(root, ".awiki-cli", "cache"),
			DatabaseFile:         filepath.Join(root, ".awiki-cli", "data", "awiki-cli.db"),
			LegacyCredentialsDir: filepath.Join(root, "legacy"),
			LegacyDataDir:        filepath.Join(root, "legacy-data"),
		},
		ServiceBaseURL:     serviceBaseURL,
		DIDDomain:          testenv.Domain(),
		ANPServiceEndpoint: testenv.BaseURL() + "/anp-im/rpc",
		ANPServiceDID:      testenv.ServiceDID(),
		OutputFormat:       "json",
	}
	return resolved, identity.NewManager(resolved.Paths)
}

func saveServiceTestIdentity(t *testing.T, manager *identity.Manager, identityName string, handle string, jwtToken string) *identity.StoredIdentity {
	t.Helper()
	generated, err := identity.GenerateIdentity(identity.GenerateOptions{
		Hostname:           testenv.Domain(),
		PathPrefix:         []string{handle},
		ProofDomain:        testenv.Domain(),
		ANPServiceEndpoint: testenv.BaseURL() + "/anp-im/rpc",
		ANPServiceDID:      testenv.ServiceDID(),
	})
	if err != nil {
		t.Fatalf("identity.GenerateIdentity() error = %v", err)
	}
	record, err := manager.Save(identity.SaveInput{
		IdentityName:            identityName,
		DID:                     generated.DID,
		UniqueID:                generated.UniqueID,
		UserID:                  "user-1",
		DisplayName:             "Alice",
		Handle:                  handle,
		JWTToken:                jwtToken,
		DIDDocument:             generated.DIDDocument,
		Key1PrivatePEM:          generated.Key1PrivatePEM,
		Key1PublicPEM:           generated.Key1PublicPEM,
		E2EESigningPrivatePEM:   generated.E2EESigningPrivatePEM,
		E2EEAgreementPrivatePEM: generated.E2EEAgreementPrivatePEM,
	})
	if err != nil {
		t.Fatalf("manager.Save() error = %v", err)
	}
	return record
}

func TestServiceRegisterPhoneSendsNormalizedOTPRequest(t *testing.T) {
	t.Parallel()

	var (
		gotMethod string
		gotPhone  string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user-service/handle/rpc" {
			t.Fatalf("r.URL.Path = %q, want /user-service/handle/rpc", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("json.NewDecoder().Decode() error = %v", err)
		}
		gotMethod, _ = payload["method"].(string)
		params, _ := payload["params"].(map[string]any)
		gotPhone, _ = params["phone"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"sent":true},"id":"req-1"}`))
	}))
	defer server.Close()

	resolved, manager := newIdentityServiceWorkspace(t, server.URL)
	service, err := identity.NewService(resolved)
	if err != nil {
		t.Fatalf("identity.NewService() error = %v", err)
	}

	result, err := service.Register(context.Background(), identity.RegisterParams{
		Handle: "alice",
		Phone:  "13800138000",
	})
	if err != nil {
		t.Fatalf("Service.Register() error = %v", err)
	}
	if gotMethod != "send_otp" {
		t.Fatalf("rpc method = %q, want send_otp", gotMethod)
	}
	if gotPhone != "+8613800138000" {
		t.Fatalf("rpc phone = %q, want +8613800138000", gotPhone)
	}
	if got := result.Data["verification_state"]; got != "otp_sent" {
		t.Fatalf("result.Data[verification_state] = %#v, want otp_sent", got)
	}
	if got := result.Data["identity_name"]; got != "alice" {
		t.Fatalf("result.Data[identity_name] = %#v, want alice", got)
	}
	identities, err := manager.List()
	if err != nil {
		t.Fatalf("manager.List() error = %v", err)
	}
	if len(identities) != 0 {
		t.Fatalf("manager.List() = %#v, want no local identities before OTP verification", identities)
	}
}

func TestServiceRegisterEmailVerifiedCreatesIdentity(t *testing.T) {
	t.Parallel()

	var (
		statusQueryEmail string
		registerMethod   string
		registerEmail    string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user-service/auth/email-status":
			statusQueryEmail = r.URL.Query().Get("email")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"email":"alice@example.com","verified":true,"verified_at":"2026-01-01T00:00:00Z"}`))
		case "/user-service/did-auth/rpc":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("json.NewDecoder().Decode() error = %v", err)
			}
			registerMethod, _ = payload["method"].(string)
			params, _ := payload["params"].(map[string]any)
			registerEmail, _ = params["email"].(string)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"user_id":"user-1","access_token":"jwt-1","handle":"alice"},"id":"req-1"}`))
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	resolved, manager := newIdentityServiceWorkspace(t, server.URL)
	service, err := identity.NewService(resolved)
	if err != nil {
		t.Fatalf("identity.NewService() error = %v", err)
	}

	result, err := service.Register(context.Background(), identity.RegisterParams{
		Handle: "alice",
		Email:  "  Alice@Example.COM  ",
	})
	if err != nil {
		t.Fatalf("Service.Register() error = %v", err)
	}
	if statusQueryEmail != "alice@example.com" {
		t.Fatalf("email status query = %q, want alice@example.com", statusQueryEmail)
	}
	if registerMethod != "register" {
		t.Fatalf("register rpc method = %q, want register", registerMethod)
	}
	if registerEmail != "alice@example.com" {
		t.Fatalf("register rpc email = %q, want alice@example.com", registerEmail)
	}

	identityData, _ := result.Data["identity"].(*identity.IdentitySummary)
	if identityData == nil {
		t.Fatalf("result.Data[identity] = %#v, want *identity.IdentitySummary", result.Data["identity"])
	}
	if identityData.Handle != "alice" || !identityData.HasJWT || !identityData.UserState.ReadyForMessaging {
		t.Fatalf("identity summary = %#v, want handle alice with jwt and ready state", identityData)
	}

	stored, err := manager.Load("alice")
	if err != nil {
		t.Fatalf("manager.Load() error = %v", err)
	}
	if stored.Handle != "alice" || stored.JWTToken != "jwt-1" {
		t.Fatalf("stored identity = %#v, want handle alice and jwt-1", stored)
	}
}

func TestServiceRegisterFullHandleUsesExplicitDomainForDID(t *testing.T) {
	t.Parallel()

	var (
		registerMethod string
		registerHandle string
		registerDID    string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user-service/auth/email-status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"email":"alice@example.com","verified":true,"verified_at":"2026-01-01T00:00:00Z"}`))
		case "/user-service/did-auth/rpc":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("json.NewDecoder().Decode() error = %v", err)
			}
			registerMethod, _ = payload["method"].(string)
			params, _ := payload["params"].(map[string]any)
			registerHandle, _ = params["handle"].(string)
			document, _ := params["did_document"].(map[string]any)
			registerDID, _ = document["id"].(string)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"user_id":"user-1","access_token":"jwt-1","handle":"alice","full_handle":"alice.partner.test"},"id":"req-1"}`))
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	resolved, manager := newIdentityServiceWorkspace(t, server.URL)
	service, err := identity.NewService(resolved)
	if err != nil {
		t.Fatalf("identity.NewService() error = %v", err)
	}

	result, err := service.Register(context.Background(), identity.RegisterParams{
		Handle: "alice.partner.test",
		Email:  "alice@example.com",
	})
	if err != nil {
		t.Fatalf("Service.Register() error = %v", err)
	}
	if registerMethod != "register" {
		t.Fatalf("register rpc method = %q, want register", registerMethod)
	}
	if registerHandle != "alice" {
		t.Fatalf("register handle = %q, want alice", registerHandle)
	}
	if want := "did:wba:partner.test:alice:"; !strings.HasPrefix(registerDID, want) {
		t.Fatalf("register did = %q, want prefix %q", registerDID, want)
	}

	identityData, _ := result.Data["identity"].(*identity.IdentitySummary)
	if identityData == nil || identityData.Handle != "alice" || identityData.FullHandle != "alice.partner.test" {
		t.Fatalf("identity summary = %#v, want alice/alice.partner.test", identityData)
	}
	stored, err := manager.Load("alice.partner.test")
	if err != nil {
		t.Fatalf("manager.Load() error = %v", err)
	}
	if stored.Handle != "alice" || stored.FullHandle != "alice.partner.test" {
		t.Fatalf("stored identity = %#v, want alice/alice.partner.test", stored)
	}
}

func TestServiceBindPhoneUsesAuthenticatedRequestAndSanitizesOTP(t *testing.T) {
	t.Parallel()

	var (
		gotAuth  string
		gotPhone string
		gotCode  string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user-service/auth/phone-bind-verify" {
			t.Fatalf("r.URL.Path = %q, want /user-service/auth/phone-bind-verify", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("json.NewDecoder().Decode() error = %v", err)
		}
		gotPhone, _ = payload["phone"].(string)
		gotCode, _ = payload["code"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"bound":true}`))
	}))
	defer server.Close()

	resolved, manager := newIdentityServiceWorkspace(t, server.URL)
	resolved.ActiveIdentity = "alice"
	saveServiceTestIdentity(t, manager, "alice", "alice", "jwt-legacy")

	service, err := identity.NewService(resolved)
	if err != nil {
		t.Fatalf("identity.NewService() error = %v", err)
	}
	result, err := service.Bind(context.Background(), identity.BindParams{
		Phone: "13800138000",
		OTP:   " 12 34 56 ",
	})
	if err != nil {
		t.Fatalf("Service.Bind() error = %v", err)
	}
	if gotAuth != "Bearer jwt-legacy" {
		t.Fatalf("Authorization = %q, want Bearer jwt-legacy", gotAuth)
	}
	if gotPhone != "+8613800138000" {
		t.Fatalf("phone payload = %q, want +8613800138000", gotPhone)
	}
	if gotCode != "123456" {
		t.Fatalf("code payload = %q, want 123456", gotCode)
	}
	if got := result.Data["verification_state"]; got != "completed" {
		t.Fatalf("result.Data[verification_state] = %#v, want completed", got)
	}
}

func TestServiceResolveByDIDReturnsWarningsForNonFatalLookupFailures(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user-service/handle/rpc":
			http.Error(w, "lookup unavailable", http.StatusBadGateway)
		case "/user-service/did/profile/rpc":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("json.NewDecoder().Decode() error = %v", err)
			}
			method, _ := payload["method"].(string)
			if method == "resolve" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"did":"` + testenv.DID("user", "alice") + `"},"id":"req-1"}`))
				return
			}
			http.Error(w, "profile unavailable", http.StatusBadGateway)
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	resolved, _ := newIdentityServiceWorkspace(t, server.URL)
	service, err := identity.NewService(resolved)
	if err != nil {
		t.Fatalf("identity.NewService() error = %v", err)
	}

	result, err := service.Resolve(context.Background(), "", testenv.DID("user", "alice"))
	if err != nil {
		t.Fatalf("Service.Resolve() error = %v", err)
	}
	if len(result.Warnings) != 2 {
		t.Fatalf("len(result.Warnings) = %d, want 2: %#v", len(result.Warnings), result.Warnings)
	}
	if _, ok := result.Data["resolve"].(map[string]any); !ok {
		t.Fatalf("result.Data[resolve] = %#v, want resolve payload", result.Data["resolve"])
	}
	if _, ok := result.Data["lookup"]; ok {
		t.Fatalf("result.Data[lookup] = %#v, want omitted on lookup failure", result.Data["lookup"])
	}
	if _, ok := result.Data["public_profile"]; ok {
		t.Fatalf("result.Data[public_profile] = %#v, want omitted on profile failure", result.Data["public_profile"])
	}
}

func TestServiceResolveFullHandleUsesLookupThenProfileByDID(t *testing.T) {
	t.Parallel()

	var lookupHandle string
	var profileDID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user-service/handle/rpc":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("json.NewDecoder().Decode() error = %v", err)
			}
			params, _ := payload["params"].(map[string]any)
			lookupHandle, _ = params["handle"].(string)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"handle":"alice","full_handle":"alice.partner.test","did":"did:wba:partner.test:alice:e1_test"},"id":"req-1"}`))
		case "/user-service/did/profile/rpc":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("json.NewDecoder().Decode() error = %v", err)
			}
			params, _ := payload["params"].(map[string]any)
			profileDID, _ = params["did"].(string)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"did":"did:wba:partner.test:alice:e1_test","handle":"alice"},"id":"req-1"}`))
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	resolved, _ := newIdentityServiceWorkspace(t, server.URL)
	service, err := identity.NewService(resolved)
	if err != nil {
		t.Fatalf("identity.NewService() error = %v", err)
	}

	if _, err := service.Resolve(context.Background(), "alice.partner.test", ""); err != nil {
		t.Fatalf("Service.Resolve() error = %v", err)
	}
	if lookupHandle != "alice.partner.test" {
		t.Fatalf("lookup handle = %q, want alice.partner.test", lookupHandle)
	}
	if profileDID != "did:wba:partner.test:alice:e1_test" {
		t.Fatalf("profile did = %q, want did:wba:partner.test:alice:e1_test", profileDID)
	}
}

func TestServiceGetProfileByHandleReturnsBareAndFullHandleSubject(t *testing.T) {
	t.Parallel()

	var (
		lookupHandle string
		profileDID   string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user-service/handle/rpc":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("json.NewDecoder().Decode() error = %v", err)
			}
			params, _ := payload["params"].(map[string]any)
			lookupHandle, _ = params["handle"].(string)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"did":"` + testenv.DID("alice", "e1_test") + `"},"id":"req-1"}`))
		case "/user-service/did/profile/rpc":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("json.NewDecoder().Decode() error = %v", err)
			}
			params, _ := payload["params"].(map[string]any)
			profileDID, _ = params["did"].(string)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"did":"` + testenv.DID("alice", "e1_test") + `","handle":"alice"},"id":"req-1"}`))
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	resolved, _ := newIdentityServiceWorkspace(t, server.URL)
	service, err := identity.NewService(resolved)
	if err != nil {
		t.Fatalf("identity.NewService() error = %v", err)
	}

	result, err := service.GetProfile(context.Background(), false, "alice", "")
	if err != nil {
		t.Fatalf("Service.GetProfile() error = %v", err)
	}
	if lookupHandle != testenv.FullHandle("alice") {
		t.Fatalf("lookup handle = %q, want %q", lookupHandle, testenv.FullHandle("alice"))
	}
	if profileDID != testenv.DID("alice", "e1_test") {
		t.Fatalf("profile did = %q, want %q", profileDID, testenv.DID("alice", "e1_test"))
	}
	subject, _ := result.Data["subject"].(map[string]any)
	if got, _ := subject["handle"].(string); got != "alice" {
		t.Fatalf("subject.handle = %q, want alice", got)
	}
	if got, _ := subject["full_handle"].(string); got != testenv.FullHandle("alice") {
		t.Fatalf("subject.full_handle = %q, want %q", got, testenv.FullHandle("alice"))
	}
	if got, _ := subject["domain"].(string); got != testenv.Domain() {
		t.Fatalf("subject.domain = %q, want %q", got, testenv.Domain())
	}
	if got, _ := subject["did"].(string); got != testenv.DID("alice", "e1_test") {
		t.Fatalf("subject.did = %q, want %q", got, testenv.DID("alice", "e1_test"))
	}
}

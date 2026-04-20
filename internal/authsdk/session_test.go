package authsdk

import (
	"net/http"
	"testing"
)

func TestCaptureTokenPersistsOnlyConfiguredScopes(t *testing.T) {
	t.Parallel()

	var persisted []string
	session := NewSession("", "", "alice", "did:wba:example.com:user:alice:e1", "", func(token string) error {
		persisted = append(persisted, token)
		return nil
	})
	session.RememberScope("https://home.example/rpc")

	remoteToken := session.CaptureToken("https://remote.example/rpc", http.Header{
		"Authentication-Info": []string{`access_token="remote-token", token_type="Bearer", expires_in=3600`},
	})
	if remoteToken != "remote-token" {
		t.Fatalf("remote token = %q, want %q", remoteToken, "remote-token")
	}
	if got := session.CurrentJWT(); got != "" {
		t.Fatalf("CurrentJWT() after remote capture = %q, want empty", got)
	}
	if len(persisted) != 0 {
		t.Fatalf("persisted remote token unexpectedly: %#v", persisted)
	}

	localToken := session.CaptureToken("https://home.example/rpc", http.Header{
		"Authentication-Info": []string{`access_token="fresh-home-token", token_type="Bearer", expires_in=3600`},
	})
	if localToken != "fresh-home-token" {
		t.Fatalf("local token = %q, want %q", localToken, "fresh-home-token")
	}
	if got := session.CurrentJWT(); got != "fresh-home-token" {
		t.Fatalf("CurrentJWT() after local capture = %q, want refreshed token", got)
	}
	if len(persisted) != 1 || persisted[0] != "fresh-home-token" {
		t.Fatalf("persisted = %#v, want fresh local token", persisted)
	}
}

func TestCaptureTokenStillAcceptsLegacyAuthorizationResponseHeader(t *testing.T) {
	t.Parallel()

	var persisted []string
	session := NewSession("", "", "alice", "did:wba:example.com:user:alice:e1", "", func(token string) error {
		persisted = append(persisted, token)
		return nil
	})
	session.RememberScope("https://home.example/rpc")

	token := session.CaptureToken("https://home.example/rpc", http.Header{
		"Authorization": []string{"Bearer legacy-token"},
	})
	if token != "legacy-token" {
		t.Fatalf("token = %q, want legacy-token", token)
	}
	if got := session.CurrentJWT(); got != "legacy-token" {
		t.Fatalf("CurrentJWT() = %q, want legacy-token", got)
	}
	if len(persisted) != 1 || persisted[0] != "legacy-token" {
		t.Fatalf("persisted = %#v, want legacy-token", persisted)
	}
}

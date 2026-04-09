package authsdk

import "net/http"
import "testing"

func TestCaptureTokenPersistsOnlyConfiguredScopes(t *testing.T) {
	t.Parallel()

	var persisted []string
	session := NewSession("", "", "alice", "did:wba:example.com:user:alice:e1", "", func(token string) error {
		persisted = append(persisted, token)
		return nil
	})
	session.SetBearer("https://home.example/rpc", "home-token")

	remoteToken := session.CaptureToken("https://remote.example/rpc", http.Header{
		"Authorization": []string{"Bearer remote-token"},
	})
	if remoteToken != "remote-token" {
		t.Fatalf("remote token = %q, want %q", remoteToken, "remote-token")
	}
	if got := session.CurrentJWT(); got != "home-token" {
		t.Fatalf("CurrentJWT() after remote capture = %q, want %q", got, "home-token")
	}
	if len(persisted) != 0 {
		t.Fatalf("persisted remote token unexpectedly: %#v", persisted)
	}

	localToken := session.CaptureToken("https://home.example/rpc", http.Header{
		"Authorization": []string{"Bearer refreshed-home-token"},
	})
	if localToken != "refreshed-home-token" {
		t.Fatalf("local token = %q, want %q", localToken, "refreshed-home-token")
	}
	if got := session.CurrentJWT(); got != "refreshed-home-token" {
		t.Fatalf("CurrentJWT() after local capture = %q, want refreshed token", got)
	}
	if len(persisted) != 1 || persisted[0] != "refreshed-home-token" {
		t.Fatalf("persisted = %#v, want refreshed local token", persisted)
	}
}

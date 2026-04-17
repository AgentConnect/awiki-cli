package listener

import "testing"

func TestSessionWarningsReportsDisconnectedSessions(t *testing.T) {
	t.Parallel()

	warnings := SessionWarnings([]SessionStatus{
		{IdentityName: "alice", Connected: true},
		{IdentityName: "bob", Connected: false, LastError: "refresh websocket session JWT: unauthorized"},
		{IdentityName: "carol", Connected: false},
	})

	if len(warnings) != 2 {
		t.Fatalf("len(SessionWarnings()) = %d, want 2", len(warnings))
	}
	if warnings[0] != "websocket session for identity bob is disconnected: refresh websocket session JWT: unauthorized" {
		t.Fatalf("warnings[0] = %q", warnings[0])
	}
	if warnings[1] != "websocket session for identity carol is disconnected" {
		t.Fatalf("warnings[1] = %q", warnings[1])
	}
}

func TestHasDisconnectedSessions(t *testing.T) {
	t.Parallel()

	if HasDisconnectedSessions([]SessionStatus{{IdentityName: "alice", Connected: true}}) {
		t.Fatal("HasDisconnectedSessions() = true, want false")
	}
	if !HasDisconnectedSessions([]SessionStatus{{IdentityName: "bob", Connected: false}}) {
		t.Fatal("HasDisconnectedSessions() = false, want true")
	}
}

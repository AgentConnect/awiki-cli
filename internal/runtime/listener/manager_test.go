package listener

import "testing"

func TestMergeSavedRuntimeStatusPrefersRunningHostNotifyState(t *testing.T) {
	t.Parallel()

	status := Status{
		Running: true,
		PID:     30656,
		HostNotify: HostNotifyStatus{
			Enabled: true,
			Sink:    "openclaw",
			HookURL: "http://127.0.0.1:18789/hooks/agent",
		},
	}
	saved := Status{
		StartedAt: "2026-04-17T05:18:13Z",
		PID:       30656,
		BootID:    "boot-new",
		Sessions: []SessionStatus{{
			IdentityName: "zhuocheng",
			Connected:    true,
		}},
		HostNotify: HostNotifyStatus{
			Enabled:   true,
			Sink:      "log",
			FilePath:  "/tmp/host-notify.events.jsonl",
			HookURL:   "http://127.0.0.1:9999/hooks/agent",
			LastError: "sink boom",
		},
	}

	mergeSavedRuntimeStatus(&status, saved)

	if status.StartedAt != saved.StartedAt {
		t.Fatalf("status.StartedAt = %q, want %q", status.StartedAt, saved.StartedAt)
	}
	if status.PID != saved.PID {
		t.Fatalf("status.PID = %d, want %d", status.PID, saved.PID)
	}
	if status.BootID != "boot-new" {
		t.Fatalf("status.BootID = %q, want boot-new", status.BootID)
	}
	if len(status.Sessions) != 1 || status.Sessions[0].IdentityName != "zhuocheng" {
		t.Fatalf("status.Sessions = %#v", status.Sessions)
	}
	if status.HostNotify.Sink != "log" {
		t.Fatalf("status.HostNotify.Sink = %q, want log", status.HostNotify.Sink)
	}
	if status.HostNotify.FilePath != "/tmp/host-notify.events.jsonl" {
		t.Fatalf("status.HostNotify.FilePath = %q", status.HostNotify.FilePath)
	}
	if status.HostNotify.HookURL != "http://127.0.0.1:9999/hooks/agent" {
		t.Fatalf("status.HostNotify.HookURL = %q", status.HostNotify.HookURL)
	}
	if status.HostNotify.LastError != "sink boom" {
		t.Fatalf("status.HostNotify.LastError = %q", status.HostNotify.LastError)
	}
}

func TestMergeSavedRuntimeStatusKeepsConfiguredHostNotifyWhenNotRunning(t *testing.T) {
	t.Parallel()

	status := Status{
		Running: false,
		HostNotify: HostNotifyStatus{
			Enabled: true,
			Sink:    "openclaw",
			HookURL: "http://127.0.0.1:18789/hooks/agent",
		},
	}
	saved := Status{
		HostNotify: HostNotifyStatus{
			Enabled: true,
			Sink:    "log",
			HookURL: "http://127.0.0.1:9999/hooks/agent",
		},
	}

	mergeSavedRuntimeStatus(&status, saved)

	if status.HostNotify.Sink != "openclaw" {
		t.Fatalf("status.HostNotify.Sink = %q, want openclaw", status.HostNotify.Sink)
	}
	if status.HostNotify.HookURL != "http://127.0.0.1:18789/hooks/agent" {
		t.Fatalf("status.HostNotify.HookURL = %q", status.HostNotify.HookURL)
	}
}

func TestMergeSavedRuntimeStatusSkipsMismatchedPID(t *testing.T) {
	t.Parallel()

	status := Status{
		Running: true,
		PID:     200,
		HostNotify: HostNotifyStatus{
			Enabled: true,
			Sink:    "openclaw",
			HookURL: "http://127.0.0.1:18789/hooks/agent",
		},
	}
	saved := Status{
		PID:    100,
		BootID: "boot-old",
		HostNotify: HostNotifyStatus{
			Enabled: true,
			Sink:    "log",
			HookURL: "http://127.0.0.1:9999/hooks/agent",
		},
	}

	mergeSavedRuntimeStatus(&status, saved)

	if status.PID != 200 {
		t.Fatalf("status.PID = %d, want 200", status.PID)
	}
	if status.BootID != "" {
		t.Fatalf("status.BootID = %q, want empty", status.BootID)
	}
	if status.HostNotify.Sink != "openclaw" {
		t.Fatalf("status.HostNotify.Sink = %q, want openclaw", status.HostNotify.Sink)
	}
}

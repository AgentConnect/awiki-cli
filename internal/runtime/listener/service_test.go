package listener

import (
	"errors"
	"testing"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	servicepkg "github.com/kardianos/service"
)

func TestWaitForServiceStatusWithWaitsForBridgeAvailability(t *testing.T) {
	t.Parallel()

	statuses := []Status{
		{
			Installed:       true,
			Running:         true,
			BridgeAvailable: false,
		},
		{
			Installed:       true,
			Running:         true,
			BridgeAvailable: true,
		},
	}
	callCount := 0

	status, err := waitForServiceStatusWith(
		func() (Status, error) {
			current := statuses[len(statuses)-1]
			if callCount < len(statuses) {
				current = statuses[callCount]
			}
			callCount++
			return current, nil
		},
		true,
		true,
		100*time.Millisecond,
		time.Millisecond,
	)
	if err != nil {
		t.Fatalf("waitForServiceStatusWith() error = %v", err)
	}
	if !status.BridgeAvailable {
		t.Fatalf("waitForServiceStatusWith() bridge_available = false, want true")
	}
	if callCount < 2 {
		t.Fatalf("waitForServiceStatusWith() callCount = %d, want at least 2", callCount)
	}
}

func TestStartServiceAutoInstallsWhenMissing(t *testing.T) {
	resolved := &appconfig.Resolved{RuntimeMode: "websocket"}
	fakeSvc := &fakeService{}
	originalNewServiceFunc := newServiceFunc
	originalServiceStatusForFunc := serviceStatusForFunc
	originalEnsureInstalledFunc := ensureInstalledFunc
	originalStatusForFunc := statusForFunc
	t.Cleanup(func() {
		newServiceFunc = originalNewServiceFunc
		serviceStatusForFunc = originalServiceStatusForFunc
		ensureInstalledFunc = originalEnsureInstalledFunc
		statusForFunc = originalStatusForFunc
	})

	newServiceFunc = func(*appconfig.Resolved, servicepkg.Interface) (servicepkg.Service, error) {
		return fakeSvc, nil
	}
	statusChecks := 0
	serviceStatusForFunc = func(*appconfig.Resolved) (bool, bool, string, string, error) {
		statusChecks++
		switch statusChecks {
		case 1:
			return false, false, "test", "listener", nil
		default:
			return true, false, "test", "listener", nil
		}
	}
	autoInstallCalled := false
	ensureInstalledFunc = func(*appconfig.Resolved) (Status, error) {
		autoInstallCalled = true
		return Status{Installed: true}, nil
	}
	statusForFunc = func(*appconfig.Resolved) (Status, error) {
		return Status{
			Installed:       true,
			Running:         true,
			BridgeAvailable: true,
		}, nil
	}

	status, err := StartService(resolved)
	if err != nil {
		t.Fatalf("StartService() error = %v", err)
	}
	if !autoInstallCalled {
		t.Fatal("StartService() did not auto-install the missing listener service")
	}
	if fakeSvc.startCalls != 1 {
		t.Fatalf("fakeSvc.startCalls = %d, want 1", fakeSvc.startCalls)
	}
	if !status.Installed || !status.Running || !status.BridgeAvailable {
		t.Fatalf("StartService() status = %#v, want installed/running/bridge_available true", status)
	}
}

type fakeService struct {
	startCalls int
}

func (s *fakeService) Run() error { return nil }

func (s *fakeService) Start() error {
	s.startCalls++
	return nil
}

func (s *fakeService) Stop() error { return nil }

func (s *fakeService) Restart() error { return nil }

func (s *fakeService) Install() error { return nil }

func (s *fakeService) Uninstall() error { return nil }

func (s *fakeService) Logger(chan<- error) (servicepkg.Logger, error) {
	return nil, errors.New("not implemented")
}

func (s *fakeService) SystemLogger(chan<- error) (servicepkg.Logger, error) {
	return nil, errors.New("not implemented")
}

func (s *fakeService) String() string { return "fake-service" }

func (s *fakeService) Platform() string { return "test" }

func (s *fakeService) Status() (servicepkg.Status, error) {
	return servicepkg.StatusRunning, nil
}

package listener

import (
	"testing"
	"time"
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

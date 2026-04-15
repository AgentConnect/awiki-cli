package message

import (
	"errors"
	"testing"
)

func TestWebsocketFallbackWarningsUseReadableTransportDetails(t *testing.T) {
	t.Parallel()

	err := errors.New("message transport is unavailable: local websocket bridge request failed: websocket session is not connected for identity zhuocheng")

	httpWarning := websocketHTTPFallbackWarning(err)
	if httpWarning != "WebSocket listener was unavailable for this identity; used HTTP fallback. Details: local websocket bridge request failed: websocket session is not connected for identity zhuocheng" {
		t.Fatalf("websocketHTTPFallbackWarning() = %q", httpWarning)
	}

	cacheWarning := websocketCacheFallbackWarning(err)
	if cacheWarning != "WebSocket listener was unavailable for this identity; loaded data from local cache. Details: local websocket bridge request failed: websocket session is not connected for identity zhuocheng" {
		t.Fatalf("websocketCacheFallbackWarning() = %q", cacheWarning)
	}
}

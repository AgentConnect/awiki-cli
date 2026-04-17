package message

import (
	"fmt"
	"strings"
)

func websocketHTTPFallbackWarning(err error) string {
	detail := websocketTransportDetail(err)
	if detail == "" {
		return "WebSocket listener was unavailable for this identity; used HTTP fallback."
	}
	return fmt.Sprintf("WebSocket listener was unavailable for this identity; used HTTP fallback. Details: %s", detail)
}

func websocketCacheFallbackWarning(err error) string {
	detail := websocketTransportDetail(err)
	if detail == "" {
		return "WebSocket listener was unavailable for this identity; loaded data from local cache."
	}
	return fmt.Sprintf("WebSocket listener was unavailable for this identity; loaded data from local cache. Details: %s", detail)
}

func websocketTransportDetail(err error) string {
	if err == nil {
		return ""
	}
	detail := strings.TrimSpace(err.Error())
	prefix := ErrTransportUnavailable.Error() + ":"
	if strings.HasPrefix(detail, prefix) {
		detail = strings.TrimSpace(strings.TrimPrefix(detail, prefix))
	}
	return detail
}

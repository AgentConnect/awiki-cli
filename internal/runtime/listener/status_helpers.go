package listener

import "fmt"

func SessionWarnings(sessions []SessionStatus) []string {
	warnings := make([]string, 0)
	for _, session := range sessions {
		if session.Connected {
			continue
		}
		if session.LastError != "" {
			warnings = append(warnings, fmt.Sprintf("websocket session for identity %s is disconnected: %s", session.IdentityName, session.LastError))
			continue
		}
		warnings = append(warnings, fmt.Sprintf("websocket session for identity %s is disconnected", session.IdentityName))
	}
	return warnings
}

func HasDisconnectedSessions(sessions []SessionStatus) bool {
	for _, session := range sessions {
		if !session.Connected {
			return true
		}
	}
	return false
}

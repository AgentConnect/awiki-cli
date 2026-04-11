package mail

import "strings"

func stringFromAny(value any) string {
	text, _ := value.(string)
	return text
}

func intValueFromAny(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return fallback
	}
}

func listLength(value any) int {
	slice, ok := value.([]any)
	if !ok {
		return 0
	}
	return len(slice)
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

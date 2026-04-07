package message

import "strings"

func stringFromAny(value any) string {
	text, _ := value.(string)
	return text
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case string:
		return typed == "1" || strings.EqualFold(typed, "true")
	default:
		return false
	}
}

func int64PtrFromAny(value any) *int64 {
	switch typed := value.(type) {
	case int64:
		value := typed
		return &value
	case int:
		value := int64(typed)
		return &value
	case float64:
		value := int64(typed)
		return &value
	default:
		return nil
	}
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

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

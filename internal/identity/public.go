package identity

import "encoding/json"

// PublicData returns a sanitized copy of identity-domain command output.
// Internal linkage fields such as user_id must not be exposed through the
// public CLI contract.
func PublicData(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}
	sanitized, ok := sanitizePublicValue(data).(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return sanitized
}

func sanitizePublicValue(value any) any {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return value
	}
	return sanitizePublicDecoded(decoded)
}

func sanitizePublicDecoded(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		sanitized := make(map[string]any, len(typed))
		for key, item := range typed {
			if isInternalIdentityKey(key) {
				continue
			}
			sanitized[key] = sanitizePublicDecoded(item)
		}
		return sanitized
	case []any:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, sanitizePublicDecoded(item))
		}
		return items
	default:
		return value
	}
}

func isInternalIdentityKey(key string) bool {
	switch key {
	case "user_id", "userId", "UserID":
		return true
	default:
		return false
	}
}

package skillbundle

import (
	"fmt"
	"strings"
)

func Index(kind string, tag string) (IndexResult, error) {
	bundle, err := Load()
	if err != nil {
		return IndexResult{}, err
	}
	result, err := rootPreview(bundle)
	if err != nil {
		return IndexResult{}, err
	}
	if strings.TrimSpace(kind) == "" && strings.TrimSpace(tag) == "" {
		return result, nil
	}
	filtered := make([]ChildDocMeta, 0, len(result.Children))
	for _, child := range result.Children {
		if kind != "" && !strings.EqualFold(strings.TrimSpace(kind), child.Kind) {
			continue
		}
		if tag != "" && !hasTag(child.Tags, tag) {
			continue
		}
		filtered = append(filtered, child)
	}
	result.Children = filtered
	return result, nil
}

func ValidateKind(kind string) error {
	if strings.TrimSpace(kind) == "" {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(kind), "reference") {
		return nil
	}
	return fmt.Errorf("unsupported skill kind %q", kind)
}

func hasTag(tags []string, tag string) bool {
	needle := strings.TrimSpace(strings.ToLower(tag))
	for _, candidate := range tags {
		if strings.TrimSpace(strings.ToLower(candidate)) == needle {
			return true
		}
	}
	return false
}

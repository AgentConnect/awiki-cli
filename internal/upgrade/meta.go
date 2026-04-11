package upgrade

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

func LoadMeta(path string) (*Meta, error) {
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read workspace meta: %w", err)
	}
	var meta Meta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("parse workspace meta: %w", err)
	}
	return &meta, nil
}

func SaveMeta(path string, meta Meta) error {
	if path == "" {
		return fmt.Errorf("workspace meta path is required")
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal workspace meta: %w", err)
	}
	if err := writeAtomicFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write workspace meta: %w", err)
	}
	return nil
}

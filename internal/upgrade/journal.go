package upgrade

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

func LoadJournal(path string) (*Journal, error) {
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read workspace upgrade journal: %w", err)
	}
	var journal Journal
	if err := json.Unmarshal(raw, &journal); err != nil {
		return nil, fmt.Errorf("parse workspace upgrade journal: %w", err)
	}
	return &journal, nil
}

func SaveJournal(path string, journal Journal) error {
	if path == "" {
		return fmt.Errorf("workspace journal path is required")
	}
	raw, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal workspace upgrade journal: %w", err)
	}
	if err := writeAtomicFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write workspace upgrade journal: %w", err)
	}
	return nil
}

func ClearJournal(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove workspace upgrade journal: %w", err)
	}
	return nil
}

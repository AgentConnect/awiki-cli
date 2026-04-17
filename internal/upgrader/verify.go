package upgrader

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

func VerifySHA256(path string, expected string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("artifact path is required")
	}
	expected = strings.TrimSpace(strings.ToLower(expected))
	if expected == "" {
		return fmt.Errorf("artifact sha256 is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open artifact for sha256 verification: %w", err)
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("hash artifact: %w", err)
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	if actual != expected {
		return fmt.Errorf("artifact sha256 mismatch: got %s want %s", actual, expected)
	}
	return nil
}

package identity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

var safePathComponent = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type Manager struct {
	paths appconfig.Paths
}

func NewManager(paths appconfig.Paths) *Manager {
	return &Manager{paths: paths}
}

func (m *Manager) RootDir() string {
	return m.paths.IdentityDir
}

func (m *Manager) LegacyRootDir() string {
	return m.paths.LegacyCredentialsDir
}

func (m *Manager) indexPath() string {
	return filepath.Join(m.RootDir(), IndexFileName)
}

func (m *Manager) EnsureRoot() error {
	if err := os.MkdirAll(m.RootDir(), 0o700); err != nil {
		return fmt.Errorf("create identity root: %w", err)
	}
	return os.Chmod(m.RootDir(), 0o700)
}

func (m *Manager) BuildPaths(dirName string) Paths {
	rootDir := m.RootDir()
	identityDir := filepath.Join(rootDir, dirName)
	return Paths{
		RootDir:                  rootDir,
		DirName:                  dirName,
		IdentityDir:              identityDir,
		IdentityPath:             filepath.Join(identityDir, IdentityFileName),
		AuthPath:                 filepath.Join(identityDir, AuthFileName),
		DIDDocumentPath:          filepath.Join(identityDir, DIDDocumentFileName),
		Key1PrivatePath:          filepath.Join(identityDir, Key1PrivateFileName),
		Key1PublicPath:           filepath.Join(identityDir, Key1PublicFileName),
		E2EESigningPrivatePath:   filepath.Join(identityDir, E2EESigningPrivateFileName),
		E2EEAgreementPrivatePath: filepath.Join(identityDir, E2EEAgreementPrivateFileName),
		E2EEStatePath:            filepath.Join(identityDir, E2EEStateFileName),
	}
}

func sanitizeComponent(raw string) string {
	sanitized := safePathComponent.ReplaceAllString(raw, "_")
	return strings.Trim(sanitized, "._-")
}

func preferredDirName(uniqueID string) (string, error) {
	sanitized := sanitizeComponent(uniqueID)
	if sanitized == "" {
		return "", fmt.Errorf("%w: unique_id is required", ErrInvalidInput)
	}
	return sanitized, nil
}

func sanitizeIdentityName(raw string) string {
	return sanitizeComponent(strings.ToLower(strings.TrimSpace(raw)))
}

func defaultIndex() IndexPayload {
	return IndexPayload{
		SchemaVersion:         IndexSchemaVersion,
		DefaultCredentialName: "",
		Credentials:           map[string]IndexEntry{},
	}
}

func normalizeIndexPayload(payload IndexPayload) IndexPayload {
	if payload.SchemaVersion == 0 {
		payload.SchemaVersion = IndexSchemaVersion
	}
	if payload.Credentials == nil {
		payload.Credentials = map[string]IndexEntry{}
	}
	if payload.DefaultCredentialName == "" {
		if _, ok := payload.Credentials["default"]; ok {
			payload.DefaultCredentialName = "default"
		}
	}
	keys := make([]string, 0, len(payload.Credentials))
	for name := range payload.Credentials {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		entry := payload.Credentials[name]
		entry.CredentialName = name
		entry.IsDefault = payload.DefaultCredentialName == name
		payload.Credentials[name] = entry
	}
	return payload
}

func loadIndexFrom(path string) (IndexPayload, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultIndex(), nil
		}
		return IndexPayload{}, err
	}
	var payload IndexPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return IndexPayload{}, fmt.Errorf("parse identity index: %w", err)
	}
	if payload.SchemaVersion != 0 && payload.SchemaVersion != 2 && payload.SchemaVersion != 3 {
		return IndexPayload{}, fmt.Errorf("unsupported identity index schema version: %d", payload.SchemaVersion)
	}
	payload = normalizeIndexPayload(payload)
	return payload, nil
}

func saveIndexTo(path string, payload IndexPayload) error {
	normalized := normalizeIndexPayload(payload)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func (m *Manager) LoadIndex() (IndexPayload, error) {
	return loadIndexFrom(m.indexPath())
}

func (m *Manager) SaveIndex(payload IndexPayload) error {
	if err := m.EnsureRoot(); err != nil {
		return err
	}
	return saveIndexTo(m.indexPath(), payload)
}

func (m *Manager) resolveEntryName(requested string, index IndexPayload) (string, IndexEntry, bool) {
	if entry, ok := index.Credentials[requested]; ok {
		return requested, entry, true
	}
	if requested == "" || requested == "default" {
		if index.DefaultCredentialName != "" {
			entry, ok := index.Credentials[index.DefaultCredentialName]
			return index.DefaultCredentialName, entry, ok
		}
	}
	return "", IndexEntry{}, false
}

func (m *Manager) PathsForIdentity(name string) (Paths, error) {
	index, err := m.LoadIndex()
	if err != nil {
		return Paths{}, err
	}
	_, entry, ok := m.resolveEntryName(name, index)
	if !ok {
		return Paths{}, fmt.Errorf("%w: %s", ErrIdentityNotFound, name)
	}
	return m.BuildPaths(entry.DirName), nil
}

func writeSecureJSON(path string, payload any) error {
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func writeSecureText(path string, content string) error {
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func ensureDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

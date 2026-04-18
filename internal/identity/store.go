package identity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (m *Manager) save(input SaveInput) (*StoredIdentity, error) {
	if strings.TrimSpace(input.IdentityName) == "" {
		return nil, fmt.Errorf("%w: identity name is required", ErrInvalidInput)
	}
	if strings.TrimSpace(input.DID) == "" || strings.TrimSpace(input.UniqueID) == "" {
		return nil, fmt.Errorf("%w: did and unique_id are required", ErrInvalidInput)
	}
	if err := m.EnsureRoot(); err != nil {
		return nil, err
	}
	index, err := m.LoadIndex()
	if err != nil {
		return nil, err
	}
	identityName := input.IdentityName
	if existing, ok := index.Credentials[identityName]; ok {
		if existing.DID != input.DID && !input.ReplaceExisting {
			return nil, fmt.Errorf("%w: identity %s already exists for did %s", ErrIdentityConflict, identityName, existing.DID)
		}
	}

	dirName, err := preferredDirName(input.UniqueID)
	if err != nil {
		return nil, err
	}
	for name, entry := range index.Credentials {
		if name == identityName {
			continue
		}
		if entry.DirName == dirName && entry.DID != input.DID {
			return nil, fmt.Errorf("%w: dir %s already used by identity %s", ErrIdentityConflict, dirName, name)
		}
	}

	paths := m.BuildPaths(dirName)
	if err := ensureDir(paths.IdentityDir); err != nil {
		return nil, fmt.Errorf("create identity directory: %w", err)
	}
	createdAt := time.Now().UTC().Format(time.RFC3339)
	if existing, err := m.Load(identityName); err == nil && existing != nil && existing.CreatedAt != "" {
		createdAt = existing.CreatedAt
	}

	identityPayload := map[string]any{
		"did":        input.DID,
		"unique_id":  input.UniqueID,
		"created_at": createdAt,
	}
	if input.UserID != "" {
		identityPayload["user_id"] = input.UserID
	}
	if input.DisplayName != "" {
		identityPayload["name"] = input.DisplayName
	}
	if input.Handle != "" {
		identityPayload["handle"] = input.Handle
	}
	if err := writeSecureJSON(paths.IdentityPath, identityPayload); err != nil {
		return nil, fmt.Errorf("write identity payload: %w", err)
	}
	if err := writeSecureJSON(paths.AuthPath, map[string]any{"jwt_token": nullableString(input.JWTToken)}); err != nil {
		return nil, fmt.Errorf("write auth payload: %w", err)
	}
	if input.DIDDocument != nil {
		if err := writeSecureJSON(paths.DIDDocumentPath, input.DIDDocument); err != nil {
			return nil, fmt.Errorf("write did document: %w", err)
		}
	}
	if input.Key1PrivatePEM != "" {
		if err := writeSecureText(paths.Key1PrivatePath, input.Key1PrivatePEM); err != nil {
			return nil, fmt.Errorf("write key-1 private key: %w", err)
		}
	}
	if input.Key1PublicPEM != "" {
		if err := writeSecureText(paths.Key1PublicPath, input.Key1PublicPEM); err != nil {
			return nil, fmt.Errorf("write key-1 public key: %w", err)
		}
	}
	if input.E2EESigningPrivatePEM != "" {
		if err := writeSecureText(paths.E2EESigningPrivatePath, input.E2EESigningPrivatePEM); err != nil {
			return nil, fmt.Errorf("write e2ee signing private key: %w", err)
		}
	}
	if input.E2EEAgreementPrivatePEM != "" {
		if err := writeSecureText(paths.E2EEAgreementPrivatePath, input.E2EEAgreementPrivatePEM); err != nil {
			return nil, fmt.Errorf("write e2ee agreement private key: %w", err)
		}
	}

	entry := IndexEntry{
		CredentialName: identityName,
		DirName:        dirName,
		DID:            input.DID,
		UniqueID:       input.UniqueID,
		UserID:         input.UserID,
		Name:           input.DisplayName,
		Handle:         input.Handle,
		CreatedAt:      createdAt,
		IsDefault:      index.DefaultCredentialName == identityName || index.DefaultCredentialName == "",
	}
	if index.DefaultCredentialName == "" {
		index.DefaultCredentialName = identityName
	}
	index.Credentials[identityName] = entry
	if err := m.SaveIndex(index); err != nil {
		return nil, err
	}
	return m.Load(identityName)
}

func (m *Manager) Save(input SaveInput) (*StoredIdentity, error) {
	return m.save(input)
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (m *Manager) Load(name string) (*StoredIdentity, error) {
	index, err := m.LoadIndex()
	if err != nil {
		return nil, err
	}
	resolvedName, entry, ok := m.resolveEntryName(name, index)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrIdentityNotFound, name)
	}
	paths := m.BuildPaths(entry.DirName)
	if err := ensureIdentityPrivateKeysCompatible(paths); err != nil {
		return nil, err
	}
	identityPayload, err := readJSONMap(paths.IdentityPath)
	if err != nil {
		return nil, err
	}
	authPayload, err := readJSONMap(paths.AuthPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	record := &StoredIdentity{
		IdentityName: resolvedName,
		DirName:      entry.DirName,
		DID:          stringValue(identityPayload["did"], entry.DID),
		UniqueID:     stringValue(identityPayload["unique_id"], entry.UniqueID),
		UserID:       stringValue(identityPayload["user_id"], entry.UserID),
		DisplayName:  stringValue(identityPayload["name"], entry.Name),
		Handle:       stringValue(identityPayload["handle"], entry.Handle),
		CreatedAt:    stringValue(identityPayload["created_at"], entry.CreatedAt),
		IsDefault:    index.DefaultCredentialName == resolvedName,
	}
	record.JWTToken = stringValue(authPayload["jwt_token"], "")
	record.DIDDocument, _ = readJSONMap(paths.DIDDocumentPath)
	record.Key1PrivatePEM = readText(paths.Key1PrivatePath)
	record.Key1PublicPEM = readText(paths.Key1PublicPath)
	record.E2EESigningPrivatePEM = readText(paths.E2EESigningPrivatePath)
	record.E2EEAgreementPrivatePEM = readText(paths.E2EEAgreementPrivatePath)
	return record, nil
}

func (m *Manager) UpdateJWT(name string, jwtToken string) error {
	record, err := m.Load(name)
	if err != nil {
		return err
	}
	paths := m.BuildPaths(record.DirName)
	return writeSecureJSON(paths.AuthPath, map[string]any{"jwt_token": nullableString(jwtToken)})
}

func (m *Manager) UpdateDisplayName(name string, displayName string) error {
	record, err := m.Load(name)
	if err != nil {
		return err
	}
	paths := m.BuildPaths(record.DirName)
	payload, err := readJSONMap(paths.IdentityPath)
	if err != nil {
		return err
	}
	payload["name"] = displayName
	if err := writeSecureJSON(paths.IdentityPath, payload); err != nil {
		return err
	}
	index, err := m.LoadIndex()
	if err != nil {
		return err
	}
	entry, ok := index.Credentials[record.IdentityName]
	if !ok {
		return fmt.Errorf("%w: %s", ErrIdentityNotFound, record.IdentityName)
	}
	entry.Name = displayName
	index.Credentials[record.IdentityName] = entry
	return m.SaveIndex(index)
}

func (m *Manager) BackupIdentityForDIDReplacement(name string, plannedNewDID string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("%w: identity name is required", ErrInvalidInput)
	}
	if strings.TrimSpace(plannedNewDID) == "" {
		return "", fmt.Errorf("%w: planned new did is required", ErrInvalidInput)
	}
	if err := m.EnsureRoot(); err != nil {
		return "", err
	}

	index, err := m.LoadIndex()
	if err != nil {
		return "", err
	}
	resolvedName, entry, ok := m.resolveEntryName(name, index)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrIdentityNotFound, name)
	}
	current, err := m.Load(resolvedName)
	if err != nil {
		return "", err
	}
	oldPaths := m.BuildPaths(entry.DirName)
	info, err := os.Stat(oldPaths.IdentityDir)
	if err != nil {
		return "", fmt.Errorf("stat identity directory before backup: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: identity path is not a directory: %s", ErrInvalidInput, oldPaths.IdentityDir)
	}

	linkedNames := make([]string, 0, 1)
	for candidateName, candidateEntry := range index.Credentials {
		if candidateEntry.DirName == entry.DirName || candidateEntry.DID == entry.DID {
			linkedNames = append(linkedNames, candidateName)
		}
	}
	sort.Strings(linkedNames)

	createdAt := time.Now().UTC()
	backupName := strings.Join([]string{
		createdAt.Format("20060102T150405.000000000Z"),
		sanitizeIdentityName(resolvedName),
		sanitizeComponent(entry.DirName),
	}, "-")
	backupDir := uniqueBackupDir(filepath.Join(m.legacyBackupRoot(), "replace-did", backupName))
	if err := copyDir(oldPaths.IdentityDir, backupDir); err != nil {
		return "", fmt.Errorf("backup identity directory before DID replacement: %w", err)
	}

	manifest := map[string]any{
		"reason":                "replace_did",
		"created_at":            createdAt.Format(time.RFC3339Nano),
		"identity_name":         resolvedName,
		"linked_identity_names": linkedNames,
		"old_did":               current.DID,
		"old_dir_name":          entry.DirName,
		"planned_new_did":       plannedNewDID,
	}
	if err := writeSecureJSON(filepath.Join(backupDir, "backup_manifest.json"), manifest); err != nil {
		return "", fmt.Errorf("write DID replacement backup manifest: %w", err)
	}
	return backupDir, nil
}

func (m *Manager) ReplaceIdentity(name string, input SaveInput) (*StoredIdentity, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("%w: identity name is required", ErrInvalidInput)
	}
	if strings.TrimSpace(input.DID) == "" || strings.TrimSpace(input.UniqueID) == "" {
		return nil, fmt.Errorf("%w: did and unique_id are required", ErrInvalidInput)
	}
	if err := m.EnsureRoot(); err != nil {
		return nil, err
	}

	index, err := m.LoadIndex()
	if err != nil {
		return nil, err
	}
	resolvedName, entry, ok := m.resolveEntryName(name, index)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrIdentityNotFound, name)
	}
	current, err := m.Load(resolvedName)
	if err != nil {
		return nil, err
	}

	createdAt := current.CreatedAt
	if createdAt == "" {
		createdAt = time.Now().UTC().Format(time.RFC3339)
	}
	if strings.TrimSpace(input.UserID) == "" {
		input.UserID = current.UserID
	}
	if strings.TrimSpace(input.DisplayName) == "" {
		input.DisplayName = current.DisplayName
	}
	if strings.TrimSpace(input.Handle) == "" {
		input.Handle = current.Handle
	}

	newDirName, err := preferredDirName(input.UniqueID)
	if err != nil {
		return nil, err
	}

	linkedNames := make([]string, 0, 1)
	linkedLookup := map[string]struct{}{}
	for candidateName, candidateEntry := range index.Credentials {
		if candidateEntry.DirName == entry.DirName || candidateEntry.DID == entry.DID {
			linkedNames = append(linkedNames, candidateName)
			linkedLookup[candidateName] = struct{}{}
		}
	}
	if len(linkedNames) == 0 {
		linkedNames = append(linkedNames, resolvedName)
		linkedLookup[resolvedName] = struct{}{}
	}

	for candidateName, candidateEntry := range index.Credentials {
		if _, ok := linkedLookup[candidateName]; ok {
			continue
		}
		if candidateEntry.DID == input.DID {
			return nil, fmt.Errorf("%w: did %s already belongs to identity %s", ErrIdentityConflict, input.DID, candidateName)
		}
		if candidateEntry.DirName == newDirName {
			return nil, fmt.Errorf("%w: dir %s already used by identity %s", ErrIdentityConflict, newDirName, candidateName)
		}
	}

	oldPaths := m.BuildPaths(entry.DirName)
	newPaths := m.BuildPaths(newDirName)
	if err := ensureDir(newPaths.IdentityDir); err != nil {
		return nil, fmt.Errorf("create identity directory: %w", err)
	}

	identityPayload := map[string]any{
		"did":        input.DID,
		"unique_id":  input.UniqueID,
		"created_at": createdAt,
	}
	if input.UserID != "" {
		identityPayload["user_id"] = input.UserID
	}
	if input.DisplayName != "" {
		identityPayload["name"] = input.DisplayName
	}
	if input.Handle != "" {
		identityPayload["handle"] = input.Handle
	}
	if err := writeSecureJSON(newPaths.IdentityPath, identityPayload); err != nil {
		return nil, fmt.Errorf("write identity payload: %w", err)
	}
	if err := writeSecureJSON(newPaths.AuthPath, map[string]any{"jwt_token": nullableString(input.JWTToken)}); err != nil {
		return nil, fmt.Errorf("write auth payload: %w", err)
	}
	if input.DIDDocument != nil {
		if err := writeSecureJSON(newPaths.DIDDocumentPath, input.DIDDocument); err != nil {
			return nil, fmt.Errorf("write did document: %w", err)
		}
	}
	if input.Key1PrivatePEM != "" {
		if err := writeSecureText(newPaths.Key1PrivatePath, input.Key1PrivatePEM); err != nil {
			return nil, fmt.Errorf("write key-1 private key: %w", err)
		}
	}
	if input.Key1PublicPEM != "" {
		if err := writeSecureText(newPaths.Key1PublicPath, input.Key1PublicPEM); err != nil {
			return nil, fmt.Errorf("write key-1 public key: %w", err)
		}
	}
	if input.E2EESigningPrivatePEM != "" {
		if err := writeSecureText(newPaths.E2EESigningPrivatePath, input.E2EESigningPrivatePEM); err != nil {
			return nil, fmt.Errorf("write e2ee signing private key: %w", err)
		}
	}
	if input.E2EEAgreementPrivatePEM != "" {
		if err := writeSecureText(newPaths.E2EEAgreementPrivatePath, input.E2EEAgreementPrivatePEM); err != nil {
			return nil, fmt.Errorf("write e2ee agreement private key: %w", err)
		}
	}
	_ = os.Remove(newPaths.E2EEStatePath)

	for _, linkedName := range linkedNames {
		linkedEntry := index.Credentials[linkedName]
		linkedEntry.CredentialName = linkedName
		linkedEntry.DirName = newDirName
		linkedEntry.DID = input.DID
		linkedEntry.UniqueID = input.UniqueID
		linkedEntry.UserID = input.UserID
		linkedEntry.Name = input.DisplayName
		linkedEntry.Handle = input.Handle
		linkedEntry.CreatedAt = createdAt
		linkedEntry.IsDefault = index.DefaultCredentialName == linkedName
		index.Credentials[linkedName] = linkedEntry
	}
	if err := m.SaveIndex(index); err != nil {
		return nil, err
	}
	if oldPaths.IdentityDir != newPaths.IdentityDir {
		_ = os.RemoveAll(oldPaths.IdentityDir)
	}
	return m.Load(resolvedName)
}

func (m *Manager) List() ([]IdentitySummary, error) {
	index, err := m.LoadIndex()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(index.Credentials))
	for name := range index.Credentials {
		names = append(names, name)
	}
	sort.Strings(names)
	summaries := make([]IdentitySummary, 0, len(names))
	for _, name := range names {
		summary, err := m.summaryFor(index.Credentials[name], index.DefaultCredentialName)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, *summary)
	}
	return summaries, nil
}

func (m *Manager) summaryFor(entry IndexEntry, defaultName string) (*IdentitySummary, error) {
	paths := m.BuildPaths(entry.DirName)
	authPayload, err := readJSONMap(paths.AuthPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	summary := &IdentitySummary{
		IdentityName:            entry.CredentialName,
		DID:                     entry.DID,
		UniqueID:                entry.UniqueID,
		UserID:                  entry.UserID,
		DisplayName:             entry.Name,
		Handle:                  entry.Handle,
		CreatedAt:               entry.CreatedAt,
		DirName:                 entry.DirName,
		IsDefault:               defaultName == entry.CredentialName,
		HasJWT:                  stringValue(authPayload["jwt_token"], "") != "",
		HasDIDDocument:          fileExists(paths.DIDDocumentPath),
		HasKey1Private:          fileExists(paths.Key1PrivatePath),
		HasKey1Public:           fileExists(paths.Key1PublicPath),
		HasE2EESigningPrivate:   fileExists(paths.E2EESigningPrivatePath),
		HasE2EEAgreementPrivate: fileExists(paths.E2EEAgreementPrivatePath),
	}
	summary.UserState = EvaluateIdentitySummaryUserState(summary)
	return summary, nil
}

func (m *Manager) Current() (*IdentitySummary, error) {
	index, err := m.LoadIndex()
	if err != nil {
		return nil, err
	}
	if index.DefaultCredentialName == "" {
		if _, ok := index.Credentials["default"]; !ok {
			return nil, fmt.Errorf("%w", ErrNoDefaultIdentity)
		}
		index.DefaultCredentialName = "default"
	}
	entry, ok := index.Credentials[index.DefaultCredentialName]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoDefaultIdentity, index.DefaultCredentialName)
	}
	return m.summaryFor(entry, index.DefaultCredentialName)
}

func (m *Manager) SetDefault(name string) (*IdentitySummary, error) {
	index, err := m.LoadIndex()
	if err != nil {
		return nil, err
	}
	entry, ok := index.Credentials[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrIdentityNotFound, name)
	}
	index.DefaultCredentialName = name
	index.Credentials[name] = entry
	if err := m.SaveIndex(index); err != nil {
		return nil, err
	}
	return m.summaryFor(entry, name)
}

func readJSONMap(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse json %s: %w", path, err)
	}
	return payload, nil
}

func readText(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(raw)
}

func stringValue(value any, fallback string) string {
	text, ok := value.(string)
	if ok {
		return text
	}
	return fallback
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func chooseDefaultIdentityName(requested string, existing []IdentitySummary, fallback string) string {
	if sanitized := sanitizeIdentityName(requested); sanitized != "" {
		return sanitized
	}
	if len(existing) == 0 {
		return "default"
	}
	base := sanitizeIdentityName(fallback)
	if base == "" {
		base = "identity"
	}
	used := map[string]struct{}{}
	for _, summary := range existing {
		used[summary.IdentityName] = struct{}{}
	}
	if _, ok := used[base]; !ok {
		return base
	}
	for idx := 2; idx < 1000; idx++ {
		candidate := fmt.Sprintf("%s-%d", base, idx)
		if _, ok := used[candidate]; !ok {
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d", base, time.Now().Unix())
}

func chooseNamedIdentity(requested string, existing []IdentitySummary, fallback string) string {
	if sanitized := sanitizeIdentityName(requested); sanitized != "" {
		return sanitized
	}
	base := sanitizeIdentityName(fallback)
	if base == "" {
		base = "identity"
	}
	used := map[string]struct{}{}
	for _, summary := range existing {
		used[summary.IdentityName] = struct{}{}
	}
	if _, ok := used[base]; !ok {
		return base
	}
	for idx := 2; idx < 1000; idx++ {
		candidate := fmt.Sprintf("%s-%d", base, idx)
		if _, ok := used[candidate]; !ok {
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d", base, time.Now().Unix())
}

func PreviewDefaultIdentityName(requested string, existing []IdentitySummary, fallback string) string {
	return chooseDefaultIdentityName(requested, existing, fallback)
}

func PreviewNamedIdentity(requested string, existing []IdentitySummary, fallback string) string {
	return chooseNamedIdentity(requested, existing, fallback)
}

func (m *Manager) legacyBackupRoot() string {
	return filepath.Join(m.RootDir(), LegacyBackupDirName)
}

func uniqueBackupDir(base string) string {
	candidate := base
	for idx := 2; fileExists(candidate); idx++ {
		candidate = fmt.Sprintf("%s-%d", base, idx)
	}
	return candidate
}

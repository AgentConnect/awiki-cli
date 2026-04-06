package identity

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (m *Manager) ScanLegacy() (*LegacyScan, error) {
	root := m.LegacyRootDir()
	scan := &LegacyScan{
		RootDir:           root,
		IndexedEntries:    map[string]IndexEntry{},
		LegacyCredentials: []LegacyFlatIdentity{},
		InvalidJSONFiles:  []map[string]string{},
		OrphanE2EEFiles:   []map[string]string{},
		Hint:              LegacyLayoutHint,
	}
	if strings.TrimSpace(root) == "" {
		return scan, nil
	}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return scan, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return scan, nil
	}

	indexPath := filepath.Join(root, IndexFileName)
	if _, err := os.Stat(indexPath); err == nil {
		payload, err := loadIndexFrom(indexPath)
		if err != nil {
			return nil, err
		}
		scan.IndexedLayout = true
		for name, entry := range payload.Credentials {
			scan.IndexedEntries[name] = entry
		}
	}

	e2eeCandidates := map[string]string{}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		switch {
		case name == IndexFileName:
			continue
		case strings.HasSuffix(name, "_did_document.json"):
			continue
		case strings.HasPrefix(name, LegacyE2EEPrefix):
			e2eeCandidates[strings.TrimSuffix(strings.TrimPrefix(name, LegacyE2EEPrefix), ".json")] = filepath.Join(root, name)
			continue
		}

		fullPath := filepath.Join(root, name)
		raw, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, err
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			scan.InvalidJSONFiles = append(scan.InvalidJSONFiles, map[string]string{
				"file":   name,
				"reason": fmt.Sprintf("invalid_json: %v", err),
			})
			continue
		}
		did, didOK := payload["did"].(string)
		privateKeyPEM, keyOK := payload["private_key_pem"].(string)
		if !didOK || did == "" || !keyOK || privateKeyPEM == "" {
			scan.InvalidJSONFiles = append(scan.InvalidJSONFiles, map[string]string{
				"file":   name,
				"reason": "not_a_legacy_credential_payload",
			})
			continue
		}
		uniqueID := stringValue(payload["unique_id"], "")
		if uniqueID == "" {
			parts := strings.Split(did, ":")
			uniqueID = parts[len(parts)-1]
		}
		scan.LegacyCredentials = append(scan.LegacyCredentials, LegacyFlatIdentity{
			CredentialName: strings.TrimSuffix(name, ".json"),
			Path:           fullPath,
			DID:            did,
			UniqueID:       uniqueID,
			Handle:         stringValue(payload["handle"], ""),
		})
	}

	legacyNames := map[string]struct{}{}
	for _, entry := range scan.LegacyCredentials {
		legacyNames[entry.CredentialName] = struct{}{}
	}
	for name, path := range e2eeCandidates {
		if _, ok := legacyNames[name]; ok {
			continue
		}
		scan.OrphanE2EEFiles = append(scan.OrphanE2EEFiles, map[string]string{
			"credential_name": name,
			"file":            path,
		})
	}
	sort.Slice(scan.LegacyCredentials, func(i, j int) bool {
		return scan.LegacyCredentials[i].CredentialName < scan.LegacyCredentials[j].CredentialName
	})
	sort.Slice(scan.InvalidJSONFiles, func(i, j int) bool {
		return scan.InvalidJSONFiles[i]["file"] < scan.InvalidJSONFiles[j]["file"]
	})
	sort.Slice(scan.OrphanE2EEFiles, func(i, j int) bool {
		return scan.OrphanE2EEFiles[i]["credential_name"] < scan.OrphanE2EEFiles[j]["credential_name"]
	})
	scan.HasLegacy = scan.IndexedLayout || len(scan.LegacyCredentials) > 0 || len(scan.InvalidJSONFiles) > 0 || len(scan.OrphanE2EEFiles) > 0
	return scan, nil
}

func (m *Manager) ImportLegacy(name string) (*ImportResult, error) {
	scan, err := m.ScanLegacy()
	if err != nil {
		return nil, err
	}
	result := &ImportResult{}
	if !scan.HasLegacy {
		return nil, fmt.Errorf("%w: no legacy layout detected", ErrLegacyNotFound)
	}
	if name == "" {
		if len(scan.IndexedEntries) == 1 {
			for candidate := range scan.IndexedEntries {
				name = candidate
			}
		} else if len(scan.LegacyCredentials) == 1 {
			name = scan.LegacyCredentials[0].CredentialName
		} else {
			return nil, fmt.Errorf("%w: multiple legacy identities detected, specify --name or --all", ErrInvalidInput)
		}
	}

	if entry, ok := scan.IndexedEntries[name]; ok {
		summary, err := m.importIndexedEntry(name, entry, scan)
		if err != nil {
			return nil, err
		}
		result.Imported = append(result.Imported, *summary)
		return result, nil
	}
	for _, legacy := range scan.LegacyCredentials {
		if legacy.CredentialName != name {
			continue
		}
		summary, err := m.importFlatLegacy(legacy)
		if err != nil {
			return nil, err
		}
		result.Imported = append(result.Imported, *summary)
		return result, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrLegacyNotFound, name)
}

func (m *Manager) ImportAllLegacy() (*ImportResult, error) {
	scan, err := m.ScanLegacy()
	if err != nil {
		return nil, err
	}
	if !scan.HasLegacy {
		return nil, fmt.Errorf("%w: no legacy layout detected", ErrLegacyNotFound)
	}
	result := &ImportResult{}
	names := make([]string, 0, len(scan.IndexedEntries))
	for name := range scan.IndexedEntries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		summary, err := m.importIndexedEntry(name, scan.IndexedEntries[name], scan)
		if err != nil {
			if strings.Contains(err.Error(), ErrIdentityConflict.Error()) {
				result.Skipped = append(result.Skipped, name)
				continue
			}
			return nil, err
		}
		result.Imported = append(result.Imported, *summary)
	}
	for _, legacy := range scan.LegacyCredentials {
		summary, err := m.importFlatLegacy(legacy)
		if err != nil {
			if strings.Contains(err.Error(), ErrIdentityConflict.Error()) {
				result.Skipped = append(result.Skipped, legacy.CredentialName)
				continue
			}
			return nil, err
		}
		result.Imported = append(result.Imported, *summary)
	}
	return result, nil
}

func (m *Manager) importIndexedEntry(name string, entry IndexEntry, scan *LegacyScan) (*IdentitySummary, error) {
	index, err := m.LoadIndex()
	if err != nil {
		return nil, err
	}
	if existing, ok := index.Credentials[name]; ok && existing.DID != entry.DID {
		return nil, fmt.Errorf("%w: identity %s already exists", ErrIdentityConflict, name)
	}
	srcDir := filepath.Join(scan.RootDir, entry.DirName)
	dst := m.BuildPaths(entry.DirName)
	if err := ensureDir(m.RootDir()); err != nil {
		return nil, err
	}
	if err := copyDir(srcDir, dst.IdentityDir); err != nil {
		return nil, err
	}
	entry.CredentialName = name
	if index.DefaultCredentialName == "" && scan.IndexedLayout {
		if payload, err := loadIndexFrom(filepath.Join(scan.RootDir, IndexFileName)); err == nil && payload.DefaultCredentialName == name {
			index.DefaultCredentialName = name
			entry.IsDefault = true
		}
	}
	index.Credentials[name] = entry
	if err := m.SaveIndex(index); err != nil {
		return nil, err
	}
	return m.summaryFor(entry, index.DefaultCredentialName)
}

func (m *Manager) importFlatLegacy(legacy LegacyFlatIdentity) (*IdentitySummary, error) {
	index, err := m.LoadIndex()
	if err != nil {
		return nil, err
	}
	if existing, ok := index.Credentials[legacy.CredentialName]; ok && existing.DID != legacy.DID {
		return nil, fmt.Errorf("%w: identity %s already exists", ErrIdentityConflict, legacy.CredentialName)
	}
	payload, err := readJSONMap(legacy.Path)
	if err != nil {
		return nil, err
	}
	dirName, err := preferredDirName(legacy.UniqueID)
	if err != nil {
		return nil, err
	}
	input := SaveInput{
		IdentityName:            legacy.CredentialName,
		DID:                     legacy.DID,
		UniqueID:                legacy.UniqueID,
		UserID:                  stringValue(payload["user_id"], ""),
		DisplayName:             stringValue(payload["name"], ""),
		Handle:                  stringValue(payload["handle"], legacy.Handle),
		JWTToken:                stringValue(payload["jwt_token"], ""),
		DIDDocument:             mapValue(payload["did_document"]),
		Key1PrivatePEM:          stringValue(payload["private_key_pem"], ""),
		Key1PublicPEM:           stringValue(payload["public_key_pem"], ""),
		E2EESigningPrivatePEM:   stringValue(payload["e2ee_signing_private_pem"], ""),
		E2EEAgreementPrivatePEM: stringValue(payload["e2ee_agreement_private_pem"], ""),
	}
	record, err := m.save(input)
	if err != nil {
		return nil, err
	}
	e2eeStatePath := filepath.Join(scanFlatRoot(legacy.Path), LegacyE2EEPrefix+legacy.CredentialName+".json")
	if fileExists(e2eeStatePath) {
		dst := m.BuildPaths(dirName)
		raw, readErr := os.ReadFile(e2eeStatePath)
		if readErr == nil {
			_ = os.WriteFile(dst.E2EEStatePath, raw, 0o600)
			_ = os.Chmod(dst.E2EEStatePath, 0o600)
		}
	}
	return &IdentitySummary{
		IdentityName:            record.IdentityName,
		DID:                     record.DID,
		UniqueID:                record.UniqueID,
		UserID:                  record.UserID,
		DisplayName:             record.DisplayName,
		Handle:                  record.Handle,
		CreatedAt:               record.CreatedAt,
		DirName:                 record.DirName,
		IsDefault:               record.IsDefault,
		HasJWT:                  record.JWTToken != "",
		HasDIDDocument:          record.DIDDocument != nil,
		HasKey1Private:          record.Key1PrivatePEM != "",
		HasKey1Public:           record.Key1PublicPEM != "",
		HasE2EESigningPrivate:   record.E2EESigningPrivatePEM != "",
		HasE2EEAgreementPrivate: record.E2EEAgreementPrivatePEM != "",
	}, nil
}

func scanFlatRoot(path string) string {
	return filepath.Dir(path)
}

func mapValue(value any) map[string]any {
	if value == nil {
		return nil
	}
	payload, ok := value.(map[string]any)
	if ok {
		return payload
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	return decoded
}

func copyDir(src, dst string) error {
	if err := ensureDir(dst); err != nil {
		return err
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			if rel == "." {
				return nil
			}
			return ensureDir(target)
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := ensureDir(filepath.Dir(dst)); err != nil {
		return err
	}
	from, err := os.Open(src)
	if err != nil {
		return err
	}
	defer from.Close()
	to, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer to.Close()
	if _, err := io.Copy(to, from); err != nil {
		return err
	}
	if err := to.Sync(); err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o600
	}
	return os.Chmod(dst, mode)
}

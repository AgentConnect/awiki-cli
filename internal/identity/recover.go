package identity

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type recoverPlan struct {
	TargetHandle         string
	TargetLocalPart      string
	EffectiveDomain      string
	HandleKey            string
	FinalIdentityName    string
	TempIdentityName     string
	BackupPathPreview    string
	SameHandleCandidates []IdentitySummary
	ExcludedIdentities   []IdentitySummary
}

type RecoverFinalizeError struct {
	Err              error
	BackupPath       string
	TempIdentityName string
	NewDID           string
}

func (e *RecoverFinalizeError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%v (backup_path=%s temp_identity_name=%s new_did=%s)", e.Err, e.BackupPath, e.TempIdentityName, e.NewDID)
}

func (e *RecoverFinalizeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func canonicalHandle(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func (p recoverPlan) ArchivedIdentityNames() []string {
	names := make([]string, 0, len(p.SameHandleCandidates))
	for _, summary := range p.SameHandleCandidates {
		names = append(names, summary.IdentityName)
	}
	return names
}

func (p recoverPlan) ArchivedDIDs() []string {
	dids := make([]string, 0, len(p.SameHandleCandidates))
	for _, summary := range p.SameHandleCandidates {
		if strings.TrimSpace(summary.DID) == "" {
			continue
		}
		dids = append(dids, summary.DID)
	}
	return dids
}

func (p recoverPlan) OldOwnerDIDsInMergeOrder() []string {
	dids := make([]string, 0, len(p.SameHandleCandidates))
	seen := make(map[string]struct{}, len(p.SameHandleCandidates))
	for _, summary := range p.SameHandleCandidates {
		did := strings.TrimSpace(summary.DID)
		if did == "" {
			continue
		}
		if _, ok := seen[did]; ok {
			continue
		}
		seen[did] = struct{}{}
		dids = append(dids, did)
	}
	return dids
}

func (s *Service) buildRecoverPlan(params RecoverParams) (recoverPlan, error) {
	target, err := NormalizeHandleInput(params.Handle, s.config.DIDDomain)
	if err != nil {
		return recoverPlan{}, err
	}
	existing, err := s.manager.List()
	if err != nil {
		return recoverPlan{}, err
	}
	identityBase := target.LocalPart
	if target.ExplicitDomain {
		identityBase = target.FullHandle
	}
	finalIdentityName := sanitizeIdentityName(identityBase)
	if finalIdentityName == "" {
		return recoverPlan{}, fmt.Errorf("%w: handle %q cannot be used as an identity name", ErrInvalidInput, params.Handle)
	}

	handleKey := canonicalHandle(target.FullHandle)
	sameHandle := make([]IdentitySummary, 0)
	excluded := make([]IdentitySummary, 0)
	for _, summary := range existing {
		if canonicalHandle(defaultString(summary.FullHandle, deriveFullHandleFromDID(summary.Handle, summary.DID))) == handleKey {
			sameHandle = append(sameHandle, summary)
			continue
		}
		excluded = append(excluded, summary)
	}
	sort.SliceStable(sameHandle, func(i, j int) bool {
		left := sameHandle[i]
		right := sameHandle[j]
		switch compareRFC3339(left.CreatedAt, right.CreatedAt) {
		case -1:
			return true
		case 1:
			return false
		default:
			return left.IdentityName < right.IdentityName
		}
	})
	for _, summary := range excluded {
		if summary.IdentityName == finalIdentityName {
			return recoverPlan{}, fmt.Errorf("%w: identity name %s is already used by another handle", ErrIdentityConflict, finalIdentityName)
		}
	}

	tempBase := finalIdentityName + "-recover-tmp"
	tempIdentityName := chooseNamedIdentity(tempBase, existing, tempBase)
	return recoverPlan{
		TargetHandle:         target.FullHandle,
		TargetLocalPart:      target.LocalPart,
		EffectiveDomain:      target.EffectiveDomain,
		HandleKey:            handleKey,
		FinalIdentityName:    finalIdentityName,
		TempIdentityName:     tempIdentityName,
		BackupPathPreview:    s.manager.PreviewRecoverHandleBackupPath(target.FullHandle),
		SameHandleCandidates: sameHandle,
		ExcludedIdentities:   excluded,
	}, nil
}

func (m *Manager) PreviewRecoverHandleBackupPath(handle string) string {
	return filepath.Join(m.legacyBackupRoot(), "recover-handle", backupPreviewName(handle))
}

func (m *Manager) BackupIdentitiesForHandleRecovery(handle string, candidates []IdentitySummary, plannedFinalName string, plannedTempName string, activeBefore string) (string, error) {
	if strings.TrimSpace(handle) == "" {
		return "", fmt.Errorf("%w: handle is required", ErrInvalidInput)
	}
	if err := m.EnsureRoot(); err != nil {
		return "", err
	}
	index, err := m.LoadIndex()
	if err != nil {
		return "", err
	}
	createdAt := time.Now().UTC()
	backupDir := uniqueBackupDir(filepath.Join(m.legacyBackupRoot(), "recover-handle", backupDirName(createdAt, handle)))
	if err := ensureDir(backupDir); err != nil {
		return "", fmt.Errorf("create recover backup directory: %w", err)
	}
	if err := saveIndexTo(filepath.Join(backupDir, "index.before.json"), index); err != nil {
		return "", fmt.Errorf("write recover backup index snapshot: %w", err)
	}
	if strings.TrimSpace(m.paths.ConfigFile) != "" {
		if raw, err := os.ReadFile(m.paths.ConfigFile); err == nil {
			if err := writeSecureText(filepath.Join(backupDir, "config.before.yaml"), string(raw)); err != nil {
				return "", fmt.Errorf("write recover backup config snapshot: %w", err)
			}
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("read config before recover backup: %w", err)
		}
	}

	archivedNames := make([]string, 0, len(candidates))
	archivedDIDs := make([]string, 0, len(candidates))
	archivedDirs := make([]string, 0, len(candidates))
	for idx, summary := range candidates {
		archivedNames = append(archivedNames, summary.IdentityName)
		archivedDIDs = append(archivedDIDs, summary.DID)
		archivedDirs = append(archivedDirs, summary.DirName)
		src := m.BuildPaths(summary.DirName).IdentityDir
		info, err := os.Stat(src)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("stat identity directory before recover backup: %w", err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("%w: identity path is not a directory: %s", ErrInvalidInput, src)
		}
		targetName := fmt.Sprintf("%02d-%s", idx+1, sanitizeComponent(summary.IdentityName))
		if strings.TrimSpace(targetName) == "" {
			targetName = fmt.Sprintf("%02d-%s", idx+1, sanitizeComponent(summary.DirName))
		}
		if err := copyDir(src, filepath.Join(backupDir, "identities", targetName)); err != nil {
			return "", fmt.Errorf("backup identity directory before recover: %w", err)
		}
	}
	manifest := map[string]any{
		"reason":                  "recover_handle",
		"created_at":              createdAt.Format(time.RFC3339Nano),
		"handle":                  handle,
		"archived_identity_names": archivedNames,
		"archived_dids":           archivedDIDs,
		"archived_dir_names":      archivedDirs,
		"default_before":          index.DefaultCredentialName,
		"active_before":           strings.TrimSpace(activeBefore),
		"planned_final_identity":  plannedFinalName,
		"planned_temp_identity":   plannedTempName,
	}
	if err := writeSecureJSON(filepath.Join(backupDir, "backup_manifest.json"), manifest); err != nil {
		return "", fmt.Errorf("write recover backup manifest: %w", err)
	}
	return backupDir, nil
}

func (m *Manager) PromoteRecoveredHandle(finalIdentityName string, tempIdentityName string, archivedIdentityNames []string) (*StoredIdentity, bool, error) {
	if strings.TrimSpace(finalIdentityName) == "" {
		return nil, false, fmt.Errorf("%w: final identity name is required", ErrInvalidInput)
	}
	if strings.TrimSpace(tempIdentityName) == "" {
		return nil, false, fmt.Errorf("%w: temporary identity name is required", ErrInvalidInput)
	}
	index, err := m.LoadIndex()
	if err != nil {
		return nil, false, err
	}
	tempEntry, ok := index.Credentials[tempIdentityName]
	if !ok {
		return nil, false, fmt.Errorf("%w: %s", ErrIdentityNotFound, tempIdentityName)
	}
	archivedSet := make(map[string]struct{}, len(archivedIdentityNames))
	for _, name := range archivedIdentityNames {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			archivedSet[trimmed] = struct{}{}
		}
	}
	for name := range index.Credentials {
		if name == tempIdentityName {
			continue
		}
		if _, ok := archivedSet[name]; ok {
			continue
		}
		if name == finalIdentityName {
			return nil, false, fmt.Errorf("%w: identity name %s is already used by another live identity", ErrIdentityConflict, finalIdentityName)
		}
	}

	for name := range archivedSet {
		delete(index.Credentials, name)
	}
	delete(index.Credentials, tempIdentityName)
	tempEntry.CredentialName = finalIdentityName
	tempEntry.IsDefault = false
	index.Credentials[finalIdentityName] = tempEntry

	defaultUpdated := false
	if currentDefault := strings.TrimSpace(index.DefaultCredentialName); currentDefault != "" {
		if currentDefault == tempIdentityName {
			index.DefaultCredentialName = finalIdentityName
			defaultUpdated = true
		} else if _, ok := archivedSet[currentDefault]; ok {
			index.DefaultCredentialName = finalIdentityName
			defaultUpdated = true
		}
	}
	if err := m.SaveIndex(index); err != nil {
		return nil, false, err
	}
	record, err := m.Load(finalIdentityName)
	if err != nil {
		return nil, false, err
	}
	return record, defaultUpdated, nil
}

func backupPreviewName(handle string) string {
	handlePart := sanitizeIdentityName(handle)
	if handlePart == "" {
		handlePart = "handle"
	}
	return fmt.Sprintf("<timestamp>-%s", handlePart)
}

func backupDirName(createdAt time.Time, handle string) string {
	handlePart := sanitizeIdentityName(handle)
	if handlePart == "" {
		handlePart = "handle"
	}
	return fmt.Sprintf("%s-%s", createdAt.UTC().Format("20060102T150405.000000000Z"), handlePart)
}

func compareRFC3339(left string, right string) int {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	switch {
	case left == "" && right == "":
		return 0
	case left == "":
		return 1
	case right == "":
		return -1
	}
	leftTime, leftErr := time.Parse(time.RFC3339Nano, left)
	rightTime, rightErr := time.Parse(time.RFC3339Nano, right)
	if leftErr == nil && rightErr == nil {
		switch {
		case leftTime.Before(rightTime):
			return -1
		case leftTime.After(rightTime):
			return 1
		default:
			return 0
		}
	}
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

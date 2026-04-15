package message

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/agentconnect/awiki-cli/internal/store"
)

const contactLookupTimeout = 5 * time.Second

func (s *Service) syncPeerHandle(
	ctx context.Context,
	db *sql.DB,
	ownerDID string,
	peerDID string,
	knownHandle string,
	sourceType string,
	sourceGroupID string,
) (string, error) {
	ownerDID = strings.TrimSpace(ownerDID)
	peerDID = strings.TrimSpace(peerDID)
	if ownerDID == "" || peerDID == "" || ownerDID == peerDID {
		return "", nil
	}
	resolvedHandle, err := store.ResolveContactHandleByDID(ctx, db, ownerDID, peerDID)
	if err != nil {
		return "", err
	}
	handle := normalizeHandleValue(knownHandle)
	if handle == "" {
		handle = resolvedHandle
	}
	if handle == "" && s.remote != nil {
		lookupCtx, cancel := context.WithTimeout(ctx, contactLookupTimeout)
		defer cancel()
		lookup, lookupErr := s.remote.LookupHandleByDID(lookupCtx, peerDID)
		if lookupErr != nil {
			return resolvedHandle, lookupErr
		}
		if lookup != nil {
			handle = normalizeHandleValue(lookup.Handle)
		}
	}
	if handle == "" {
		return resolvedHandle, nil
	}
	messaged := true
	if err := store.UpsertContact(ctx, db, store.ContactRecord{
		OwnerDID:      ownerDID,
		DID:           peerDID,
		Handle:        handle,
		SourceType:    sourceType,
		SourceGroupID: sourceGroupID,
		Messaged:      &messaged,
		FirstSeenAt:   time.Now().UTC().Format(time.RFC3339),
		LastSeenAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return resolvedHandle, err
	}
	return handle, nil
}

func (s *Service) syncDirectPeerHandles(
	ctx context.Context,
	db *sql.DB,
	recordOwnerDID string,
	messages []map[string]any,
	knownHandle string,
	sourceType string,
) []string {
	seen := make(map[string]struct{}, len(messages))
	warnings := make([]string, 0)
	for _, message := range messages {
		senderDID := stringFromAny(message["sender_did"])
		receiverDID := stringFromAny(message["receiver_did"])
		peerDID := senderDID
		if peerDID == recordOwnerDID {
			peerDID = receiverDID
		}
		peerDID = strings.TrimSpace(peerDID)
		if peerDID == "" || peerDID == recordOwnerDID {
			continue
		}
		if _, ok := seen[peerDID]; ok {
			continue
		}
		seen[peerDID] = struct{}{}
		_, err := s.syncPeerHandle(ctx, db, recordOwnerDID, peerDID, knownHandle, sourceType, "")
		if err != nil {
			warnings = append(warnings, "Failed to sync contact handle for "+peerDID+": "+err.Error())
		}
	}
	return warnings
}

func normalizeHandleValue(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	value = strings.TrimPrefix(value, "wba://")
	if index := strings.Index(value, "."); index > 0 {
		return value[:index]
	}
	return value
}

func mergePeerDIDs(current string, historical []string) []string {
	seen := make(map[string]struct{}, len(historical)+1)
	result := make([]string, 0, len(historical)+1)
	if did := strings.TrimSpace(current); did != "" {
		seen[did] = struct{}{}
		result = append(result, did)
	}
	for _, did := range historical {
		did = strings.TrimSpace(did)
		if did == "" {
			continue
		}
		if _, ok := seen[did]; ok {
			continue
		}
		seen[did] = struct{}{}
		result = append(result, did)
	}
	return result
}

func (s *Service) peerDIDsForHandleFromStore(ctx context.Context, ownerDID string, handle string, currentDID string) ([]string, error) {
	handle = normalizeHandleValue(handle)
	if handle == "" {
		return mergePeerDIDs(currentDID, nil), nil
	}
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	dids, err := store.ListDIDsByHandle(ctx, db, ownerDID, handle)
	if err != nil {
		return nil, err
	}
	return mergePeerDIDs(currentDID, dids), nil
}

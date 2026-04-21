package listener

import (
	"context"
	"strings"
	"time"

	"github.com/agentconnect/awiki-cli/internal/store"
	"github.com/agentconnect/awiki-cli/internal/transportcfg"
)

func (s *Supervisor) syncIncomingContact(
	ctx context.Context,
	session *session,
	senderDID string,
	sourceType string,
	sourceGroupID string,
) (string, error) {
	ownerDID := strings.TrimSpace(session.record.DID)
	senderDID = strings.TrimSpace(senderDID)
	if ownerDID == "" || senderDID == "" || ownerDID == senderDID {
		return "", nil
	}
	localHandle, err := store.ResolveContactHandleByDID(ctx, s.db, ownerDID, senderDID)
	if err != nil {
		return "", err
	}
	if localHandle != "" {
		return localHandle, nil
	}
	if s.remote == nil {
		return "", nil
	}
	lookupCtx, cancel := transportcfg.WithProfileTimeout(ctx, transportcfg.ProfileRPCDefault)
	defer cancel()
	result, err := s.remote.LookupHandleByDID(lookupCtx, senderDID)
	if err != nil || result == nil {
		return "", err
	}
	messaged := true
	handle := normalizeListenerHandle(result.Handle)
	if handle == "" {
		return "", nil
	}
	if err := store.UpsertContact(lookupCtx, s.db, store.ContactRecord{
		OwnerDID:      ownerDID,
		DID:           senderDID,
		Handle:        handle,
		SourceType:    sourceType,
		SourceGroupID: sourceGroupID,
		Messaged:      &messaged,
		FirstSeenAt:   time.Now().UTC().Format(time.RFC3339),
		LastSeenAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return "", err
	}
	return handle, nil
}

func normalizeListenerHandle(value string) string {
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

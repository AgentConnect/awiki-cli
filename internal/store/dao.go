package store

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

func StoreMessage(ctx context.Context, db *sql.DB, record MessageRecord) error {
	if strings.TrimSpace(record.MsgID) == "" {
		return fmt.Errorf("msg_id is required")
	}
	if strings.TrimSpace(record.ThreadID) == "" {
		return fmt.Errorf("thread_id is required")
	}
	now := nowUTC()
	_, err := db.ExecContext(ctx, storeMessageSQL(),
		messageRecordArgs(record, now)...,
	)
	return err
}

func StoreMessagesBatch(ctx context.Context, db *sql.DB, batch []MessageRecord) error {
	if len(batch) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, storeMessageSQL())
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	now := nowUTC()
	for _, record := range batch {
		if _, err := stmt.ExecContext(ctx, messageRecordArgs(record, now)...); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func storeMessageSQL() string {
	return `
INSERT INTO messages
    (msg_id, owner_did, thread_id, direction, sender_did, receiver_did, group_id, group_did,
     content_type, content, title, server_seq, sent_at, stored_at, is_e2ee, is_read,
     sender_name, metadata, credential_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(msg_id, owner_did)
DO UPDATE SET
    thread_id = excluded.thread_id,
    direction = excluded.direction,
    sender_did = excluded.sender_did,
    receiver_did = excluded.receiver_did,
    group_id = excluded.group_id,
    group_did = excluded.group_did,
    content_type = CASE
        WHEN excluded.content_type IN ('application/anp-direct-init+json', 'application/anp-direct-cipher+json')
             AND messages.content_type NOT IN ('application/anp-direct-init+json', 'application/anp-direct-cipher+json')
        THEN messages.content_type
        ELSE excluded.content_type
    END,
    content = CASE
        WHEN excluded.content_type IN ('application/anp-direct-init+json', 'application/anp-direct-cipher+json')
             AND messages.content_type NOT IN ('application/anp-direct-init+json', 'application/anp-direct-cipher+json')
        THEN messages.content
        ELSE excluded.content
    END,
    title = excluded.title,
    server_seq = COALESCE(excluded.server_seq, messages.server_seq),
    sent_at = COALESCE(excluded.sent_at, messages.sent_at),
    is_e2ee = CASE WHEN excluded.is_e2ee = 1 OR messages.is_e2ee = 1 THEN 1 ELSE 0 END,
    is_read = CASE WHEN excluded.is_read = 1 OR messages.is_read = 1 THEN 1 ELSE 0 END,
    sender_name = COALESCE(excluded.sender_name, messages.sender_name),
    metadata = CASE
        WHEN excluded.content_type IN ('application/anp-direct-init+json', 'application/anp-direct-cipher+json')
             AND messages.content_type NOT IN ('application/anp-direct-init+json', 'application/anp-direct-cipher+json')
        THEN messages.metadata
        ELSE COALESCE(excluded.metadata, messages.metadata)
    END,
    credential_name = COALESCE(excluded.credential_name, messages.credential_name)`
}

func messageRecordArgs(record MessageRecord, now string) []any {
	return []any{
		record.MsgID,
		normalizeOwnerDID(record.OwnerDID),
		record.ThreadID,
		record.Direction,
		normalizeOptionalString(record.SenderDID),
		normalizeOptionalString(record.ReceiverDID),
		normalizeOptionalString(record.GroupID),
		normalizeOptionalString(record.GroupDID),
		defaultString(record.ContentType, "text"),
		record.Content,
		normalizeOptionalString(record.Title),
		normalizeOptionalInt64(record.ServerSeq),
		normalizeOptionalString(record.SentAt),
		defaultString(record.StoredAt, now),
		boolToInt(record.IsE2EE),
		boolToInt(record.IsRead),
		normalizeOptionalString(record.SenderName),
		normalizeMetadata(record.Metadata),
		normalizeCredentialName(record.CredentialName),
	}
}

func QueueE2EEOutbox(ctx context.Context, db *sql.DB, record E2EEOutboxRecord) (string, error) {
	outboxID := defaultString(record.OutboxID, generateID())
	now := nowUTC()
	_, err := db.ExecContext(ctx, `
INSERT INTO e2ee_outbox
    (outbox_id, owner_did, peer_did, session_id, original_type, plaintext, local_status,
     attempt_count, sent_msg_id, sent_server_seq, last_error_code, retry_hint, failed_msg_id,
     failed_server_seq, metadata, last_attempt_at, created_at, updated_at, credential_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		outboxID,
		normalizeOwnerDID(record.OwnerDID),
		record.PeerDID,
		normalizeOptionalString(record.SessionID),
		defaultString(record.OriginalType, "text"),
		record.Plaintext,
		defaultString(record.LocalStatus, "queued"),
		record.AttemptCount,
		normalizeOptionalString(record.SentMsgID),
		normalizeOptionalInt64(record.SentServerSeq),
		normalizeOptionalString(record.LastErrorCode),
		normalizeOptionalString(record.RetryHint),
		normalizeOptionalString(record.FailedMsgID),
		normalizeOptionalInt64(record.FailedServerSeq),
		normalizeMetadata(record.Metadata),
		normalizeOptionalString(record.LastAttemptAt),
		defaultString(record.CreatedAt, now),
		defaultString(record.UpdatedAt, now),
		normalizeCredentialName(record.CredentialName),
	)
	return outboxID, err
}

func MarkE2EEOutboxSent(ctx context.Context, db *sql.DB, outboxID string, ownerDID string, sessionID string, sentMsgID string, sentServerSeq *int64, metadata string) error {
	_, err := db.ExecContext(ctx, `
UPDATE e2ee_outbox
SET session_id = COALESCE(?, session_id),
    local_status = 'sent',
    attempt_count = attempt_count + 1,
    sent_msg_id = COALESCE(?, sent_msg_id),
    sent_server_seq = COALESCE(?, sent_server_seq),
    metadata = COALESCE(?, metadata),
    last_attempt_at = ?,
    updated_at = ?,
    last_error_code = NULL,
    retry_hint = NULL,
    failed_msg_id = NULL,
    failed_server_seq = NULL
WHERE outbox_id = ? AND owner_did = ?`,
		normalizeOptionalString(sessionID),
		normalizeOptionalString(sentMsgID),
		normalizeOptionalInt64(sentServerSeq),
		normalizeMetadata(metadata),
		nowUTC(),
		nowUTC(),
		outboxID,
		normalizeOwnerDID(ownerDID),
	)
	return err
}

func MarkE2EEOutboxFailed(ctx context.Context, db *sql.DB, outboxID string, ownerDID string, errorCode string, retryHint string, failedMsgID string, failedServerSeq *int64, metadata string) error {
	_, err := db.ExecContext(ctx, `
UPDATE e2ee_outbox
SET local_status = 'failed',
    last_error_code = ?,
    retry_hint = COALESCE(?, retry_hint),
    failed_msg_id = COALESCE(?, failed_msg_id),
    failed_server_seq = COALESCE(?, failed_server_seq),
    metadata = COALESCE(?, metadata),
    updated_at = ?
WHERE outbox_id = ? AND owner_did = ?`,
		errorCode,
		normalizeOptionalString(retryHint),
		normalizeOptionalString(failedMsgID),
		normalizeOptionalInt64(failedServerSeq),
		normalizeMetadata(metadata),
		nowUTC(),
		outboxID,
		normalizeOwnerDID(ownerDID),
	)
	return err
}

func UpdateE2EEOutboxStatus(ctx context.Context, db *sql.DB, outboxID string, ownerDID string, credentialName string, status string) error {
	if strings.TrimSpace(ownerDID) != "" {
		_, err := db.ExecContext(ctx, `UPDATE e2ee_outbox SET local_status = ?, updated_at = ? WHERE outbox_id = ? AND owner_did = ?`,
			status, nowUTC(), outboxID, normalizeOwnerDID(ownerDID))
		return err
	}
	_, err := db.ExecContext(ctx, `UPDATE e2ee_outbox SET local_status = ?, updated_at = ? WHERE outbox_id = ? AND credential_name = ?`,
		status, nowUTC(), outboxID, normalizeCredentialName(credentialName))
	return err
}

func SetE2EEOutboxFailureByID(ctx context.Context, db *sql.DB, outboxID string, ownerDID string, credentialName string, errorCode string, retryHint string, metadata string) error {
	if strings.TrimSpace(ownerDID) != "" {
		_, err := db.ExecContext(ctx, `
UPDATE e2ee_outbox
SET local_status = 'failed',
    last_error_code = ?,
    retry_hint = COALESCE(?, retry_hint),
    metadata = COALESCE(?, metadata),
    updated_at = ?
WHERE outbox_id = ? AND owner_did = ?`,
			errorCode, normalizeOptionalString(retryHint), normalizeMetadata(metadata), nowUTC(), outboxID, normalizeOwnerDID(ownerDID))
		return err
	}
	_, err := db.ExecContext(ctx, `
UPDATE e2ee_outbox
SET local_status = 'failed',
    last_error_code = ?,
    retry_hint = COALESCE(?, retry_hint),
    metadata = COALESCE(?, metadata),
    updated_at = ?
WHERE outbox_id = ? AND credential_name = ?`,
		errorCode, normalizeOptionalString(retryHint), normalizeMetadata(metadata), nowUTC(), outboxID, normalizeCredentialName(credentialName))
	return err
}

func GetE2EEOutbox(ctx context.Context, db *sql.DB, outboxID string, ownerDID string, credentialName string) (map[string]any, error) {
	if strings.TrimSpace(ownerDID) != "" {
		return queryOneMap(ctx, db, `SELECT * FROM e2ee_outbox WHERE outbox_id = ? AND owner_did = ?`, outboxID, normalizeOwnerDID(ownerDID))
	}
	return queryOneMap(ctx, db, `SELECT * FROM e2ee_outbox WHERE outbox_id = ? AND credential_name = ?`, outboxID, normalizeCredentialName(credentialName))
}

func ListE2EEOutbox(ctx context.Context, db *sql.DB, ownerDID string, credentialName string, localStatus string) ([]map[string]any, error) {
	switch {
	case strings.TrimSpace(ownerDID) != "" && strings.TrimSpace(localStatus) != "":
		return queryMaps(ctx, db, `SELECT * FROM e2ee_outbox WHERE owner_did = ? AND local_status = ? ORDER BY updated_at DESC`, normalizeOwnerDID(ownerDID), localStatus)
	case strings.TrimSpace(ownerDID) != "":
		return queryMaps(ctx, db, `SELECT * FROM e2ee_outbox WHERE owner_did = ? ORDER BY updated_at DESC`, normalizeOwnerDID(ownerDID))
	case strings.TrimSpace(localStatus) != "":
		return queryMaps(ctx, db, `SELECT * FROM e2ee_outbox WHERE credential_name = ? AND local_status = ? ORDER BY updated_at DESC`, normalizeCredentialName(credentialName), localStatus)
	default:
		return queryMaps(ctx, db, `SELECT * FROM e2ee_outbox WHERE credential_name = ? ORDER BY updated_at DESC`, normalizeCredentialName(credentialName))
	}
}

func GetMessageByID(ctx context.Context, db *sql.DB, msgID string, ownerDID string, credentialName string) (map[string]any, error) {
	if strings.TrimSpace(ownerDID) != "" {
		return queryOneMap(ctx, db, `SELECT * FROM messages WHERE msg_id = ? AND owner_did = ?`, msgID, normalizeOwnerDID(ownerDID))
	}
	return queryOneMap(ctx, db, `SELECT * FROM messages WHERE msg_id = ? AND credential_name = ?`, msgID, normalizeCredentialName(credentialName))
}

func GetContactByDID(ctx context.Context, db *sql.DB, ownerDID string, did string) (map[string]any, error) {
	return queryOneMap(ctx, db, `SELECT * FROM contacts WHERE owner_did = ? AND did = ?`, normalizeOwnerDID(ownerDID), strings.TrimSpace(did))
}

func GetCurrentContactByHandle(ctx context.Context, db *sql.DB, ownerDID string, handle string) (map[string]any, error) {
	return queryOneMap(
		ctx,
		db,
		`SELECT * FROM contacts WHERE owner_did = ? AND handle = ?`,
		normalizeOwnerDID(ownerDID),
		strings.TrimSpace(handle),
	)
}

func ResolveContactHandleByDID(ctx context.Context, db *sql.DB, ownerDID string, did string) (string, error) {
	row, err := queryOneMap(
		ctx,
		db,
		`SELECT handle FROM contacts WHERE owner_did = ? AND did = ? AND TRIM(COALESCE(handle, '')) <> ''`,
		normalizeOwnerDID(ownerDID),
		strings.TrimSpace(did),
	)
	if err == nil {
		return stringFromAny(row["handle"]), nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	row, err = queryOneMap(
		ctx,
		db,
		`SELECT handle
FROM contact_handle_bindings
WHERE owner_did = ? AND did = ?
ORDER BY is_current DESC, last_seen_at DESC
LIMIT 1`,
		normalizeOwnerDID(ownerDID),
		strings.TrimSpace(did),
	)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return stringFromAny(row["handle"]), nil
}

func ListDIDsByHandle(ctx context.Context, db *sql.DB, ownerDID string, handle string) ([]string, error) {
	rows, err := queryMaps(
		ctx,
		db,
		`SELECT did
FROM contact_handle_bindings
WHERE owner_did = ? AND handle = ?
ORDER BY is_current DESC, last_seen_at DESC`,
		normalizeOwnerDID(ownerDID),
		strings.TrimSpace(handle),
	)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		row, rowErr := queryOneMap(
			ctx,
			db,
			`SELECT did FROM contacts WHERE owner_did = ? AND handle = ?`,
			normalizeOwnerDID(ownerDID),
			strings.TrimSpace(handle),
		)
		if rowErr == sql.ErrNoRows {
			return nil, nil
		}
		if rowErr != nil {
			return nil, rowErr
		}
		return []string{stringFromAny(row["did"])}, nil
	}
	seen := make(map[string]struct{}, len(rows))
	result := make([]string, 0, len(rows))
	for _, row := range rows {
		did := stringFromAny(row["did"])
		if did == "" {
			continue
		}
		if _, ok := seen[did]; ok {
			continue
		}
		seen[did] = struct{}{}
		result = append(result, did)
	}
	return result, nil
}

func UpsertContact(ctx context.Context, db *sql.DB, record ContactRecord) error {
	if strings.TrimSpace(record.DID) == "" {
		return fmt.Errorf("contact did is required")
	}
	ownerDID := normalizeOwnerDID(record.OwnerDID)
	handle := strings.TrimSpace(record.Handle)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	now := nowUTC()
	existingByDID, err := queryMapsWithQueryer(ctx, tx, `SELECT did, handle FROM contacts WHERE owner_did = ? AND did = ?`, ownerDID, record.DID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	existingByHandle := []map[string]any{}
	if handle != "" {
		existingByHandle, err = queryMapsWithQueryer(ctx, tx, `SELECT did, handle FROM contacts WHERE owner_did = ? AND handle = ?`, ownerDID, handle)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if handle != "" && len(existingByHandle) > 0 && stringFromAny(existingByHandle[0]["did"]) != record.DID {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE contacts SET handle = NULL, last_seen_at = ? WHERE owner_did = ? AND did = ?`,
			now,
			ownerDID,
			stringFromAny(existingByHandle[0]["did"]),
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if len(existingByDID) > 0 {
		_, err = tx.ExecContext(ctx, `
UPDATE contacts
SET name = COALESCE(?, name),
    handle = COALESCE(?, handle),
    nick_name = COALESCE(?, nick_name),
    bio = COALESCE(?, bio),
    profile_md = COALESCE(?, profile_md),
    tags = COALESCE(?, tags),
    relationship = COALESCE(?, relationship),
    source_type = COALESCE(?, source_type),
    source_name = COALESCE(?, source_name),
    source_group_id = COALESCE(?, source_group_id),
    connected_at = COALESCE(?, connected_at),
    recommended_reason = COALESCE(?, recommended_reason),
    followed = COALESCE(?, followed),
    messaged = COALESCE(?, messaged),
    note = COALESCE(?, note),
    first_seen_at = COALESCE(?, first_seen_at),
    last_seen_at = ?,
    metadata = COALESCE(?, metadata)
WHERE owner_did = ? AND did = ?`,
			normalizeOptionalString(record.Name),
			normalizeOptionalString(handle),
			normalizeOptionalString(record.NickName),
			normalizeOptionalString(record.Bio),
			normalizeOptionalString(record.ProfileMD),
			normalizeOptionalString(record.Tags),
			normalizeOptionalString(record.Relationship),
			normalizeOptionalString(record.SourceType),
			normalizeOptionalString(record.SourceName),
			normalizeOptionalString(record.SourceGroupID),
			normalizeOptionalString(record.ConnectedAt),
			normalizeOptionalString(record.RecommendedReason),
			normalizeOptionalBool(record.Followed),
			normalizeOptionalBool(record.Messaged),
			normalizeOptionalString(record.Note),
			normalizeOptionalString(record.FirstSeenAt),
			now,
			normalizeMetadata(record.Metadata),
			ownerDID,
			record.DID,
		)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
	} else {
		_, err = tx.ExecContext(ctx, `
INSERT INTO contacts
    (owner_did, did, name, handle, nick_name, bio, profile_md, tags, relationship, source_type, source_name,
     source_group_id, connected_at, recommended_reason, followed, messaged, note, first_seen_at, last_seen_at, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ownerDID, record.DID,
			normalizeOptionalString(record.Name),
			normalizeOptionalString(record.Handle),
			normalizeOptionalString(record.NickName),
			normalizeOptionalString(record.Bio),
			normalizeOptionalString(record.ProfileMD),
			normalizeOptionalString(record.Tags),
			normalizeOptionalString(record.Relationship),
			normalizeOptionalString(record.SourceType),
			normalizeOptionalString(record.SourceName),
			normalizeOptionalString(record.SourceGroupID),
			normalizeOptionalString(record.ConnectedAt),
			normalizeOptionalString(record.RecommendedReason),
			defaultBoolValue(record.Followed),
			defaultBoolValue(record.Messaged),
			normalizeOptionalString(record.Note),
			defaultString(record.FirstSeenAt, now),
			defaultString(record.LastSeenAt, now),
			normalizeMetadata(record.Metadata),
		)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if handle != "" {
		if err := upsertContactHandleBinding(ctx, tx, ContactHandleBindingRecord{
			OwnerDID:       ownerDID,
			Handle:         handle,
			DID:            strings.TrimSpace(record.DID),
			IsCurrent:      true,
			FirstSeenAt:    defaultString(record.FirstSeenAt, now),
			LastSeenAt:     defaultString(record.LastSeenAt, now),
			SourceType:     record.SourceType,
			SourceGroupID:  record.SourceGroupID,
			Metadata:       record.Metadata,
			CredentialName: record.CredentialName,
		}); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func upsertContactHandleBinding(ctx context.Context, tx *sql.Tx, record ContactHandleBindingRecord) error {
	handle := strings.TrimSpace(record.Handle)
	did := strings.TrimSpace(record.DID)
	if handle == "" || did == "" {
		return nil
	}
	ownerDID := normalizeOwnerDID(record.OwnerDID)
	firstSeenAt := defaultString(record.FirstSeenAt, nowUTC())
	lastSeenAt := defaultString(record.LastSeenAt, firstSeenAt)
	if record.IsCurrent {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE contact_handle_bindings
SET is_current = 0,
    last_seen_at = CASE
        WHEN last_seen_at IS NULL OR last_seen_at < ? THEN ?
        ELSE last_seen_at
    END
WHERE owner_did = ? AND handle = ? AND did <> ?`,
			lastSeenAt,
			lastSeenAt,
			ownerDID,
			handle,
			did,
		); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO contact_handle_bindings
    (owner_did, handle, did, is_current, first_seen_at, last_seen_at, source_type, source_group_id, metadata, credential_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(owner_did, handle, did)
DO UPDATE SET
    is_current = excluded.is_current,
    first_seen_at = COALESCE(contact_handle_bindings.first_seen_at, excluded.first_seen_at),
    last_seen_at = excluded.last_seen_at,
    source_type = COALESCE(excluded.source_type, contact_handle_bindings.source_type),
    source_group_id = COALESCE(excluded.source_group_id, contact_handle_bindings.source_group_id),
    metadata = COALESCE(excluded.metadata, contact_handle_bindings.metadata),
    credential_name = COALESCE(excluded.credential_name, contact_handle_bindings.credential_name)`,
		ownerDID,
		handle,
		did,
		boolToInt(record.IsCurrent),
		firstSeenAt,
		lastSeenAt,
		normalizeOptionalString(record.SourceType),
		normalizeOptionalString(record.SourceGroupID),
		normalizeMetadata(record.Metadata),
		normalizeCredentialName(record.CredentialName),
	)
	return err
}

func AppendRelationshipEvent(ctx context.Context, db *sql.DB, record RelationshipEventRecord) (string, error) {
	eventID := defaultString(record.EventID, generateID())
	now := nowUTC()
	_, err := db.ExecContext(ctx, `
INSERT INTO relationship_events
    (event_id, owner_did, target_did, target_handle, event_type, source_type, source_name, source_group_id,
     reason, score, status, created_at, updated_at, metadata, credential_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		eventID,
		normalizeOwnerDID(record.OwnerDID),
		record.TargetDID,
		normalizeOptionalString(record.TargetHandle),
		record.EventType,
		normalizeOptionalString(record.SourceType),
		normalizeOptionalString(record.SourceName),
		normalizeOptionalString(record.SourceGroupID),
		normalizeOptionalString(record.Reason),
		normalizeOptionalFloat64(record.Score),
		defaultString(record.Status, "pending"),
		defaultString(record.CreatedAt, now),
		defaultString(record.UpdatedAt, now),
		normalizeMetadata(record.Metadata),
		normalizeCredentialName(record.CredentialName),
	)
	return eventID, err
}

func UpsertGroup(ctx context.Context, db *sql.DB, record GroupRecord) error {
	ownerDID := normalizeOwnerDID(record.OwnerDID)
	if ownerDID == "" || strings.TrimSpace(record.GroupID) == "" {
		return fmt.Errorf("owner_did and group_id are required")
	}
	now := nowUTC()
	_, err := db.ExecContext(ctx, `
INSERT OR REPLACE INTO groups
    (owner_did, group_id, group_did, name, group_mode, slug, description, goal, rules, message_prompt,
     doc_url, group_owner_did, group_owner_handle, my_role, membership_status, join_enabled, join_code,
     join_code_expires_at, member_count, last_synced_seq, last_read_seq, last_message_at, remote_created_at,
     remote_updated_at, stored_at, metadata, credential_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ownerDID,
		record.GroupID,
		normalizeOptionalString(record.GroupDID),
		normalizeOptionalString(record.Name),
		defaultString(record.GroupMode, "general"),
		normalizeOptionalString(record.Slug),
		normalizeOptionalString(record.Description),
		normalizeOptionalString(record.Goal),
		normalizeOptionalString(record.Rules),
		normalizeOptionalString(record.MessagePrompt),
		normalizeOptionalString(record.DocURL),
		normalizeOptionalString(record.GroupOwnerDID),
		normalizeOptionalString(record.GroupOwnerHandle),
		normalizeOptionalString(record.MyRole),
		defaultString(record.MembershipStatus, "active"),
		normalizeOptionalBool(record.JoinEnabled),
		normalizeOptionalString(record.JoinCode),
		normalizeOptionalString(record.JoinCodeExpiresAt),
		normalizeOptionalInt64(record.MemberCount),
		normalizeOptionalInt64(record.LastSyncedSeq),
		normalizeOptionalInt64(record.LastReadSeq),
		normalizeOptionalString(record.LastMessageAt),
		normalizeOptionalString(record.RemoteCreatedAt),
		normalizeOptionalString(record.RemoteUpdatedAt),
		defaultString(record.StoredAt, now),
		normalizeMetadata(record.Metadata),
		normalizeCredentialName(record.CredentialName),
	)
	return err
}

func ReplaceGroupMembers(ctx context.Context, db *sql.DB, ownerDID string, groupID string, members []GroupMemberRecord, credentialName string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM group_members WHERE owner_did = ? AND group_id = ?`, normalizeOwnerDID(ownerDID), groupID); err != nil {
		_ = tx.Rollback()
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO group_members
    (owner_did, group_id, user_id, member_did, member_handle, profile_url, role, status,
     joined_at, sent_message_count, last_synced_at, metadata, credential_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	now := nowUTC()
	zero := int64(0)
	for _, member := range members {
		if strings.TrimSpace(member.UserID) == "" {
			continue
		}
		if _, err := stmt.ExecContext(ctx,
			normalizeOwnerDID(ownerDID),
			groupID,
			member.UserID,
			normalizeOptionalString(member.MemberDID),
			normalizeOptionalString(member.MemberHandle),
			normalizeOptionalString(member.ProfileURL),
			normalizeOptionalString(member.Role),
			defaultString(member.Status, "active"),
			normalizeOptionalString(member.JoinedAt),
			normalizeOptionalInt64(defaultInt64Ptr(member.SentMessageCount, &zero)),
			defaultString(member.LastSyncedAt, now),
			normalizeMetadata(member.Metadata),
			normalizeCredentialName(defaultString(member.CredentialName, credentialName)),
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func UpsertGroupMember(ctx context.Context, db *sql.DB, record GroupMemberRecord) error {
	zero := int64(0)
	_, err := db.ExecContext(ctx, `
INSERT OR REPLACE INTO group_members
    (owner_did, group_id, user_id, member_did, member_handle, profile_url, role, status,
     joined_at, sent_message_count, last_synced_at, metadata, credential_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		normalizeOwnerDID(record.OwnerDID),
		record.GroupID,
		record.UserID,
		normalizeOptionalString(record.MemberDID),
		normalizeOptionalString(record.MemberHandle),
		normalizeOptionalString(record.ProfileURL),
		normalizeOptionalString(record.Role),
		defaultString(record.Status, "active"),
		normalizeOptionalString(record.JoinedAt),
		normalizeOptionalInt64(defaultInt64Ptr(record.SentMessageCount, &zero)),
		defaultString(record.LastSyncedAt, nowUTC()),
		normalizeMetadata(record.Metadata),
		normalizeCredentialName(record.CredentialName),
	)
	return err
}

func SyncGroupMemberFromSystemEvent(ctx context.Context, db *sql.DB, ownerDID string, groupID string, systemEvent map[string]any, credentialName string) (bool, error) {
	subject, ok := systemEvent["subject"].(map[string]any)
	if !ok {
		return false, nil
	}
	userID, _ := subject["id"].(string)
	if userID == "" {
		return false, nil
	}
	kind, _ := systemEvent["kind"].(string)
	statusByKind := map[string]string{"member_joined": "active", "member_left": "left", "member_kicked": "kicked"}
	status, ok := statusByKind[kind]
	if !ok {
		return false, nil
	}
	if err := UpsertGroupMember(ctx, db, GroupMemberRecord{
		OwnerDID:       ownerDID,
		GroupID:        groupID,
		UserID:         userID,
		MemberDID:      stringFromAny(subject["did"]),
		MemberHandle:   stringFromAny(subject["handle"]),
		ProfileURL:     stringFromAny(subject["profile_url"]),
		Role:           "member",
		Status:         status,
		Metadata:       metadataFromAny(map[string]any{"system_event": systemEvent}),
		CredentialName: credentialName,
	}); err != nil {
		return false, err
	}
	return true, nil
}

func DeleteGroupMembers(ctx context.Context, db *sql.DB, ownerDID string, groupID string, targetDID string, targetUserID string) (int64, error) {
	var (
		result sql.Result
		err    error
	)
	switch {
	case strings.TrimSpace(targetDID) != "":
		result, err = db.ExecContext(ctx, `DELETE FROM group_members WHERE owner_did = ? AND group_id = ? AND member_did = ?`, normalizeOwnerDID(ownerDID), groupID, targetDID)
	case strings.TrimSpace(targetUserID) != "":
		result, err = db.ExecContext(ctx, `DELETE FROM group_members WHERE owner_did = ? AND group_id = ? AND user_id = ?`, normalizeOwnerDID(ownerDID), groupID, targetUserID)
	default:
		result, err = db.ExecContext(ctx, `DELETE FROM group_members WHERE owner_did = ? AND group_id = ?`, normalizeOwnerDID(ownerDID), groupID)
	}
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func RebindOwnerDID(ctx context.Context, db *sql.DB, oldOwnerDID string, newOwnerDID string) (map[string]int64, error) {
	oldOwnerDID = normalizeOwnerDID(oldOwnerDID)
	newOwnerDID = normalizeOwnerDID(newOwnerDID)
	result := map[string]int64{
		"messages":                0,
		"contacts":                0,
		"contact_handle_bindings": 0,
		"relationship_events":     0,
		"groups":                  0,
		"group_members":           0,
	}
	if oldOwnerDID == "" || newOwnerDID == "" || oldOwnerDID == newOwnerDID {
		return result, nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, table := range []string{"messages", "contacts", "contact_handle_bindings", "relationship_events", "groups", "group_members"} {
		var count int64
		if err := tx.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE owner_did = ?", table), oldOwnerDID).Scan(&count); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		result[table] = count
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE OR IGNORE %s SET owner_did = ? WHERE owner_did = ?", table), newOwnerDID, oldOwnerDID); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func ClearOwnerE2EEData(ctx context.Context, db *sql.DB, ownerDID string) (map[string]int64, error) {
	ownerDID = normalizeOwnerDID(ownerDID)
	result := map[string]int64{"e2ee_outbox": 0, "e2ee_sessions": 0}
	if ownerDID == "" {
		return result, nil
	}
	for _, table := range []string{"e2ee_outbox", "e2ee_sessions"} {
		var count int64
		if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE owner_did = ?", table), ownerDID).Scan(&count); err != nil {
			return nil, err
		}
		result[table] = count
		if _, err := db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE owner_did = ?", table), ownerDID); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func ExecuteSQL(ctx context.Context, db *sql.DB, statement string, params ...any) ([]map[string]any, error) {
	sqlText := strings.TrimSpace(strings.TrimSuffix(statement, ";"))
	if sqlText == "" {
		return nil, fmt.Errorf("%w: empty statement", ErrUnsafeSQL)
	}
	if strings.Contains(sqlText, ";") {
		return nil, fmt.Errorf("%w: multiple statements are not allowed", ErrUnsafeSQL)
	}
	upper := strings.ToUpper(sqlText)
	for _, pattern := range forbiddenPatterns {
		if pattern.MatchString(upper) {
			return nil, fmt.Errorf("%w: forbidden SQL operation", ErrUnsafeSQL)
		}
	}
	if matched, _ := regexp.MatchString(`(?i)^\s*DELETE\b`, sqlText); matched {
		if !regexp.MustCompile(`(?i)\bWHERE\b`).MatchString(sqlText) {
			return nil, fmt.Errorf("%w: DELETE without WHERE clause is not allowed", ErrUnsafeSQL)
		}
	}
	if strings.HasPrefix(upper, "SELECT") {
		return queryMaps(ctx, db, sqlText, params...)
	}
	result, err := db.ExecContext(ctx, sqlText, params...)
	if err != nil {
		return nil, err
	}
	rowsAffected, _ := result.RowsAffected()
	return []map[string]any{{"rows_affected": rowsAffected}}, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func defaultBoolValue(value *bool) int {
	if value == nil {
		return 0
	}
	return boolToInt(*value)
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func stringFromAny(value any) string {
	text, _ := value.(string)
	return text
}

func generateID() string {
	return fmt.Sprintf("local-%d", time.Now().UnixNano())
}

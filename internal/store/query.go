package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func ListInboxMessages(ctx context.Context, db *sql.DB, ownerDID string, limit int, peerDID string, unreadOnly bool) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `
SELECT *
FROM messages
WHERE owner_did = ?
  AND direction = 0`
	args := []any{normalizeOwnerDID(ownerDID)}
	if unreadOnly {
		query += " AND is_read = 0"
	}
	if strings.TrimSpace(peerDID) != "" {
		query += " AND (sender_did = ? OR receiver_did = ?)"
		args = append(args, peerDID, peerDID)
	}
	query += " ORDER BY COALESCE(sent_at, stored_at) DESC LIMIT ?"
	args = append(args, limit)
	return queryMaps(ctx, db, query, args...)
}

func ListThreadMessages(ctx context.Context, db *sql.DB, ownerDID string, threadID string, limit int) ([]map[string]any, error) {
	if strings.TrimSpace(threadID) == "" {
		return nil, fmt.Errorf("thread_id is required")
	}
	if limit <= 0 {
		limit = 50
	}
	return queryMaps(ctx, db, `
SELECT *
FROM messages
WHERE owner_did = ? AND thread_id = ?
ORDER BY COALESCE(sent_at, stored_at) DESC
LIMIT ?`, normalizeOwnerDID(ownerDID), threadID, limit)
}

func ListDirectMessagesByPeerDIDs(ctx context.Context, db *sql.DB, ownerDID string, peerDIDs []string, limit int, unreadOnly bool, inboxOnly bool) ([]map[string]any, error) {
	if len(peerDIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	normalizedOwner := normalizeOwnerDID(ownerDID)
	normalizedPeers := make([]string, 0, len(peerDIDs))
	seen := make(map[string]struct{}, len(peerDIDs))
	for _, did := range peerDIDs {
		did = strings.TrimSpace(did)
		if did == "" {
			continue
		}
		if _, ok := seen[did]; ok {
			continue
		}
		seen[did] = struct{}{}
		normalizedPeers = append(normalizedPeers, did)
	}
	if len(normalizedPeers) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(normalizedPeers)), ",")
	query := `
SELECT *
FROM messages
WHERE owner_did = ?
  AND COALESCE(group_did, group_id) IS NULL`
	args := make([]any, 0, 2+len(normalizedPeers)*2)
	args = append(args, normalizedOwner)
	if inboxOnly {
		query += " AND direction = 0"
	}
	if unreadOnly {
		query += " AND is_read = 0"
	}
	query += fmt.Sprintf(`
  AND (
        (sender_did IN (%s) AND receiver_did = ?)
     OR (receiver_did IN (%s) AND sender_did = ?)
  )
ORDER BY COALESCE(sent_at, stored_at) DESC
LIMIT ?`, placeholders, placeholders)
	for _, did := range normalizedPeers {
		args = append(args, did)
	}
	args = append(args, normalizedOwner)
	for _, did := range normalizedPeers {
		args = append(args, did)
	}
	args = append(args, normalizedOwner, limit)
	return queryMaps(ctx, db, query, args...)
}

func ListGroupInboxMessages(ctx context.Context, db *sql.DB, ownerDID string, limit int, groupID string, unreadOnly bool) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `
SELECT *
FROM messages
WHERE owner_did = ?
  AND direction = 0
  AND COALESCE(group_did, group_id) IS NOT NULL`
	args := []any{normalizeOwnerDID(ownerDID)}
	if unreadOnly {
		query += " AND is_read = 0"
	}
	if strings.TrimSpace(groupID) != "" {
		query += " AND (group_did = ? OR group_id = ?)"
		args = append(args, groupID, groupID)
	}
	query += " ORDER BY COALESCE(sent_at, stored_at) DESC LIMIT ?"
	args = append(args, limit)
	return queryMaps(ctx, db, query, args...)
}

func ListGroupMessages(ctx context.Context, db *sql.DB, ownerDID string, groupID string, limit int, sinceSeq *int64) ([]map[string]any, error) {
	if strings.TrimSpace(groupID) == "" {
		return nil, fmt.Errorf("group_id is required")
	}
	if limit <= 0 {
		limit = 50
	}
	query := `
SELECT *
FROM messages
WHERE owner_did = ?
  AND (group_did = ? OR group_id = ?)`
	args := []any{normalizeOwnerDID(ownerDID), groupID, groupID}
	if sinceSeq != nil {
		query += " AND COALESCE(server_seq, 0) > ?"
		args = append(args, *sinceSeq)
	}
	query += " ORDER BY COALESCE(server_seq, 0) DESC, COALESCE(sent_at, stored_at) DESC LIMIT ?"
	args = append(args, limit)
	return queryMaps(ctx, db, query, args...)
}

func GetGroupSnapshot(ctx context.Context, db *sql.DB, ownerDID string, groupID string) (map[string]any, error) {
	return queryOneMap(ctx, db, `
SELECT *
FROM groups
WHERE owner_did = ? AND (group_id = ? OR group_did = ?)`,
		normalizeOwnerDID(ownerDID), groupID, groupID,
	)
}

func ListCachedGroupMembers(ctx context.Context, db *sql.DB, ownerDID string, groupID string, limit int) ([]map[string]any, error) {
	if strings.TrimSpace(groupID) == "" {
		return nil, fmt.Errorf("group_id is required")
	}
	if limit <= 0 {
		limit = 100
	}
	return queryMaps(ctx, db, `
SELECT *
FROM group_members
WHERE owner_did = ? AND group_id = ?
ORDER BY role ASC, member_handle ASC, member_did ASC
LIMIT ?`, normalizeOwnerDID(ownerDID), groupID, limit)
}

func ListMessagesByIDs(ctx context.Context, db *sql.DB, ownerDID string, messageIDs []string) ([]map[string]any, error) {
	if len(messageIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, 0, len(messageIDs))
	args := make([]any, 0, len(messageIDs)+1)
	args = append(args, normalizeOwnerDID(ownerDID))
	for _, id := range messageIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	query := fmt.Sprintf(`SELECT * FROM messages WHERE owner_did = ? AND msg_id IN (%s)`, strings.Join(placeholders, ","))
	return queryMaps(ctx, db, query, args...)
}

func MarkMessagesRead(ctx context.Context, db *sql.DB, ownerDID string, messageIDs []string) (int64, error) {
	if len(messageIDs) == 0 {
		return 0, nil
	}
	placeholders := make([]string, 0, len(messageIDs))
	args := make([]any, 0, len(messageIDs)+1)
	args = append(args, normalizeOwnerDID(ownerDID))
	for _, id := range messageIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	query := fmt.Sprintf(
		`UPDATE messages SET is_read = 1 WHERE owner_did = ? AND msg_id IN (%s)`,
		strings.Join(placeholders, ","),
	)
	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ListNotifications returns the most recent mail.notification messages for a given owner.
// These are stored by the runtime listener when it receives mail.notification events
// from message-service v2 via the websocket channel.
func ListNotifications(ctx context.Context, db *sql.DB, ownerDID string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 20
	}
	return queryMaps(ctx, db, `
SELECT *
FROM messages
WHERE owner_did = ?
  AND content_type = 'mail.notification'
ORDER BY COALESCE(sent_at, stored_at) DESC
LIMIT ?`, normalizeOwnerDID(ownerDID), limit)
}

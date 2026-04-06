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

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

type sqlQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

var (
	forbiddenPatterns = []*regexp.Regexp{
		regexp.MustCompile(`\bDROP\b`),
		regexp.MustCompile(`\bTRUNCATE\b`),
	}
)

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func MakeThreadID(myDID string, peerDID string, groupID string) string {
	if strings.TrimSpace(groupID) != "" {
		return fmt.Sprintf("group:%s", strings.TrimSpace(groupID))
	}
	if strings.TrimSpace(peerDID) != "" {
		pair := []string{strings.TrimSpace(myDID), strings.TrimSpace(peerDID)}
		sort.Strings(pair)
		return fmt.Sprintf("dm:%s:%s", pair[0], pair[1])
	}
	return fmt.Sprintf("dm:%s:unknown", strings.TrimSpace(myDID))
}

func normalizeOwnerDID(value string) string {
	return strings.TrimSpace(value)
}

func normalizeCredentialName(value string) string {
	return strings.TrimSpace(value)
}

func normalizeOptionalString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func normalizeOptionalBool(value *bool) any {
	if value == nil {
		return nil
	}
	if *value {
		return 1
	}
	return 0
}

func normalizeOptionalInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func normalizeOptionalFloat64(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func defaultInt64Ptr(value *int64, fallback *int64) *int64 {
	if value != nil {
		return value
	}
	return fallback
}

func normalizeMetadata(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func metadataFromAny(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return string(raw)
	}
}

func queryMaps(ctx context.Context, db *sql.DB, query string, args ...any) ([]map[string]any, error) {
	return queryMapsWithQueryer(ctx, db, query, args...)
}

func queryMapsWithQueryer(ctx context.Context, queryer sqlQueryer, query string, args ...any) ([]map[string]any, error) {
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for index := range values {
			pointers[index] = &values[index]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		item := make(map[string]any, len(columns))
		for index, column := range columns {
			item[column] = convertSQLiteValue(values[index])
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func queryOneMap(ctx context.Context, db *sql.DB, query string, args ...any) (map[string]any, error) {
	return queryOneMapWithQueryer(ctx, db, query, args...)
}

func queryOneMapWithQueryer(ctx context.Context, queryer sqlQueryer, query string, args ...any) (map[string]any, error) {
	rows, err := queryMapsWithQueryer(ctx, queryer, query, args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sql.ErrNoRows
	}
	return rows[0], nil
}

func convertSQLiteValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	default:
		return typed
	}
}

func tableExists(ctx context.Context, db *sql.DB, tableName string) (bool, error) {
	var exists int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", tableName)
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists > 0, nil
}

func viewExists(ctx context.Context, db *sql.DB, viewName string) (bool, error) {
	var exists int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'view' AND name = ?", viewName)
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists > 0, nil
}

func columnNames(ctx context.Context, db *sql.DB, tableName string) (map[string]struct{}, error) {
	rows, err := queryMaps(ctx, db, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return nil, err
	}
	columns := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		name, _ := row["name"].(string)
		if name != "" {
			columns[name] = struct{}{}
		}
	}
	return columns, nil
}

func schemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	var version int
	row := db.QueryRowContext(ctx, "PRAGMA user_version")
	if err := row.Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

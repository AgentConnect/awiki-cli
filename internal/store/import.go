package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
)

func ScanLegacyDatabase(ctx context.Context, paths appconfig.Paths) (*LegacyScan, error) {
	legacyPath := strings.TrimSpace(paths.LegacyDataDir)
	if !strings.HasSuffix(strings.ToLower(legacyPath), ".db") {
		legacyPath = filepath.Join(paths.LegacyDataDir, "database", "awiki.db")
	}
	scan := &LegacyScan{
		Path: legacyPath,
	}
	db, err := OpenReadOnly(scan.Path)
	if err != nil {
		if strings.Contains(err.Error(), "no such file") || strings.Contains(err.Error(), "unable to open database file") {
			return scan, nil
		}
		return nil, err
	}
	defer db.Close()
	scan.Exists = true
	version, err := schemaVersion(ctx, db)
	if err != nil {
		return nil, err
	}
	scan.SchemaVersion = version
	rows, err := queryMaps(ctx, db, `SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if name, ok := row["name"].(string); ok {
			scan.Tables = append(scan.Tables, name)
		}
	}
	return scan, nil
}

func ImportLegacyDatabase(ctx context.Context, targetDB *sql.DB, paths appconfig.Paths, manager *identity.Manager) (*ImportReport, error) {
	scan, err := ScanLegacyDatabase(ctx, paths)
	if err != nil {
		return nil, err
	}
	if !scan.Exists {
		return nil, ErrLegacyDatabaseNotFound
	}
	legacyDB, err := OpenReadOnly(scan.Path)
	if err != nil {
		return nil, err
	}
	defer legacyDB.Close()
	if err := EnsureSchema(ctx, targetDB); err != nil {
		return nil, err
	}
	report := &ImportReport{
		SourcePath:          scan.Path,
		SourceSchemaVersion: scan.SchemaVersion,
		ImportedRows:        map[string]int{},
	}
	ownerByCredential := map[string]string{}
	defaultOwner := ""
	if manager != nil {
		if identities, listErr := manager.List(); listErr == nil {
			for _, summary := range identities {
				ownerByCredential[summary.IdentityName] = summary.DID
				if summary.IsDefault {
					defaultOwner = summary.DID
				}
			}
			if defaultOwner == "" && len(identities) == 1 {
				defaultOwner = identities[0].DID
			}
		}
	}
	if scan.SchemaVersion > 0 && scan.SchemaVersion < 6 && defaultOwner == "" {
		return nil, fmt.Errorf("%w: legacy schema < 6 requires at least one imported identity so owner_did can be inferred", ErrUnsupportedLegacySchema)
	}
	importers := []struct {
		name string
		fn   func(context.Context, *sql.DB, *sql.DB, map[string]string, string) (int, error)
	}{
		{name: "messages", fn: importMessages},
		{name: "e2ee_outbox", fn: importE2EEOutbox},
		{name: "contacts", fn: importContacts},
		{name: "groups", fn: importGroups},
		{name: "group_members", fn: importGroupMembers},
		{name: "relationship_events", fn: importRelationshipEvents},
		{name: "e2ee_sessions", fn: importE2EESessions},
	}
	for _, importer := range importers {
		count, importErr := importer.fn(ctx, legacyDB, targetDB, ownerByCredential, defaultOwner)
		if importErr != nil {
			if errorsIsMissingTable(importErr) {
				report.SkippedTables = append(report.SkippedTables, importer.name)
				continue
			}
			return nil, fmt.Errorf("import %s: %w", importer.name, importErr)
		}
		report.ImportedRows[importer.name] = count
	}
	sort.Strings(report.SkippedTables)
	return report, nil
}

func inferOwnerDID(row map[string]any, ownerByCredential map[string]string, defaultOwner string) string {
	if owner, ok := row["owner_did"].(string); ok && strings.TrimSpace(owner) != "" {
		return owner
	}
	if credential, ok := row["credential_name"].(string); ok && credential != "" {
		if owner, ok := ownerByCredential[credential]; ok {
			return owner
		}
	}
	return defaultOwner
}

func importMessages(ctx context.Context, src, dst *sql.DB, ownerByCredential map[string]string, defaultOwner string) (int, error) {
	rows, err := queryMaps(ctx, src, `SELECT * FROM messages`)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, row := range rows {
		msgID := stringFromAny(row["msg_id"])
		if msgID == "" {
			continue
		}
		threadID := stringFromAny(row["thread_id"])
		if threadID == "" {
			threadID = MakeThreadID(inferOwnerDID(row, ownerByCredential, defaultOwner), stringFromAny(row["sender_did"]), stringFromAny(row["group_id"]))
		}
		record := MessageRecord{
			MsgID:          msgID,
			OwnerDID:       inferOwnerDID(row, ownerByCredential, defaultOwner),
			ThreadID:       threadID,
			Direction:      intFromAny(row["direction"]),
			SenderDID:      stringFromAny(row["sender_did"]),
			ReceiverDID:    stringFromAny(row["receiver_did"]),
			GroupID:        stringFromAny(row["group_id"]),
			GroupDID:       stringFromAny(row["group_did"]),
			ContentType:    defaultString(stringFromAny(row["content_type"]), "text"),
			Content:        stringFromAny(row["content"]),
			Title:          stringFromAny(row["title"]),
			ServerSeq:      int64PtrFromAny(row["server_seq"]),
			SentAt:         stringFromAny(row["sent_at"]),
			IsE2EE:         boolFromAny(row["is_e2ee"]),
			IsRead:         boolFromAny(row["is_read"]),
			SenderName:     stringFromAny(row["sender_name"]),
			Metadata:       metadataFromAny(row["metadata"]),
			CredentialName: stringFromAny(row["credential_name"]),
		}
		if err := StoreMessage(ctx, dst, record); err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

func importE2EEOutbox(ctx context.Context, src, dst *sql.DB, ownerByCredential map[string]string, defaultOwner string) (int, error) {
	rows, err := queryMaps(ctx, src, `SELECT * FROM e2ee_outbox`)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, row := range rows {
		record := E2EEOutboxRecord{
			OutboxID:        stringFromAny(row["outbox_id"]),
			OwnerDID:        inferOwnerDID(row, ownerByCredential, defaultOwner),
			PeerDID:         stringFromAny(row["peer_did"]),
			SessionID:       stringFromAny(row["session_id"]),
			OriginalType:    defaultString(stringFromAny(row["original_type"]), "text"),
			Plaintext:       stringFromAny(row["plaintext"]),
			LocalStatus:     defaultString(stringFromAny(row["local_status"]), "queued"),
			AttemptCount:    intFromAny(row["attempt_count"]),
			SentMsgID:       stringFromAny(row["sent_msg_id"]),
			SentServerSeq:   int64PtrFromAny(row["sent_server_seq"]),
			LastErrorCode:   stringFromAny(row["last_error_code"]),
			RetryHint:       stringFromAny(row["retry_hint"]),
			FailedMsgID:     stringFromAny(row["failed_msg_id"]),
			FailedServerSeq: int64PtrFromAny(row["failed_server_seq"]),
			Metadata:        metadataFromAny(row["metadata"]),
			LastAttemptAt:   stringFromAny(row["last_attempt_at"]),
			CreatedAt:       stringFromAny(row["created_at"]),
			UpdatedAt:       stringFromAny(row["updated_at"]),
			CredentialName:  stringFromAny(row["credential_name"]),
		}
		if _, err := QueueE2EEOutbox(ctx, dst, record); err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

func importContacts(ctx context.Context, src, dst *sql.DB, ownerByCredential map[string]string, defaultOwner string) (int, error) {
	rows, err := queryMaps(ctx, src, `SELECT * FROM contacts`)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, row := range rows {
		did := stringFromAny(row["did"])
		if did == "" {
			continue
		}
		record := ContactRecord{
			OwnerDID:          inferOwnerDID(row, ownerByCredential, defaultOwner),
			DID:               did,
			Name:              stringFromAny(row["name"]),
			Handle:            stringFromAny(row["handle"]),
			NickName:          stringFromAny(row["nick_name"]),
			Bio:               stringFromAny(row["bio"]),
			ProfileMD:         stringFromAny(row["profile_md"]),
			Tags:              stringFromAny(row["tags"]),
			Relationship:      stringFromAny(row["relationship"]),
			SourceType:        stringFromAny(row["source_type"]),
			SourceName:        stringFromAny(row["source_name"]),
			SourceGroupID:     stringFromAny(row["source_group_id"]),
			ConnectedAt:       stringFromAny(row["connected_at"]),
			RecommendedReason: stringFromAny(row["recommended_reason"]),
			Followed:          boolPtrFromAny(row["followed"]),
			Messaged:          boolPtrFromAny(row["messaged"]),
			Note:              stringFromAny(row["note"]),
			FirstSeenAt:       stringFromAny(row["first_seen_at"]),
			LastSeenAt:        stringFromAny(row["last_seen_at"]),
			Metadata:          metadataFromAny(row["metadata"]),
		}
		if err := UpsertContact(ctx, dst, record); err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

func importGroups(ctx context.Context, src, dst *sql.DB, ownerByCredential map[string]string, defaultOwner string) (int, error) {
	rows, err := queryMaps(ctx, src, `SELECT * FROM groups`)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, row := range rows {
		groupID := stringFromAny(row["group_id"])
		if groupID == "" {
			continue
		}
		record := GroupRecord{
			OwnerDID:          inferOwnerDID(row, ownerByCredential, defaultOwner),
			GroupID:           groupID,
			GroupDID:          stringFromAny(row["group_did"]),
			Name:              stringFromAny(row["name"]),
			GroupMode:         defaultString(stringFromAny(row["group_mode"]), "general"),
			Slug:              stringFromAny(row["slug"]),
			Description:       stringFromAny(row["description"]),
			Goal:              stringFromAny(row["goal"]),
			Rules:             stringFromAny(row["rules"]),
			MessagePrompt:     stringFromAny(row["message_prompt"]),
			DocURL:            stringFromAny(row["doc_url"]),
			GroupOwnerDID:     stringFromAny(row["group_owner_did"]),
			GroupOwnerHandle:  stringFromAny(row["group_owner_handle"]),
			MyRole:            stringFromAny(row["my_role"]),
			MembershipStatus:  defaultString(stringFromAny(row["membership_status"]), "active"),
			JoinEnabled:       boolPtrFromAny(row["join_enabled"]),
			JoinCode:          stringFromAny(row["join_code"]),
			JoinCodeExpiresAt: stringFromAny(row["join_code_expires_at"]),
			MemberCount:       int64PtrFromAny(row["member_count"]),
			LastSyncedSeq:     int64PtrFromAny(row["last_synced_seq"]),
			LastReadSeq:       int64PtrFromAny(row["last_read_seq"]),
			LastMessageAt:     stringFromAny(row["last_message_at"]),
			RemoteCreatedAt:   stringFromAny(row["remote_created_at"]),
			RemoteUpdatedAt:   stringFromAny(row["remote_updated_at"]),
			Metadata:          metadataFromAny(row["metadata"]),
			CredentialName:    stringFromAny(row["credential_name"]),
		}
		if err := UpsertGroup(ctx, dst, record); err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

func importGroupMembers(ctx context.Context, src, dst *sql.DB, ownerByCredential map[string]string, defaultOwner string) (int, error) {
	rows, err := queryMaps(ctx, src, `SELECT * FROM group_members`)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, row := range rows {
		record := GroupMemberRecord{
			OwnerDID:         inferOwnerDID(row, ownerByCredential, defaultOwner),
			GroupID:          stringFromAny(row["group_id"]),
			UserID:           stringFromAny(row["user_id"]),
			MemberDID:        stringFromAny(row["member_did"]),
			MemberHandle:     stringFromAny(row["member_handle"]),
			ProfileURL:       stringFromAny(row["profile_url"]),
			Role:             stringFromAny(row["role"]),
			Status:           defaultString(stringFromAny(row["status"]), "active"),
			JoinedAt:         stringFromAny(row["joined_at"]),
			SentMessageCount: int64PtrFromAny(row["sent_message_count"]),
			Metadata:         metadataFromAny(row["metadata"]),
			CredentialName:   stringFromAny(row["credential_name"]),
		}
		if record.GroupID == "" || record.UserID == "" {
			continue
		}
		if err := UpsertGroupMember(ctx, dst, record); err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

func importRelationshipEvents(ctx context.Context, src, dst *sql.DB, ownerByCredential map[string]string, defaultOwner string) (int, error) {
	rows, err := queryMaps(ctx, src, `SELECT * FROM relationship_events`)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, row := range rows {
		record := RelationshipEventRecord{
			EventID:        stringFromAny(row["event_id"]),
			OwnerDID:       inferOwnerDID(row, ownerByCredential, defaultOwner),
			TargetDID:      stringFromAny(row["target_did"]),
			TargetHandle:   stringFromAny(row["target_handle"]),
			EventType:      stringFromAny(row["event_type"]),
			SourceType:     stringFromAny(row["source_type"]),
			SourceName:     stringFromAny(row["source_name"]),
			SourceGroupID:  stringFromAny(row["source_group_id"]),
			Reason:         stringFromAny(row["reason"]),
			Score:          float64PtrFromAny(row["score"]),
			Status:         defaultString(stringFromAny(row["status"]), "pending"),
			CreatedAt:      stringFromAny(row["created_at"]),
			UpdatedAt:      stringFromAny(row["updated_at"]),
			Metadata:       metadataFromAny(row["metadata"]),
			CredentialName: stringFromAny(row["credential_name"]),
		}
		if record.TargetDID == "" || record.EventType == "" {
			continue
		}
		if _, err := AppendRelationshipEvent(ctx, dst, record); err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

func importE2EESessions(ctx context.Context, src, dst *sql.DB, ownerByCredential map[string]string, defaultOwner string) (int, error) {
	rows, err := queryMaps(ctx, src, `SELECT * FROM e2ee_sessions`)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, row := range rows {
		_, err := dst.ExecContext(ctx, `
INSERT OR REPLACE INTO e2ee_sessions
    (owner_did, peer_did, session_id, is_initiator, send_chain_key, recv_chain_key,
     send_seq, recv_seq, expires_at, created_at, active_at, peer_confirmed, credential_name, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			inferOwnerDID(row, ownerByCredential, defaultOwner),
			stringFromAny(row["peer_did"]),
			stringFromAny(row["session_id"]),
			boolToInt(boolFromAny(row["is_initiator"])),
			stringFromAny(row["send_chain_key"]),
			stringFromAny(row["recv_chain_key"]),
			intFromAny(row["send_seq"]),
			intFromAny(row["recv_seq"]),
			normalizeOptionalFloat64(float64PtrFromAny(row["expires_at"])),
			stringFromAny(row["created_at"]),
			normalizeOptionalString(stringFromAny(row["active_at"])),
			boolToInt(boolFromAny(row["peer_confirmed"])),
			stringFromAny(row["credential_name"]),
			defaultString(stringFromAny(row["updated_at"]), nowUTC()),
		)
		if err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

func errorsIsMissingTable(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no such table")
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case int64:
		return typed != 0
	case int:
		return typed != 0
	case float64:
		return typed != 0
	case bool:
		return typed
	case string:
		return typed == "1" || strings.EqualFold(typed, "true")
	default:
		return false
	}
}

func boolPtrFromAny(value any) *bool {
	switch value.(type) {
	case nil:
		return nil
	}
	converted := boolFromAny(value)
	return &converted
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		if typed == "" {
			return 0
		}
		var out int
		fmt.Sscanf(typed, "%d", &out)
		return out
	default:
		return 0
	}
}

func int64PtrFromAny(value any) *int64 {
	switch typed := value.(type) {
	case nil:
		return nil
	case int64:
		v := typed
		return &v
	case int:
		v := int64(typed)
		return &v
	case float64:
		v := int64(typed)
		return &v
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		var out int64
		fmt.Sscanf(typed, "%d", &out)
		return &out
	default:
		return nil
	}
}

func float64PtrFromAny(value any) *float64 {
	switch typed := value.(type) {
	case nil:
		return nil
	case float64:
		v := typed
		return &v
	case int64:
		v := float64(typed)
		return &v
	case int:
		v := float64(typed)
		return &v
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		var out float64
		fmt.Sscanf(typed, "%f", &out)
		return &out
	default:
		return nil
	}
}

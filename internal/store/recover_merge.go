package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func MergeRecoveredHandleLocalState(ctx context.Context, paths appconfig.Paths, oldOwnerDIDs []string, newOwnerDID string, finalCredentialName string) (map[string]int64, map[string]int64, error) {
	storeMerge := map[string]int64{
		"messages":                0,
		"contacts":                0,
		"contact_handle_bindings": 0,
		"relationship_events":     0,
		"groups":                  0,
		"group_members":           0,
	}
	e2eeCleanup := map[string]int64{
		"e2ee_outbox":   0,
		"e2ee_sessions": 0,
	}
	if !storeFileExists(paths.DatabaseFile) {
		return storeMerge, e2eeCleanup, nil
	}
	owners := normalizeRecoverOwnerDIDs(oldOwnerDIDs, newOwnerDID)
	if len(owners) == 0 {
		return storeMerge, e2eeCleanup, nil
	}

	db, err := Open(paths)
	if err != nil {
		return storeMerge, e2eeCleanup, err
	}
	defer db.Close()
	if err := EnsureSchema(ctx, db); err != nil {
		return storeMerge, e2eeCleanup, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return storeMerge, e2eeCleanup, err
	}
	ownerSet := make(map[string]struct{}, len(owners))
	for _, owner := range owners {
		ownerSet[owner] = struct{}{}
	}
	affectedHandles := make(map[string]struct{})

	if storeMerge["messages"], err = mergeRecoveredMessages(ctx, tx, owners, ownerSet, newOwnerDID, finalCredentialName); err != nil {
		_ = tx.Rollback()
		return storeMerge, e2eeCleanup, err
	}
	if storeMerge["contacts"], err = mergeRecoveredContacts(ctx, tx, owners, newOwnerDID, finalCredentialName, affectedHandles); err != nil {
		_ = tx.Rollback()
		return storeMerge, e2eeCleanup, err
	}
	if storeMerge["contact_handle_bindings"], err = mergeRecoveredContactHandleBindings(ctx, tx, owners, newOwnerDID, finalCredentialName, affectedHandles); err != nil {
		_ = tx.Rollback()
		return storeMerge, e2eeCleanup, err
	}
	if err := normalizeRecoveredCurrentHandles(ctx, tx, newOwnerDID, affectedHandles); err != nil {
		_ = tx.Rollback()
		return storeMerge, e2eeCleanup, err
	}
	if storeMerge["relationship_events"], err = mergeRecoveredRelationshipEvents(ctx, tx, owners, newOwnerDID, finalCredentialName); err != nil {
		_ = tx.Rollback()
		return storeMerge, e2eeCleanup, err
	}
	if storeMerge["groups"], err = mergeRecoveredGroups(ctx, tx, owners, ownerSet, newOwnerDID, finalCredentialName); err != nil {
		_ = tx.Rollback()
		return storeMerge, e2eeCleanup, err
	}
	if storeMerge["group_members"], err = mergeRecoveredGroupMembers(ctx, tx, owners, ownerSet, newOwnerDID, finalCredentialName); err != nil {
		_ = tx.Rollback()
		return storeMerge, e2eeCleanup, err
	}
	if e2eeCleanup, err = clearRecoveredOwnerE2EEData(ctx, tx, owners); err != nil {
		_ = tx.Rollback()
		return storeMerge, e2eeCleanup, err
	}
	if err := tx.Commit(); err != nil {
		return storeMerge, e2eeCleanup, err
	}
	return storeMerge, e2eeCleanup, nil
}

func mergeRecoveredMessages(ctx context.Context, tx *sql.Tx, oldOwnerDIDs []string, oldOwnerSet map[string]struct{}, newOwnerDID string, finalCredentialName string) (int64, error) {
	var count int64
	for _, oldOwner := range oldOwnerDIDs {
		rows, err := queryMapsWithQueryer(ctx, tx, `SELECT * FROM messages WHERE owner_did = ? ORDER BY COALESCE(sent_at, stored_at) ASC, msg_id ASC`, oldOwner)
		if err != nil {
			return count, err
		}
		count += int64(len(rows))
		for _, row := range rows {
			merged := normalizeRecoveredMessageRow(row, oldOwnerSet, newOwnerDID, finalCredentialName)
			existing, err := queryOneMapWithQueryer(ctx, tx, `SELECT * FROM messages WHERE owner_did = ? AND msg_id = ?`, newOwnerDID, merged.MsgID)
			if err != nil && err != sql.ErrNoRows {
				return count, err
			}
			if err == nil {
				merged = mergeRecoveredMessage(existing, merged)
			}
			if err := upsertRecoveredMessage(ctx, tx, merged); err != nil {
				return count, err
			}
		}
	}
	return count, deleteRowsForOwners(ctx, tx, "messages", oldOwnerDIDs)
}

func mergeRecoveredContacts(ctx context.Context, tx *sql.Tx, oldOwnerDIDs []string, newOwnerDID string, finalCredentialName string, affectedHandles map[string]struct{}) (int64, error) {
	var count int64
	for _, oldOwner := range oldOwnerDIDs {
		rows, err := queryMapsWithQueryer(ctx, tx, `SELECT * FROM contacts WHERE owner_did = ? ORDER BY COALESCE(last_seen_at, first_seen_at, connected_at) ASC, did ASC`, oldOwner)
		if err != nil {
			return count, err
		}
		count += int64(len(rows))
		for _, row := range rows {
			record := normalizeRecoveredContactRow(row, newOwnerDID, finalCredentialName)
			if handle := strings.TrimSpace(record.Handle); handle != "" {
				affectedHandles[handle] = struct{}{}
			}
			existing, err := queryOneMapWithQueryer(ctx, tx, `SELECT * FROM contacts WHERE owner_did = ? AND did = ?`, newOwnerDID, record.DID)
			if err != nil && err != sql.ErrNoRows {
				return count, err
			}
			if err == nil {
				record = mergeRecoveredContact(existing, record)
			}
			if err := upsertRecoveredContact(ctx, tx, record); err != nil {
				return count, err
			}
		}
	}
	return count, deleteRowsForOwners(ctx, tx, "contacts", oldOwnerDIDs)
}

func mergeRecoveredContactHandleBindings(ctx context.Context, tx *sql.Tx, oldOwnerDIDs []string, newOwnerDID string, finalCredentialName string, affectedHandles map[string]struct{}) (int64, error) {
	var count int64
	for _, oldOwner := range oldOwnerDIDs {
		rows, err := queryMapsWithQueryer(ctx, tx, `SELECT * FROM contact_handle_bindings WHERE owner_did = ? ORDER BY COALESCE(last_seen_at, first_seen_at) ASC, handle ASC, did ASC`, oldOwner)
		if err != nil {
			return count, err
		}
		count += int64(len(rows))
		for _, row := range rows {
			record := normalizeRecoveredContactHandleBindingRow(row, newOwnerDID, finalCredentialName)
			if handle := strings.TrimSpace(record.Handle); handle != "" {
				affectedHandles[handle] = struct{}{}
			}
			existing, err := queryOneMapWithQueryer(ctx, tx, `SELECT * FROM contact_handle_bindings WHERE owner_did = ? AND handle = ? AND did = ?`, newOwnerDID, record.Handle, record.DID)
			if err != nil && err != sql.ErrNoRows {
				return count, err
			}
			if err == nil {
				record = mergeRecoveredContactHandleBinding(existing, record)
			}
			if err := upsertRecoveredContactHandleBinding(ctx, tx, record); err != nil {
				return count, err
			}
		}
	}
	return count, deleteRowsForOwners(ctx, tx, "contact_handle_bindings", oldOwnerDIDs)
}

func mergeRecoveredRelationshipEvents(ctx context.Context, tx *sql.Tx, oldOwnerDIDs []string, newOwnerDID string, finalCredentialName string) (int64, error) {
	var count int64
	for _, oldOwner := range oldOwnerDIDs {
		rows, err := queryMapsWithQueryer(ctx, tx, `SELECT * FROM relationship_events WHERE owner_did = ? ORDER BY COALESCE(updated_at, created_at) ASC, event_id ASC`, oldOwner)
		if err != nil {
			return count, err
		}
		count += int64(len(rows))
		for _, row := range rows {
			record := normalizeRecoveredRelationshipEventRow(row, newOwnerDID, finalCredentialName)
			existing, err := queryOneMapWithQueryer(ctx, tx, `SELECT * FROM relationship_events WHERE event_id = ?`, record.EventID)
			if err != nil && err != sql.ErrNoRows {
				return count, err
			}
			if err == nil {
				record = mergeRecoveredRelationshipEvent(existing, record)
			}
			if err := upsertRecoveredRelationshipEvent(ctx, tx, record); err != nil {
				return count, err
			}
		}
	}
	return count, deleteRowsForOwners(ctx, tx, "relationship_events", oldOwnerDIDs)
}

func mergeRecoveredGroups(ctx context.Context, tx *sql.Tx, oldOwnerDIDs []string, oldOwnerSet map[string]struct{}, newOwnerDID string, finalCredentialName string) (int64, error) {
	var count int64
	for _, oldOwner := range oldOwnerDIDs {
		rows, err := queryMapsWithQueryer(ctx, tx, `SELECT * FROM groups WHERE owner_did = ? ORDER BY COALESCE(remote_updated_at, last_message_at, stored_at) ASC, group_id ASC`, oldOwner)
		if err != nil {
			return count, err
		}
		count += int64(len(rows))
		for _, row := range rows {
			record := normalizeRecoveredGroupRow(row, oldOwnerSet, newOwnerDID, finalCredentialName)
			existing, err := queryOneMapWithQueryer(ctx, tx, `SELECT * FROM groups WHERE owner_did = ? AND group_id = ?`, newOwnerDID, record.GroupID)
			if err != nil && err != sql.ErrNoRows {
				return count, err
			}
			if err == nil {
				record = mergeRecoveredGroup(existing, record)
			}
			if err := upsertRecoveredGroup(ctx, tx, record); err != nil {
				return count, err
			}
		}
	}
	return count, deleteRowsForOwners(ctx, tx, "groups", oldOwnerDIDs)
}

func mergeRecoveredGroupMembers(ctx context.Context, tx *sql.Tx, oldOwnerDIDs []string, oldOwnerSet map[string]struct{}, newOwnerDID string, finalCredentialName string) (int64, error) {
	var count int64
	for _, oldOwner := range oldOwnerDIDs {
		rows, err := queryMapsWithQueryer(ctx, tx, `SELECT * FROM group_members WHERE owner_did = ? ORDER BY group_id ASC, user_id ASC`, oldOwner)
		if err != nil {
			return count, err
		}
		count += int64(len(rows))
		for _, row := range rows {
			record := normalizeRecoveredGroupMemberRow(row, oldOwnerSet, newOwnerDID, finalCredentialName)
			existing, err := queryOneMapWithQueryer(ctx, tx, `SELECT * FROM group_members WHERE owner_did = ? AND group_id = ? AND user_id = ?`, newOwnerDID, record.GroupID, record.UserID)
			if err != nil && err != sql.ErrNoRows {
				return count, err
			}
			if err == nil {
				record = mergeRecoveredGroupMember(existing, record)
			}
			if err := upsertRecoveredGroupMember(ctx, tx, record); err != nil {
				return count, err
			}
		}
	}
	return count, deleteRowsForOwners(ctx, tx, "group_members", oldOwnerDIDs)
}

func clearRecoveredOwnerE2EEData(ctx context.Context, tx *sql.Tx, oldOwnerDIDs []string) (map[string]int64, error) {
	result := map[string]int64{
		"e2ee_outbox":   0,
		"e2ee_sessions": 0,
	}
	for _, table := range []string{"e2ee_outbox", "e2ee_sessions"} {
		total, err := countRowsForOwners(ctx, tx, table, oldOwnerDIDs)
		if err != nil {
			return result, err
		}
		result[table] = total
		if err := deleteRowsForOwners(ctx, tx, table, oldOwnerDIDs); err != nil {
			return result, err
		}
	}
	return result, nil
}

func normalizeRecoveredMessageRow(row map[string]any, oldOwnerSet map[string]struct{}, newOwnerDID string, finalCredentialName string) MessageRecord {
	senderDID := remapRecoveredSelfDID(stringFromAny(row["sender_did"]), oldOwnerSet, newOwnerDID)
	receiverDID := remapRecoveredSelfDID(stringFromAny(row["receiver_did"]), oldOwnerSet, newOwnerDID)
	groupID := stringFromAny(row["group_id"])
	groupDID := stringFromAny(row["group_did"])
	threadID := ""
	if groupKey := firstNonEmpty(groupID, groupDID); groupKey != "" {
		threadID = MakeThreadID(newOwnerDID, "", groupKey)
	} else {
		peerDID := firstRecoveredPeerDID(senderDID, receiverDID, newOwnerDID)
		threadID = MakeThreadID(newOwnerDID, peerDID, "")
	}
	return MessageRecord{
		MsgID:          stringFromAny(row["msg_id"]),
		OwnerDID:       newOwnerDID,
		ThreadID:       threadID,
		Direction:      intFromAny(row["direction"]),
		SenderDID:      senderDID,
		ReceiverDID:    receiverDID,
		GroupID:        groupID,
		GroupDID:       groupDID,
		ContentType:    stringFromAny(row["content_type"]),
		Content:        stringFromAny(row["content"]),
		Title:          stringFromAny(row["title"]),
		ServerSeq:      int64PtrFromAny(row["server_seq"]),
		SentAt:         stringFromAny(row["sent_at"]),
		StoredAt:       stringFromAny(row["stored_at"]),
		IsE2EE:         boolFromAny(row["is_e2ee"]),
		IsRead:         boolFromAny(row["is_read"]),
		SenderName:     stringFromAny(row["sender_name"]),
		Metadata:       stringFromAny(row["metadata"]),
		CredentialName: finalCredentialName,
	}
}

func normalizeRecoveredContactRow(row map[string]any, newOwnerDID string, finalCredentialName string) ContactRecord {
	return ContactRecord{
		OwnerDID:          newOwnerDID,
		DID:               stringFromAny(row["did"]),
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
		Metadata:          stringFromAny(row["metadata"]),
		CredentialName:    finalCredentialName,
	}
}

func normalizeRecoveredContactHandleBindingRow(row map[string]any, newOwnerDID string, finalCredentialName string) ContactHandleBindingRecord {
	return ContactHandleBindingRecord{
		OwnerDID:       newOwnerDID,
		Handle:         stringFromAny(row["handle"]),
		DID:            stringFromAny(row["did"]),
		IsCurrent:      false,
		FirstSeenAt:    stringFromAny(row["first_seen_at"]),
		LastSeenAt:     stringFromAny(row["last_seen_at"]),
		SourceType:     stringFromAny(row["source_type"]),
		SourceGroupID:  stringFromAny(row["source_group_id"]),
		Metadata:       stringFromAny(row["metadata"]),
		CredentialName: finalCredentialName,
	}
}

func normalizeRecoveredRelationshipEventRow(row map[string]any, newOwnerDID string, finalCredentialName string) RelationshipEventRecord {
	return RelationshipEventRecord{
		EventID:        stringFromAny(row["event_id"]),
		OwnerDID:       newOwnerDID,
		TargetDID:      stringFromAny(row["target_did"]),
		TargetHandle:   stringFromAny(row["target_handle"]),
		EventType:      stringFromAny(row["event_type"]),
		SourceType:     stringFromAny(row["source_type"]),
		SourceName:     stringFromAny(row["source_name"]),
		SourceGroupID:  stringFromAny(row["source_group_id"]),
		Reason:         stringFromAny(row["reason"]),
		Score:          float64PtrFromAny(row["score"]),
		Status:         stringFromAny(row["status"]),
		CreatedAt:      stringFromAny(row["created_at"]),
		UpdatedAt:      stringFromAny(row["updated_at"]),
		Metadata:       stringFromAny(row["metadata"]),
		CredentialName: finalCredentialName,
	}
}

func normalizeRecoveredGroupRow(row map[string]any, oldOwnerSet map[string]struct{}, newOwnerDID string, finalCredentialName string) GroupRecord {
	return GroupRecord{
		OwnerDID:          newOwnerDID,
		GroupID:           stringFromAny(row["group_id"]),
		GroupDID:          stringFromAny(row["group_did"]),
		Name:              stringFromAny(row["name"]),
		GroupMode:         stringFromAny(row["group_mode"]),
		Slug:              stringFromAny(row["slug"]),
		Description:       stringFromAny(row["description"]),
		Goal:              stringFromAny(row["goal"]),
		Rules:             stringFromAny(row["rules"]),
		MessagePrompt:     stringFromAny(row["message_prompt"]),
		DocURL:            stringFromAny(row["doc_url"]),
		GroupOwnerDID:     remapRecoveredSelfDID(stringFromAny(row["group_owner_did"]), oldOwnerSet, newOwnerDID),
		GroupOwnerHandle:  stringFromAny(row["group_owner_handle"]),
		MyRole:            stringFromAny(row["my_role"]),
		MembershipStatus:  stringFromAny(row["membership_status"]),
		JoinEnabled:       boolPtrFromAny(row["join_enabled"]),
		JoinCode:          stringFromAny(row["join_code"]),
		JoinCodeExpiresAt: stringFromAny(row["join_code_expires_at"]),
		MemberCount:       int64PtrFromAny(row["member_count"]),
		LastSyncedSeq:     int64PtrFromAny(row["last_synced_seq"]),
		LastReadSeq:       int64PtrFromAny(row["last_read_seq"]),
		LastMessageAt:     stringFromAny(row["last_message_at"]),
		RemoteCreatedAt:   stringFromAny(row["remote_created_at"]),
		RemoteUpdatedAt:   stringFromAny(row["remote_updated_at"]),
		StoredAt:          stringFromAny(row["stored_at"]),
		Metadata:          stringFromAny(row["metadata"]),
		CredentialName:    finalCredentialName,
	}
}

func normalizeRecoveredGroupMemberRow(row map[string]any, oldOwnerSet map[string]struct{}, newOwnerDID string, finalCredentialName string) GroupMemberRecord {
	return GroupMemberRecord{
		OwnerDID:         newOwnerDID,
		GroupID:          stringFromAny(row["group_id"]),
		UserID:           stringFromAny(row["user_id"]),
		MemberDID:        remapRecoveredSelfDID(stringFromAny(row["member_did"]), oldOwnerSet, newOwnerDID),
		MemberHandle:     stringFromAny(row["member_handle"]),
		ProfileURL:       stringFromAny(row["profile_url"]),
		Role:             stringFromAny(row["role"]),
		Status:           stringFromAny(row["status"]),
		JoinedAt:         stringFromAny(row["joined_at"]),
		SentMessageCount: int64PtrFromAny(row["sent_message_count"]),
		LastSyncedAt:     stringFromAny(row["last_synced_at"]),
		Metadata:         stringFromAny(row["metadata"]),
		CredentialName:   finalCredentialName,
	}
}

func mergeRecoveredMessage(existing map[string]any, incoming MessageRecord) MessageRecord {
	merged := incoming
	merged.ThreadID = defaultString(incoming.ThreadID, stringFromAny(existing["thread_id"]))
	merged.Direction = incoming.Direction
	merged.SenderDID = chooseLaterNonEmpty(stringFromAny(existing["sender_did"]), incoming.SenderDID)
	merged.ReceiverDID = chooseLaterNonEmpty(stringFromAny(existing["receiver_did"]), incoming.ReceiverDID)
	merged.GroupID = chooseLaterNonEmpty(stringFromAny(existing["group_id"]), incoming.GroupID)
	merged.GroupDID = chooseLaterNonEmpty(stringFromAny(existing["group_did"]), incoming.GroupDID)
	merged.ContentType = chooseLaterNonEmpty(stringFromAny(existing["content_type"]), incoming.ContentType)
	merged.Content = chooseLaterNonEmpty(stringFromAny(existing["content"]), incoming.Content)
	merged.Title = chooseLaterNonEmpty(stringFromAny(existing["title"]), incoming.Title)
	merged.ServerSeq = maxInt64Ptr(int64PtrFromAny(existing["server_seq"]), incoming.ServerSeq)
	merged.SentAt = laterTimeString(stringFromAny(existing["sent_at"]), incoming.SentAt)
	merged.StoredAt = laterTimeString(stringFromAny(existing["stored_at"]), incoming.StoredAt)
	merged.IsE2EE = boolFromAny(existing["is_e2ee"]) || incoming.IsE2EE
	merged.IsRead = boolFromAny(existing["is_read"]) || incoming.IsRead
	merged.SenderName = chooseLaterNonEmpty(stringFromAny(existing["sender_name"]), incoming.SenderName)
	merged.Metadata = chooseLaterNonEmpty(stringFromAny(existing["metadata"]), incoming.Metadata)
	merged.CredentialName = chooseLaterNonEmpty(stringFromAny(existing["credential_name"]), incoming.CredentialName)
	return merged
}

func mergeRecoveredContact(existing map[string]any, incoming ContactRecord) ContactRecord {
	merged := incoming
	merged.Name = chooseLaterNonEmpty(stringFromAny(existing["name"]), incoming.Name)
	merged.Handle = chooseLaterNonEmpty(stringFromAny(existing["handle"]), incoming.Handle)
	merged.NickName = chooseLaterNonEmpty(stringFromAny(existing["nick_name"]), incoming.NickName)
	merged.Bio = chooseLaterNonEmpty(stringFromAny(existing["bio"]), incoming.Bio)
	merged.ProfileMD = chooseLaterNonEmpty(stringFromAny(existing["profile_md"]), incoming.ProfileMD)
	merged.Tags = chooseLaterNonEmpty(stringFromAny(existing["tags"]), incoming.Tags)
	merged.Relationship = chooseLaterNonEmpty(stringFromAny(existing["relationship"]), incoming.Relationship)
	merged.SourceType = chooseLaterNonEmpty(stringFromAny(existing["source_type"]), incoming.SourceType)
	merged.SourceName = chooseLaterNonEmpty(stringFromAny(existing["source_name"]), incoming.SourceName)
	merged.SourceGroupID = chooseLaterNonEmpty(stringFromAny(existing["source_group_id"]), incoming.SourceGroupID)
	merged.ConnectedAt = laterTimeString(stringFromAny(existing["connected_at"]), incoming.ConnectedAt)
	merged.RecommendedReason = chooseLaterNonEmpty(stringFromAny(existing["recommended_reason"]), incoming.RecommendedReason)
	merged.Followed = boolPtr(boolFromAny(existing["followed"]) || boolFromPtr(incoming.Followed))
	merged.Messaged = boolPtr(boolFromAny(existing["messaged"]) || boolFromPtr(incoming.Messaged))
	merged.Note = chooseLaterNonEmpty(stringFromAny(existing["note"]), incoming.Note)
	merged.FirstSeenAt = earlierTimeString(stringFromAny(existing["first_seen_at"]), incoming.FirstSeenAt)
	merged.LastSeenAt = laterTimeString(stringFromAny(existing["last_seen_at"]), incoming.LastSeenAt)
	merged.Metadata = chooseLaterNonEmpty(stringFromAny(existing["metadata"]), incoming.Metadata)
	merged.CredentialName = chooseLaterNonEmpty(stringFromAny(existing["credential_name"]), incoming.CredentialName)
	return merged
}

func mergeRecoveredContactHandleBinding(existing map[string]any, incoming ContactHandleBindingRecord) ContactHandleBindingRecord {
	merged := incoming
	merged.IsCurrent = false
	merged.FirstSeenAt = earlierTimeString(stringFromAny(existing["first_seen_at"]), incoming.FirstSeenAt)
	merged.LastSeenAt = laterTimeString(stringFromAny(existing["last_seen_at"]), incoming.LastSeenAt)
	merged.SourceType = chooseLaterNonEmpty(stringFromAny(existing["source_type"]), incoming.SourceType)
	merged.SourceGroupID = chooseLaterNonEmpty(stringFromAny(existing["source_group_id"]), incoming.SourceGroupID)
	merged.Metadata = chooseLaterNonEmpty(stringFromAny(existing["metadata"]), incoming.Metadata)
	merged.CredentialName = chooseLaterNonEmpty(stringFromAny(existing["credential_name"]), incoming.CredentialName)
	return merged
}

func mergeRecoveredRelationshipEvent(existing map[string]any, incoming RelationshipEventRecord) RelationshipEventRecord {
	merged := incoming
	merged.TargetDID = chooseLaterNonEmpty(stringFromAny(existing["target_did"]), incoming.TargetDID)
	merged.TargetHandle = chooseLaterNonEmpty(stringFromAny(existing["target_handle"]), incoming.TargetHandle)
	merged.EventType = chooseLaterNonEmpty(stringFromAny(existing["event_type"]), incoming.EventType)
	merged.SourceType = chooseLaterNonEmpty(stringFromAny(existing["source_type"]), incoming.SourceType)
	merged.SourceName = chooseLaterNonEmpty(stringFromAny(existing["source_name"]), incoming.SourceName)
	merged.SourceGroupID = chooseLaterNonEmpty(stringFromAny(existing["source_group_id"]), incoming.SourceGroupID)
	merged.Reason = chooseLaterNonEmpty(stringFromAny(existing["reason"]), incoming.Reason)
	if incoming.Score == nil {
		merged.Score = float64PtrFromAny(existing["score"])
	}
	merged.Status = chooseLaterNonEmpty(stringFromAny(existing["status"]), incoming.Status)
	merged.CreatedAt = earlierTimeString(stringFromAny(existing["created_at"]), incoming.CreatedAt)
	merged.UpdatedAt = laterTimeString(stringFromAny(existing["updated_at"]), incoming.UpdatedAt)
	merged.Metadata = chooseLaterNonEmpty(stringFromAny(existing["metadata"]), incoming.Metadata)
	merged.CredentialName = chooseLaterNonEmpty(stringFromAny(existing["credential_name"]), incoming.CredentialName)
	return merged
}

func mergeRecoveredGroup(existing map[string]any, incoming GroupRecord) GroupRecord {
	merged := incoming
	merged.GroupDID = chooseLaterNonEmpty(stringFromAny(existing["group_did"]), incoming.GroupDID)
	merged.Name = chooseLaterNonEmpty(stringFromAny(existing["name"]), incoming.Name)
	merged.GroupMode = chooseLaterNonEmpty(stringFromAny(existing["group_mode"]), incoming.GroupMode)
	merged.Slug = chooseLaterNonEmpty(stringFromAny(existing["slug"]), incoming.Slug)
	merged.Description = chooseLaterNonEmpty(stringFromAny(existing["description"]), incoming.Description)
	merged.Goal = chooseLaterNonEmpty(stringFromAny(existing["goal"]), incoming.Goal)
	merged.Rules = chooseLaterNonEmpty(stringFromAny(existing["rules"]), incoming.Rules)
	merged.MessagePrompt = chooseLaterNonEmpty(stringFromAny(existing["message_prompt"]), incoming.MessagePrompt)
	merged.DocURL = chooseLaterNonEmpty(stringFromAny(existing["doc_url"]), incoming.DocURL)
	merged.GroupOwnerDID = chooseLaterNonEmpty(stringFromAny(existing["group_owner_did"]), incoming.GroupOwnerDID)
	merged.GroupOwnerHandle = chooseLaterNonEmpty(stringFromAny(existing["group_owner_handle"]), incoming.GroupOwnerHandle)
	merged.MyRole = chooseLaterNonEmpty(stringFromAny(existing["my_role"]), incoming.MyRole)
	merged.MembershipStatus = chooseLaterNonEmpty(stringFromAny(existing["membership_status"]), incoming.MembershipStatus)
	if incoming.JoinEnabled == nil {
		merged.JoinEnabled = boolPtrFromAny(existing["join_enabled"])
	}
	merged.JoinCode = chooseLaterNonEmpty(stringFromAny(existing["join_code"]), incoming.JoinCode)
	merged.JoinCodeExpiresAt = laterTimeString(stringFromAny(existing["join_code_expires_at"]), incoming.JoinCodeExpiresAt)
	merged.MemberCount = maxInt64Ptr(int64PtrFromAny(existing["member_count"]), incoming.MemberCount)
	merged.LastSyncedSeq = maxInt64Ptr(int64PtrFromAny(existing["last_synced_seq"]), incoming.LastSyncedSeq)
	merged.LastReadSeq = maxInt64Ptr(int64PtrFromAny(existing["last_read_seq"]), incoming.LastReadSeq)
	merged.LastMessageAt = laterTimeString(stringFromAny(existing["last_message_at"]), incoming.LastMessageAt)
	merged.RemoteCreatedAt = earlierTimeString(stringFromAny(existing["remote_created_at"]), incoming.RemoteCreatedAt)
	merged.RemoteUpdatedAt = laterTimeString(stringFromAny(existing["remote_updated_at"]), incoming.RemoteUpdatedAt)
	merged.StoredAt = laterTimeString(stringFromAny(existing["stored_at"]), incoming.StoredAt)
	merged.Metadata = chooseLaterNonEmpty(stringFromAny(existing["metadata"]), incoming.Metadata)
	merged.CredentialName = chooseLaterNonEmpty(stringFromAny(existing["credential_name"]), incoming.CredentialName)
	return merged
}

func mergeRecoveredGroupMember(existing map[string]any, incoming GroupMemberRecord) GroupMemberRecord {
	merged := incoming
	merged.MemberDID = chooseLaterNonEmpty(stringFromAny(existing["member_did"]), incoming.MemberDID)
	merged.MemberHandle = chooseLaterNonEmpty(stringFromAny(existing["member_handle"]), incoming.MemberHandle)
	merged.ProfileURL = chooseLaterNonEmpty(stringFromAny(existing["profile_url"]), incoming.ProfileURL)
	merged.Role = chooseLaterNonEmpty(stringFromAny(existing["role"]), incoming.Role)
	merged.Status = chooseLaterNonEmpty(stringFromAny(existing["status"]), incoming.Status)
	merged.JoinedAt = earlierTimeString(stringFromAny(existing["joined_at"]), incoming.JoinedAt)
	merged.SentMessageCount = maxInt64Ptr(int64PtrFromAny(existing["sent_message_count"]), incoming.SentMessageCount)
	merged.LastSyncedAt = laterTimeString(stringFromAny(existing["last_synced_at"]), incoming.LastSyncedAt)
	merged.Metadata = chooseLaterNonEmpty(stringFromAny(existing["metadata"]), incoming.Metadata)
	merged.CredentialName = chooseLaterNonEmpty(stringFromAny(existing["credential_name"]), incoming.CredentialName)
	return merged
}

func upsertRecoveredMessage(ctx context.Context, tx *sql.Tx, record MessageRecord) error {
	storedAt := defaultString(record.StoredAt, nowUTC())
	_, err := tx.ExecContext(ctx, `
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
    content_type = excluded.content_type,
    content = excluded.content,
    title = excluded.title,
    server_seq = excluded.server_seq,
    sent_at = excluded.sent_at,
    stored_at = excluded.stored_at,
    is_e2ee = excluded.is_e2ee,
    is_read = excluded.is_read,
    sender_name = excluded.sender_name,
    metadata = excluded.metadata,
    credential_name = excluded.credential_name`,
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
		storedAt,
		boolToInt(record.IsE2EE),
		boolToInt(record.IsRead),
		normalizeOptionalString(record.SenderName),
		normalizeMetadata(record.Metadata),
		normalizeCredentialName(record.CredentialName),
	)
	return err
}

func upsertRecoveredContact(ctx context.Context, tx *sql.Tx, record ContactRecord) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO contacts
    (owner_did, did, name, handle, nick_name, bio, profile_md, tags, relationship, source_type, source_name,
     source_group_id, connected_at, recommended_reason, followed, messaged, note, first_seen_at, last_seen_at, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(owner_did, did)
DO UPDATE SET
    name = excluded.name,
    handle = excluded.handle,
    nick_name = excluded.nick_name,
    bio = excluded.bio,
    profile_md = excluded.profile_md,
    tags = excluded.tags,
    relationship = excluded.relationship,
    source_type = excluded.source_type,
    source_name = excluded.source_name,
    source_group_id = excluded.source_group_id,
    connected_at = excluded.connected_at,
    recommended_reason = excluded.recommended_reason,
    followed = excluded.followed,
    messaged = excluded.messaged,
    note = excluded.note,
    first_seen_at = excluded.first_seen_at,
    last_seen_at = excluded.last_seen_at,
    metadata = excluded.metadata`,
		normalizeOwnerDID(record.OwnerDID),
		record.DID,
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
		normalizeOptionalString(record.FirstSeenAt),
		normalizeOptionalString(record.LastSeenAt),
		normalizeMetadata(record.Metadata),
	)
	return err
}

func upsertRecoveredContactHandleBinding(ctx context.Context, tx *sql.Tx, record ContactHandleBindingRecord) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO contact_handle_bindings
    (owner_did, handle, did, is_current, first_seen_at, last_seen_at, source_type, source_group_id, metadata, credential_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(owner_did, handle, did)
DO UPDATE SET
    is_current = excluded.is_current,
    first_seen_at = excluded.first_seen_at,
    last_seen_at = excluded.last_seen_at,
    source_type = excluded.source_type,
    source_group_id = excluded.source_group_id,
    metadata = excluded.metadata,
    credential_name = excluded.credential_name`,
		normalizeOwnerDID(record.OwnerDID),
		record.Handle,
		record.DID,
		boolToInt(record.IsCurrent),
		record.FirstSeenAt,
		record.LastSeenAt,
		normalizeOptionalString(record.SourceType),
		normalizeOptionalString(record.SourceGroupID),
		normalizeMetadata(record.Metadata),
		normalizeCredentialName(record.CredentialName),
	)
	return err
}

func upsertRecoveredRelationshipEvent(ctx context.Context, tx *sql.Tx, record RelationshipEventRecord) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO relationship_events
    (event_id, owner_did, target_did, target_handle, event_type, source_type, source_name, source_group_id,
     reason, score, status, created_at, updated_at, metadata, credential_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(event_id)
DO UPDATE SET
    owner_did = excluded.owner_did,
    target_did = excluded.target_did,
    target_handle = excluded.target_handle,
    event_type = excluded.event_type,
    source_type = excluded.source_type,
    source_name = excluded.source_name,
    source_group_id = excluded.source_group_id,
    reason = excluded.reason,
    score = excluded.score,
    status = excluded.status,
    created_at = excluded.created_at,
    updated_at = excluded.updated_at,
    metadata = excluded.metadata,
    credential_name = excluded.credential_name`,
		record.EventID,
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
		record.CreatedAt,
		record.UpdatedAt,
		normalizeMetadata(record.Metadata),
		normalizeCredentialName(record.CredentialName),
	)
	return err
}

func upsertRecoveredGroup(ctx context.Context, tx *sql.Tx, record GroupRecord) error {
	storedAt := defaultString(record.StoredAt, nowUTC())
	_, err := tx.ExecContext(ctx, `
INSERT INTO groups
    (owner_did, group_id, group_did, name, group_mode, slug, description, goal, rules, message_prompt,
     doc_url, group_owner_did, group_owner_handle, my_role, membership_status, join_enabled, join_code,
     join_code_expires_at, member_count, last_synced_seq, last_read_seq, last_message_at,
     remote_created_at, remote_updated_at, stored_at, metadata, credential_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(owner_did, group_id)
DO UPDATE SET
    group_did = excluded.group_did,
    name = excluded.name,
    group_mode = excluded.group_mode,
    slug = excluded.slug,
    description = excluded.description,
    goal = excluded.goal,
    rules = excluded.rules,
    message_prompt = excluded.message_prompt,
    doc_url = excluded.doc_url,
    group_owner_did = excluded.group_owner_did,
    group_owner_handle = excluded.group_owner_handle,
    my_role = excluded.my_role,
    membership_status = excluded.membership_status,
    join_enabled = excluded.join_enabled,
    join_code = excluded.join_code,
    join_code_expires_at = excluded.join_code_expires_at,
    member_count = excluded.member_count,
    last_synced_seq = excluded.last_synced_seq,
    last_read_seq = excluded.last_read_seq,
    last_message_at = excluded.last_message_at,
    remote_created_at = excluded.remote_created_at,
    remote_updated_at = excluded.remote_updated_at,
    stored_at = excluded.stored_at,
    metadata = excluded.metadata,
    credential_name = excluded.credential_name`,
		normalizeOwnerDID(record.OwnerDID),
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
		storedAt,
		normalizeMetadata(record.Metadata),
		normalizeCredentialName(record.CredentialName),
	)
	return err
}

func upsertRecoveredGroupMember(ctx context.Context, tx *sql.Tx, record GroupMemberRecord) error {
	lastSyncedAt := defaultString(record.LastSyncedAt, nowUTC())
	_, err := tx.ExecContext(ctx, `
INSERT INTO group_members
    (owner_did, group_id, user_id, member_did, member_handle, profile_url, role, status,
     joined_at, sent_message_count, last_synced_at, metadata, credential_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(owner_did, group_id, user_id)
DO UPDATE SET
    member_did = excluded.member_did,
    member_handle = excluded.member_handle,
    profile_url = excluded.profile_url,
    role = excluded.role,
    status = excluded.status,
    joined_at = excluded.joined_at,
    sent_message_count = excluded.sent_message_count,
    last_synced_at = excluded.last_synced_at,
    metadata = excluded.metadata,
    credential_name = excluded.credential_name`,
		normalizeOwnerDID(record.OwnerDID),
		record.GroupID,
		record.UserID,
		normalizeOptionalString(record.MemberDID),
		normalizeOptionalString(record.MemberHandle),
		normalizeOptionalString(record.ProfileURL),
		normalizeOptionalString(record.Role),
		defaultString(record.Status, "active"),
		normalizeOptionalString(record.JoinedAt),
		normalizeOptionalInt64(record.SentMessageCount),
		lastSyncedAt,
		normalizeMetadata(record.Metadata),
		normalizeCredentialName(record.CredentialName),
	)
	return err
}

func normalizeRecoveredCurrentHandles(ctx context.Context, tx *sql.Tx, ownerDID string, affectedHandles map[string]struct{}) error {
	for handle := range affectedHandles {
		rows, err := queryMapsWithQueryer(ctx, tx, `
SELECT did
FROM contact_handle_bindings
WHERE owner_did = ? AND handle = ?
ORDER BY COALESCE(last_seen_at, first_seen_at, '') DESC, did DESC`, ownerDID, handle)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			continue
		}
		currentDID := stringFromAny(rows[0]["did"])
		if _, err := tx.ExecContext(ctx, `
UPDATE contact_handle_bindings
SET is_current = CASE WHEN did = ? THEN 1 ELSE 0 END
WHERE owner_did = ? AND handle = ?`,
			currentDID,
			normalizeOwnerDID(ownerDID),
			handle,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE contacts
SET handle = CASE WHEN did = ? THEN ? ELSE NULL END
WHERE owner_did = ? AND (did = ? OR handle = ?)`,
			currentDID,
			handle,
			normalizeOwnerDID(ownerDID),
			currentDID,
			handle,
		); err != nil {
			return err
		}
	}
	return nil
}

func deleteRowsForOwners(ctx context.Context, tx *sql.Tx, table string, owners []string) error {
	if len(owners) == 0 {
		return nil
	}
	placeholders := make([]string, 0, len(owners))
	args := make([]any, 0, len(owners))
	for _, owner := range owners {
		placeholders = append(placeholders, "?")
		args = append(args, owner)
	}
	_, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE owner_did IN (%s)", table, strings.Join(placeholders, ",")), args...)
	return err
}

func countRowsForOwners(ctx context.Context, tx *sql.Tx, table string, owners []string) (int64, error) {
	if len(owners) == 0 {
		return 0, nil
	}
	placeholders := make([]string, 0, len(owners))
	args := make([]any, 0, len(owners))
	for _, owner := range owners {
		placeholders = append(placeholders, "?")
		args = append(args, owner)
	}
	var count int64
	if err := tx.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE owner_did IN (%s)", table, strings.Join(placeholders, ",")), args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func normalizeRecoverOwnerDIDs(oldOwnerDIDs []string, newOwnerDID string) []string {
	normalized := make([]string, 0, len(oldOwnerDIDs))
	seen := make(map[string]struct{}, len(oldOwnerDIDs))
	newOwnerDID = normalizeOwnerDID(newOwnerDID)
	for _, owner := range oldOwnerDIDs {
		owner = normalizeOwnerDID(owner)
		if owner == "" || owner == newOwnerDID {
			continue
		}
		if _, ok := seen[owner]; ok {
			continue
		}
		seen[owner] = struct{}{}
		normalized = append(normalized, owner)
	}
	return normalized
}

func remapRecoveredSelfDID(value string, oldOwnerSet map[string]struct{}, newOwnerDID string) string {
	value = strings.TrimSpace(value)
	if _, ok := oldOwnerSet[value]; ok {
		return newOwnerDID
	}
	return value
}

func firstRecoveredPeerDID(senderDID string, receiverDID string, ownerDID string) string {
	switch {
	case strings.TrimSpace(senderDID) != "" && senderDID != ownerDID:
		return senderDID
	case strings.TrimSpace(receiverDID) != "" && receiverDID != ownerDID:
		return receiverDID
	default:
		return firstNonEmpty(senderDID, receiverDID)
	}
}

func chooseLaterNonEmpty(existing string, incoming string) string {
	if strings.TrimSpace(incoming) != "" {
		return incoming
	}
	return existing
}

func earlierTimeString(existing string, incoming string) string {
	switch compareTimeStrings(existing, incoming) {
	case 1:
		return incoming
	case -1, 0:
		return defaultString(existing, incoming)
	default:
		return defaultString(incoming, existing)
	}
}

func laterTimeString(existing string, incoming string) string {
	switch compareTimeStrings(existing, incoming) {
	case -1:
		return incoming
	case 1, 0:
		return defaultString(existing, incoming)
	default:
		return defaultString(incoming, existing)
	}
}

func compareTimeStrings(left string, right string) int {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	switch {
	case left == "" && right == "":
		return 0
	case left == "":
		return -1
	case right == "":
		return 1
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

func maxInt64Ptr(left *int64, right *int64) *int64 {
	switch {
	case left == nil:
		return right
	case right == nil:
		return left
	case *left >= *right:
		return left
	default:
		return right
	}
}

func boolFromPtr(value *bool) bool {
	return value != nil && *value
}

func boolPtr(value bool) *bool {
	result := value
	return &result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

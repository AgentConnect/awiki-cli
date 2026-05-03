package message

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/runtime"
	"github.com/agentconnect/awiki-cli/internal/store"
	"github.com/agentconnect/awiki-cli/internal/traceutil"
)

func (s *Service) CreateGroup(ctx context.Context, request GroupCreateRequest) (*CommandResult, error) {
	if strings.TrimSpace(request.Name) == "" {
		return nil, ErrGroupRequired
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	transport, warnings, err := s.groupControlTransport(record)
	if err != nil {
		return nil, err
	}
	result, err := transport.CreateGroup(ctx, request)
	if err != nil {
		return nil, err
	}
	groupDID := stringFromAny(result["group_did"])
	warnings = append(warnings, s.syncGroupState(ctx, record, groupDID, true)...)
	var e2eeResult map[string]any
	if groupRequestUsesE2EE(request) {
		e2eeCandidate, e2eeWarnings := s.createGroupE2EE(ctx, record, groupDID)
		e2eeResult, warnings = appendE2EEResult(warnings, e2eeCandidate, e2eeWarnings)
	}
	snapshot, _ := s.readCachedGroupSnapshot(ctx, record, groupDID)
	members, _ := s.readCachedGroupMembers(ctx, record, groupDID, 100)
	data := map[string]any{
		"group":    snapshot,
		"members":  members,
		"delivery": result,
		"source":   groupControlSource(result),
	}
	if e2eeResult != nil {
		data["e2ee"] = e2eeResult
	}
	return &CommandResult{
		Data:     data,
		Summary:  fmt.Sprintf("Created group %s", groupDID),
		Warnings: compactWarnings(warnings),
	}, nil
}

func (s *Service) GetGroup(ctx context.Context, request GroupGetRequest) (*CommandResult, error) {
	if strings.TrimSpace(request.Group) == "" {
		return nil, ErrGroupRequired
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	transport, warnings, err := s.groupControlTransport(record)
	if err != nil {
		return nil, err
	}
	result, err := transport.GetGroup(ctx, request)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, s.persistGroupSnapshot(ctx, record, result)...)
	snapshot, _ := s.readCachedGroupSnapshot(ctx, record, request.Group)
	if len(snapshot) == 0 {
		snapshot = normalizeGroupSnapshot(result)
	}
	return &CommandResult{Data: map[string]any{"group": snapshot, "source": groupControlSource(result)}, Summary: "Loaded group snapshot", Warnings: compactWarnings(warnings)}, nil
}

func (s *Service) JoinGroup(ctx context.Context, request GroupJoinRequest) (*CommandResult, error) {
	if strings.TrimSpace(request.Group) == "" {
		return nil, ErrGroupRequired
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	transport, warnings, err := s.groupControlTransport(record)
	if err != nil {
		return nil, err
	}
	result, err := transport.JoinGroup(ctx, request)
	if err != nil {
		return nil, err
	}
	groupDID := stringFromAny(result["group_did"])
	warnings = append(warnings, s.syncGroupState(ctx, record, groupDID, true)...)
	snapshot, _ := s.readCachedGroupSnapshot(ctx, record, groupDID)
	return &CommandResult{Data: map[string]any{"group": snapshot, "delivery": result, "source": groupControlSource(result)}, Summary: fmt.Sprintf("Joined group %s", groupDID), Warnings: compactWarnings(warnings)}, nil
}

func (s *Service) AddGroupMember(ctx context.Context, request GroupMemberRequest) (*CommandResult, error) {
	return s.mutateGroupMember(ctx, request, "add")
}

func (s *Service) RemoveGroupMember(ctx context.Context, request GroupMemberRequest) (*CommandResult, error) {
	return s.mutateGroupMember(ctx, request, "remove")
}

func (s *Service) mutateGroupMember(ctx context.Context, request GroupMemberRequest, action string) (*CommandResult, error) {
	if strings.TrimSpace(request.Group) == "" {
		return nil, ErrGroupRequired
	}
	if strings.TrimSpace(request.Member) == "" {
		return nil, ErrMemberRequired
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	memberDID, memberHandle, err := s.resolveTarget(ctx, request.Member)
	if err != nil {
		return nil, err
	}
	request.Member = memberDID
	var preMutationSnapshot map[string]any
	if action == "add" || action == "remove" {
		preMutationSnapshot, _ = s.readCachedGroupSnapshot(ctx, record, request.Group)
	}
	if action == "remove" && groupMemberMutationUsesE2EE(request, preMutationSnapshot, nil) {
		e2eeResult, e2eeWarnings, err := s.removeGroupMemberE2EE(ctx, record, request)
		if err != nil {
			return nil, err
		}
		warnings := append([]string(nil), e2eeWarnings...)
		warnings = append(warnings, s.syncGroupState(ctx, record, request.Group, true)...)
		snapshot, _ := s.readCachedGroupSnapshot(ctx, record, request.Group)
		members, _ := s.readCachedGroupMembers(ctx, record, request.Group, 100)
		return &CommandResult{
			Data: map[string]any{
				"group":    snapshot,
				"members":  members,
				"delivery": e2eeResult["delivery"],
				"member":   map[string]any{"did": memberDID, "handle": memberHandle},
				"e2ee":     e2eeResult,
			},
			Summary:  "Removed member from group with group E2EE",
			Warnings: compactWarnings(warnings),
		}, nil
	}
	transport, warnings, err := s.groupControlTransport(record)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if action == "add" {
		result, err = transport.AddGroupMember(ctx, request)
	} else {
		result, err = transport.RemoveGroupMember(ctx, request)
	}
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, s.syncGroupState(ctx, record, request.Group, true)...)
	snapshot, _ := s.readCachedGroupSnapshot(ctx, record, request.Group)
	members, _ := s.readCachedGroupMembers(ctx, record, request.Group, 100)
	var e2eeResult map[string]any
	if action == "add" && groupMemberMutationUsesE2EE(request, preMutationSnapshot, snapshot) {
		e2eeCandidate, e2eeWarnings := s.addGroupMemberE2EE(ctx, record, request.Group, memberDID)
		e2eeResult, warnings = appendE2EEResult(warnings, e2eeCandidate, e2eeWarnings)
	}
	data := map[string]any{"group": snapshot, "members": members, "delivery": result, "member": map[string]any{"did": memberDID, "handle": memberHandle}}
	if e2eeResult != nil {
		data["e2ee"] = e2eeResult
	}
	return &CommandResult{Data: data, Summary: fmt.Sprintf("Updated group membership via %s", action), Warnings: compactWarnings(warnings)}, nil
}

func (s *Service) LeaveGroup(ctx context.Context, request GroupLeaveRequest) (*CommandResult, error) {
	if strings.TrimSpace(request.Group) == "" {
		return nil, ErrGroupRequired
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	cachedSnapshot, snapshotErr := s.readCachedGroupSnapshot(ctx, record, request.Group)
	if snapshotErr == nil && isActiveGroupOwner(cachedSnapshot) {
		return nil, ErrGroupOwnerCannotLeave
	}
	if groupSnapshotUsesE2EE(cachedSnapshot) {
		e2eeResult, e2eeWarnings, err := s.leaveGroupE2EE(ctx, record, request)
		if err != nil {
			return nil, err
		}
		warnings := append([]string(nil), e2eeWarnings...)
		warnings = append(warnings, s.markCachedGroupLeft(ctx, record, request.Group)...)
		return &CommandResult{
			Data: map[string]any{
				"delivery": e2eeResult["delivery"],
				"group":    request.Group,
				"e2ee":     e2eeResult,
			},
			Summary:  fmt.Sprintf("Left group %s with group E2EE", request.Group),
			Warnings: compactWarnings(warnings),
		}, nil
	}
	transport, warnings, err := s.groupControlTransport(record)
	if err != nil {
		return nil, err
	}
	result, err := transport.LeaveGroup(ctx, request)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, s.markCachedGroupLeft(ctx, record, request.Group)...)
	return &CommandResult{Data: map[string]any{"delivery": result, "group": request.Group}, Summary: fmt.Sprintf("Left group %s", request.Group), Warnings: compactWarnings(warnings)}, nil
}

func (s *Service) UpdateGroup(ctx context.Context, request GroupUpdateRequest) (*CommandResult, error) {
	if strings.TrimSpace(request.Group) == "" {
		return nil, ErrGroupRequired
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	profilePatch := buildGroupProfilePatch(request.Name, request.Description, request.Discoverability, request.Slug, request.Goal, request.Rules, request.MessagePrompt, request.DocURL)
	policyPatch := buildGroupPolicyPatch(request.AdmissionMode, request.AttachmentsAllowed, request.MaxMembers, request.MemberMaxMessages, request.MemberMaxTotalChars)
	if len(profilePatch) == 0 && len(policyPatch) == 0 {
		return nil, fmt.Errorf("group update requires at least one mutable field")
	}
	transport, warnings, err := s.groupControlTransport(record)
	if err != nil {
		return nil, err
	}
	responses := make([]map[string]any, 0, 2)
	if len(profilePatch) > 0 {
		result, callErr := transport.UpdateGroupProfile(ctx, GroupGetRequest{Group: request.Group}, profilePatch)
		if callErr != nil {
			return nil, callErr
		}
		responses = append(responses, result)
	}
	if len(policyPatch) > 0 {
		result, callErr := transport.UpdateGroupPolicy(ctx, GroupGetRequest{Group: request.Group}, policyPatch)
		if callErr != nil {
			return nil, callErr
		}
		responses = append(responses, result)
	}
	warnings = append(warnings, s.syncGroupState(ctx, record, request.Group, false)...)
	snapshot, _ := s.readCachedGroupSnapshot(ctx, record, request.Group)
	return &CommandResult{Data: map[string]any{"group": snapshot, "delivery": responses}, Summary: fmt.Sprintf("Updated group %s", request.Group), Warnings: compactWarnings(warnings)}, nil
}

func (s *Service) GroupMembers(ctx context.Context, request GroupMembersRequest) (*CommandResult, error) {
	if strings.TrimSpace(request.Group) == "" {
		return nil, ErrGroupRequired
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	transport, warnings, err := s.groupControlTransport(record)
	if err != nil {
		return nil, err
	}
	result, err := transport.ListGroupMembers(ctx, request)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, s.persistGroupMembers(ctx, record, request.Group, result)...)
	members, _ := s.readCachedGroupMembers(ctx, record, request.Group, request.Limit)
	if len(members) == 0 {
		members = groupMembersFromResult(result["members"])
	}
	total := intValueFromAny(result["total"], len(members))
	return &CommandResult{Data: map[string]any{"group": request.Group, "members": members, "total": total, "source": groupControlSource(result)}, Summary: fmt.Sprintf("Loaded %d group members", total), Warnings: compactWarnings(warnings)}, nil
}

func (s *Service) GroupMessages(ctx context.Context, request GroupMessagesRequest) (*CommandResult, error) {
	if strings.TrimSpace(request.Group) == "" {
		return nil, ErrGroupRequired
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	sourceMode := s.runtimeConfig().Mode
	transport, warnings, err := s.transportFor(record)
	if err != nil {
		return nil, err
	}
	result, err := transport.ListGroupMessages(ctx, request)
	if err != nil {
		wsErr := err
		cached, cacheErr := s.readCachedGroupMessages(ctx, record, request.Group, request.Limit, request.Cursor)
		if cacheErr == nil && len(cached) > 0 {
			return &CommandResult{Data: map[string]any{"group": request.Group, "messages": cached, "total": len(cached), "source": "local_ws_cache_fallback"}, Summary: "Loaded group messages from local cache", Warnings: []string{websocketCacheFallbackWarning(wsErr)}}, nil
		}
		httpTransport, httpWarnings, httpErr := s.httpTransport(record)
		if httpErr != nil {
			return nil, err
		}
		result, err = httpTransport.ListGroupMessages(ctx, request)
		if err != nil {
			return nil, err
		}
		sourceMode = runtime.ModeHTTP
		traceutil.MarkFallback(ctx, "websocket_to_http", wsErr)
		warnings = append(warnings, websocketHTTPFallbackWarning(wsErr))
		warnings = append(warnings, httpWarnings...)
	}
	decryptWarnings, decryptedResult := s.maybeDecryptGroupMessages(ctx, record, request.Group, result)
	warnings = append(warnings, decryptWarnings...)
	if decryptedResult != nil {
		result = decryptedResult
	}
	warnings = append(warnings, s.persistGroupMessages(ctx, record, request.Group, result)...)
	messages, _ := s.readCachedGroupMessages(ctx, record, request.Group, request.Limit, request.Cursor)
	if len(messages) == 0 {
		messages = messagesFromResult(result["messages"])
	}
	total := intValueFromAny(result["total"], len(messages))
	return &CommandResult{Data: map[string]any{"group": request.Group, "messages": messages, "total": total, "has_more": boolFromAny(result["has_more"]), "next_since_seq": result["next_since_seq"], "source": sourceWithDefault(result, sourceMode)}, Summary: fmt.Sprintf("Loaded %d group messages", total), Warnings: compactWarnings(warnings)}, nil
}

func (s *Service) sendGroup(ctx context.Context, request SendRequest) (*CommandResult, error) {
	if strings.TrimSpace(request.Group) == "" {
		return nil, ErrGroupRequired
	}
	if strings.TrimSpace(request.Text) == "" {
		return nil, ErrTextRequired
	}
	if request.SecureMode == "on" {
		// For groups, --secure on selects the explicit group E2EE path when the
		// cached group summary indicates group-e2ee.
		record, err := s.requireActiveIdentity(request.IdentityName)
		if err != nil {
			return nil, err
		}
		snapshot, _ := s.readCachedGroupSnapshot(ctx, record, request.Group)
		if !groupSnapshotUsesE2EE(snapshot) {
			return nil, ErrSecureNotSupported
		}
		return s.sendGroupE2EE(ctx, record, request)
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	snapshot, _ := s.readCachedGroupSnapshot(ctx, record, request.Group)
	if groupSnapshotUsesE2EE(snapshot) {
		return s.sendGroupE2EE(ctx, record, request)
	}
	sourceMode := s.runtimeConfig().Mode
	transport, warnings, err := s.transportFor(record)
	if err != nil {
		return nil, err
	}
	result, err := transport.SendGroup(ctx, request)
	if err != nil {
		wsErr := err
		httpTransport, httpWarnings, httpErr := s.httpTransport(record)
		if httpErr != nil {
			return nil, err
		}
		result, err = httpTransport.SendGroup(ctx, request)
		if err != nil {
			return nil, err
		}
		sourceMode = runtime.ModeHTTP
		traceutil.MarkFallback(ctx, "websocket_to_http", wsErr)
		warnings = append(warnings, websocketHTTPFallbackWarning(wsErr))
		warnings = append(warnings, httpWarnings...)
	}
	return s.persistGroupSendResult(ctx, record, request, result, warnings, sourceMode)
}

func appendE2EEResult(warnings []string, result map[string]any, e2eeWarnings []string) (map[string]any, []string) {
	warnings = append(warnings, e2eeWarnings...)
	if result == nil {
		return nil, warnings
	}
	return result, warnings
}

func (s *Service) syncGroupState(ctx context.Context, record *identity.StoredIdentity, groupDID string, includeMembers bool) []string {
	if strings.TrimSpace(groupDID) == "" {
		return nil
	}
	httpTransport, _, err := s.httpTransport(record)
	if err != nil {
		return []string{fmt.Sprintf("Failed to prepare group sync transport: %v", err)}
	}
	warnings := make([]string, 0)
	groupResult, err := httpTransport.GetGroup(ctx, GroupGetRequest{Group: groupDID})
	if err != nil {
		return []string{fmt.Sprintf("Failed to refresh group snapshot: %v", err)}
	}
	warnings = append(warnings, s.persistGroupSnapshot(ctx, record, groupResult)...)
	if includeMembers {
		memberResult, memberErr := httpTransport.ListGroupMembers(ctx, GroupMembersRequest{Group: groupDID, Limit: 100})
		if memberErr != nil {
			warnings = append(warnings, fmt.Sprintf("Failed to refresh group members: %v", memberErr))
		} else {
			warnings = append(warnings, s.persistGroupMembers(ctx, record, groupDID, memberResult)...)
		}
	}
	return compactWarnings(warnings)
}

func (s *Service) persistGroupSendResult(ctx context.Context, record *identity.StoredIdentity, request SendRequest, result *groupSendResult, warnings []string, sourceMode string) (*CommandResult, error) {
	finish := traceutil.LocalDBPhase(ctx, "persist_group_send")
	defer finish()
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	msgID := result.MessageID
	if strings.TrimSpace(result.GroupDID) != "" && strings.TrimSpace(result.GroupEventSeq) != "" {
		msgID = fmt.Sprintf("%s:%s", result.GroupDID, result.GroupEventSeq)
	} else if msgID == "" {
		msgID = "msg-" + generateOperationID()
	}
	groupKey := groupStorageKey(request.Group)
	if err := store.StoreMessage(ctx, db, store.MessageRecord{
		MsgID:          msgID,
		OwnerDID:       record.DID,
		ThreadID:       store.MakeThreadID(record.DID, "", groupKey),
		Direction:      1,
		SenderDID:      record.DID,
		GroupID:        groupKey,
		GroupDID:       request.Group,
		ContentType:    contentTypeForMessageType(request.MessageType),
		Content:        request.Text,
		SentAt:         result.AcceptedAt,
		IsRead:         true,
		Metadata:       metadataString(map[string]any{"group_event_seq": result.GroupEventSeq, "group_state_version": result.GroupStateVersion, "operation_id": result.OperationID}),
		CredentialName: record.IdentityName,
	}); err != nil {
		warnings = append(warnings, fmt.Sprintf("Failed to persist local group message: %v", err))
	}
	warnings = append(warnings, s.touchCachedGroup(ctx, record, request.Group, result.AcceptedAt, result.GroupEventSeq, result.GroupStateVersion)...)
	return &CommandResult{Data: map[string]any{"action": "send_message", "target": map[string]any{"kind": "group", "did": request.Group}, "message": map[string]any{"id": msgID, "type": request.MessageType, "secure": false, "sent_at": result.AcceptedAt}, "delivery": result, "source": transportSource(sourceMode)}, Summary: fmt.Sprintf("Sent a group %s message", request.MessageType), Warnings: compactWarnings(warnings)}, nil
}

func (s *Service) persistGroupSnapshot(ctx context.Context, record *identity.StoredIdentity, raw map[string]any) []string {
	snapshot := normalizeGroupSnapshot(raw)
	if len(snapshot) == 0 {
		return nil
	}
	finish := traceutil.LocalDBPhase(ctx, "persist_group_snapshot")
	defer finish()
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return []string{fmt.Sprintf("Failed to open local store for group snapshot: %v", err)}
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return []string{fmt.Sprintf("Failed to ensure local schema for group snapshot: %v", err)}
	}
	groupDID := stringFromAny(snapshot["group_did"])
	if groupDID == "" {
		return nil
	}
	memberCount := int64PtrFromAny(snapshot["member_count"])
	lastSyncedSeq := int64PtrFromAny(snapshot["group_event_seq"])
	recordToStore := store.GroupRecord{
		OwnerDID:         record.DID,
		GroupID:          groupStorageKey(groupDID),
		GroupDID:         groupDID,
		Name:             stringFromAny(snapshot["name"]),
		Slug:             stringFromAny(snapshot["slug"]),
		Description:      stringFromAny(snapshot["description"]),
		Goal:             stringFromAny(snapshot["goal"]),
		Rules:            stringFromAny(snapshot["rules"]),
		MessagePrompt:    stringFromAny(snapshot["message_prompt"]),
		DocURL:           stringFromAny(snapshot["doc_url"]),
		GroupOwnerDID:    stringFromAny(snapshot["owner_did"]),
		MyRole:           stringFromAny(snapshot["member_role"]),
		MembershipStatus: stringFromAny(snapshot["member_status"]),
		JoinEnabled:      boolPtrFromAny(snapshot["join_enabled"]),
		MemberCount:      memberCount,
		LastSyncedSeq:    lastSyncedSeq,
		RemoteCreatedAt:  stringFromAny(snapshot["created_at"]),
		RemoteUpdatedAt:  stringFromAny(snapshot["updated_at"]),
		Metadata:         metadataString(snapshot),
		CredentialName:   record.IdentityName,
	}
	if err := store.UpsertGroup(ctx, db, recordToStore); err != nil {
		return []string{fmt.Sprintf("Failed to persist group snapshot: %v", err)}
	}
	return nil
}

func (s *Service) persistGroupMembers(ctx context.Context, record *identity.StoredIdentity, groupDID string, raw map[string]any) []string {
	members := groupMembersFromResult(raw["members"])
	if len(members) == 0 {
		return nil
	}
	finish := traceutil.LocalDBPhase(ctx, "persist_group_members")
	defer finish()
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return []string{fmt.Sprintf("Failed to open local store for group members: %v", err)}
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return []string{fmt.Sprintf("Failed to ensure local schema for group members: %v", err)}
	}
	records := make([]store.GroupMemberRecord, 0, len(members))
	for _, member := range members {
		memberDID := stringFromAny(member["agent_did"])
		if memberDID == "" {
			continue
		}
		memberHandle := normalizeHandleValue(defaultString(
			stringFromAny(member["handle"]),
			defaultString(stringFromAny(member["member_handle"]), stringFromAny(member["agent_handle"])),
		))
		records = append(records, store.GroupMemberRecord{
			OwnerDID:       record.DID,
			GroupID:        groupStorageKey(groupDID),
			UserID:         memberDID,
			MemberDID:      memberDID,
			MemberHandle:   memberHandle,
			Role:           stringFromAny(member["role"]),
			Status:         stringFromAny(member["status"]),
			JoinedAt:       stringFromAny(member["joined_at"]),
			Metadata:       metadataString(member),
			CredentialName: record.IdentityName,
		})
	}
	if err := store.ReplaceGroupMembers(ctx, db, record.DID, groupStorageKey(groupDID), records, record.IdentityName); err != nil {
		return []string{fmt.Sprintf("Failed to persist group members: %v", err)}
	}
	return nil
}

func (s *Service) persistGroupMessages(ctx context.Context, record *identity.StoredIdentity, groupDID string, raw map[string]any) []string {
	messages := messagesFromResult(raw["messages"])
	if len(messages) == 0 {
		return nil
	}
	finish := traceutil.LocalDBPhase(ctx, "persist_group_messages")
	defer finish()
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return []string{fmt.Sprintf("Failed to open local store for group messages: %v", err)}
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return []string{fmt.Sprintf("Failed to ensure local schema for group messages: %v", err)}
	}
	batch := make([]store.MessageRecord, 0, len(messages))
	for _, item := range messages {
		msgID := stringFromAny(item["id"])
		if msgID == "" {
			msgID = stringFromAny(item["message_id"])
		}
		if msgID == "" {
			continue
		}
		direction := 0
		if stringFromAny(item["sender_did"]) == record.DID {
			direction = 1
		}
		contentType := stringFromAny(item["content_type"])
		contentValue := item["content"]
		content := stringFromAny(contentValue)
		if content == "" {
			content = metadataString(contentValue)
		}
		batch = append(batch, store.MessageRecord{
			MsgID:          msgID,
			OwnerDID:       record.DID,
			ThreadID:       store.MakeThreadID(record.DID, "", groupStorageKey(groupDID)),
			Direction:      direction,
			SenderDID:      stringFromAny(item["sender_did"]),
			GroupID:        groupStorageKey(groupDID),
			GroupDID:       groupDID,
			ContentType:    defaultString(contentType, inferGroupMessageContentType(item)),
			Content:        content,
			ServerSeq:      int64PtrFromAny(item["server_seq"]),
			SentAt:         defaultString(stringFromAny(item["sent_at"]), stringFromAny(item["created_at"])),
			IsRead:         boolFromAny(item["is_read"]),
			Metadata:       metadataString(item),
			CredentialName: record.IdentityName,
		})
	}
	if err := store.StoreMessagesBatch(ctx, db, batch); err != nil {
		return []string{fmt.Sprintf("Failed to persist group messages: %v", err)}
	}
	if len(messages) > 0 {
		latest := messages[0]
		_ = store.UpsertGroup(ctx, db, store.GroupRecord{
			OwnerDID:       record.DID,
			GroupID:        groupStorageKey(groupDID),
			GroupDID:       groupDID,
			LastSyncedSeq:  int64PtrFromAny(raw["next_since_seq"]),
			LastMessageAt:  defaultString(stringFromAny(latest["sent_at"]), stringFromAny(latest["created_at"])),
			CredentialName: record.IdentityName,
			Metadata:       metadataString(map[string]any{"source": "group.list_messages"}),
		})
	}
	return nil
}

func (s *Service) touchCachedGroup(ctx context.Context, record *identity.StoredIdentity, groupDID string, sentAt string, groupEventSeq string, groupStateVersion string) []string {
	finish := traceutil.LocalDBPhase(ctx, "touch_group_cache")
	defer finish()
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return []string{fmt.Sprintf("Failed to open local store for group cache update: %v", err)}
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return []string{fmt.Sprintf("Failed to ensure local schema for group cache update: %v", err)}
	}
	if err := store.UpsertGroup(ctx, db, store.GroupRecord{OwnerDID: record.DID, GroupID: groupStorageKey(groupDID), GroupDID: groupDID, LastMessageAt: sentAt, LastSyncedSeq: parseInt64Ptr(groupEventSeq), CredentialName: record.IdentityName, Metadata: metadataString(map[string]any{"group_state_version": groupStateVersion})}); err != nil {
		return []string{fmt.Sprintf("Failed to update group cache: %v", err)}
	}
	return nil
}

func (s *Service) markCachedGroupLeft(ctx context.Context, record *identity.StoredIdentity, groupDID string) []string {
	finish := traceutil.LocalDBPhase(ctx, "mark_group_left")
	defer finish()
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return []string{fmt.Sprintf("Failed to open local store for leave projection: %v", err)}
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return []string{fmt.Sprintf("Failed to ensure local schema for leave projection: %v", err)}
	}
	groupKey := groupStorageKey(groupDID)
	if err := store.UpsertGroup(ctx, db, store.GroupRecord{OwnerDID: record.DID, GroupID: groupKey, GroupDID: groupDID, MyRole: "", MembershipStatus: "left", CredentialName: record.IdentityName}); err != nil {
		return []string{fmt.Sprintf("Failed to update local group leave status: %v", err)}
	}
	if _, err := store.DeleteGroupMembers(ctx, db, record.DID, groupKey, "", ""); err != nil {
		return []string{fmt.Sprintf("Failed to clear local group members after leave: %v", err)}
	}
	return nil
}

func (s *Service) readCachedGroupSnapshot(ctx context.Context, record *identity.StoredIdentity, groupDID string) (map[string]any, error) {
	finish := traceutil.LocalDBPhase(ctx, "read_group_snapshot_cache")
	defer finish()
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	row, err := store.GetGroupSnapshot(ctx, db, record.DID, groupStorageKey(groupDID))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return row, nil
}

func (s *Service) readCachedGroupMembers(ctx context.Context, record *identity.StoredIdentity, groupDID string, limit int) ([]map[string]any, error) {
	finish := traceutil.LocalDBPhase(ctx, "read_group_members_cache")
	defer finish()
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	return store.ListCachedGroupMembers(ctx, db, record.DID, groupStorageKey(groupDID), limit)
}

func (s *Service) readCachedGroupMessages(ctx context.Context, record *identity.StoredIdentity, groupDID string, limit int, cursor string) ([]map[string]any, error) {
	finish := traceutil.LocalDBPhase(ctx, "read_group_messages_cache")
	defer finish()
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	return store.ListGroupMessages(ctx, db, record.DID, groupStorageKey(groupDID), limit, parseInt64Ptr(cursor))
}

func normalizeGroupSnapshot(raw map[string]any) map[string]any {
	if raw == nil {
		return nil
	}
	if snapshot, ok := raw["group_snapshot"].(map[string]any); ok {
		return snapshot
	}
	if groupDID := stringFromAny(raw["group_did"]); groupDID != "" {
		name := ""
		if profile, ok := raw["group_profile"].(map[string]any); ok {
			name = stringFromAny(profile["display_name"])
			return map[string]any{
				"group_did":           groupDID,
				"group_state_version": raw["group_state_version"],
				"name":                name,
				"description":         profile["description"],
				"discoverability":     profile["discoverability"],
				"member_count":        raw["member_count"],
				"group_profile":       profile,
				"group_policy":        raw["group_policy"],
			}
		}
	}
	return nil
}

func groupMembersFromResult(value any) []map[string]any {
	return messagesFromResult(value)
}

func shouldUseCachedGroupFallback(err error) bool {
	return !isInactiveGroupViewerError(err)
}

func isInactiveGroupViewerError(err error) bool {
	if err == nil {
		return false
	}
	var serviceErr *ServiceError
	if errors.As(err, &serviceErr) && serviceErr.RPCCode == 2501 {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "viewer is not an active member")
}

func isActiveGroupOwner(snapshot map[string]any) bool {
	if len(snapshot) == 0 {
		return false
	}
	role := strings.TrimSpace(defaultString(stringFromAny(snapshot["my_role"]), stringFromAny(snapshot["member_role"])))
	status := strings.TrimSpace(defaultString(stringFromAny(snapshot["membership_status"]), stringFromAny(snapshot["member_status"])))
	return role == "owner" && status == "active"
}

func inferGroupMessageContentType(message map[string]any) string {
	systemEvent, _ := message["system_event"].(map[string]any)
	subjectMethod := stringFromAny(systemEvent["subject_method"])
	switch subjectMethod {
	case "group.join", "group.add":
		return "group_system_member_joined"
	case "group.leave":
		return "group_system_member_left"
	case "group.remove":
		return "group_system_member_kicked"
	default:
		if systemEvent != nil {
			return "application/json"
		}
		return "text/plain"
	}
}

func groupStorageKey(groupDID string) string {
	return strings.TrimSpace(groupDID)
}

func parseInt64Ptr(value string) *int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func boolPtrFromAny(value any) *bool {
	switch typed := value.(type) {
	case bool:
		value := typed
		return &value
	case int:
		value := typed != 0
		return &value
	case int64:
		value := typed != 0
		return &value
	case float64:
		value := typed != 0
		return &value
	case string:
		if typed == "" {
			return nil
		}
		value := strings.EqualFold(typed, "true") || typed == "1"
		return &value
	default:
		return nil
	}
}

func compactWarnings(warnings []string) []string {
	if len(warnings) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(warnings))
	result := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		warning = strings.TrimSpace(warning)
		if warning == "" {
			continue
		}
		if _, ok := seen[warning]; ok {
			continue
		}
		seen[warning] = struct{}{}
		result = append(result, warning)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func encodeAnyString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func readGroupInboxFromCache(ctx context.Context, resolvedPathOpener func() (*sql.DB, error), ownerDID string, groupDID string, limit int, unreadOnly bool) ([]map[string]any, error) {
	finish := traceutil.LocalDBPhase(ctx, "read_group_inbox_cache")
	defer finish()
	db, err := resolvedPathOpener()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	return store.ListGroupInboxMessages(ctx, db, ownerDID, limit, groupStorageKey(groupDID), unreadOnly)
}

func (s *Service) readGroupInboxFromCache(ctx context.Context, record *identity.StoredIdentity, groupDID string, limit int, unreadOnly bool) ([]map[string]any, error) {
	return readGroupInboxFromCache(ctx, func() (*sql.DB, error) { return store.Open(s.resolved.Paths) }, record.DID, groupDID, limit, unreadOnly)
}

func readAllLocalGroupInbox(ctx context.Context, resolvedPathOpener func() (*sql.DB, error), ownerDID string, limit int, unreadOnly bool) ([]map[string]any, error) {
	finish := traceutil.LocalDBPhase(ctx, "read_all_group_inbox_cache")
	defer finish()
	db, err := resolvedPathOpener()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	return store.ListGroupInboxMessages(ctx, db, ownerDID, limit, "", unreadOnly)
}

func (s *Service) readAllLocalGroupInbox(ctx context.Context, record *identity.StoredIdentity, limit int, unreadOnly bool) ([]map[string]any, error) {
	return readAllLocalGroupInbox(ctx, func() (*sql.DB, error) { return store.Open(s.resolved.Paths) }, record.DID, limit, unreadOnly)
}

func mergeInboxMessages(limit int, left []map[string]any, right []map[string]any) []map[string]any {
	all := make([]map[string]any, 0, len(left)+len(right))
	all = append(all, left...)
	all = append(all, right...)
	if len(all) <= 1 {
		if limit > 0 && len(all) > limit {
			return all[:limit]
		}
		return all
	}
	sort.SliceStable(all, func(i, j int) bool {
		leftTS := defaultString(stringFromAny(all[i]["sent_at"]), stringFromAny(all[i]["stored_at"]))
		rightTS := defaultString(stringFromAny(all[j]["sent_at"]), stringFromAny(all[j]["stored_at"]))
		return leftTS > rightTS
	})
	if limit > 0 && len(all) > limit {
		return all[:limit]
	}
	return all
}

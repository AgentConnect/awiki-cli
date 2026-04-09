package message

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/store"
)

func (s *Service) sendDirectAttachment(ctx context.Context, request SendRequest) (*CommandResult, error) {
	if strings.TrimSpace(request.Target) == "" {
		return nil, ErrTargetRequired
	}
	if strings.TrimSpace(request.SecureMode) == "on" {
		return nil, ErrSecureNotSupported
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	targetDID, targetHandle, err := s.resolveTarget(ctx, request.Target)
	if err != nil {
		return nil, err
	}
	transport, warnings, err := s.httpTransport(record)
	if err != nil {
		return nil, err
	}
	if s.runtimeConfig().Mode == "websocket" {
		warnings = append(warnings, "Attachment messages use HTTP transport even when runtime.mode is websocket.")
	}
	prepared, slot, manifest, err := s.prepareAttachmentUpload(ctx, transport, "agent", targetDID, request)
	if err != nil {
		return nil, err
	}
	result, err := transport.SendDirectAttachment(ctx, targetDID, manifest)
	if err != nil {
		return nil, err
	}
	return s.persistDirectAttachmentSendResult(
		ctx,
		record,
		targetDID,
		targetHandle,
		request.Text,
		prepared,
		slot,
		manifest,
		result,
		warnings,
	)
}

func (s *Service) sendGroupAttachment(ctx context.Context, request SendRequest) (*CommandResult, error) {
	if strings.TrimSpace(request.Group) == "" {
		return nil, ErrGroupRequired
	}
	if strings.TrimSpace(request.SecureMode) == "on" {
		return nil, ErrSecureNotSupported
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	transport, warnings, err := s.httpTransport(record)
	if err != nil {
		return nil, err
	}
	if s.runtimeConfig().Mode == "websocket" {
		warnings = append(warnings, "Attachment messages use HTTP transport even when runtime.mode is websocket.")
	}
	prepared, slot, manifest, err := s.prepareAttachmentUpload(ctx, transport, "group", request.Group, request)
	if err != nil {
		return nil, err
	}
	result, err := transport.SendGroupAttachment(ctx, request.Group, manifest)
	if err != nil {
		return nil, err
	}
	return s.persistGroupAttachmentSendResult(
		ctx,
		record,
		request.Group,
		request.Text,
		prepared,
		slot,
		manifest,
		result,
		warnings,
	)
}

func (s *Service) prepareAttachmentUpload(
	ctx context.Context,
	transport *HTTPTransport,
	targetKind string,
	targetDID string,
	request SendRequest,
) (*preparedAttachment, *attachmentCreateSlotResult, map[string]any, error) {
	prepared, err := loadAttachmentFile(request.FilePath, request.MIMEType)
	if err != nil {
		return nil, nil, nil, err
	}
	slot, err := transport.CreateAttachmentSlot(ctx, targetKind, targetDID, prepared)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := transport.UploadAttachmentObject(ctx, slot.UploadURI, slot.UploadHeaders, prepared.Payload); err != nil {
		return nil, nil, nil, err
	}
	if _, err := transport.CommitAttachmentObject(ctx, slot.ControlServiceDID, prepared, slot); err != nil {
		return nil, nil, nil, err
	}
	return prepared, slot, buildAttachmentManifest(prepared, slot, request.Text), nil
}

func (s *Service) persistDirectAttachmentSendResult(
	ctx context.Context,
	record *identity.StoredIdentity,
	targetDID string,
	targetHandle string,
	caption string,
	prepared *preparedAttachment,
	slot *attachmentCreateSlotResult,
	manifest map[string]any,
	result *directSendResult,
	warnings []string,
) (*CommandResult, error) {
	db, err := store.Open(s.resolved.Paths)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := store.EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	if err := store.StoreMessage(ctx, db, store.MessageRecord{
		MsgID:       result.MessageID,
		OwnerDID:    record.DID,
		ThreadID:    store.MakeThreadID(record.DID, targetDID, ""),
		Direction:   1,
		SenderDID:   record.DID,
		ReceiverDID: targetDID,
		ContentType: attachmentManifestContentType,
		Content:     manifestContentString(manifest),
		SentAt:      result.AcceptedAt,
		IsRead:      true,
		Metadata: metadataString(map[string]any{
			"delivery_state":      result.DeliveryState,
			"operation_id":        result.OperationID,
			"target_handle":       targetHandle,
			"attachment_id":       slot.AttachmentID,
			"object_uri":          slot.ObjectURI,
			"control_service_did": slot.ControlServiceDID,
			"caption":             caption,
		}),
		CredentialName: record.IdentityName,
	}); err != nil {
		warnings = append(warnings, fmt.Sprintf("Failed to persist local message: %v", err))
	}
	return &CommandResult{
		Data: map[string]any{
			"action": "send_attachment",
			"target": map[string]any{
				"did":    targetDID,
				"handle": targetHandle,
				"kind":   "direct",
			},
			"message": map[string]any{
				"id":           result.MessageID,
				"type":         attachmentMessageType,
				"content_type": attachmentManifestContentType,
				"caption":      caption,
				"secure":       false,
				"sent_at":      result.AcceptedAt,
			},
			"attachment": map[string]any{
				"attachment_id":       slot.AttachmentID,
				"filename":            prepared.Filename,
				"mime_type":           prepared.MIMEType,
				"size":                prepared.SizeString,
				"digest":              map[string]any{"alg": "sha-256", "value_b64u": prepared.DigestB64U},
				"object_uri":          slot.ObjectURI,
				"control_service_did": slot.ControlServiceDID,
			},
			"delivery": result,
		},
		Summary:  "Sent a direct attachment message",
		Warnings: compactWarnings(warnings),
	}, nil
}

func (s *Service) persistGroupAttachmentSendResult(
	ctx context.Context,
	record *identity.StoredIdentity,
	groupDID string,
	caption string,
	prepared *preparedAttachment,
	slot *attachmentCreateSlotResult,
	manifest map[string]any,
	result *groupSendResult,
	warnings []string,
) (*CommandResult, error) {
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
	groupKey := groupStorageKey(groupDID)
	if err := store.StoreMessage(ctx, db, store.MessageRecord{
		MsgID:       msgID,
		OwnerDID:    record.DID,
		ThreadID:    store.MakeThreadID(record.DID, "", groupKey),
		Direction:   1,
		SenderDID:   record.DID,
		GroupID:     groupKey,
		GroupDID:    groupDID,
		ContentType: attachmentManifestContentType,
		Content:     manifestContentString(manifest),
		SentAt:      result.AcceptedAt,
		IsRead:      true,
		Metadata: metadataString(map[string]any{
			"group_event_seq":     result.GroupEventSeq,
			"group_state_version": result.GroupStateVersion,
			"operation_id":        result.OperationID,
			"attachment_id":       slot.AttachmentID,
			"object_uri":          slot.ObjectURI,
			"control_service_did": slot.ControlServiceDID,
			"caption":             caption,
		}),
		CredentialName: record.IdentityName,
	}); err != nil {
		warnings = append(warnings, fmt.Sprintf("Failed to persist local group message: %v", err))
	}
	warnings = append(
		warnings,
		s.touchCachedGroup(ctx, record, groupDID, result.AcceptedAt, result.GroupEventSeq, result.GroupStateVersion)...,
	)
	return &CommandResult{
		Data: map[string]any{
			"action": "send_attachment",
			"target": map[string]any{"kind": "group", "did": groupDID},
			"message": map[string]any{
				"id":           msgID,
				"type":         attachmentMessageType,
				"content_type": attachmentManifestContentType,
				"caption":      caption,
				"secure":       false,
				"sent_at":      result.AcceptedAt,
			},
			"attachment": map[string]any{
				"attachment_id":       slot.AttachmentID,
				"filename":            prepared.Filename,
				"mime_type":           prepared.MIMEType,
				"size":                prepared.SizeString,
				"digest":              map[string]any{"alg": "sha-256", "value_b64u": prepared.DigestB64U},
				"object_uri":          slot.ObjectURI,
				"control_service_did": slot.ControlServiceDID,
			},
			"delivery": result,
		},
		Summary:  "Sent a group attachment message",
		Warnings: compactWarnings(warnings),
	}, nil
}

func (s *Service) DownloadAttachment(ctx context.Context, request AttachmentDownloadRequest) (*CommandResult, error) {
	if strings.TrimSpace(request.MessageID) == "" {
		return nil, ErrMessageIDRequired
	}
	if strings.TrimSpace(request.OutputPath) == "" {
		return nil, ErrOutputPathRequired
	}
	if strings.TrimSpace(request.With) == "" && strings.TrimSpace(request.Group) == "" {
		return nil, ErrDownloadTargetNeeded
	}
	if strings.TrimSpace(request.With) != "" && strings.TrimSpace(request.Group) != "" {
		return nil, ErrDownloadTargetConflict
	}
	record, err := s.requireActiveIdentity(request.IdentityName)
	if err != nil {
		return nil, err
	}
	transport, warnings, err := s.httpTransport(record)
	if err != nil {
		return nil, err
	}
	if s.runtimeConfig().Mode == "websocket" {
		warnings = append(warnings, "Attachment downloads use HTTP transport even when runtime.mode is websocket.")
	}
	var (
		messages    []map[string]any
		messagePeer string
	)
	if strings.TrimSpace(request.Group) != "" {
		raw, fetchErr := transport.ListGroupMessages(ctx, GroupMessagesRequest{
			Group: request.Group,
			Limit: 100,
		})
		if fetchErr != nil {
			return nil, fetchErr
		}
		messages = messagesFromResult(raw["messages"])
	} else {
		peerDID, _, resolveErr := s.resolveTarget(ctx, request.With)
		if resolveErr != nil {
			return nil, resolveErr
		}
		messagePeer = peerDID
		raw, fetchErr := transport.GetHistory(ctx, HistoryRequest{
			With:  peerDID,
			Limit: 100,
		})
		if fetchErr != nil {
			return nil, fetchErr
		}
		messages = messagesFromResult(raw["messages"])
	}
	selection, err := findAttachmentSelection(messages, request.MessageID, request.AttachmentID)
	if err != nil {
		return nil, err
	}
	ticketParams, err := BuildAttachmentDownloadTicketRPCParams(
		record,
		nil,
		selection.ControlServiceDID,
		selection.MessageID,
		request.Group,
		selection,
	)
	if err != nil {
		return nil, err
	}
	ticket, err := transport.GetAttachmentDownloadTicket(ctx, ticketParams)
	if err != nil {
		return nil, err
	}
	payload, headers, err := transport.DownloadAttachmentObject(ctx, selection.ObjectURI, ticket.DownloadTicketB64U)
	if err != nil {
		return nil, err
	}
	outputPath := filepath.Clean(request.OutputPath)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(outputPath, payload, 0o600); err != nil {
		return nil, err
	}
	return &CommandResult{
		Data: map[string]any{
			"action":     "download_attachment",
			"message_id": selection.MessageID,
			"target": map[string]any{
				"kind": groupOrDirectKind(request.Group),
				"did":  groupOrDirectTarget(request.Group, messagePeer),
			},
			"attachment": map[string]any{
				"attachment_id":       selection.AttachmentID,
				"filename":            selection.Filename,
				"mime_type":           selection.MIMEType,
				"size":                selection.Size,
				"digest":              map[string]any{"alg": "sha-256", "value_b64u": selection.DigestB64U},
				"object_uri":          selection.ObjectURI,
				"control_service_did": selection.ControlServiceDID,
				"caption":             selection.Caption,
			},
			"output": map[string]any{
				"path":         outputPath,
				"size_bytes":   len(payload),
				"content_type": headers.Get("Content-Type"),
			},
		},
		Summary:  fmt.Sprintf("Downloaded attachment to %s", outputPath),
		Warnings: compactWarnings(warnings),
	}, nil
}

func groupOrDirectKind(groupDID string) string {
	if strings.TrimSpace(groupDID) != "" {
		return "group"
	}
	return "direct"
}

func groupOrDirectTarget(groupDID string, peerDID string) string {
	if strings.TrimSpace(groupDID) != "" {
		return groupDID
	}
	return peerDID
}

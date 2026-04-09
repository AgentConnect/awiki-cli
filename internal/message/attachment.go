package message

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	attachmentManifestContentType = "application/anp-attachment-manifest+json"
	attachmentMessageType         = "attachment_manifest"
)

type preparedAttachment struct {
	FilePath   string
	Filename   string
	MIMEType   string
	SizeBytes  int64
	SizeString string
	DigestB64U string
	Payload    []byte
}

type attachmentCreateSlotResult struct {
	AttachmentID      string            `json:"attachment_id"`
	SlotID            string            `json:"slot_id"`
	UploadURI         string            `json:"upload_uri"`
	UploadHeaders     map[string]string `json:"upload_headers"`
	ObjectURI         string            `json:"object_uri"`
	CommitToken       string            `json:"commit_token"`
	ExpiresAt         string            `json:"expires_at"`
	RequestServiceDID string            `json:"-"`
}

type attachmentCommitObjectResult struct {
	Committed    bool   `json:"committed"`
	AttachmentID string `json:"attachment_id"`
	ObjectURI    string `json:"object_uri"`
	CommittedAt  string `json:"committed_at"`
}

type attachmentDownloadTicketResult struct {
	DownloadTicketB64U string         `json:"download_ticket_b64u"`
	ExpiresAt          string         `json:"expires_at"`
	TicketBinding      map[string]any `json:"ticket_binding"`
}

type attachmentSelection struct {
	MessageID    string
	RequestedID  string
	SenderDID    string
	AttachmentID string
	Filename     string
	MIMEType     string
	Size         string
	DigestB64U   string
	ObjectURI    string
	Caption      string
}

func loadAttachmentFile(filePath string, mimeOverride string) (*preparedAttachment, error) {
	path := strings.TrimSpace(filePath)
	if path == "" {
		return nil, ErrFilePathRequired
	}
	payload, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	filename := filepath.Base(path)
	if filename == "." || filename == string(filepath.Separator) {
		return nil, fmt.Errorf("attachment filename could not be derived from %q", path)
	}
	mimeType := strings.TrimSpace(mimeOverride)
	if mimeType == "" {
		mimeType = detectAttachmentMIMEType(filename, payload)
	}
	sum := sha256.Sum256(payload)
	return &preparedAttachment{
		FilePath:   path,
		Filename:   filename,
		MIMEType:   mimeType,
		SizeBytes:  int64(len(payload)),
		SizeString: fmt.Sprintf("%d", len(payload)),
		DigestB64U: base64.RawURLEncoding.EncodeToString(sum[:]),
		Payload:    payload,
	}, nil
}

func detectAttachmentMIMEType(filename string, payload []byte) string {
	if guessed := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename))); guessed != "" {
		return guessed
	}
	if len(payload) == 0 {
		return "application/octet-stream"
	}
	sniffLength := len(payload)
	if sniffLength > 512 {
		sniffLength = 512
	}
	return http.DetectContentType(payload[:sniffLength])
}

func buildAttachmentManifest(prepared *preparedAttachment, slot *attachmentCreateSlotResult, caption string) map[string]any {
	manifest := map[string]any{
		"attachments": []map[string]any{{
			"attachment_id": slot.AttachmentID,
			"filename":      prepared.Filename,
			"mime_type":     prepared.MIMEType,
			"size":          prepared.SizeString,
			"digest": map[string]any{
				"alg":        "sha-256",
				"value_b64u": prepared.DigestB64U,
			},
			"access_info": map[string]any{
				"object_uri": slot.ObjectURI,
			},
			"encryption_info": map[string]any{
				"mode": "none",
			},
		}},
		"primary_attachment_id": slot.AttachmentID,
	}
	if strings.TrimSpace(caption) != "" {
		manifest["caption"] = caption
	}
	return manifest
}

func manifestContentString(manifest map[string]any) string {
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func findAttachmentSelection(messages []map[string]any, requestedMessageID string, requestedAttachmentID string) (*attachmentSelection, error) {
	for _, message := range messages {
		viewID := stringFromAny(message["id"])
		rawMessageID := stringFromAny(message["message_id"])
		actualMessageID := rawMessageID
		if actualMessageID == "" {
			actualMessageID = viewID
		}
		if requestedMessageID != viewID && requestedMessageID != actualMessageID {
			continue
		}
		content, err := decodeAttachmentContent(message["content"])
		if err != nil {
			return nil, err
		}
		attachments := attachmentsFromContent(content["attachments"])
		if len(attachments) == 0 {
			return nil, ErrAttachmentMessageInvalid
		}
		selected, err := selectAttachmentEntry(attachments, requestedAttachmentID)
		if err != nil {
			return nil, err
		}
		accessInfo, ok := selected["access_info"].(map[string]any)
		if !ok {
			return nil, ErrAttachmentMessageInvalid
		}
		digest, _ := selected["digest"].(map[string]any)
		return &attachmentSelection{
			MessageID:    actualMessageID,
			RequestedID:  viewID,
			SenderDID:    stringFromAny(message["sender_did"]),
			AttachmentID: stringFromAny(selected["attachment_id"]),
			Filename:     stringFromAny(selected["filename"]),
			MIMEType:     stringFromAny(selected["mime_type"]),
			Size:         stringFromAny(selected["size"]),
			DigestB64U:   stringFromAny(digest["value_b64u"]),
			ObjectURI:    stringFromAny(accessInfo["object_uri"]),
			Caption:      stringFromAny(content["caption"]),
		}, nil
	}
	return nil, ErrMessageNotFound
}

func decodeAttachmentContent(value any) (map[string]any, error) {
	content, err := mapFromAny(value)
	if err != nil {
		return nil, ErrAttachmentMessageInvalid
	}
	if len(content) == 0 {
		return nil, ErrAttachmentMessageInvalid
	}
	return content, nil
}

func mapFromAny(value any) (map[string]any, error) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, nil
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, ErrAttachmentMessageInvalid
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(typed), &decoded); err != nil {
			return nil, err
		}
		return decoded, nil
	default:
		return nil, ErrAttachmentMessageInvalid
	}
}

func attachmentsFromContent(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	attachments := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if attachment, ok := item.(map[string]any); ok {
			attachments = append(attachments, attachment)
		}
	}
	return attachments
}

func selectAttachmentEntry(attachments []map[string]any, requestedAttachmentID string) (map[string]any, error) {
	if len(attachments) == 0 {
		return nil, ErrAttachmentNotFound
	}
	if strings.TrimSpace(requestedAttachmentID) == "" {
		if len(attachments) > 1 {
			return nil, ErrAttachmentIDRequired
		}
		return attachments[0], nil
	}
	for _, attachment := range attachments {
		if stringFromAny(attachment["attachment_id"]) == requestedAttachmentID {
			return attachment, nil
		}
	}
	return nil, ErrAttachmentNotFound
}

func (t *HTTPTransport) CreateAttachmentSlot(
	ctx context.Context,
	targetKind string,
	targetDID string,
	prepared *preparedAttachment,
) (*attachmentCreateSlotResult, error) {
	serviceDID, err := t.GetMessageServiceDID(ctx)
	if err != nil {
		return nil, err
	}
	params, err := BuildAttachmentCreateSlotRPCParams(
		t.auth.record,
		nil,
		serviceDID,
		targetKind,
		targetDID,
		prepared,
	)
	if err != nil {
		return nil, err
	}
	var result attachmentCreateSlotResult
	if err := t.rpcCall(ctx, "attachment.create_slot", params, &result); err != nil {
		return nil, err
	}
	result.RequestServiceDID = serviceDID
	return &result, nil
}

func (t *HTTPTransport) UploadAttachmentObject(
	ctx context.Context,
	uploadURI string,
	headers map[string]string,
	payload []byte,
) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURI, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := t.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		raw, _ := io.ReadAll(response.Body)
		return &ServiceError{StatusCode: response.StatusCode, Message: strings.TrimSpace(string(raw))}
	}
	return nil
}

func (t *HTTPTransport) CommitAttachmentObject(
	ctx context.Context,
	serviceDID string,
	prepared *preparedAttachment,
	slot *attachmentCreateSlotResult,
) (*attachmentCommitObjectResult, error) {
	params, err := BuildAttachmentCommitObjectRPCParams(
		t.auth.record,
		nil,
		serviceDID,
		prepared,
		slot,
	)
	if err != nil {
		return nil, err
	}
	var result attachmentCommitObjectResult
	if err := t.rpcCall(ctx, "attachment.commit_object", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (t *HTTPTransport) SendDirectAttachment(
	ctx context.Context,
	targetDID string,
	manifest map[string]any,
) (*directSendResult, error) {
	params, err := BuildDirectAttachmentSendRPCParams(t.auth.record, nil, targetDID, manifest)
	if err != nil {
		return nil, err
	}
	var result directSendResult
	if err := t.rpcCall(ctx, "direct.send", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (t *HTTPTransport) SendGroupAttachment(
	ctx context.Context,
	groupDID string,
	manifest map[string]any,
) (*groupSendResult, error) {
	params, err := BuildGroupAttachmentSendRPCParams(t.auth.record, nil, groupDID, manifest)
	if err != nil {
		return nil, err
	}
	var result groupSendResult
	if err := t.rpcCall(ctx, "group.send", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (t *HTTPTransport) GetAttachmentDownloadTicket(
	ctx context.Context,
	params map[string]any,
) (*attachmentDownloadTicketResult, error) {
	var result attachmentDownloadTicketResult
	if err := t.rpcCall(ctx, "attachment.get_download_ticket", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (t *HTTPTransport) DownloadAttachmentObject(
	ctx context.Context,
	objectURI string,
	downloadTicket string,
) ([]byte, http.Header, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, objectURI, nil)
	if err != nil {
		return nil, nil, err
	}
	request.Header.Set("Authorization", "Bearer "+downloadTicket)
	response, err := t.httpClient.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, nil, err
	}
	if response.StatusCode >= http.StatusBadRequest {
		return nil, response.Header.Clone(), &ServiceError{
			StatusCode: response.StatusCode,
			Message:    strings.TrimSpace(string(raw)),
		}
	}
	return raw, response.Header.Clone(), nil
}

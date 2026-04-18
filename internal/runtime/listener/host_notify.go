package listener

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	runtimecfg "github.com/agentconnect/awiki-cli/internal/runtime"
	"github.com/agentconnect/awiki-cli/internal/runtime/openclawnotify"
)

const hostNotificationVersion = "1.0"

// HostNotificationEvent is the normalized event delivered from awiki-cli to a
// host-facing notification sink.
type HostNotificationEvent struct {
	Version    string `json:"version"`
	ID         string `json:"id"`
	Topic      string `json:"topic"`
	ReceivedAt string `json:"received_at"`
	Data       any    `json:"data,omitempty"`
}

// DirectMessageNotificationData is the minimal direct-message payload exposed
// to host integrations.
type DirectMessageNotificationData struct {
	Channel         string `json:"channel"`
	MessageID       string `json:"message_id"`
	OperationID     string `json:"operation_id,omitempty"`
	ConversationID  string `json:"conversation_id,omitempty"`
	SenderHandle    string `json:"sender_handle,omitempty"`
	SenderDID       string `json:"sender_did"`
	RecipientHandle string `json:"recipient_handle,omitempty"`
	RecipientDID    string `json:"recipient_did"`
	Profile         string `json:"profile,omitempty"`
	SecurityProfile string `json:"security_profile,omitempty"`
	ContentType     string `json:"content_type"`
	Text            string `json:"text,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
}

// GroupMessageNotificationData is the minimal group-message payload exposed to
// host integrations.
type GroupMessageNotificationData struct {
	Channel           string `json:"channel"`
	MessageID         string `json:"message_id"`
	OperationID       string `json:"operation_id,omitempty"`
	GroupDID          string `json:"group_did"`
	SenderHandle      string `json:"sender_handle,omitempty"`
	SenderDID         string `json:"sender_did"`
	RecipientHandle   string `json:"recipient_handle,omitempty"`
	RecipientDID      string `json:"recipient_did"`
	Profile           string `json:"profile,omitempty"`
	SecurityProfile   string `json:"security_profile,omitempty"`
	ContentType       string `json:"content_type"`
	Text              string `json:"text,omitempty"`
	GroupStateVersion string `json:"group_state_version,omitempty"`
	GroupEventSeq     string `json:"group_event_seq,omitempty"`
	AcceptedAt        string `json:"accepted_at,omitempty"`
}

// GroupStateChangedNotificationData is the minimal group-state payload exposed
// to host integrations.
type GroupStateChangedNotificationData struct {
	Channel           string `json:"channel"`
	EventID           string `json:"event_id"`
	EventType         string `json:"event_type,omitempty"`
	GroupDID          string `json:"group_did"`
	RecipientDID      string `json:"recipient_did"`
	ActorDID          string `json:"actor_did,omitempty"`
	SubjectDID        string `json:"subject_did,omitempty"`
	SubjectMethod     string `json:"subject_method,omitempty"`
	MembershipStatus  string `json:"membership_status,omitempty"`
	GroupStateVersion string `json:"group_state_version,omitempty"`
	GroupEventSeq     string `json:"group_event_seq,omitempty"`
	ChangedAt         string `json:"changed_at,omitempty"`
}

// HostNotifySink receives normalized host notification events.
type HostNotifySink interface {
	Notify(context.Context, HostNotificationEvent) error
	Close() error
}

type noopHostNotifySink struct{}

func (s *noopHostNotifySink) Notify(context.Context, HostNotificationEvent) error {
	return nil
}

func (s *noopHostNotifySink) Close() error {
	return nil
}

type logHostNotifySink struct{}

func (s *logHostNotifySink) Notify(_ context.Context, event HostNotificationEvent) error {
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	log.Printf("host notification %s", string(raw))
	return nil
}

func (s *logHostNotifySink) Close() error {
	return nil
}

type fileHostNotifySink struct {
	mu   sync.Mutex
	file *os.File
}

func newFileHostNotifySink(path string) (*fileHostNotifySink, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("host notify file sink requires a file path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create host notify sink dir: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open host notify sink file: %w", err)
	}
	return &fileHostNotifySink{file: file}, nil
}

func (s *fileHostNotifySink) Notify(_ context.Context, event HostNotificationEvent) error {
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.file.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("write host notify event: %w", err)
	}
	return s.file.Sync()
}

func (s *fileHostNotifySink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}

func newHostNotifySink(resolved *appconfig.Resolved) (HostNotifySink, HostNotifyStatus, error) {
	config := runtimecfg.Resolve(resolved).HostNotify
	status := HostNotifyStatus{
		Enabled:   config.Enabled,
		Sink:      config.Sink,
		FilePath:  config.FilePath,
		HookURL:   config.OpenClaw.HookURL,
		AgentID:   config.OpenClaw.AgentID,
		HookName:  config.OpenClaw.HookName,
		NotifyURL: config.Hermes.NotifyURL,
	}
	if !config.Enabled {
		return &noopHostNotifySink{}, status, nil
	}
	switch config.Sink {
	case "noop":
		return &noopHostNotifySink{}, status, nil
	case "log":
		return &logHostNotifySink{}, status, nil
	case "file":
		sink, err := newFileHostNotifySink(config.FilePath)
		if err != nil {
			return nil, status, err
		}
		return sink, status, nil
	case "openclaw":
		settings, err := openclawnotify.ResolveSettings(resolved)
		if err != nil {
			return nil, status, err
		}
		status.HookURL = settings.HookURL
		sink, err := newOpenClawHostNotifySink(resolved)
		if err != nil {
			return nil, status, err
		}
		return sink, status, nil
	case "hermes", "webhook":
		sink, err := newHermesHostNotifySink(resolved, config.Hermes)
		if err != nil {
			return nil, status, err
		}
		return sink, status, nil
	default:
		return nil, status, fmt.Errorf("unsupported host notify sink %q", config.Sink)
	}
}

// NormalizeHostNotification converts a websocket notification into a compact
// host-facing event.
func NormalizeHostNotification(notification map[string]any, receivedAt time.Time) (*HostNotificationEvent, bool) {
	receivedAt = normalizeReceivedAt(receivedAt)
	switch stringValue(notification["method"]) {
	case "direct.incoming":
		return normalizeDirectIncoming(notification, receivedAt)
	case "group.incoming":
		return normalizeGroupIncoming(notification, receivedAt)
	case "group.state_changed":
		return normalizeGroupStateChanged(notification, receivedAt)
	default:
		return nil, false
	}
}

func ApplyHostNotificationHandles(event *HostNotificationEvent, senderHandle string, recipientHandle string) {
	if event == nil {
		return
	}
	switch data := event.Data.(type) {
	case DirectMessageNotificationData:
		data.SenderHandle = fallbackString(strings.TrimSpace(senderHandle), data.SenderHandle)
		data.RecipientHandle = fallbackString(strings.TrimSpace(recipientHandle), data.RecipientHandle)
		event.Data = data
	case GroupMessageNotificationData:
		data.SenderHandle = fallbackString(strings.TrimSpace(senderHandle), data.SenderHandle)
		data.RecipientHandle = fallbackString(strings.TrimSpace(recipientHandle), data.RecipientHandle)
		event.Data = data
	}
}

func normalizeDirectIncoming(notification map[string]any, receivedAt time.Time) (*HostNotificationEvent, bool) {
	params := mapValue(notification["params"])
	meta := mapValue(params["meta"])
	body := mapValue(params["body"])
	target := mapValue(meta["target"])
	recipientDID := stringValue(target["did"])
	senderDID := stringValue(meta["sender_did"])
	if recipientDID == "" || senderDID == "" {
		return nil, false
	}
	messageID := resolveDirectMessageID(meta, notification)
	data := DirectMessageNotificationData{
		Channel:         "direct",
		MessageID:       messageID,
		OperationID:     stringValue(meta["operation_id"]),
		ConversationID:  stringValue(body["conversation_id"]),
		SenderDID:       senderDID,
		RecipientDID:    recipientDID,
		Profile:         stringValue(meta["profile"]),
		SecurityProfile: stringValue(meta["security_profile"]),
		ContentType:     fallbackString(stringValue(meta["content_type"]), "text/plain"),
		Text:            stringValue(body["text"]),
		CreatedAt:       stringValue(meta["created_at"]),
	}
	return &HostNotificationEvent{
		Version:    hostNotificationVersion,
		ID:         messageID,
		Topic:      "im.message.received",
		ReceivedAt: receivedAt.Format(time.RFC3339),
		Data:       data,
	}, true
}

func normalizeGroupIncoming(notification map[string]any, receivedAt time.Time) (*HostNotificationEvent, bool) {
	params := mapValue(notification["params"])
	meta := mapValue(params["meta"])
	body := mapValue(params["body"])
	target := mapValue(meta["target"])
	recipientDID := stringValue(target["did"])
	groupDID := stringValue(body["group_did"])
	senderDID := stringValue(meta["sender_did"])
	if recipientDID == "" || groupDID == "" || senderDID == "" {
		return nil, false
	}
	messageID := resolveGroupMessageID(meta, body, notification)
	data := GroupMessageNotificationData{
		Channel:           "group",
		MessageID:         messageID,
		OperationID:       stringValue(meta["operation_id"]),
		GroupDID:          groupDID,
		SenderDID:         senderDID,
		RecipientDID:      recipientDID,
		Profile:           stringValue(meta["profile"]),
		SecurityProfile:   stringValue(meta["security_profile"]),
		ContentType:       fallbackString(stringValue(meta["content_type"]), "text/plain"),
		Text:              stringValue(body["text"]),
		GroupStateVersion: stringLikeValue(body["group_state_version"]),
		GroupEventSeq:     stringLikeValue(body["group_event_seq"]),
		AcceptedAt:        stringValue(body["accepted_at"]),
	}
	return &HostNotificationEvent{
		Version:    hostNotificationVersion,
		ID:         messageID,
		Topic:      "im.group.message.received",
		ReceivedAt: receivedAt.Format(time.RFC3339),
		Data:       data,
	}, true
}

func normalizeGroupStateChanged(notification map[string]any, receivedAt time.Time) (*HostNotificationEvent, bool) {
	params := mapValue(notification["params"])
	meta := mapValue(params["meta"])
	body := mapValue(params["body"])
	target := mapValue(meta["target"])
	recipientDID := stringValue(target["did"])
	groupDID := stringValue(body["group_did"])
	if recipientDID == "" || groupDID == "" {
		return nil, false
	}
	eventID := resolveGroupStateEventID(meta, body, notification)
	data := GroupStateChangedNotificationData{
		Channel:           "group",
		EventID:           eventID,
		EventType:         fallbackString(stringValue(body["event_type"]), inferGroupStateEventType(body)),
		GroupDID:          groupDID,
		RecipientDID:      recipientDID,
		ActorDID:          stringValue(body["actor_did"]),
		SubjectDID:        stringValue(body["subject_did"]),
		SubjectMethod:     stringValue(body["subject_method"]),
		MembershipStatus:  stringValue(body["membership_status"]),
		GroupStateVersion: stringLikeValue(body["group_state_version"]),
		GroupEventSeq:     stringLikeValue(body["group_event_seq"]),
		ChangedAt:         stringValue(body["changed_at"]),
	}
	return &HostNotificationEvent{
		Version:    hostNotificationVersion,
		ID:         eventID,
		Topic:      "im.group.state.changed",
		ReceivedAt: receivedAt.Format(time.RFC3339),
		Data:       data,
	}, true
}

func normalizeReceivedAt(receivedAt time.Time) time.Time {
	if receivedAt.IsZero() {
		return time.Now().UTC()
	}
	return receivedAt.UTC()
}

func resolveDirectMessageID(meta map[string]any, notification map[string]any) string {
	messageID := stringValue(meta["message_id"])
	if messageID != "" {
		return messageID
	}
	operationID := stringValue(meta["operation_id"])
	if operationID != "" {
		return operationID
	}
	return generatedHostNotificationID(notification)
}

func resolveGroupMessageID(meta map[string]any, body map[string]any, notification map[string]any) string {
	messageID := stringValue(meta["message_id"])
	if messageID != "" {
		return messageID
	}
	groupDID := stringValue(body["group_did"])
	groupEventSeq := stringLikeValue(body["group_event_seq"])
	if groupDID != "" && groupEventSeq != "" {
		return fmt.Sprintf("%s:%s", groupDID, groupEventSeq)
	}
	operationID := stringValue(meta["operation_id"])
	if operationID != "" {
		return operationID
	}
	return generatedHostNotificationID(notification)
}

func resolveGroupStateEventID(meta map[string]any, body map[string]any, notification map[string]any) string {
	eventID := stringValue(body["event_id"])
	if eventID != "" {
		return eventID
	}
	groupDID := stringValue(body["group_did"])
	groupEventSeq := stringLikeValue(body["group_event_seq"])
	if groupDID != "" && groupEventSeq != "" {
		return fmt.Sprintf("%s:%s", groupDID, groupEventSeq)
	}
	operationID := stringValue(meta["operation_id"])
	if operationID != "" {
		return operationID
	}
	return generatedHostNotificationID(notification)
}

func inferGroupStateEventType(body map[string]any) string {
	subjectMethod := stringValue(body["subject_method"])
	membershipStatus := stringValue(body["membership_status"])
	switch membershipStatus {
	case "active", "activated":
		return "member-activated"
	case "removed":
		return "member-removed"
	case "left":
		return "member-left"
	}
	switch subjectMethod {
	case "group.add":
		return "member-activated"
	case "group.remove":
		return "member-removed"
	case "group.leave":
		return "member-left"
	case "group.update_profile":
		return "group-profile-updated"
	case "group.update_policy":
		return "group-policy-updated"
	default:
		return ""
	}
}

func generatedHostNotificationID(notification map[string]any) string {
	raw, err := json.Marshal(notification)
	if err != nil {
		raw = []byte(fmt.Sprintf("%v", notification))
	}
	sum := sha256.Sum256(raw)
	return "hostevt-" + hex.EncodeToString(sum[:8])
}

func mapValue(value any) map[string]any {
	mapped, _ := value.(map[string]any)
	return mapped
}

func stringLikeValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case int:
		return fmt.Sprintf("%d", typed)
	case int64:
		return fmt.Sprintf("%d", typed)
	case float64:
		return fmt.Sprintf("%.0f", typed)
	default:
		return ""
	}
}

package listener

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agentconnect/awiki-cli/internal/authsdk"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
	"github.com/agentconnect/awiki-cli/internal/message"
	"github.com/agentconnect/awiki-cli/internal/runtime"
	"github.com/agentconnect/awiki-cli/internal/store"
)

type Supervisor struct {
	resolved *appconfig.Resolved
	manager  *identity.Manager
	statusMu sync.Mutex
	status   Status

	sessionsMu sync.Mutex
	sessions   map[string]*session
	listener   net.Listener
	db         *sql.DB
}

type session struct {
	identityName string
	record       *identity.StoredIdentity
	client       *WSClient
	lastError    string
	connected    bool
	ctx          context.Context
	cancelFunc   context.CancelFunc
	initResult   chan error
	initOnce     sync.Once
	mu           sync.RWMutex
}

const (
	sessionReconnectBaseDelay = time.Second
	sessionReconnectMaxDelay  = 30 * time.Second
	sessionPingInterval       = 60 * time.Second
)

func NewSupervisor(resolved *appconfig.Resolved) (*Supervisor, error) {
	db, err := store.Open(resolved.Paths)
	if err != nil {
		return nil, err
	}
	if err := store.EnsureSchema(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	pidFile, logFile, statusFile, socketPath, err := paths(resolved)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Supervisor{
		resolved: resolved,
		manager:  identity.NewManager(resolved.Paths),
		status: Status{
			Mode:       runtime.Resolve(resolved).Mode,
			PIDFile:    pidFile,
			LogFile:    logFile,
			StatusFile: statusFile,
			SocketPath: socketPath,
			Running:    true,
			PID:        os.Getpid(),
			StartedAt:  time.Now().UTC().Format(time.RFC3339),
		},
		sessions: map[string]*session{},
		db:       db,
	}, nil
}

func (s *Supervisor) Close() error {
	s.sessionsMu.Lock()
	for _, session := range s.sessions {
		if session.cancelFunc != nil {
			session.cancelFunc()
		}
		session.closeCurrentClient()
	}
	s.sessionsMu.Unlock()
	if s.listener != nil {
		_ = s.listener.Close()
	}
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *Supervisor) Run(ctx context.Context) error {
	if runtime.Resolve(s.resolved).Mode != runtime.ModeWebSocket {
		return fmt.Errorf("runtime mode must be websocket before starting the listener")
	}
	if err := writePID(s.status.PIDFile, s.status.PID); err != nil {
		return err
	}
	if err := s.writeStatus(); err != nil {
		return err
	}
	if err := s.startSocket(); err != nil {
		return err
	}
	if err := s.startKnownSessions(ctx); err != nil {
		return err
	}
	go s.watchNewIdentities(ctx)
	<-ctx.Done()
	return nil
}

func (s *Supervisor) startSocket() error {
	if err := os.MkdirAll(filepath.Dir(s.status.SocketPath), 0o700); err != nil {
		return err
	}
	_ = os.Remove(s.status.SocketPath)
	listener, err := net.Listen("unix", s.status.SocketPath)
	if err != nil {
		return err
	}
	s.listener = listener
	go s.acceptLoop()
	return nil
}

func (s *Supervisor) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *Supervisor) handleConn(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		_ = json.NewEncoder(conn).Encode(runtime.BridgeResponse{OK: false, Error: &runtime.BridgeError{Message: err.Error()}})
		return
	}
	var request runtime.BridgeRequest
	if err := json.Unmarshal(line, &request); err != nil {
		_ = json.NewEncoder(conn).Encode(runtime.BridgeResponse{OK: false, Error: &runtime.BridgeError{Message: err.Error()}})
		return
	}
	result, err := s.handleBridgeRequest(request)
	if err != nil {
		_ = json.NewEncoder(conn).Encode(runtime.BridgeResponse{OK: false, Error: &runtime.BridgeError{Message: err.Error()}})
		return
	}
	_ = json.NewEncoder(conn).Encode(runtime.BridgeResponse{OK: true, Result: result})
}

func (s *Supervisor) handleBridgeRequest(request runtime.BridgeRequest) (map[string]any, error) {
	session, err := s.ensureSession(request.IdentityName)
	if err != nil {
		return nil, err
	}
	record := session.currentRecord()
	client := session.currentClient()
	if record == nil || client == nil {
		return nil, fmt.Errorf("websocket session is not connected for identity %s", session.identityName)
	}
	switch request.Method {
	case "direct.send":
		target, _ := request.Params["target"].(string)
		text, _ := request.Params["text"].(string)
		msgType, _ := request.Params["type"].(string)
		params, err := message.BuildDirectSendRPCParams(record, s.manager, target, text, msgType)
		if err != nil {
			return nil, err
		}
		return client.SendRPC(context.Background(), "direct.send", params)
	case "inbox.get":
		params := message.BuildInboxRPCParams(record, message.InboxRequest{
			Limit:      intValue(request.Params["limit"]),
			With:       stringValue(request.Params["with"]),
			UnreadOnly: boolValue(request.Params["unread"]),
			MarkRead:   boolValue(request.Params["mark_read"]),
		})
		return client.SendRPC(context.Background(), "inbox.get", params)
	case "direct.get_history":
		params, err := message.BuildHistoryRPCParams(record, message.HistoryRequest{
			With:   stringValue(request.Params["with"]),
			Limit:  intValue(request.Params["limit"]),
			Cursor: stringValue(request.Params["cursor"]),
		})
		if err != nil {
			return nil, err
		}
		return client.SendRPC(context.Background(), "direct.get_history", params)
	case "inbox.mark_read":
		rawIDs, _ := request.Params["message_ids"].([]any)
		messageIDs := make([]string, 0, len(rawIDs))
		for _, rawID := range rawIDs {
			if id := stringValue(rawID); id != "" {
				messageIDs = append(messageIDs, id)
			}
		}
		params, err := message.BuildMarkReadRPCParams(record, message.MarkReadRequest{MessageIDs: messageIDs})
		if err != nil {
			return nil, err
		}
		result, err := client.SendRPC(context.Background(), "inbox.mark_read", params)
		if err == nil {
			_, _ = store.MarkMessagesRead(context.Background(), s.db, record.DID, messageIDs)
		}
		return result, err
	case "group.create":
		serviceDID, err := s.fetchMessageServiceDID(session)
		if err != nil {
			return nil, err
		}
		params, err := message.BuildGroupCreateRPCParams(record, s.manager, serviceDID, message.GroupCreateRequest{
			Name:                stringValue(request.Params["name"]),
			Description:         stringValue(request.Params["description"]),
			Discoverability:     stringValue(request.Params["discoverability"]),
			AdmissionMode:       stringValue(request.Params["admission_mode"]),
			Slug:                stringValue(request.Params["slug"]),
			Goal:                stringValue(request.Params["goal"]),
			Rules:               stringValue(request.Params["rules"]),
			MessagePrompt:       stringValue(request.Params["message_prompt"]),
			DocURL:              stringValue(request.Params["doc_url"]),
			AttachmentsAllowed:  boolPtrValue(request.Params["attachments_allowed"]),
			MaxMembers:          stringValue(request.Params["max_members"]),
			MemberMaxMessages:   int64PtrValue(request.Params["member_max_messages"]),
			MemberMaxTotalChars: int64PtrValue(request.Params["member_max_total_chars"]),
		})
		if err != nil {
			return nil, err
		}
		return client.SendRPC(context.Background(), "group.create", params)
	case "group.get_info":
		params, err := message.BuildGroupGetInfoRPCParams(record, message.GroupInfoRequest{
			Group:             stringValue(request.Params["group"]),
			IncludePolicy:     boolValue(request.Params["include_policy"]),
			IncludeMemberList: boolValue(request.Params["include_member_list"]),
		})
		if err != nil {
			return nil, err
		}
		return client.SendRPC(context.Background(), "group.get_info", params)
	case "group.join":
		params, err := message.BuildGroupJoinRPCParams(record, s.manager, message.GroupJoinRequest{
			Group:      stringValue(request.Params["group"]),
			ReasonText: stringValue(request.Params["reason_text"]),
		})
		if err != nil {
			return nil, err
		}
		return client.SendRPC(context.Background(), "group.join", params)
	case "group.add":
		params, err := message.BuildGroupAddRPCParams(record, s.manager, message.GroupMemberRequest{
			Group:      stringValue(request.Params["group"]),
			Member:     stringValue(request.Params["member"]),
			Role:       stringValue(request.Params["role"]),
			ReasonText: stringValue(request.Params["reason_text"]),
		})
		if err != nil {
			return nil, err
		}
		return client.SendRPC(context.Background(), "group.add", params)
	case "group.remove":
		params, err := message.BuildGroupRemoveRPCParams(record, s.manager, message.GroupMemberRequest{
			Group:      stringValue(request.Params["group"]),
			Member:     stringValue(request.Params["member"]),
			ReasonText: stringValue(request.Params["reason_text"]),
		})
		if err != nil {
			return nil, err
		}
		return client.SendRPC(context.Background(), "group.remove", params)
	case "group.leave":
		params, err := message.BuildGroupLeaveRPCParams(record, s.manager, message.GroupLeaveRequest{Group: stringValue(request.Params["group"])})
		if err != nil {
			return nil, err
		}
		return client.SendRPC(context.Background(), "group.leave", params)
	case "group.update_profile":
		patch, _ := request.Params["patch"].(map[string]any)
		params, err := message.BuildGroupUpdateProfileRPCParams(record, s.manager, stringValue(request.Params["group"]), patch)
		if err != nil {
			return nil, err
		}
		return client.SendRPC(context.Background(), "group.update_profile", params)
	case "group.update_policy":
		patch, _ := request.Params["patch"].(map[string]any)
		params, err := message.BuildGroupUpdatePolicyRPCParams(record, s.manager, stringValue(request.Params["group"]), patch)
		if err != nil {
			return nil, err
		}
		return client.SendRPC(context.Background(), "group.update_policy", params)
	case "group.send":
		params, err := message.BuildGroupSendRPCParams(record, s.manager, stringValue(request.Params["group"]), stringValue(request.Params["text"]), stringValue(request.Params["type"]))
		if err != nil {
			return nil, err
		}
		return client.SendRPC(context.Background(), "group.send", params)
	case "group.get":
		params, err := message.BuildGroupGetRPCParams(record, message.GroupGetRequest{Group: stringValue(request.Params["group"])})
		if err != nil {
			return nil, err
		}
		return client.SendRPC(context.Background(), "group.get", params)
	case "group.list_members":
		params, err := message.BuildGroupMembersRPCParams(record, message.GroupMembersRequest{Group: stringValue(request.Params["group"]), Limit: intValue(request.Params["limit"])})
		if err != nil {
			return nil, err
		}
		return client.SendRPC(context.Background(), "group.list_members", params)
	case "group.list_messages":
		params, err := message.BuildGroupMessagesRPCParams(record, message.GroupMessagesRequest{Group: stringValue(request.Params["group"]), Limit: intValue(request.Params["limit"]), Cursor: stringValue(request.Params["cursor"])})
		if err != nil {
			return nil, err
		}
		return client.SendRPC(context.Background(), "group.list_messages", params)
	default:
		return nil, fmt.Errorf("unsupported websocket bridge method: %s", request.Method)
	}
}

func (s *Supervisor) ensureSession(identityName string) (*session, error) {
	identityName = strings.TrimSpace(identityName)
	if identityName == "" {
		current, err := s.manager.Current()
		if err != nil {
			return nil, err
		}
		identityName = current.IdentityName
	}
	s.sessionsMu.Lock()
	existing := s.sessions[identityName]
	if existing != nil {
		s.sessionsMu.Unlock()
		return existing, nil
	}
	sessionCtx, sessionCancel := context.WithCancel(context.Background())
	newSession := &session{
		identityName: identityName,
		ctx:          sessionCtx,
		cancelFunc:   sessionCancel,
		initResult:   make(chan error, 1),
	}
	s.sessions[identityName] = newSession
	s.sessionsMu.Unlock()
	go s.runSessionLoop(newSession)
	select {
	case err := <-newSession.initResult:
		if err != nil {
			return newSession, err
		}
		return newSession, nil
	case <-time.After(15 * time.Second):
		return newSession, fmt.Errorf("websocket session bootstrap timed out for identity %s", identityName)
	}
}

func (s *Supervisor) startKnownSessions(ctx context.Context) error {
	identities, err := s.manager.List()
	if err != nil {
		return err
	}
	for _, summary := range identities {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if _, err := s.ensureSession(summary.IdentityName); err != nil {
			s.recordSessionError(summary.IdentityName, summary.DID, err)
		}
	}
	s.refreshStatus()
	return nil
}

func (s *Supervisor) watchNewIdentities(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			identities, err := s.manager.List()
			if err != nil {
				continue
			}
			for _, summary := range identities {
				s.sessionsMu.Lock()
				_, ok := s.sessions[summary.IdentityName]
				s.sessionsMu.Unlock()
				if ok {
					continue
				}
				if _, err := s.ensureSession(summary.IdentityName); err != nil {
					s.recordSessionError(summary.IdentityName, summary.DID, err)
				}
			}
			s.refreshStatus()
		}
	}
}

func (s *Supervisor) consumeNotifications(ctx context.Context, session *session, client *WSClient) error {
	pingTicker := time.NewTicker(sessionPingInterval)
	defer pingTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pingTicker.C:
			pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			err := client.Ping(pingCtx)
			cancel()
			if err != nil {
				return fmt.Errorf("websocket ping failed: %w", err)
			}
		case notification, ok := <-client.Notifications():
			if !ok {
				if err := client.ReaderError(); err != nil {
					return err
				}
				return fmt.Errorf("websocket notification loop closed")
			}
			s.handleNotification(ctx, session, notification)
		}
	}
}

func (s *Supervisor) handleNotification(ctx context.Context, session *session, notification map[string]any) {
	if record, ok := messageRecordFromDirectIncoming(notification, session.record.IdentityName); ok {
		_ = store.StoreMessage(ctx, s.db, record)
		return
	}
	if record, ok := messageRecordFromGroupIncoming(notification, session.record.IdentityName); ok {
		_ = store.StoreMessage(ctx, s.db, record)
		return
	}
	groupRecord, memberRecord, messageRecord, ok := recordsFromGroupStateChanged(notification, session.record.IdentityName)
	if !ok {
		return
	}
	if groupRecord != nil {
		_ = store.UpsertGroup(ctx, s.db, *groupRecord)
	}
	if memberRecord != nil {
		_ = store.UpsertGroupMember(ctx, s.db, *memberRecord)
	}
	if messageRecord != nil {
		_ = store.StoreMessage(ctx, s.db, *messageRecord)
	}
}

func messageRecordFromDirectIncoming(notification map[string]any, identityName string) (store.MessageRecord, bool) {
	method, _ := notification["method"].(string)
	if method != "direct.incoming" {
		return store.MessageRecord{}, false
	}
	params, ok := notification["params"].(map[string]any)
	if !ok {
		return store.MessageRecord{}, false
	}
	meta, _ := params["meta"].(map[string]any)
	body, _ := params["body"].(map[string]any)
	target, _ := meta["target"].(map[string]any)
	targetDID := stringValue(target["did"])
	senderDID := stringValue(meta["sender_did"])
	if targetDID == "" || senderDID == "" {
		return store.MessageRecord{}, false
	}
	contentType := stringValue(meta["content_type"])
	if contentType == "" {
		contentType = "text/plain"
	}
	sentAt := stringValue(meta["created_at"])
	if sentAt == "" {
		sentAt = time.Now().UTC().Format(time.RFC3339)
	}
	return store.MessageRecord{
		MsgID:          stringValue(meta["message_id"]),
		OwnerDID:       targetDID,
		ThreadID:       store.MakeThreadID(targetDID, senderDID, ""),
		Direction:      0,
		SenderDID:      senderDID,
		ReceiverDID:    targetDID,
		ContentType:    contentType,
		Content:        stringValue(body["text"]),
		SentAt:         sentAt,
		IsRead:         false,
		Metadata:       metadataValue(params),
		CredentialName: identityName,
	}, true
}

func messageRecordFromGroupIncoming(notification map[string]any, identityName string) (store.MessageRecord, bool) {
	method, _ := notification["method"].(string)
	if method != "group.incoming" {
		return store.MessageRecord{}, false
	}
	params, ok := notification["params"].(map[string]any)
	if !ok {
		return store.MessageRecord{}, false
	}
	meta, _ := params["meta"].(map[string]any)
	body, _ := params["body"].(map[string]any)
	target, _ := meta["target"].(map[string]any)
	ownerDID := stringValue(target["did"])
	groupDID := stringValue(body["group_did"])
	senderDID := stringValue(meta["sender_did"])
	if ownerDID == "" || groupDID == "" {
		return store.MessageRecord{}, false
	}
	content := stringValue(body["text"])
	if content == "" {
		content = metadataValue(body["payload"])
	}
	contentType := stringValue(meta["content_type"])
	if contentType == "" {
		contentType = "text/plain"
	}
	sentAt := stringValue(body["accepted_at"])
	if sentAt == "" {
		sentAt = stringValue(meta["created_at"])
	}
	serverSeq := int64PtrValue(body["group_event_seq"])
	return store.MessageRecord{
		MsgID:          fallbackString(stringValue(meta["message_id"]), fmt.Sprintf("%s:%s", groupDID, stringValue(body["group_event_seq"]))),
		OwnerDID:       ownerDID,
		ThreadID:       store.MakeThreadID(ownerDID, "", groupDID),
		Direction:      boolToDirection(senderDID == ownerDID),
		SenderDID:      senderDID,
		GroupID:        groupDID,
		GroupDID:       groupDID,
		ContentType:    contentType,
		Content:        content,
		ServerSeq:      serverSeq,
		SentAt:         sentAt,
		IsRead:         senderDID == ownerDID,
		Metadata:       metadataValue(params),
		CredentialName: identityName,
	}, true
}

func recordsFromGroupStateChanged(notification map[string]any, identityName string) (*store.GroupRecord, *store.GroupMemberRecord, *store.MessageRecord, bool) {
	method, _ := notification["method"].(string)
	if method != "group.state_changed" {
		return nil, nil, nil, false
	}
	params, ok := notification["params"].(map[string]any)
	if !ok {
		return nil, nil, nil, false
	}
	meta, _ := params["meta"].(map[string]any)
	body, _ := params["body"].(map[string]any)
	target, _ := meta["target"].(map[string]any)
	ownerDID := stringValue(target["did"])
	groupDID := stringValue(body["group_did"])
	if ownerDID == "" || groupDID == "" {
		return nil, nil, nil, false
	}
	groupRecord := &store.GroupRecord{
		OwnerDID:       ownerDID,
		GroupID:        groupDID,
		GroupDID:       groupDID,
		LastSyncedSeq:  int64PtrValue(body["group_event_seq"]),
		LastMessageAt:  stringValue(body["changed_at"]),
		Metadata:       metadataValue(body),
		CredentialName: identityName,
	}
	subjectDID := stringValue(body["subject_did"])
	var memberRecord *store.GroupMemberRecord
	if subjectDID != "" {
		memberRecord = &store.GroupMemberRecord{
			OwnerDID:       ownerDID,
			GroupID:        groupDID,
			UserID:         subjectDID,
			MemberDID:      subjectDID,
			Status:         membershipStatusFromEvent(body),
			Role:           "member",
			JoinedAt:       stringValue(body["changed_at"]),
			Metadata:       metadataValue(body),
			CredentialName: identityName,
		}
	}
	content := systemEventText(body)
	messageRecord := &store.MessageRecord{
		MsgID:          fallbackString(stringValue(body["event_id"]), fmt.Sprintf("%s:%s", groupDID, stringValue(body["group_event_seq"]))),
		OwnerDID:       ownerDID,
		ThreadID:       store.MakeThreadID(ownerDID, "", groupDID),
		Direction:      0,
		SenderDID:      stringValue(body["actor_did"]),
		GroupID:        groupDID,
		GroupDID:       groupDID,
		ContentType:    inferSystemContentType(stringValue(body["subject_method"])),
		Content:        content,
		ServerSeq:      int64PtrValue(body["group_event_seq"]),
		SentAt:         stringValue(body["changed_at"]),
		IsRead:         false,
		Metadata:       metadataValue(body),
		CredentialName: identityName,
	}
	return groupRecord, memberRecord, messageRecord, true
}

func (s *Supervisor) refreshStatus() {
	s.sessionsMu.Lock()
	sessions := make([]SessionStatus, 0, len(s.sessions))
	for identityName, session := range s.sessions {
		record, connected, lastError := session.snapshot()
		did := ""
		if record != nil {
			did = record.DID
		}
		sessions = append(sessions, SessionStatus{
			IdentityName: identityName,
			DID:          did,
			Connected:    connected,
			LastError:    lastError,
		})
	}
	s.sessionsMu.Unlock()

	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	s.status.Sessions = sessions
	_ = writeStatus(s.status.StatusFile, s.status)
}

func (s *Supervisor) recordSessionError(identityName string, did string, err error) {
	s.sessionsMu.Lock()
	existingSession := s.sessions[identityName]
	if existingSession == nil {
		existingSession = &session{identityName: identityName, record: &identity.StoredIdentity{IdentityName: identityName, DID: did}}
		s.sessions[identityName] = existingSession
	}
	existingSession.markDisconnected(err)
	s.sessionsMu.Unlock()
	s.refreshStatus()
}

func (s *Supervisor) writeStatus() error {
	s.refreshStatus()
	return nil
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func metadataValue(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case string:
		return typed == "1" || strings.EqualFold(typed, "true")
	default:
		return false
	}
}

func fallbackString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func boolPtrValue(value any) *bool {
	switch typed := value.(type) {
	case bool:
		value := typed
		return &value
	default:
		return nil
	}
}

func int64PtrValue(value any) *int64 {
	switch typed := value.(type) {
	case int:
		value := int64(typed)
		return &value
	case int64:
		value := typed
		return &value
	case float64:
		value := int64(typed)
		return &value
	case string:
		if typed == "" {
			return nil
		}
		if parsed, err := strconv.ParseInt(typed, 10, 64); err == nil {
			return &parsed
		}
		return nil
	default:
		return nil
	}
}

func boolToDirection(sentBySelf bool) int {
	if sentBySelf {
		return 1
	}
	return 0
}

func membershipStatusFromEvent(body map[string]any) string {
	if status := stringValue(body["membership_status"]); status != "" {
		return status
	}
	switch stringValue(body["subject_method"]) {
	case "group.add", "group.join":
		return "active"
	case "group.leave":
		return "left"
	case "group.remove":
		return "removed"
	default:
		return "active"
	}
}

func inferSystemContentType(subjectMethod string) string {
	switch subjectMethod {
	case "group.add", "group.join":
		return "group_system_member_joined"
	case "group.leave":
		return "group_system_member_left"
	case "group.remove":
		return "group_system_member_kicked"
	default:
		return "application/json"
	}
}

func systemEventText(body map[string]any) string {
	subjectDID := stringValue(body["subject_did"])
	if subjectDID == "" {
		subjectDID = "A member"
	}
	switch stringValue(body["subject_method"]) {
	case "group.add":
		return fmt.Sprintf("%s was added to the group.", subjectDID)
	case "group.join":
		return fmt.Sprintf("%s joined the group.", subjectDID)
	case "group.leave":
		return fmt.Sprintf("%s left the group.", subjectDID)
	case "group.remove":
		return fmt.Sprintf("%s was removed from the group.", subjectDID)
	case "group.update_profile":
		return "The group profile was updated."
	case "group.update_policy":
		return "The group policy was updated."
	default:
		return "The group state changed."
	}
}

func (s *Supervisor) fetchMessageServiceDID(session *session) (string, error) {
	client := session.currentClient()
	if client == nil {
		return "", fmt.Errorf("websocket session is not connected for identity %s", session.identityName)
	}
	result, err := client.SendRPC(context.Background(), "anp.get_capabilities", map[string]any{})
	if err != nil {
		return "", err
	}
	serviceDID := stringValue(result["service_did"])
	if serviceDID == "" {
		return "", fmt.Errorf("message service capabilities response is missing service_did")
	}
	return serviceDID, nil
}

func (s *Supervisor) runSessionLoop(session *session) {
	delay := sessionReconnectBaseDelay
	for {
		select {
		case <-session.ctx.Done():
			session.closeCurrentClient()
			return
		default:
		}

		record, client, err := s.connectSession(session.identityName)
		if err != nil {
			session.markDisconnected(err)
			session.signalInitial(err)
			s.refreshStatus()
			if !sleepWithContext(session.ctx, delay) {
				return
			}
			delay = minDuration(delay*2, sessionReconnectMaxDelay)
			continue
		}

		delay = sessionReconnectBaseDelay
		session.markConnected(record, client)
		session.signalInitial(nil)
		s.refreshStatus()

		err = s.consumeNotifications(session.ctx, session, client)
		_ = client.Close()
		session.markDisconnected(err)
		s.refreshStatus()
		if session.ctx.Err() != nil {
			return
		}
		if !sleepWithContext(session.ctx, delay) {
			return
		}
		delay = minDuration(delay*2, sessionReconnectMaxDelay)
	}
}

func (s *Supervisor) connectSession(identityName string) (*identity.StoredIdentity, *WSClient, error) {
	record, err := s.manager.Load(identityName)
	if err != nil {
		return nil, nil, err
	}
	userState := identity.EvaluateStoredIdentityUserState(record)
	if !userState.ReadyForMessaging {
		return nil, nil, identity.UserRegistrationError(record.IdentityName, userState)
	}
	paths, pathErr := s.manager.PathsForIdentity(identityName)
	if pathErr != nil {
		return nil, nil, pathErr
	}
	authSession := authsdk.NewSession(
		paths.DIDDocumentPath,
		paths.Key1PrivatePath,
		record.IdentityName,
		record.DID,
		record.JWTToken,
		func(token string) error { return s.manager.UpdateJWT(record.IdentityName, token) },
	)
	if strings.TrimSpace(record.JWTToken) != "" {
		authSession.SetBearer(s.resolved.UserServiceURL, record.JWTToken)
		if strings.TrimSpace(s.resolved.MessageServiceURL) != "" {
			authSession.SetBearer(s.resolved.MessageServiceURL, record.JWTToken)
		}
	}
	client, err := NewWSClient(s.resolved, authSession)
	if err != nil {
		return nil, nil, err
	}
	connectCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.Connect(connectCtx); err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	record.JWTToken = authSession.CurrentJWT()
	return record, client, nil
}

func (s *session) currentClient() *WSClient {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client
}

func (s *session) currentRecord() *identity.StoredIdentity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.record
}

func (s *session) snapshot() (*identity.StoredIdentity, bool, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.record, s.connected, s.lastError
}

func (s *session) markConnected(record *identity.StoredIdentity, client *WSClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil && s.client != client {
		_ = s.client.Close()
	}
	s.record = record
	s.client = client
	s.connected = true
	s.lastError = ""
}

func (s *session) markDisconnected(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		_ = s.client.Close()
	}
	s.client = nil
	s.connected = false
	if err != nil && !errors.Is(err, context.Canceled) {
		s.lastError = err.Error()
	}
}

func (s *session) closeCurrentClient() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		_ = s.client.Close()
		s.client = nil
	}
	s.connected = false
}

func (s *session) signalInitial(err error) {
	s.initOnce.Do(func() {
		s.initResult <- err
		close(s.initResult)
	})
}

func sleepWithContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func minDuration(value time.Duration, max time.Duration) time.Duration {
	if value > max {
		return max
	}
	return value
}

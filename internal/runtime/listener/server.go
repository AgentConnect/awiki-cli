package listener

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	directe2ee "github.com/agent-network-protocol/anp/golang/direct_e2ee"
	"github.com/agentconnect/awiki-cli/internal/anpsdk"
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
	remote   *identity.RemoteClient
	statusMu sync.Mutex
	status   Status

	sessionsMu           sync.Mutex
	sessions             map[string]*session
	localNotificationsMu sync.Mutex
	localNotifications   map[string][]map[string]any
	listener             net.Listener
	db                   *sql.DB
	hostNotify           HostNotifySink
}

type session struct {
	identityName  string
	record        *identity.StoredIdentity
	client        *WSClient
	secureRPCCall func(context.Context, string, map[string]any) (map[string]any, error)
	lastError     string
	connected     bool
	ctx           context.Context
	cancelFunc    context.CancelFunc
	initResult    chan error
	initOnce      sync.Once
	mu            sync.RWMutex
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
	bootID, err := resolveRuntimeBootID(resolved)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	pidFile, logFile, statusFile, socketPath, err := paths(resolved)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	hostNotifySink, hostNotifyStatus, err := newHostNotifySink(resolved)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	remote, err := identity.NewRemoteClient(resolved)
	if err != nil {
		_ = db.Close()
		_ = hostNotifySink.Close()
		return nil, err
	}
	return &Supervisor{
		resolved: resolved,
		manager:  identity.NewManager(resolved.Paths),
		remote:   remote,
		status: Status{
			Mode:       runtime.Resolve(resolved).Mode,
			Installed:  runningInListenerServiceMode(),
			BootID:     bootID,
			PIDFile:    pidFile,
			LogFile:    logFile,
			StatusFile: statusFile,
			SocketPath: socketPath,
			Running:    true,
			PID:        os.Getpid(),
			StartedAt:  time.Now().UTC().Format(time.RFC3339),
			HostNotify: hostNotifyStatus,
		},
		sessions:           map[string]*session{},
		localNotifications: map[string][]map[string]any{},
		db:                 db,
		hostNotify:         hostNotifySink,
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
	if s.hostNotify != nil {
		_ = s.hostNotify.Close()
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
	listener, err := runtime.ListenBridge(s.status.SocketPath)
	if err != nil {
		return err
	}
	s.listener = listener
	s.setBridgeAvailable(true)
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
	notification = s.normalizeDirectSecureNotification(ctx, session, notification)
	if notification == nil {
		return
	}
	receivedAt := time.Now().UTC()
	event, shouldNotify := NormalizeHostNotification(notification, receivedAt)
	if record, ok := messageRecordFromDirectIncoming(notification, session.record.IdentityName); ok {
		senderHandle, _ := s.syncIncomingContact(ctx, session, record.SenderDID, "direct.incoming", "")
		ApplyHostNotificationHandles(event, senderHandle, normalizeListenerHandle(session.record.Handle))
		_ = store.StoreMessage(ctx, s.db, record)
		s.dispatchHostNotification(ctx, event, shouldNotify)
		return
	}
	if record, ok := messageRecordFromMailNotification(notification, session.record.IdentityName); ok {
		_ = store.StoreMessage(ctx, s.db, record)
		s.dispatchHostNotification(ctx, event, shouldNotify)
		return
	}
	if record, ok := messageRecordFromGroupIncoming(notification, session.record.IdentityName); ok {
		senderHandle, _ := s.syncIncomingContact(ctx, session, record.SenderDID, "group.incoming", record.GroupDID)
		ApplyHostNotificationHandles(event, senderHandle, normalizeListenerHandle(session.record.Handle))
		_ = store.StoreMessage(ctx, s.db, record)
		s.dispatchHostNotification(ctx, event, shouldNotify)
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
	s.dispatchHostNotification(ctx, event, shouldNotify)
}

func (s *Supervisor) normalizeDirectSecureNotification(ctx context.Context, session *session, notification map[string]any) map[string]any {
	if !isDirectSecureIncomingNotification(notification) {
		return notification
	}
	record := session.currentRecord()
	if record == nil {
		return notification
	}
	rpcCall := session.secureRPC()
	if rpcCall == nil {
		return notification
	}
	client, err := message.NewSecureE2EEClientForRecord(ctx, s.manager, record, func(method string, params map[string]any) (map[string]any, error) {
		return rpcCall(ctx, method, params)
	})
	if err != nil {
		return notification
	}
	params, _ := notification["params"].(map[string]any)
	result, err := client.ProcessIncoming(ctx, params)
	if err != nil {
		return notification
	}
	if stringValue(result["state"]) != "decrypted" {
		return notification
	}
	plaintext, ok := result["plaintext"].(map[string]any)
	if !ok {
		return notification
	}
	meta, _ := params["meta"].(map[string]any)
	originalBody := params["body"]
	originalContentType := stringValue(meta["content_type"])
	meta["content_type"] = stringValue(plaintext["application_content_type"])
	params["body"] = plaintextBodyToNotificationBody(plaintext)
	params["secure_state"] = "decrypted"
	params["secure_wire_content_type"] = originalContentType
	params["secure_wire_body"] = originalBody
	if message.IsSecureAckPlaintext(plaintext) {
		peerDID := stringValue(meta["sender_did"])
		_ = message.FlushQueuedSecureOutbox(ctx, s.resolved, s.manager, record, peerDID, func(method string, params map[string]any) (map[string]any, error) {
			return rpcCall(ctx, method, params)
		})
		notification["method"] = "direct.secure.ack"
		return notification
	}
	if message.IsSecureInitPlaintext(plaintext) {
		notification["method"] = "direct.secure.init"
	}
	if originalContentType == "application/anp-direct-init+json" {
		sessionID := stringValue(mapValue(originalBody)["session_id"])
		messageID := stringValue(meta["message_id"])
		if sessionID != "" && messageID != "" {
			ackID := "ack-" + sessionID
			if !s.deliverLocalSecureAckInProcess(ctx, record, stringValue(meta["sender_did"]), sessionID, messageID, ackID) {
				ackResult, ackErr := client.SendJSON(ctx, stringValue(meta["sender_did"]), message.BuildSecureAckPayload(sessionID, messageID), ackID, ackID)
				if ackErr == nil {
					s.deliverLocalSecureAck(ctx, record.DID, stringValue(meta["sender_did"]), ackID, ackResult)
				} else {
				}
			}
			s.flushPeerQueuedSecureOutbox(ctx, stringValue(meta["sender_did"]), record.DID)
		}
	}
	return notification
}

func isDirectSecureIncomingNotification(notification map[string]any) bool {
	method, _ := notification["method"].(string)
	if method != "direct.incoming" {
		return false
	}
	params, _ := notification["params"].(map[string]any)
	meta, _ := params["meta"].(map[string]any)
	return isSecureDirectWireContentType(stringValue(meta["content_type"]))
}

func isSecureDirectWireContentType(contentType string) bool {
	switch contentType {
	case "application/anp-direct-init+json", "application/anp-direct-cipher+json":
		return true
	default:
		return false
	}
}

func secureNotificationFromMessageView(messageView map[string]any) (map[string]any, error) {
	body, ok := messageView["content"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("content is not a direct-e2ee object")
	}
	senderDID := stringValue(messageView["sender_did"])
	receiverDID := stringValue(messageView["receiver_did"])
	messageID := stringValue(messageView["id"])
	if senderDID == "" || receiverDID == "" || messageID == "" {
		return nil, fmt.Errorf("missing sender_did/receiver_did/id")
	}
	params := map[string]any{
		"meta": map[string]any{
			"sender_did":       senderDID,
			"target":           map[string]any{"kind": "agent", "did": receiverDID},
			"message_id":       messageID,
			"profile":          "anp.direct.e2ee.v1",
			"security_profile": "direct-e2ee",
			"content_type":     stringValue(messageView["content_type"]),
		},
		"body": body,
	}
	if serverSeq := messageView["server_seq"]; serverSeq != nil {
		params["server_seq"] = serverSeq
	}
	return map[string]any{"method": "direct.incoming", "params": params}, nil
}

func (s *Supervisor) flushPeerQueuedSecureOutbox(ctx context.Context, ownerDID string, peerDID string) {
	s.sessionsMu.Lock()
	sessions := make([]*session, 0, len(s.sessions))
	for _, item := range s.sessions {
		sessions = append(sessions, item)
	}
	s.sessionsMu.Unlock()
	for _, item := range sessions {
		record := item.currentRecord()
		if record == nil || record.DID != ownerDID {
			continue
		}
		rpcCall := item.secureRPC()
		if rpcCall == nil {
			return
		}
		warnings := message.FlushQueuedSecureOutbox(ctx, s.resolved, s.manager, record, peerDID, func(method string, params map[string]any) (map[string]any, error) {
			return rpcCall(ctx, method, params)
		})
		log.Printf("listener queued secure outbox flush owner_did=%s peer_did=%s warnings=%v", ownerDID, peerDID, warnings)
		return
	}
}

func (s *Supervisor) deliverLocalSecureAck(ctx context.Context, senderDID string, recipientDID string, fallbackMessageID string, ackResult map[string]any) {
	targetSession := s.activeSessionByDID(recipientDID)
	if targetSession == nil {
		return
	}
	body, _ := ackResult["body"].(map[string]any)
	if len(body) == 0 {
		return
	}
	messageID := fallbackString(stringValue(ackResult["message_id"]), fallbackMessageID)
	notification := map[string]any{
		"method": "direct.incoming",
		"params": map[string]any{
			"meta": map[string]any{
				"sender_did":       senderDID,
				"target":           map[string]any{"kind": "agent", "did": recipientDID},
				"message_id":       messageID,
				"profile":          "anp.direct.e2ee.v1",
				"security_profile": "direct-e2ee",
				"content_type":     "application/anp-direct-cipher+json",
			},
			"body": body,
		},
	}
	s.handleNotification(ctx, targetSession, notification)
}

func (s *Supervisor) deliverLocalSecureAckInProcess(ctx context.Context, senderRecord *identity.StoredIdentity, recipientDID string, sessionID string, repliedMessageID string, ackMessageID string) bool {
	if senderRecord == nil {
		log.Printf("listener local secure ack skipped: sender record missing")
		return false
	}
	recipientRecord := s.recordByDID(recipientDID)
	if recipientRecord == nil {
		log.Printf("listener local secure ack skipped: recipient %s not managed locally", recipientDID)
		return false
	}
	paths, err := s.manager.PathsForIdentity(senderRecord.IdentityName)
	if err != nil {
		log.Printf("listener local secure ack skipped: sender paths error: %v", err)
		return false
	}
	// Rebuild the sender-side session via file store so we can emit one local encrypted ack
	// even when the service/websocket ack path is unavailable during reconnect recovery.
	fileStore, err := anpsdk.NewFileSessionStore(filepath.Join(paths.IdentityDir, "p5-e2ee-sessions"))
	if err != nil {
		log.Printf("listener local secure ack skipped: session store error: %v", err)
		return false
	}
	senderSession, ok, err := fileStore.FindByPeerDID(recipientDID)
	if err != nil || !ok {
		log.Printf("listener local secure ack skipped: sender session lookup peer=%s ok=%v err=%v", recipientDID, ok, err)
		return false
	}
	candidateSession := senderSession
	builder := directe2ee.DirectE2eeSession{}
	_, ackBody, err := builder.EncryptFollowUp(
		&candidateSession,
		directe2ee.DirectEnvelopeMetadata{
			SenderDID:       senderRecord.DID,
			RecipientDID:    recipientDID,
			MessageID:       ackMessageID,
			Profile:         "anp.direct.e2ee.v1",
			SecurityProfile: "direct-e2ee",
		},
		ackMessageID,
		directe2ee.NewJSONPlaintext("application/json", message.BuildSecureAckPayload(sessionID, repliedMessageID)),
	)
	if err != nil {
		log.Printf("listener local secure ack skipped: encrypt follow-up error: %v", err)
		return false
	}
	notification := map[string]any{
		"meta": map[string]any{
			"sender_did":       senderRecord.DID,
			"target":           map[string]any{"kind": "agent", "did": recipientDID},
			"message_id":       ackMessageID,
			"profile":          "anp.direct.e2ee.v1",
			"security_profile": "direct-e2ee",
			"content_type":     "application/anp-direct-cipher+json",
		},
		"body": structToMap(ackBody),
	}
	recipientClient, err := message.NewSecureE2EEClientForRecord(ctx, s.manager, recipientRecord, func(string, map[string]any) (map[string]any, error) {
		return nil, fmt.Errorf("local secure ack delivery does not use outbound rpc")
	})
	if err != nil {
		log.Printf("listener local secure ack skipped: recipient client init error: %v", err)
		return false
	}
	result, err := recipientClient.ProcessIncoming(ctx, notification)
	if err != nil || stringValue(result["state"]) != "decrypted" {
		recipientPaths, pathErr := s.manager.PathsForIdentity(recipientRecord.IdentityName)
		if pathErr != nil {
			log.Printf("listener local secure ack skipped: recipient process incoming err=%v state=%s recipientPathsErr=%v", err, stringValue(result["state"]), pathErr)
			return false
		}
		recipientStore, storeErr := anpsdk.NewFileSessionStore(filepath.Join(recipientPaths.IdentityDir, "p5-e2ee-sessions"))
		if storeErr != nil {
			log.Printf("listener local secure ack skipped: recipient process incoming err=%v state=%s recipientStoreErr=%v", err, stringValue(result["state"]), storeErr)
			return false
		}
		recipientSession, loadErr := recipientStore.LoadSession(sessionID)
		if loadErr != nil {
			log.Printf("listener local secure ack skipped: recipient process incoming err=%v state=%s recipientLoadErr=%v", err, stringValue(result["state"]), loadErr)
			return false
		}
		var ackCipher directe2ee.DirectCipherBody
		rawBody, marshalErr := json.Marshal(notification["body"])
		if marshalErr != nil {
			log.Printf("listener local secure ack skipped: recipient process incoming err=%v state=%s marshalAckErr=%v", err, stringValue(result["state"]), marshalErr)
			return false
		}
		if unmarshalErr := json.Unmarshal(rawBody, &ackCipher); unmarshalErr != nil {
			log.Printf("listener local secure ack skipped: recipient process incoming err=%v state=%s unmarshalAckErr=%v", err, stringValue(result["state"]), unmarshalErr)
			return false
		}
		_, decryptErr := builder.DecryptFollowUp(
			&recipientSession,
			directe2ee.DirectEnvelopeMetadata{
				SenderDID:       senderRecord.DID,
				RecipientDID:    recipientDID,
				MessageID:       ackMessageID,
				Profile:         "anp.direct.e2ee.v1",
				SecurityProfile: "direct-e2ee",
			},
			ackCipher,
		)
		if decryptErr != nil {
			log.Printf("listener local secure ack skipped: recipient process incoming err=%v state=%s decryptFallbackErr=%v", err, stringValue(result["state"]), decryptErr)
			return false
		}
		if saveErr := recipientStore.SaveSession(recipientSession); saveErr != nil {
			log.Printf("listener local secure ack skipped: recipient process incoming err=%v state=%s recipientSaveErr=%v", err, stringValue(result["state"]), saveErr)
			return false
		}
	}
	if err := fileStore.SaveSession(candidateSession); err != nil {
		log.Printf("listener local secure ack skipped: save sender session error: %v", err)
		return false
	}
	if targetSession := s.activeSessionByDID(recipientDID); targetSession != nil {
		if rpcCall := targetSession.secureRPC(); rpcCall != nil {
			warnings := message.FlushQueuedSecureOutbox(ctx, s.resolved, s.manager, recipientRecord, senderRecord.DID, func(method string, params map[string]any) (map[string]any, error) {
				return rpcCall(ctx, method, params)
			})
			log.Printf("listener local secure ack delivered recipient=%s sender=%s flush_warnings=%v", recipientRecord.DID, senderRecord.DID, warnings)
		}
		log.Printf("listener local secure ack delivered recipient=%s sender=%s", recipientRecord.DID, senderRecord.DID)
		return true
	}
	if s.hasRuntimeSessionForDID(recipientDID) {
		s.queueLocalNotification(recipientDID, map[string]any{
			"method": "direct.incoming",
			"params": notification,
		})
		log.Printf("listener local secure ack queued for recipient=%s sender=%s until session activates", recipientRecord.DID, senderRecord.DID)
		return true
	}
	log.Printf("listener local secure ack fallback to network: recipient session not managed recipient=%s sender=%s", recipientRecord.DID, senderRecord.DID)
	return false
}

func (s *Supervisor) activeSessionByDID(did string) *session {
	if strings.TrimSpace(did) == "" {
		return nil
	}
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	for _, item := range s.sessions {
		record := item.currentRecord()
		if record != nil && record.DID == did {
			return item
		}
	}
	return nil
}

func (s *Supervisor) recordByDID(did string) *identity.StoredIdentity {
	if strings.TrimSpace(did) == "" || s.manager == nil {
		return nil
	}
	identities, err := s.manager.List()
	if err != nil {
		return nil
	}
	for _, summary := range identities {
		if summary.DID != did {
			continue
		}
		record, err := s.manager.Load(summary.IdentityName)
		if err == nil && record != nil {
			return record
		}
	}
	return nil
}

func (s *Supervisor) hasRuntimeSessionForDID(did string) bool {
	if strings.TrimSpace(did) == "" {
		return false
	}
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	for _, item := range s.sessions {
		if record := item.currentRecord(); record != nil && record.DID == did {
			return true
		}
		if s.manager == nil {
			continue
		}
		record, err := s.manager.Load(item.identityName)
		if err == nil && record != nil && record.DID == did {
			return true
		}
	}
	return false
}

func (s *Supervisor) queueLocalNotification(recipientDID string, notification map[string]any) {
	if strings.TrimSpace(recipientDID) == "" || notification == nil {
		return
	}
	s.localNotificationsMu.Lock()
	defer s.localNotificationsMu.Unlock()
	s.localNotifications[recipientDID] = append(s.localNotifications[recipientDID], notification)
}

func (s *Supervisor) flushQueuedLocalNotifications(targetSession *session) {
	if targetSession == nil {
		return
	}
	record := targetSession.currentRecord()
	if record == nil || strings.TrimSpace(record.DID) == "" {
		return
	}
	s.localNotificationsMu.Lock()
	queued := append([]map[string]any(nil), s.localNotifications[record.DID]...)
	delete(s.localNotifications, record.DID)
	s.localNotificationsMu.Unlock()
	for _, notification := range queued {
		s.handleNotification(targetSession.ctx, targetSession, notification)
	}
}

func structToMap(value any) map[string]any {
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return map[string]any{}
	}
	return result
}

func plaintextBodyToNotificationBody(plaintext map[string]any) map[string]any {
	body := map[string]any{}
	for _, key := range []string{"conversation_id", "reply_to_message_id", "annotations"} {
		if value, ok := plaintext[key]; ok && value != nil {
			body[key] = value
		}
	}
	if text := stringValue(plaintext["text"]); text != "" {
		body["text"] = text
	}
	if payload, ok := plaintext["payload"]; ok && payload != nil {
		body["payload"] = payload
	}
	if payloadB64U := stringValue(plaintext["payload_b64u"]); payloadB64U != "" {
		body["payload_b64u"] = payloadB64U
	}
	return body
}

func (s *Supervisor) dispatchHostNotification(ctx context.Context, event *HostNotificationEvent, shouldNotify bool) {
	if !shouldNotify || event == nil || s.hostNotify == nil {
		return
	}
	if err := s.hostNotify.Notify(ctx, *event); err != nil {
		s.setHostNotifyError(err.Error())
		return
	}
	s.clearHostNotifyError()
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
	contentValue := body["text"]
	if text := stringValue(body["text"]); text == "" {
		switch {
		case body["payload"] != nil:
			contentValue = body["payload"]
		case stringValue(body["payload_b64u"]) != "":
			contentValue = body["payload_b64u"]
		default:
			contentValue = body
		}
	}
	content := stringValue(contentValue)
	if content == "" {
		content = metadataValue(contentValue)
	}
	return store.MessageRecord{
		MsgID:          stringValue(meta["message_id"]),
		OwnerDID:       targetDID,
		ThreadID:       store.MakeThreadID(targetDID, senderDID, ""),
		Direction:      0,
		SenderDID:      senderDID,
		ReceiverDID:    targetDID,
		ContentType:    contentType,
		Content:        content,
		IsE2EE:         stringValue(meta["security_profile"]) == "direct-e2ee" || stringValue(params["secure_state"]) == "decrypted",
		SentAt:         sentAt,
		IsRead:         false,
		Metadata:       metadataValue(params),
		CredentialName: identityName,
	}, true
}

// messageRecordFromMailNotification maps a lightweight mail.notification payload into a local MessageRecord.
//
// The message-service v2 side pushes notifications in the shape:
//
//	{"jsonrpc":"2.0","method":"mail.notification","params":{
//	    "mailbox_did": "...",
//	    "mailbox_address": "alice@awiki.ai",
//	    "from_addr": "sender@example.com",
//	    "subject": "Subject",
//	    "preview": "Body preview ...",
//	    "has_attachments": true,
//	    "message_id": "uuid"
//	}}
//
// We persist this as an inbound "system" message with:
//   - owner_did = mailbox_did
//   - thread_id = mail:<mailbox_address>
//   - content_type = "mail.notification"
//   - content = human-readable summary text
func messageRecordFromMailNotification(notification map[string]any, identityName string) (store.MessageRecord, bool) {
	method, _ := notification["method"].(string)
	if method != "mail.notification" {
		return store.MessageRecord{}, false
	}
	params, ok := notification["params"].(map[string]any)
	if !ok {
		return store.MessageRecord{}, false
	}
	mailboxDID := stringValue(params["mailbox_did"])
	if mailboxDID == "" {
		return store.MessageRecord{}, false
	}
	mailboxAddress := stringValue(params["mailbox_address"])
	fromAddr := stringValue(params["from_addr"])
	subject := stringValue(params["subject"])
	preview := stringValue(params["preview"])
	hasAttachments := boolValue(params["has_attachments"])
	messageID := stringValue(params["message_id"])
	if strings.TrimSpace(messageID) == "" {
		// Fallback to a locally generated identifier if message_id was not provided.
		messageID = fmt.Sprintf("mail:%s:%d", mailboxAddress, time.Now().UTC().UnixNano())
	}
	if strings.TrimSpace(mailboxAddress) == "" {
		mailboxAddress = mailboxDID
	}
	threadID := fmt.Sprintf("mail:%s", strings.TrimSpace(mailboxAddress))
	if strings.TrimSpace(subject) == "" {
		subject = "(no subject)"
	}
	sentAt := time.Now().UTC().Format(time.RFC3339)
	content := buildMailNotificationContent(mailboxAddress, fromAddr, subject, preview, hasAttachments)

	return store.MessageRecord{
		MsgID:          messageID,
		OwnerDID:       mailboxDID,
		ThreadID:       threadID,
		Direction:      0,
		SenderDID:      "",
		ReceiverDID:    mailboxDID,
		ContentType:    "mail.notification",
		Content:        content,
		Title:          "[邮件] " + subject,
		ServerSeq:      nil,
		SentAt:         sentAt,
		IsE2EE:         false,
		IsRead:         false,
		Metadata:       metadataValue(params),
		CredentialName: identityName,
	}, true
}

func buildMailNotificationContent(mailboxAddress string, fromAddr string, subject string, preview string, hasAttachments bool) string {
	contentLines := []string{
		fmt.Sprintf("[邮件] 收件邮箱: %s", mailboxAddress),
	}
	if fromAddr != "" {
		contentLines = append(contentLines, fmt.Sprintf("发件人: %s", fromAddr))
	}
	if subject != "" {
		contentLines = append(contentLines, fmt.Sprintf("主题: %s", subject))
	}
	if preview != "" {
		contentLines = append(contentLines, "", preview)
	}
	if hasAttachments {
		contentLines = append(contentLines, "", "(这封邮件包含附件)")
	}
	return strings.Join(contentLines, "\n")
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

func (s *Supervisor) setBridgeAvailable(available bool) {
	s.statusMu.Lock()
	changed := s.status.BridgeAvailable != available
	s.status.BridgeAvailable = available
	statusFile := s.status.StatusFile
	status := s.status
	s.statusMu.Unlock()
	if changed {
		_ = writeStatus(statusFile, status)
	}
}

func (s *Supervisor) setHostNotifyError(lastError string) {
	s.statusMu.Lock()
	changed := s.status.HostNotify.LastError != lastError
	s.status.HostNotify.LastError = lastError
	statusFile := s.status.StatusFile
	status := s.status
	s.statusMu.Unlock()
	if changed {
		_ = writeStatus(statusFile, status)
	}
}

func (s *Supervisor) clearHostNotifyError() {
	s.statusMu.Lock()
	if s.status.HostNotify.LastError == "" {
		s.statusMu.Unlock()
		return
	}
	s.status.HostNotify.LastError = ""
	statusFile := s.status.StatusFile
	status := s.status
	s.statusMu.Unlock()
	_ = writeStatus(statusFile, status)
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
		s.flushQueuedLocalNotifications(session)
		publishCtx, publishCancel := context.WithCancel(session.ctx)
		go s.retryPublishSecurePrekeys(publishCtx, record)
		go s.pollUnreadSecureDirectInbox(publishCtx, session, client)

		err = s.consumeNotifications(session.ctx, session, client)
		publishCancel()
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

func (s *Supervisor) retryPublishSecurePrekeys(ctx context.Context, record *identity.StoredIdentity) {
	for {
		warnings := message.PublishSecurePrekeys(ctx, s.resolved, s.manager, record)
		if len(warnings) == 0 {
			return
		}
		log.Printf("listener secure prekey publish retry identity=%s warnings=%s", record.IdentityName, strings.Join(warnings, "; "))
		if !sleepWithContext(ctx, time.Second) {
			return
		}
	}
}

func (s *Supervisor) syncUnreadSecureDirectInbox(ctx context.Context, session *session, client *WSClient) {
	record := session.currentRecord()
	if record == nil {
		return
	}
	syncCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	params := message.BuildInboxRPCParams(record, message.InboxRequest{Scope: "direct", UnreadOnly: true, Limit: 100})
	result, err := client.SendRPC(syncCtx, "inbox.get", params)
	if err != nil {
		return
	}
	items, _ := result["messages"].([]any)
	for _, item := range items {
		messageView, ok := item.(map[string]any)
		if !ok || !isSecureDirectWireContentType(stringValue(messageView["content_type"])) {
			continue
		}
		ownerDID := stringValue(messageView["receiver_did"])
		if ownerDID == "" {
			ownerDID = session.record.DID
		}
		if _, err := store.GetMessageByID(syncCtx, s.db, stringValue(messageView["id"]), ownerDID, session.identityName); err == nil {
			continue
		} else if !errors.Is(err, sql.ErrNoRows) {
			continue
		}
		notification, err := secureNotificationFromMessageView(messageView)
		if err != nil {
			continue
		}
		log.Printf("listener secure backlog replay identity=%s message_id=%s content_type=%s", session.identityName, stringValue(messageView["id"]), stringValue(messageView["content_type"]))
		s.handleNotification(syncCtx, session, notification)
	}
}

func (s *Supervisor) pollUnreadSecureDirectInbox(ctx context.Context, session *session, client *WSClient) {
	s.syncUnreadSecureDirectInbox(ctx, session, client)
	s.syncPendingConfirmationSecureHistory(ctx, session, client)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncUnreadSecureDirectInbox(ctx, session, client)
			s.syncPendingConfirmationSecureHistory(ctx, session, client)
		}
	}
}

func (s *Supervisor) syncPendingConfirmationSecureHistory(ctx context.Context, session *session, client *WSClient) {
	record := session.currentRecord()
	if record == nil {
		return
	}
	peerDIDs := s.pendingConfirmationPeerDIDs(record.IdentityName)
	if len(peerDIDs) == 0 {
		return
	}
	for _, peerDID := range peerDIDs {
		syncCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		params, buildErr := message.BuildHistoryRPCParams(record, message.HistoryRequest{With: peerDID, Limit: 50})
		if buildErr != nil {
			cancel()
			continue
		}
		result, err := client.SendRPC(syncCtx, "direct.get_history", params)
		cancel()
		if err != nil {
			continue
		}
		items, _ := result["messages"].([]any)
		for _, item := range items {
			messageView, ok := item.(map[string]any)
			if !ok || !isSecureDirectWireContentType(stringValue(messageView["content_type"])) {
				continue
			}
			if stringValue(messageView["sender_did"]) == record.DID {
				continue
			}
			ownerDID := stringValue(messageView["receiver_did"])
			if ownerDID == "" {
				ownerDID = record.DID
			}
			if _, err := store.GetMessageByID(ctx, s.db, stringValue(messageView["id"]), ownerDID, session.identityName); err == nil {
				continue
			} else if !errors.Is(err, sql.ErrNoRows) {
				continue
			}
			notification, err := secureNotificationFromMessageView(messageView)
			if err != nil {
				continue
			}
			s.handleNotification(ctx, session, notification)
		}
	}
}

func (s *Supervisor) pendingConfirmationPeerDIDs(identityName string) []string {
	if s.manager == nil || strings.TrimSpace(identityName) == "" {
		return nil
	}
	paths, err := s.manager.PathsForIdentity(identityName)
	if err != nil {
		return nil
	}
	entries, err := filepath.Glob(filepath.Join(paths.IdentityDir, "p5-e2ee-sessions", "*.json"))
	if err != nil {
		return nil
	}
	peers := make([]string, 0, len(entries))
	seen := map[string]struct{}{}
	for _, path := range entries {
		var payload struct {
			PeerDID string `json:"peer_did"`
			Status  string `json:"status"`
		}
		if err := readJSONFile(path, &payload); err != nil {
			continue
		}
		if payload.Status != "pending-confirmation" || strings.TrimSpace(payload.PeerDID) == "" {
			continue
		}
		if _, ok := seen[payload.PeerDID]; ok {
			continue
		}
		seen[payload.PeerDID] = struct{}{}
		peers = append(peers, payload.PeerDID)
	}
	return peers
}

func readJSONFile(path string, out any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
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
		authSession.SetBearer(s.resolved.ServiceBaseURL, record.JWTToken)
		authSession.SetBearer(
			appconfig.JoinBaseURL(s.resolved.ServiceBaseURL, "/user-service/did-auth/rpc"),
			record.JWTToken,
		)
		authSession.SetBearer(
			appconfig.JoinBaseURL(s.resolved.ServiceBaseURL, message.MessageWSEndpoint),
			record.JWTToken,
		)
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

func (s *session) secureRPC() func(context.Context, string, map[string]any) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.secureRPCCall != nil {
		return s.secureRPCCall
	}
	if s.client == nil {
		return nil
	}
	return s.client.SendRPC
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

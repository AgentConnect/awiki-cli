package listener

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
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
	record     *identity.StoredIdentity
	client     *WSClient
	lastError  string
	connected  bool
	cancelFunc context.CancelFunc
}

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
		if session.client != nil {
			_ = session.client.Close()
		}
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
	switch request.Method {
	case "direct.send":
		target, _ := request.Params["target"].(string)
		text, _ := request.Params["text"].(string)
		msgType, _ := request.Params["type"].(string)
		params, err := message.BuildDirectSendRPCParams(session.record, s.manager, target, text, msgType)
		if err != nil {
			return nil, err
		}
		return session.client.SendRPC(context.Background(), "direct.send", params)
	case "inbox.get":
		params := message.BuildInboxRPCParams(session.record, message.InboxRequest{
			Limit:      intValue(request.Params["limit"]),
			With:       stringValue(request.Params["with"]),
			UnreadOnly: boolValue(request.Params["unread"]),
			MarkRead:   boolValue(request.Params["mark_read"]),
		})
		return session.client.SendRPC(context.Background(), "inbox.get", params)
	case "direct.get_history":
		params, err := message.BuildHistoryRPCParams(session.record, message.HistoryRequest{
			With:   stringValue(request.Params["with"]),
			Limit:  intValue(request.Params["limit"]),
			Cursor: stringValue(request.Params["cursor"]),
		})
		if err != nil {
			return nil, err
		}
		return session.client.SendRPC(context.Background(), "direct.get_history", params)
	case "inbox.mark_read":
		rawIDs, _ := request.Params["message_ids"].([]any)
		messageIDs := make([]string, 0, len(rawIDs))
		for _, rawID := range rawIDs {
			if id := stringValue(rawID); id != "" {
				messageIDs = append(messageIDs, id)
			}
		}
		params, err := message.BuildMarkReadRPCParams(session.record, message.MarkReadRequest{MessageIDs: messageIDs})
		if err != nil {
			return nil, err
		}
		result, err := session.client.SendRPC(context.Background(), "inbox.mark_read", params)
		if err == nil {
			_, _ = store.MarkMessagesRead(context.Background(), s.db, session.record.DID, messageIDs)
		}
		return result, err
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
	s.sessionsMu.Unlock()
	if existing != nil && existing.connected {
		return existing, nil
	}
	record, err := s.manager.Load(identityName)
	if err != nil {
		return nil, err
	}
	userState := identity.EvaluateStoredIdentityUserState(record)
	if !userState.ReadyForMessaging {
		return nil, identity.UserRegistrationError(record.IdentityName, userState)
	}
	paths, pathErr := s.manager.PathsForIdentity(identityName)
	if pathErr != nil {
		return nil, pathErr
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
		return nil, err
	}
	connectCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.Connect(connectCtx); err != nil {
		return nil, err
	}
	record.JWTToken = authSession.CurrentJWT()
	sessionCtx, sessionCancel := context.WithCancel(context.Background())
	newSession := &session{
		record:     record,
		client:     client,
		connected:  true,
		cancelFunc: sessionCancel,
	}
	s.sessionsMu.Lock()
	s.sessions[identityName] = newSession
	s.sessionsMu.Unlock()
	go s.consumeNotifications(sessionCtx, newSession)
	s.refreshStatus()
	return newSession, nil
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

func (s *Supervisor) consumeNotifications(ctx context.Context, session *session) {
	for {
		select {
		case <-ctx.Done():
			return
		case notification, ok := <-session.client.Notifications():
			if !ok {
				s.recordSessionError(session.record.IdentityName, session.record.DID, fmt.Errorf("websocket notification loop closed"))
				return
			}
			s.handleNotification(ctx, session, notification)
		}
	}
}

func (s *Supervisor) handleNotification(ctx context.Context, session *session, notification map[string]any) {
	method, _ := notification["method"].(string)
	if method != "direct.incoming" {
		return
	}
	params, ok := notification["params"].(map[string]any)
	if !ok {
		return
	}
	meta, _ := params["meta"].(map[string]any)
	body, _ := params["body"].(map[string]any)
	server, _ := params["server"].(map[string]any)
	target, _ := meta["target"].(map[string]any)
	targetDID := stringValue(target["did"])
	senderDID := stringValue(meta["sender_did"])
	if targetDID == "" || senderDID == "" {
		return
	}
	content := stringValue(body["text"])
	contentType := stringValue(meta["content_type"])
	if contentType == "" {
		contentType = "text/plain"
	}
	record := store.MessageRecord{
		MsgID:          stringValue(meta["message_id"]),
		OwnerDID:       targetDID,
		ThreadID:       store.MakeThreadID(targetDID, senderDID, ""),
		Direction:      0,
		SenderDID:      senderDID,
		ReceiverDID:    targetDID,
		ContentType:    contentType,
		Content:        content,
		SentAt:         stringValue(server["received_at"]),
		IsRead:         false,
		Metadata:       metadataValue(params),
		CredentialName: session.record.IdentityName,
	}
	_ = store.StoreMessage(ctx, s.db, record)
}

func (s *Supervisor) refreshStatus() {
	s.sessionsMu.Lock()
	sessions := make([]SessionStatus, 0, len(s.sessions))
	for identityName, session := range s.sessions {
		sessions = append(sessions, SessionStatus{
			IdentityName: identityName,
			DID:          session.record.DID,
			Connected:    session.connected,
			LastError:    session.lastError,
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
		existingSession = &session{record: &identity.StoredIdentity{IdentityName: identityName, DID: did}}
		s.sessions[identityName] = existingSession
	}
	existingSession.connected = false
	if err != nil {
		existingSession.lastError = err.Error()
	}
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

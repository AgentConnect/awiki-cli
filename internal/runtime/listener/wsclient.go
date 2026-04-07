package listener

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agentconnect/awiki-cli/internal/authsdk"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/message"
	"github.com/coder/websocket"
)

type WSClient struct {
	requestURL    string
	websocketURL  string
	httpClient    *http.Client
	auth          *authsdk.Session
	conn          *websocket.Conn
	nextID        int64
	pendingMu     sync.Mutex
	pending       map[string]chan map[string]any
	notifications chan map[string]any
	readerErr     atomic.Value
	writeMu       sync.Mutex
}

func NewWSClient(resolved *appconfig.Resolved, auth *authsdk.Session) (*WSClient, error) {
	if auth == nil {
		return nil, fmt.Errorf("auth session is required for websocket mode")
	}
	targetHTTPURL := strings.TrimSpace(resolved.MessageServiceURL)
	if targetHTTPURL == "" {
		return nil, fmt.Errorf("message service url is required for websocket mode")
	}
	targetWSURL := strings.TrimSpace(resolved.MessageServiceWSURL)
	if targetWSURL == "" {
		targetWSURL = strings.Replace(targetHTTPURL, "https://", "wss://", 1)
		targetWSURL = strings.Replace(targetWSURL, "http://", "ws://", 1)
	}
	targetHTTPURL = strings.TrimRight(targetHTTPURL, "/") + message.MessageWSEndpoint
	targetWSURL = strings.TrimRight(targetWSURL, "/") + message.MessageWSEndpoint
	return &WSClient{
		requestURL:    targetHTTPURL,
		websocketURL:  targetWSURL,
		httpClient:    &http.Client{},
		auth:          auth,
		pending:       map[string]chan map[string]any{},
		notifications: make(chan map[string]any, 128),
	}, nil
}

func (c *WSClient) Connect(ctx context.Context) error {
	headers, err := c.auth.Headers(c.requestURL, http.MethodGet, nil, false)
	if err != nil {
		return err
	}
	conn, response, err := c.dial(ctx, headers)
	if err != nil {
		if response != nil && response.StatusCode == http.StatusUnauthorized {
			var retryHeaders map[string]string
			if c.auth.ShouldRetryAfter401(response.Header) {
				retryHeaders, err = c.auth.ChallengeHeaders(c.requestURL, response.Header, http.MethodGet, nil)
			} else {
				c.auth.ClearToken(c.requestURL)
				retryHeaders, err = c.auth.Headers(c.requestURL, http.MethodGet, nil, true)
			}
			if err != nil {
				return err
			}
			conn, response, err = c.dial(ctx, retryHeaders)
		}
		if err != nil {
			return err
		}
	}
	if response != nil {
		c.auth.CaptureToken(c.requestURL, response.Header)
	}
	c.conn = conn
	go c.readLoop()
	return nil
}

func (c *WSClient) dial(ctx context.Context, headers map[string]string) (*websocket.Conn, *http.Response, error) {
	httpHeaders := http.Header{}
	for key, value := range headers {
		httpHeaders.Set(key, value)
	}
	return websocket.Dial(ctx, c.websocketURL, &websocket.DialOptions{
		HTTPClient:      c.httpClient,
		HTTPHeader:      httpHeaders,
		CompressionMode: websocket.CompressionDisabled,
	})
}

func (c *WSClient) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close(websocket.StatusNormalClosure, "closing")
}

func (c *WSClient) Notifications() <-chan map[string]any {
	return c.notifications
}

func (c *WSClient) SendRPC(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("websocket not connected")
	}
	id := atomic.AddInt64(&c.nextID, 1)
	requestID := fmt.Sprintf("req-%d", id)
	request := map[string]any{"jsonrpc": "2.0", "id": requestID, "method": method}
	if params != nil {
		request["params"] = params
	}
	responseCh := make(chan map[string]any, 1)
	c.pendingMu.Lock()
	c.pending[requestID] = responseCh
	c.pendingMu.Unlock()
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, requestID)
		c.pendingMu.Unlock()
	}()
	c.writeMu.Lock()
	err := wsjsonWrite(ctx, c.conn, request)
	c.writeMu.Unlock()
	if err != nil {
		return nil, err
	}
	select {
	case response := <-responseCh:
		if errValue, ok := response["error"].(map[string]any); ok {
			return nil, fmt.Errorf("json-rpc error %v: %v", errValue["code"], errValue["message"])
		}
		if result, ok := response["result"].(map[string]any); ok {
			return result, nil
		}
		return map[string]any{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *WSClient) Ping(ctx context.Context) error {
	if c.conn == nil {
		return fmt.Errorf("websocket not connected")
	}
	return c.conn.Ping(ctx)
}

func (c *WSClient) readLoop() {
	defer close(c.notifications)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		var message map[string]any
		err := wsjsonRead(ctx, c.conn, &message)
		cancel()
		if err != nil {
			c.readerErr.Store(err)
			c.failPending(err)
			return
		}
		if rawID, ok := message["id"]; ok {
			id := requestIDFromAny(rawID)
			c.pendingMu.Lock()
			responseCh := c.pending[id]
			c.pendingMu.Unlock()
			if responseCh != nil {
				responseCh <- message
			}
			continue
		}
		select {
		case c.notifications <- message:
		default:
		}
	}
}

func (c *WSClient) failPending(err error) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	for id, responseCh := range c.pending {
		responseCh <- map[string]any{"error": map[string]any{"message": err.Error()}, "id": id}
	}
}

func wsjsonWrite(ctx context.Context, conn *websocket.Conn, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, raw)
}

func wsjsonRead(ctx context.Context, conn *websocket.Conn, out any) error {
	_, raw, err := conn.Read(ctx)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func requestIDFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case int64:
		return fmt.Sprintf("%d", typed)
	case int:
		return fmt.Sprintf("%d", typed)
	case float64:
		return fmt.Sprintf("%.0f", typed)
	default:
		return ""
	}
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func hostForURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return parsed.Host
}

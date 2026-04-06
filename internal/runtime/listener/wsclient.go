package listener

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/coder/websocket"
)

type WSClient struct {
	url           string
	httpClient    *http.Client
	conn          *websocket.Conn
	nextID        int64
	pendingMu     sync.Mutex
	pending       map[int64]chan map[string]any
	notifications chan map[string]any
	readerErr     atomic.Value
	writeMu       sync.Mutex
}

func NewWSClient(resolved *appconfig.Resolved, jwtToken string) (*WSClient, error) {
	if strings.TrimSpace(jwtToken) == "" {
		return nil, fmt.Errorf("identity JWT token is required for websocket mode")
	}
	targetURL := strings.TrimSpace(resolved.MessageServiceWSURL)
	if targetURL == "" {
		targetURL = strings.TrimRight(resolved.MessageServiceURL, "/")
		targetURL = strings.Replace(targetURL, "https://", "wss://", 1)
		targetURL = strings.Replace(targetURL, "http://", "ws://", 1)
	}
	targetURL = strings.TrimRight(targetURL, "/") + "/message/ws?token=" + url.QueryEscape(jwtToken)
	httpClient := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}}
	return &WSClient{
		url:           targetURL,
		httpClient:    httpClient,
		pending:       map[int64]chan map[string]any{},
		notifications: make(chan map[string]any, 128),
	}, nil
}

func (c *WSClient) Connect(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, c.url, &websocket.DialOptions{
		HTTPClient:      c.httpClient,
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return err
	}
	c.conn = conn
	go c.readLoop()
	return nil
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
	request := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		request["params"] = params
	}
	responseCh := make(chan map[string]any, 1)
	c.pendingMu.Lock()
	c.pending[id] = responseCh
	c.pendingMu.Unlock()
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
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
			id := int64FromAny(rawID)
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

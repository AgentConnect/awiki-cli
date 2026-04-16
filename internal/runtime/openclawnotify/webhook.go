package openclawnotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type HookRequest struct {
	Message  string `json:"message"`
	Name     string `json:"name,omitempty"`
	WakeMode string `json:"wakeMode,omitempty"`
	Deliver  bool   `json:"deliver"`
	Channel  string `json:"channel,omitempty"`
	To       string `json:"to,omitempty"`
}

type HookResponse struct {
	RunID string `json:"run_id"`
}

type WebhookClient struct {
	httpClient *http.Client
	hookURL    string
	token      string
}

func NewWebhookClient(hookURL string, token string) (*WebhookClient, error) {
	hookURL = strings.TrimSpace(hookURL)
	if hookURL == "" {
		return nil, fmt.Errorf("openclaw host notify requires runtime.host_notify.openclaw.hook_url")
	}
	if err := ValidateHookURL(hookURL); err != nil {
		return nil, err
	}
	return &WebhookClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		hookURL:    hookURL,
		token:      strings.TrimSpace(token),
	}, nil
}

func (c *WebhookClient) Send(ctx context.Context, req HookRequest) (HookResponse, error) {
	raw, err := json.Marshal(req)
	if err != nil {
		return HookResponse{}, fmt.Errorf("marshal openclaw hook payload: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.hookURL, bytes.NewReader(raw))
	if err != nil {
		return HookResponse{}, fmt.Errorf("build openclaw hook request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return HookResponse{}, fmt.Errorf("send openclaw hook request: %w", err)
	}
	defer response.Body.Close()
	rawBody, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		if len(rawBody) == 0 {
			return HookResponse{}, fmt.Errorf("openclaw hook failed status=%d", response.StatusCode)
		}
		return HookResponse{}, fmt.Errorf("openclaw hook failed status=%d: %s", response.StatusCode, strings.TrimSpace(string(rawBody)))
	}
	var payload struct {
		OK    bool   `json:"ok"`
		RunID string `json:"runId"`
	}
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return HookResponse{}, fmt.Errorf("parse openclaw hook response: %w", err)
	}
	if !payload.OK {
		return HookResponse{}, fmt.Errorf("openclaw hook was not accepted")
	}
	if strings.TrimSpace(payload.RunID) == "" {
		return HookResponse{}, fmt.Errorf("openclaw hook response did not include runId")
	}
	return HookResponse{RunID: strings.TrimSpace(payload.RunID)}, nil
}

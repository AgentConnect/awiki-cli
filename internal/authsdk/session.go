package authsdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
)

type Session struct {
	helper       *anpsdk.DIDWbaAuthHeader
	identityName string
	did          string
	jwtToken     string
	persistToken func(string) error
	persistent   map[string]struct{}
}

func NewSession(didDocumentPath string, privateKeyPath string, identityName string, did string, jwtToken string, persistToken func(string) error) *Session {
	helper := anpsdk.NewDIDWbaAuthHeader(didDocumentPath, privateKeyPath, anpsdk.AuthModeHTTPSignatures)
	return &Session{
		helper:       helper,
		identityName: identityName,
		did:          did,
		jwtToken:     jwtToken,
		persistToken: persistToken,
		persistent:   map[string]struct{}{},
	}
}

// RememberScope marks a server scope as safe to persist bearer tokens returned
// by that scope. It is intentionally separate from SetBearer so first-time
// signed requests without an existing JWT can still store the token returned in
// Authentication-Info.
func (s *Session) RememberScope(serverURL string) {
	if s == nil {
		return
	}
	if scope := authScope(serverURL); scope != "" {
		s.persistent[scope] = struct{}{}
	}
}

func (s *Session) SetBearer(serverURL string, token string) {
	token = strings.TrimSpace(token)
	if s == nil || s.helper == nil || token == "" {
		return
	}
	s.helper.UpdateToken(serverURL, map[string]string{"Authorization": "Bearer " + token})
	s.RememberScope(serverURL)
	s.jwtToken = token
}

func (s *Session) CurrentJWT() string {
	if s == nil {
		return ""
	}
	return s.jwtToken
}

func (s *Session) Headers(serverURL string, method string, body []byte, forceNew bool) (map[string]string, error) {
	if s == nil {
		return nil, fmt.Errorf("auth session is not configured")
	}
	if s.helper == nil {
		return nil, fmt.Errorf("auth session is not configured")
	}
	baseHeaders := map[string]string{"Content-Type": "application/json"}
	authHeaders, err := s.helper.GetAuthHeader(serverURL, forceNew, method, baseHeaders, body)
	if err != nil {
		return nil, err
	}
	for key, value := range authHeaders {
		baseHeaders[key] = value
	}
	return baseHeaders, nil
}

func (s *Session) ShouldRetryAfter401(headers http.Header) bool {
	if s == nil || s.helper == nil {
		return false
	}
	return s.helper.ShouldRetryAfter401(flattenHeaders(headers))
}

func (s *Session) ChallengeHeaders(serverURL string, headers http.Header, method string, body []byte) (map[string]string, error) {
	if s == nil || s.helper == nil {
		return nil, fmt.Errorf("auth session is not configured")
	}
	baseHeaders := map[string]string{"Content-Type": "application/json"}
	return s.helper.GetChallengeAuthHeader(serverURL, flattenHeaders(headers), method, baseHeaders, body)
}

func (s *Session) ClearToken(serverURL string) {
	if s == nil || s.helper == nil {
		return
	}
	s.helper.ClearToken(serverURL)
	if s.isPersistentScope(serverURL) {
		s.jwtToken = ""
	}
}

func (s *Session) CaptureToken(serverURL string, headers http.Header) string {
	if s == nil || s.helper == nil {
		return ""
	}
	token := s.helper.UpdateToken(serverURL, flattenHeaders(headers))
	if token == "" {
		return ""
	}
	if s.isPersistentScope(serverURL) {
		s.jwtToken = token
		if s.persistToken != nil {
			_ = s.persistToken(token)
		}
	}
	return token
}

func (s *Session) isPersistentScope(serverURL string) bool {
	if s == nil {
		return false
	}
	_, ok := s.persistent[authScope(serverURL)]
	return ok
}

func authScope(serverURL string) string {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return strings.TrimSpace(serverURL)
	}
	if host := parsed.Hostname(); host != "" {
		return host
	}
	return strings.TrimSpace(serverURL)
}

func (s *Session) DoJSONRPC(ctx context.Context, client *http.Client, requestURL string, method string, rpcMethod string, params any, out any) error {
	if client == nil {
		client = http.DefaultClient
	}
	payload := map[string]any{"jsonrpc": "2.0", "id": "req-1", "method": rpcMethod, "params": params}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	response, err := s.doRequest(ctx, client, requestURL, method, body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	var decoded struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    any    `json:"data,omitempty"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	if decoded.Error != nil {
		return &RPCError{Code: decoded.Error.Code, Message: decoded.Error.Message, Data: decoded.Error.Data}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(decoded.Result, out)
}

func (s *Session) EnsureJWT(ctx context.Context, client *http.Client, requestURL string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("auth session is not configured")
	}
	s.RememberScope(requestURL)
	var result map[string]any
	if err := s.DoJSONRPC(ctx, client, requestURL, http.MethodPost, "get_me", map[string]any{}, &result); err != nil {
		return "", err
	}
	if token, _ := result["access_token"].(string); strings.TrimSpace(token) != "" {
		s.SetBearer(requestURL, token)
		s.jwtToken = strings.TrimSpace(token)
		if s.persistToken != nil {
			_ = s.persistToken(s.jwtToken)
		}
		return s.jwtToken, nil
	}
	if token := strings.TrimSpace(s.jwtToken); token != "" {
		return token, nil
	}
	return "", fmt.Errorf("did-auth get_me succeeded but no access token was returned")
}

func (s *Session) DoJSON(ctx context.Context, client *http.Client, method string, requestURL string, payload any, out any) error {
	if client == nil {
		client = http.DefaultClient
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	response, err := s.doRequest(ctx, client, requestURL, method, body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if out == nil {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	return json.NewDecoder(response.Body).Decode(out)
}

func (s *Session) doRequest(ctx context.Context, client *http.Client, requestURL string, method string, body []byte) (*http.Response, error) {
	headers, err := s.Headers(requestURL, method, body, false)
	if err != nil {
		return nil, err
	}
	response, err := doHTTPRequest(ctx, client, method, requestURL, body, headers)
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusUnauthorized {
		defer response.Body.Close()
		if s.ShouldRetryAfter401(response.Header) {
			headers, err = s.ChallengeHeaders(requestURL, response.Header, method, body)
			if err != nil {
				return nil, err
			}
		} else if s.helper != nil {
			s.ClearToken(requestURL)
			headers, err = s.Headers(requestURL, method, body, true)
			if err != nil {
				return nil, err
			}
		} else {
			raw, _ := io.ReadAll(response.Body)
			return nil, &HTTPError{StatusCode: response.StatusCode, Message: strings.TrimSpace(string(raw))}
		}
		response, err = doHTTPRequest(ctx, client, method, requestURL, body, headers)
		if err != nil {
			return nil, err
		}
	}
	if response.StatusCode >= 400 {
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		return nil, &HTTPError{StatusCode: response.StatusCode, Message: strings.TrimSpace(string(raw))}
	}
	s.CaptureToken(requestURL, response.Header)
	return response, nil
}

func doHTTPRequest(ctx context.Context, client *http.Client, method string, requestURL string, body []byte, headers map[string]string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	return client.Do(request)
}

func flattenHeaders(headers http.Header) map[string]string {
	values := make(map[string]string, len(headers))
	for key, item := range headers {
		if len(item) == 0 {
			continue
		}
		values[key] = item[0]
	}
	return values
}

type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("http error %d: %s", e.StatusCode, e.Message)
}

type RPCError struct {
	Code    int
	Message string
	Data    any
}

func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

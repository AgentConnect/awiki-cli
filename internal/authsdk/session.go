package authsdk

import (
	"bytes"
	"context"
	"encoding/asn1"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
)

type Session struct {
	helper       *anpsdk.DIDWbaAuthHeader
	identityName string
	did          string
	jwtToken     string
	persistToken func(string) error
}

func NewSession(didDocumentPath string, privateKeyPath string, identityName string, did string, jwtToken string, persistToken func(string) error) *Session {
	normalizedKeyPath := privateKeyPath
	if rewrittenPath, err := normalizeSecp256k1PrivateKeyPath(privateKeyPath); err == nil && strings.TrimSpace(rewrittenPath) != "" {
		normalizedKeyPath = rewrittenPath
	}
	helper := anpsdk.NewDIDWbaAuthHeader(didDocumentPath, normalizedKeyPath, anpsdk.AuthModeHTTPSignatures)
	session := &Session{
		helper:       helper,
		identityName: identityName,
		did:          did,
		jwtToken:     jwtToken,
		persistToken: persistToken,
	}
	if strings.TrimSpace(jwtToken) != "" {
		headers := map[string]string{"Authorization": "Bearer " + jwtToken}
		helper.UpdateToken("https://awiki.ai", headers)
	}
	return session
}

func (s *Session) SetBearer(serverURL string, token string) {
	if s == nil || s.helper == nil || strings.TrimSpace(token) == "" {
		return
	}
	s.helper.UpdateToken(serverURL, map[string]string{"Authorization": "Bearer " + token})
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
	if strings.TrimSpace(s.jwtToken) != "" && !forceNew {
		return map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + s.jwtToken,
		}, nil
	}
	if s.helper == nil {
		return nil, fmt.Errorf("auth session is not configured")
	}
	headers := map[string]string{"Content-Type": "application/json"}
	return s.helper.GetAuthHeader(serverURL, forceNew, method, headers, body)
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
	s.jwtToken = ""
}

func (s *Session) CaptureToken(serverURL string, headers http.Header) string {
	if s == nil || s.helper == nil {
		return ""
	}
	token := s.helper.UpdateToken(serverURL, flattenHeaders(headers))
	if token == "" {
		return ""
	}
	s.jwtToken = token
	if s.persistToken != nil {
		_ = s.persistToken(token)
	}
	return token
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
	var result map[string]any
	if err := s.DoJSONRPC(ctx, client, requestURL, http.MethodPost, "get_me", map[string]any{}, &result); err != nil {
		return "", err
	}
	return s.jwtToken, nil
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

type pkcs8PrivateKeyInfo struct {
	Version    int
	Algorithm  pkixAlgorithmIdentifier
	PrivateKey []byte
}

type pkixAlgorithmIdentifier struct {
	Algorithm  asn1.ObjectIdentifier
	Parameters asn1.RawValue `asn1:"optional"`
}

type sec1ECPrivateKey struct {
	Version    int
	PrivateKey []byte
}

func normalizeSecp256k1PrivateKeyPath(privateKeyPath string) (string, error) {
	privateKeyPath = strings.TrimSpace(privateKeyPath)
	if privateKeyPath == "" {
		return "", nil
	}
	raw, err := os.ReadFile(filepath.Clean(privateKeyPath))
	if err != nil {
		return "", err
	}
	normalizedPEM, _, err := NormalizeSecp256k1PrivatePEM(string(raw))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(normalizedPEM) == strings.TrimSpace(string(raw)) {
		return privateKeyPath, nil
	}
	dir := filepath.Dir(privateKeyPath)
	target := filepath.Join(dir, ".awiki-cli-key-1-private-normalized.pem")
	if writeErr := os.WriteFile(target, []byte(normalizedPEM), 0o600); writeErr != nil {
		return "", writeErr
	}
	return target, nil
}

func NormalizeSecp256k1PrivatePEM(pemText string) (string, []byte, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemText)))
	if block == nil {
		return "", nil, fmt.Errorf("invalid key-1 private key pem")
	}
	switch block.Type {
	case "ANP SECP256K1 PRIVATE KEY":
		return strings.TrimSpace(pemText), append([]byte(nil), block.Bytes...), nil
	case "PRIVATE KEY":
		scalar, err := extractSecp256k1ScalarFromPKCS8(block.Bytes)
		if err != nil {
			return "", nil, err
		}
		normalized := pem.EncodeToMemory(&pem.Block{Type: "ANP SECP256K1 PRIVATE KEY", Bytes: scalar})
		return string(normalized), scalar, nil
	default:
		return "", nil, fmt.Errorf("invalid key-1 private key label: %s", block.Type)
	}
}

func extractSecp256k1ScalarFromPKCS8(der []byte) ([]byte, error) {
	var info pkcs8PrivateKeyInfo
	if _, err := asn1.Unmarshal(der, &info); err != nil {
		return nil, fmt.Errorf("parse pkcs8 private key: %w", err)
	}
	var sec1 sec1ECPrivateKey
	if _, err := asn1.Unmarshal(info.PrivateKey, &sec1); err != nil {
		return nil, fmt.Errorf("parse embedded sec1 private key: %w", err)
	}
	if len(sec1.PrivateKey) == 0 {
		return nil, fmt.Errorf("embedded sec1 private key is empty")
	}
	if len(sec1.PrivateKey) > 32 {
		sec1.PrivateKey = sec1.PrivateKey[len(sec1.PrivateKey)-32:]
	}
	if len(sec1.PrivateKey) < 32 {
		padded := make([]byte, 32)
		copy(padded[32-len(sec1.PrivateKey):], sec1.PrivateKey)
		sec1.PrivateKey = padded
	}
	return append([]byte(nil), sec1.PrivateKey...), nil
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

package identity

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

const (
	didAuthRPCEndpoint    = "/user-service/did-auth/rpc"
	handleRPCEndpoint     = "/user-service/handle/rpc"
	didProfileRPCEndpoint = "/user-service/did/profile/rpc"

	emailSendEndpoint       = "/user-service/auth/email-send"
	emailStatusEndpoint     = "/user-service/auth/email-status"
	phoneBindSendEndpoint   = "/user-service/auth/phone-bind-send"
	phoneBindVerifyEndpoint = "/user-service/auth/phone-bind-verify"
)

type ServiceError struct {
	StatusCode int
	RPCCode    int
	Message    string
	Data       any
}

func (e *ServiceError) Error() string {
	if e == nil {
		return ""
	}
	switch {
	case e.RPCCode != 0:
		return fmt.Sprintf("service rpc error %d: %s", e.RPCCode, e.Message)
	case e.StatusCode != 0:
		return fmt.Sprintf("service http error %d: %s", e.StatusCode, e.Message)
	default:
		return e.Message
	}
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    any    `json:"data,omitempty"`
	} `json:"error,omitempty"`
}

type RemoteClient struct {
	baseURL string
	client  *http.Client
}

func NewRemoteClient(resolved *appconfig.Resolved) (*RemoteClient, error) {
	if resolved == nil {
		return nil, fmt.Errorf("%w: resolved config is required", ErrInvalidInput)
	}
	httpClient, err := newHTTPClient(resolved.CABundle)
	if err != nil {
		return nil, err
	}
	return &RemoteClient{
		baseURL: strings.TrimRight(resolved.UserServiceURL, "/"),
		client:  httpClient,
	}, nil
}

func newHTTPClient(caBundle string) (*http.Client, error) {
	transport := &http.Transport{}
	if strings.TrimSpace(caBundle) != "" {
		rootCAs, err := x509.SystemCertPool()
		if err != nil || rootCAs == nil {
			rootCAs = x509.NewCertPool()
		}
		bundle, err := os.ReadFile(filepath.Clean(caBundle))
		if err != nil {
			return nil, fmt.Errorf("read ca bundle: %w", err)
		}
		if ok := rootCAs.AppendCertsFromPEM(bundle); !ok {
			return nil, fmt.Errorf("invalid ca bundle: %s", caBundle)
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: rootCAs, MinVersion: tls.VersionTLS12}
	}
	return &http.Client{Transport: transport}, nil
}

func (c *RemoteClient) rpcCall(ctx context.Context, endpoint string, method string, params any, bearer string, out any) error {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      1,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode >= 400 {
		return &ServiceError{
			StatusCode: response.StatusCode,
			Message:    strings.TrimSpace(string(raw)),
		}
	}
	var decoded rpcResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fmt.Errorf("parse rpc response: %w", err)
	}
	if decoded.Error != nil {
		return &ServiceError{
			RPCCode: decoded.Error.Code,
			Message: decoded.Error.Message,
			Data:    decoded.Error.Data,
		}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(decoded.Result, out)
}

func (c *RemoteClient) restPost(ctx context.Context, endpoint string, requestPayload any, bearer string, out any) error {
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode >= 400 {
		return &ServiceError{
			StatusCode: response.StatusCode,
			Message:    strings.TrimSpace(string(raw)),
		}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *RemoteClient) restGet(ctx context.Context, endpoint string, query url.Values, out any) error {
	target := c.baseURL + endpoint
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode >= 400 {
		return &ServiceError{
			StatusCode: response.StatusCode,
			Message:    strings.TrimSpace(string(raw)),
		}
	}
	return json.Unmarshal(raw, out)
}

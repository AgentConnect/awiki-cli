package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/authsdk"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/traceutil"
	"github.com/agentconnect/awiki-cli/internal/transportcfg"
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

type HandleLookupResult struct {
	Handle     string `json:"handle"`
	DID        string `json:"did"`
	Domain     string `json:"domain"`
	FullHandle string `json:"full_handle"`
	Status     string `json:"status"`
}

func NewRemoteClient(resolved *appconfig.Resolved) (*RemoteClient, error) {
	if resolved == nil {
		return nil, fmt.Errorf("%w: resolved config is required", ErrInvalidInput)
	}
	httpClient, err := transportcfg.NewHTTPClient(resolved.CABundle)
	if err != nil {
		return nil, err
	}
	return &RemoteClient{
		baseURL: appconfig.NormalizeBaseURL(resolved.ServiceBaseURL),
		client:  httpClient,
	}, nil
}

func (c *RemoteClient) Client() *http.Client {
	if c == nil {
		return nil
	}
	return c.client
}

func (c *RemoteClient) rpcCall(ctx context.Context, endpoint string, method string, params any, bearer string, out any) error {
	return c.rpcCallProfile(ctx, transportcfg.ProfileRPCDefault, endpoint, method, params, bearer, out)
}

func (c *RemoteClient) rpcCallProfile(ctx context.Context, profile transportcfg.Profile, endpoint string, method string, params any, bearer string, out any) error {
	timeoutCtx, cancel := transportcfg.WithProfileTimeout(ctx, profile)
	defer cancel()
	finish := traceutil.RPCPhase(ctx, method)
	defer finish()
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      "req-1",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(timeoutCtx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(body))
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

func (c *RemoteClient) RPCCall(ctx context.Context, endpoint string, method string, params any, bearer string, out any) error {
	return c.rpcCall(ctx, endpoint, method, params, bearer, out)
}

func (c *RemoteClient) AuthenticatedRPCCall(ctx context.Context, endpoint string, method string, params any, auth *authsdk.Session, out any) error {
	return c.AuthenticatedRPCCallProfile(ctx, transportcfg.ProfileRPCDefault, endpoint, method, params, auth, out)
}

func (c *RemoteClient) AuthenticatedRPCCallProfile(ctx context.Context, profile transportcfg.Profile, endpoint string, method string, params any, auth *authsdk.Session, out any) error {
	timeoutCtx, cancel := transportcfg.WithProfileTimeout(ctx, profile)
	defer cancel()
	finish := traceutil.RPCPhase(ctx, method)
	defer finish()
	return c.authenticatedRPCCall(timeoutCtx, endpoint, method, params, auth, out)
}

func (c *RemoteClient) restPost(ctx context.Context, endpoint string, requestPayload any, bearer string, out any) error {
	return c.restPostProfile(ctx, transportcfg.ProfileRPCDefault, endpoint, requestPayload, bearer, out)
}

func (c *RemoteClient) restPostProfile(ctx context.Context, profile transportcfg.Profile, endpoint string, requestPayload any, bearer string, out any) error {
	timeoutCtx, cancel := transportcfg.WithProfileTimeout(ctx, profile)
	defer cancel()
	finish := traceutil.RPCPhase(ctx, http.MethodPost+" "+endpoint)
	defer finish()
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(timeoutCtx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(body))
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

func (c *RemoteClient) RestPost(ctx context.Context, endpoint string, requestPayload any, bearer string, out any) error {
	return c.restPost(ctx, endpoint, requestPayload, bearer, out)
}

func (c *RemoteClient) AuthenticatedRestPost(ctx context.Context, endpoint string, requestPayload any, auth *authsdk.Session, out any) error {
	timeoutCtx, cancel := transportcfg.WithProfileTimeout(ctx, transportcfg.ProfileRPCDefault)
	defer cancel()
	finish := traceutil.RPCPhase(ctx, http.MethodPost+" "+endpoint)
	defer finish()
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return err
	}
	requestURL := c.baseURL + endpoint
	if err := auth.DoJSON(timeoutCtx, c.client, http.MethodPost, requestURL, requestPayload, out); err != nil {
		var httpErr *authsdk.HTTPError
		if errors.As(err, &httpErr) {
			return &ServiceError{StatusCode: httpErr.StatusCode, Message: httpErr.Message}
		}
		return err
	}
	_ = body
	return nil
}

func (c *RemoteClient) restGet(ctx context.Context, endpoint string, query url.Values, bearer string, out any) error {
	timeoutCtx, cancel := transportcfg.WithProfileTimeout(ctx, transportcfg.ProfileRPCDefault)
	defer cancel()
	finish := traceutil.RPCPhase(ctx, http.MethodGet+" "+endpoint)
	defer finish()
	target := c.baseURL + endpoint
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(timeoutCtx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	if bearer = strings.TrimSpace(bearer); bearer != "" {
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
	return json.Unmarshal(raw, out)
}

func (c *RemoteClient) RestGet(ctx context.Context, endpoint string, query url.Values, out any) error {
	return c.restGet(ctx, endpoint, query, "", out)
}

func (c *RemoteClient) LookupHandleByDID(ctx context.Context, did string) (*HandleLookupResult, error) {
	did = strings.TrimSpace(did)
	if did == "" {
		return nil, fmt.Errorf("%w: did is required", ErrInvalidInput)
	}
	var result HandleLookupResult
	if err := c.rpcCall(ctx, handleRPCEndpoint, "lookup", map[string]any{"did": did}, "", &result); err != nil {
		var serviceErr *ServiceError
		if errors.As(err, &serviceErr) {
			if serviceErr.StatusCode == http.StatusNotFound || serviceErr.RPCCode == -32002 {
				return nil, nil
			}
		}
		return nil, err
	}
	if strings.TrimSpace(result.Handle) == "" || strings.TrimSpace(result.DID) == "" {
		return nil, nil
	}
	return &result, nil
}

func (c *RemoteClient) authenticatedRPCCall(ctx context.Context, endpoint string, method string, params any, auth *authsdk.Session, out any) error {
	requestURL := c.baseURL + endpoint
	if err := auth.DoJSONRPC(ctx, c.client, requestURL, http.MethodPost, method, params, out); err != nil {
		var rpcErr *authsdk.RPCError
		if errors.As(err, &rpcErr) {
			return &ServiceError{RPCCode: rpcErr.Code, Message: rpcErr.Message, Data: rpcErr.Data}
		}
		var httpErr *authsdk.HTTPError
		if errors.As(err, &httpErr) {
			return &ServiceError{StatusCode: httpErr.StatusCode, Message: httpErr.Message}
		}
		return err
	}
	return nil
}

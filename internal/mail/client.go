package mail

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/authsdk"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
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

type Client struct {
	baseURL string
	client  *http.Client
}

func NewClient(resolved *appconfig.Resolved) (*Client, error) {
	if resolved == nil {
		return nil, fmt.Errorf("resolved config is required")
	}
	if strings.TrimSpace(resolved.MailServiceURL) == "" {
		return nil, fmt.Errorf("mail service url is required")
	}
	httpClient, err := newHTTPClient(resolved.CABundle)
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL: strings.TrimRight(resolved.MailServiceURL, "/"),
		client:  httpClient,
	}, nil
}

func (c *Client) AuthenticatedRPCCall(ctx context.Context, endpoint string, method string, params any, auth *authsdk.Session, out any) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("mail client is not configured")
	}
	if auth == nil {
		return fmt.Errorf("auth session is required")
	}
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

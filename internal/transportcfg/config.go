package transportcfg

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Profile string

const (
	ProfileBridgeFastPath Profile = "bridge_fast_path"
	ProfileHealthProbe    Profile = "health_probe"
	ProfileAuthRefresh    Profile = "auth_refresh"
	ProfileRPCDefault     Profile = "rpc_default"
	ProfileRPCReadHeavy   Profile = "rpc_read_heavy"
)

const (
	defaultBridgeHealthProbeTimeout = 750 * time.Millisecond
	defaultBridgeDialTimeout        = 1 * time.Second
	defaultBridgeWriteTimeout       = 1 * time.Second
	defaultBridgeReadTimeout        = 3 * time.Second

	defaultHTTPDialTimeout           = 8 * time.Second
	defaultHTTPKeepAlive             = 30 * time.Second
	defaultHTTPTLSHandshakeTimeout   = 8 * time.Second
	defaultHTTPResponseHeaderTimeout = 30 * time.Second
	defaultHTTPIdleConnTimeout       = 90 * time.Second
	defaultHTTPMaxIdleConns          = 32
	defaultHTTPMaxIdleConnsPerHost   = 8

	defaultProfileBridgeFastPath = 1500 * time.Millisecond
	defaultProfileHealthProbe    = 750 * time.Millisecond
	defaultProfileAuthRefresh    = 20 * time.Second
	defaultProfileRPCDefault     = 25 * time.Second
	defaultProfileRPCReadHeavy   = 35 * time.Second
)

type Config struct {
	BridgeHealthProbeTimeout time.Duration
	BridgeDialTimeout        time.Duration
	BridgeWriteTimeout       time.Duration
	BridgeReadTimeout        time.Duration

	HTTPDialTimeout           time.Duration
	HTTPKeepAlive             time.Duration
	HTTPTLSHandshakeTimeout   time.Duration
	HTTPResponseHeaderTimeout time.Duration
	HTTPIdleConnTimeout       time.Duration
	HTTPMaxIdleConns          int
	HTTPMaxIdleConnsPerHost   int

	ProfileTimeouts map[Profile]time.Duration
}

func Resolve() Config {
	config := Config{
		BridgeHealthProbeTimeout: durationFromEnv("AWIKI_CLI_TIMEOUT_BRIDGE_HEALTH_PROBE", defaultBridgeHealthProbeTimeout),
		BridgeDialTimeout:        durationFromEnv("AWIKI_CLI_TIMEOUT_BRIDGE_DIAL", defaultBridgeDialTimeout),
		BridgeWriteTimeout:       durationFromEnv("AWIKI_CLI_TIMEOUT_BRIDGE_WRITE", defaultBridgeWriteTimeout),
		BridgeReadTimeout:        durationFromEnv("AWIKI_CLI_TIMEOUT_BRIDGE_READ", defaultBridgeReadTimeout),

		// Remote HTTP requests run over a cross-region path to Silicon Valley and
		// should tolerate higher RTT, packet loss, and auth-refresh retries.
		HTTPDialTimeout:           durationFromEnv("AWIKI_CLI_TIMEOUT_HTTP_DIAL", defaultHTTPDialTimeout),
		HTTPKeepAlive:             durationFromEnv("AWIKI_CLI_TIMEOUT_HTTP_KEEPALIVE", defaultHTTPKeepAlive),
		HTTPTLSHandshakeTimeout:   durationFromEnv("AWIKI_CLI_TIMEOUT_HTTP_TLS_HANDSHAKE", defaultHTTPTLSHandshakeTimeout),
		HTTPResponseHeaderTimeout: durationFromEnv("AWIKI_CLI_TIMEOUT_HTTP_RESPONSE_HEADER", defaultHTTPResponseHeaderTimeout),
		HTTPIdleConnTimeout:       durationFromEnv("AWIKI_CLI_TIMEOUT_HTTP_IDLE_CONN", defaultHTTPIdleConnTimeout),
		HTTPMaxIdleConns:          intFromEnv("AWIKI_CLI_HTTP_MAX_IDLE_CONNS", defaultHTTPMaxIdleConns),
		HTTPMaxIdleConnsPerHost:   intFromEnv("AWIKI_CLI_HTTP_MAX_IDLE_CONNS_PER_HOST", defaultHTTPMaxIdleConnsPerHost),
		ProfileTimeouts: map[Profile]time.Duration{
			ProfileBridgeFastPath: durationFromEnv("AWIKI_CLI_TIMEOUT_PROFILE_BRIDGE_FAST_PATH", defaultProfileBridgeFastPath),
			ProfileHealthProbe:    durationFromEnv("AWIKI_CLI_TIMEOUT_PROFILE_HEALTH_PROBE", defaultProfileHealthProbe),
			ProfileAuthRefresh:    durationFromEnv("AWIKI_CLI_TIMEOUT_PROFILE_AUTH_REFRESH", defaultProfileAuthRefresh),
			ProfileRPCDefault:     durationFromEnv("AWIKI_CLI_TIMEOUT_PROFILE_RPC_DEFAULT", defaultProfileRPCDefault),
			ProfileRPCReadHeavy:   durationFromEnv("AWIKI_CLI_TIMEOUT_PROFILE_RPC_READ_HEAVY", defaultProfileRPCReadHeavy),
		},
	}
	if config.HTTPMaxIdleConns < 1 {
		config.HTTPMaxIdleConns = defaultHTTPMaxIdleConns
	}
	if config.HTTPMaxIdleConnsPerHost < 1 {
		config.HTTPMaxIdleConnsPerHost = defaultHTTPMaxIdleConnsPerHost
	}
	return config
}

func (c Config) TimeoutForProfile(profile Profile) time.Duration {
	if timeout, ok := c.ProfileTimeouts[profile]; ok && timeout > 0 {
		return timeout
	}
	return c.ProfileTimeouts[ProfileRPCDefault]
}

func WithProfileTimeout(ctx context.Context, profile Profile) (context.Context, context.CancelFunc) {
	timeout := Resolve().TimeoutForProfile(profile)
	if timeout <= 0 {
		return ctx, func() {}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if deadline, ok := ctx.Deadline(); ok {
		if time.Until(deadline) <= timeout {
			return ctx, func() {}
		}
	}
	return context.WithTimeout(ctx, timeout)
}

func NewHTTPClient(caBundle string) (*http.Client, error) {
	config := Resolve()
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   config.HTTPDialTimeout,
			KeepAlive: config.HTTPKeepAlive,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   config.HTTPTLSHandshakeTimeout,
		ResponseHeaderTimeout: config.HTTPResponseHeaderTimeout,
		IdleConnTimeout:       config.HTTPIdleConnTimeout,
		MaxIdleConns:          config.HTTPMaxIdleConns,
		MaxIdleConnsPerHost:   config.HTTPMaxIdleConnsPerHost,
	}
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

func durationFromEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
		return parsed
	}
	if millis, err := strconv.Atoi(raw); err == nil && millis > 0 {
		return time.Duration(millis) * time.Millisecond
	}
	return fallback
}

func intFromEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

package transportcfg

import (
	"testing"
	"time"
)

func TestResolveDefaults(t *testing.T) {
	unsetTransportEnv(t)

	config := Resolve()
	if config.BridgeHealthProbeTimeout != defaultBridgeHealthProbeTimeout {
		t.Fatalf("BridgeHealthProbeTimeout = %s, want %s", config.BridgeHealthProbeTimeout, defaultBridgeHealthProbeTimeout)
	}
	if config.BridgeDialTimeout != defaultBridgeDialTimeout {
		t.Fatalf("BridgeDialTimeout = %s, want %s", config.BridgeDialTimeout, defaultBridgeDialTimeout)
	}
	if config.BridgeWriteTimeout != defaultBridgeWriteTimeout {
		t.Fatalf("BridgeWriteTimeout = %s, want %s", config.BridgeWriteTimeout, defaultBridgeWriteTimeout)
	}
	if config.BridgeReadTimeout != defaultBridgeReadTimeout {
		t.Fatalf("BridgeReadTimeout = %s, want %s", config.BridgeReadTimeout, defaultBridgeReadTimeout)
	}
	if config.HTTPDialTimeout != defaultHTTPDialTimeout {
		t.Fatalf("HTTPDialTimeout = %s, want %s", config.HTTPDialTimeout, defaultHTTPDialTimeout)
	}
	if config.HTTPTLSHandshakeTimeout != defaultHTTPTLSHandshakeTimeout {
		t.Fatalf("HTTPTLSHandshakeTimeout = %s, want %s", config.HTTPTLSHandshakeTimeout, defaultHTTPTLSHandshakeTimeout)
	}
	if config.HTTPResponseHeaderTimeout != defaultHTTPResponseHeaderTimeout {
		t.Fatalf("HTTPResponseHeaderTimeout = %s, want %s", config.HTTPResponseHeaderTimeout, defaultHTTPResponseHeaderTimeout)
	}
	if got := config.TimeoutForProfile(ProfileAuthRefresh); got != defaultProfileAuthRefresh {
		t.Fatalf("ProfileAuthRefresh timeout = %s, want %s", got, defaultProfileAuthRefresh)
	}
	if got := config.TimeoutForProfile(ProfileRPCDefault); got != defaultProfileRPCDefault {
		t.Fatalf("ProfileRPCDefault timeout = %s, want %s", got, defaultProfileRPCDefault)
	}
	if got := config.TimeoutForProfile(ProfileRPCReadHeavy); got != defaultProfileRPCReadHeavy {
		t.Fatalf("ProfileRPCReadHeavy timeout = %s, want %s", got, defaultProfileRPCReadHeavy)
	}
}

func TestResolveHonorsEnvOverrides(t *testing.T) {
	unsetTransportEnv(t)

	t.Setenv("AWIKI_CLI_TIMEOUT_HTTP_DIAL", "11s")
	t.Setenv("AWIKI_CLI_TIMEOUT_HTTP_RESPONSE_HEADER", "42s")
	t.Setenv("AWIKI_CLI_TIMEOUT_PROFILE_RPC_DEFAULT", "31s")
	t.Setenv("AWIKI_CLI_TIMEOUT_PROFILE_RPC_READ_HEAVY", "47000")

	config := Resolve()
	if config.HTTPDialTimeout != 11*time.Second {
		t.Fatalf("HTTPDialTimeout = %s, want 11s", config.HTTPDialTimeout)
	}
	if config.HTTPResponseHeaderTimeout != 42*time.Second {
		t.Fatalf("HTTPResponseHeaderTimeout = %s, want 42s", config.HTTPResponseHeaderTimeout)
	}
	if got := config.TimeoutForProfile(ProfileRPCDefault); got != 31*time.Second {
		t.Fatalf("ProfileRPCDefault timeout = %s, want 31s", got)
	}
	if got := config.TimeoutForProfile(ProfileRPCReadHeavy); got != 47*time.Second {
		t.Fatalf("ProfileRPCReadHeavy timeout = %s, want 47s", got)
	}
}

func unsetTransportEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"AWIKI_CLI_TIMEOUT_BRIDGE_HEALTH_PROBE",
		"AWIKI_CLI_TIMEOUT_BRIDGE_DIAL",
		"AWIKI_CLI_TIMEOUT_BRIDGE_WRITE",
		"AWIKI_CLI_TIMEOUT_BRIDGE_READ",
		"AWIKI_CLI_TIMEOUT_HTTP_DIAL",
		"AWIKI_CLI_TIMEOUT_HTTP_KEEPALIVE",
		"AWIKI_CLI_TIMEOUT_HTTP_TLS_HANDSHAKE",
		"AWIKI_CLI_TIMEOUT_HTTP_RESPONSE_HEADER",
		"AWIKI_CLI_TIMEOUT_HTTP_IDLE_CONN",
		"AWIKI_CLI_HTTP_MAX_IDLE_CONNS",
		"AWIKI_CLI_HTTP_MAX_IDLE_CONNS_PER_HOST",
		"AWIKI_CLI_TIMEOUT_PROFILE_BRIDGE_FAST_PATH",
		"AWIKI_CLI_TIMEOUT_PROFILE_HEALTH_PROBE",
		"AWIKI_CLI_TIMEOUT_PROFILE_AUTH_REFRESH",
		"AWIKI_CLI_TIMEOUT_PROFILE_RPC_DEFAULT",
		"AWIKI_CLI_TIMEOUT_PROFILE_RPC_READ_HEAVY",
	} {
		t.Setenv(key, "")
	}
}

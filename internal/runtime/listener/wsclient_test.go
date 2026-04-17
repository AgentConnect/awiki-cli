package listener

import (
	"strings"
	"testing"

	"github.com/agentconnect/awiki-cli/internal/authsdk"
	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
)

func TestNewWSClientDerivesIMWebSocketEndpointFromServiceBaseURL(t *testing.T) {
	t.Parallel()

	client, err := NewWSClient(&appconfig.Resolved{
		ServiceBaseURL: "http://127.0.0.1:18080",
	}, dummyAuthSession())
	if err != nil {
		t.Fatalf("NewWSClient() error = %v", err)
	}
	if client.websocketURL != "ws://127.0.0.1:18080/im/ws" {
		t.Fatalf("client.websocketURL = %q, want ws://127.0.0.1:18080/im/ws", client.websocketURL)
	}
	if client.requestURL != "http://127.0.0.1:18080/im/ws" {
		t.Fatalf("client.requestURL = %q, want http://127.0.0.1:18080/im/ws", client.requestURL)
	}
}

func TestWSClientBuildDialHeadersIncludesUpgradeMetadata(t *testing.T) {
	originalVersion := buildinfo.Version
	originalHostAgent := buildinfo.HostAgent
	originalHostVersion := buildinfo.HostVersion
	originalHostCapabilities := buildinfo.HostCapabilities
	originalSkillFormatVersion := buildinfo.SkillFormatVersion
	defer func() {
		buildinfo.Version = originalVersion
		buildinfo.HostAgent = originalHostAgent
		buildinfo.HostVersion = originalHostVersion
		buildinfo.HostCapabilities = originalHostCapabilities
		buildinfo.SkillFormatVersion = originalSkillFormatVersion
	}()
	buildinfo.Version = "1.8.1"
	buildinfo.HostAgent = "codex"
	buildinfo.HostVersion = "5.4"
	buildinfo.HostCapabilities = "skills,local_exec"
	buildinfo.SkillFormatVersion = "v1"

	client, err := NewWSClient(&appconfig.Resolved{
		ServiceBaseURL: "http://127.0.0.1:18080",
		UpdateChannel:  "stable",
	}, dummyAuthSession())
	if err != nil {
		t.Fatalf("NewWSClient() error = %v", err)
	}
	headers := client.buildDialHeaders(map[string]string{"Authorization": "Bearer token"})
	if headers["Authorization"] != "Bearer token" {
		t.Fatalf("Authorization = %q, want Bearer token", headers["Authorization"])
	}
	if headers["X-Awiki-CLI-Version"] != "1.8.1" {
		t.Fatalf("X-Awiki-CLI-Version = %q", headers["X-Awiki-CLI-Version"])
	}
	if headers["X-Awiki-CLI-Channel"] != "stable" {
		t.Fatalf("X-Awiki-CLI-Channel = %q", headers["X-Awiki-CLI-Channel"])
	}
	if headers["X-Awiki-Host-Agent"] != "codex" {
		t.Fatalf("X-Awiki-Host-Agent = %q", headers["X-Awiki-Host-Agent"])
	}
	if headers["X-Awiki-Host-Version"] != "5.4" {
		t.Fatalf("X-Awiki-Host-Version = %q", headers["X-Awiki-Host-Version"])
	}
	if !strings.Contains(headers["X-Awiki-Host-Capabilities"], "skills") {
		t.Fatalf("X-Awiki-Host-Capabilities = %q", headers["X-Awiki-Host-Capabilities"])
	}
	if headers["X-Awiki-Skill-Format-Version"] != "v1" {
		t.Fatalf("X-Awiki-Skill-Format-Version = %q", headers["X-Awiki-Skill-Format-Version"])
	}
}

func dummyAuthSession() *authsdk.Session {
	return authsdk.NewSession("", "", "alice", "did:wba:example.com:user:alice", "token", nil)
}

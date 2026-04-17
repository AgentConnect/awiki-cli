package hermesbridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureRouteCreatesWebhookNotifyRouteAndUsesHomeChannel(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".env"), []byte("FEISHU_APP_ID=app-id\nFEISHU_APP_SECRET=app-secret\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(.env) error = %v", err)
	}
	state, err := EnsureRoute(EnsureRouteOptions{
		HermesHome: home,
		RouteName:  "notify",
		Deliver:    "feishu",
	})
	if err != nil {
		t.Fatalf("EnsureRoute() error = %v", err)
	}
	if !state.RouteConfigured {
		t.Fatal("state.RouteConfigured = false, want true")
	}
	if !state.RouteSecretConfigured {
		t.Fatal("state.RouteSecretConfigured = false, want true")
	}
	if state.Deliver != "feishu" {
		t.Fatalf("state.Deliver = %q, want feishu", state.Deliver)
	}
	if !state.DeliverUsesHomeChannel {
		t.Fatal("state.DeliverUsesHomeChannel = false, want true")
	}
	if !state.FeishuCredentialsConfigured {
		t.Fatal("state.FeishuCredentialsConfigured = false, want true")
	}

	raw, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatalf("os.ReadFile(config.yaml) error = %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "deliver: feishu") {
		t.Fatalf("config.yaml missing deliver=feishu: %q", text)
	}
	if strings.Contains(text, "chat_id:") {
		t.Fatalf("config.yaml unexpectedly contains fixed chat_id: %q", text)
	}
}

func TestEnsureRouteRemovesDeliverExtraChatID(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	configPath := filepath.Join(home, "config.yaml")
	input := `platforms:
  webhook:
    enabled: true
    extra:
      port: 8644
      routes:
        notify:
          secret: route-secret
          events: []
          prompt: hello
          skills: ["notify"]
          deliver: feishu
          deliver_extra:
            chat_id: oc_xxx
            keep_me: yes
FEISHU_HOME_CHANNEL: oc_home
`
	if err := os.WriteFile(configPath, []byte(input), 0o600); err != nil {
		t.Fatalf("os.WriteFile(config.yaml) error = %v", err)
	}

	state, err := EnsureRoute(EnsureRouteOptions{
		HermesHome: home,
		RouteName:  "notify",
		Deliver:    "feishu",
	})
	if err != nil {
		t.Fatalf("EnsureRoute() error = %v", err)
	}
	if !state.DeliverUsesHomeChannel {
		t.Fatal("state.DeliverUsesHomeChannel = false, want true after cleanup")
	}
	if !state.HomeChannelConfigured {
		t.Fatal("state.HomeChannelConfigured = false, want true")
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile(config.yaml) error = %v", err)
	}
	text := string(raw)
	if strings.Contains(text, "chat_id:") {
		t.Fatalf("config.yaml still contains fixed chat_id: %q", text)
	}
	if !strings.Contains(text, "keep_me:") {
		t.Fatalf("config.yaml unexpectedly removed unrelated deliver_extra field: %q", text)
	}
}

func TestValidateLocalNotifyURLRejectsRemoteHost(t *testing.T) {
	t.Parallel()

	if _, _, _, err := ValidateLocalNotifyURL("http://10.0.0.1:8765/notify/host-event"); err == nil {
		t.Fatal("ValidateLocalNotifyURL(remote) error = nil, want error")
	}
}

func TestEnsureRouteTracksTelegramHomeChannel(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	configPath := filepath.Join(home, "config.yaml")
	input := "TELEGRAM_HOME_CHANNEL: tg_home\n"
	if err := os.WriteFile(configPath, []byte(input), 0o600); err != nil {
		t.Fatalf("os.WriteFile(config.yaml) error = %v", err)
	}

	state, err := EnsureRoute(EnsureRouteOptions{
		HermesHome: home,
		RouteName:  "notify",
		Deliver:    "telegram",
	})
	if err != nil {
		t.Fatalf("EnsureRoute() error = %v", err)
	}
	if state.Deliver != "telegram" {
		t.Fatalf("state.Deliver = %q, want telegram", state.Deliver)
	}
	if state.HomeChannelKey != "TELEGRAM_HOME_CHANNEL" {
		t.Fatalf("state.HomeChannelKey = %q, want TELEGRAM_HOME_CHANNEL", state.HomeChannelKey)
	}
	if !state.HomeChannelConfigured {
		t.Fatal("state.HomeChannelConfigured = false, want true")
	}
}

func TestEnsureRouteMigratesLegacyEnglishPromptToChineseDefault(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	configPath := filepath.Join(home, "config.yaml")
	input := `platforms:
  webhook:
    enabled: true
    extra:
      port: 8644
      routes:
        notify:
          secret: route-secret
          events: []
          prompt: |
            You are an awiki external IM notification formatter.

            Format the incoming notification into one concise IM message suitable for the target platform.
            Rules:
            1. Output only the final notification body.
            2. Do not ask follow-up questions.
            3. Prefer readable sender/recipient names when present.
            4. If a DID exists, include it on a separate line.
            5. Convert time to Asia/Shanghai using YYYY-MM-DD HH:mm (Asia/Shanghai).
            6. Summarize message content in 1 to 5 short lines.
            7. If links are present, list them at the end.

            Suggested layout:
            Received External IM Notification
            Sender: <name or DID>
            Sender DID: <did if present>
            Recipient: <name or DID>
            Recipient DID: <did if present>
            Type: <private/group/state/topic>
            Time: <Asia/Shanghai time>
            Message Summary:
            <1-5 lines>

            Raw notification JSON:
            {notify_payload}
          skills: ["notify"]
          deliver: feishu
`
	if err := os.WriteFile(configPath, []byte(input), 0o600); err != nil {
		t.Fatalf("os.WriteFile(config.yaml) error = %v", err)
	}

	if _, err := EnsureRoute(EnsureRouteOptions{
		HermesHome: home,
		RouteName:  "notify",
		Deliver:    "feishu",
	}); err != nil {
		t.Fatalf("EnsureRoute() error = %v", err)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile(config.yaml) error = %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "收到外部IM消息通知") {
		t.Fatalf("config.yaml prompt not migrated to Chinese default: %q", text)
	}
	if strings.Contains(text, "Received External IM Notification") {
		t.Fatalf("config.yaml still contains legacy English prompt: %q", text)
	}
}

func TestEnsureRouteKeepsCustomPrompt(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	configPath := filepath.Join(home, "config.yaml")
	input := `platforms:
  webhook:
    enabled: true
    extra:
      port: 8644
      routes:
        notify:
          secret: route-secret
          events: []
          prompt: |
            自定义提示词：请保持这一段不被覆盖。
          skills: ["notify"]
          deliver: feishu
`
	if err := os.WriteFile(configPath, []byte(input), 0o600); err != nil {
		t.Fatalf("os.WriteFile(config.yaml) error = %v", err)
	}

	if _, err := EnsureRoute(EnsureRouteOptions{
		HermesHome: home,
		RouteName:  "notify",
		Deliver:    "feishu",
	}); err != nil {
		t.Fatalf("EnsureRoute() error = %v", err)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile(config.yaml) error = %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "自定义提示词：请保持这一段不被覆盖。") {
		t.Fatalf("config.yaml custom prompt was unexpectedly changed: %q", text)
	}
}

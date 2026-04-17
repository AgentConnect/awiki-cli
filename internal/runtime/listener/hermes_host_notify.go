package listener

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	runtimecfg "github.com/agentconnect/awiki-cli/internal/runtime"
)

const (
	hermesNotifySecretEnv        = "AWIKI_HOST_NOTIFY_HERMES_SECRET"
	legacyWebhookNotifySecretEnv = "AWIKI_HOST_NOTIFY_WEBHOOK_SECRET"
)

type hermesHostNotifySink struct {
	client    *http.Client
	notifyURL string
	secret    string
}

func newHermesHostNotifySink(resolved *appconfig.Resolved, config runtimecfg.HermesConfig) (HostNotifySink, error) {
	notifyURL := strings.TrimSpace(config.NotifyURL)
	if notifyURL == "" {
		return nil, fmt.Errorf("hermes host notify requires runtime.host_notify.hermes.notify_url")
	}
	if err := validateHermesNotifyURL(notifyURL); err != nil {
		return nil, err
	}
	secret := resolveHermesNotifySecret(resolved)
	if secret == "" {
		return nil, fmt.Errorf(
			"hermes host notify requires runtime.host_notify.hermes.secret or %s (legacy: %s)",
			hermesNotifySecretEnv,
			legacyWebhookNotifySecretEnv,
		)
	}
	return &hermesHostNotifySink{
		client:    &http.Client{Timeout: 15 * time.Second},
		notifyURL: notifyURL,
		secret:    secret,
	}, nil
}

func (s *hermesHostNotifySink) Notify(ctx context.Context, event HostNotificationEvent) error {
	rawBody, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal host notify event: %w", err)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature := buildHermesNotifySignature(rawBody, timestamp, s.secret)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.notifyURL, bytes.NewReader(rawBody))
	if err != nil {
		return fmt.Errorf("build hermes host notify request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Notify-Timestamp", timestamp)
	request.Header.Set("X-Notify-Signature", "sha256="+signature)
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("send hermes host notify request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	rawResponse, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
	if len(rawResponse) == 0 {
		return fmt.Errorf("hermes host notify failed status=%d", response.StatusCode)
	}
	return fmt.Errorf("hermes host notify failed status=%d: %s", response.StatusCode, strings.TrimSpace(string(rawResponse)))
}

func (s *hermesHostNotifySink) Close() error {
	return nil
}

func buildHermesNotifySignature(rawBody []byte, timestamp string, secret string) string {
	signingInput := append([]byte(timestamp+"."), rawBody...)
	sum := hmac.New(sha256.New, []byte(secret))
	sum.Write(signingInput)
	return hex.EncodeToString(sum.Sum(nil))
}

func validateHermesNotifyURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse runtime.host_notify.hermes.notify_url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("runtime.host_notify.hermes.notify_url must use http or https")
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return fmt.Errorf("runtime.host_notify.hermes.notify_url must include a host")
	}
	return nil
}

func resolveHermesNotifySecret(resolved *appconfig.Resolved) string {
	if resolved != nil && strings.TrimSpace(resolved.Paths.ConfigFile) != "" {
		fileConfig, _, err := appconfig.ReadFileConfig(resolved.Paths.ConfigFile)
		if err == nil {
			secret := strings.TrimSpace(fileConfig.Runtime.HostNotify.Hermes.Secret)
			if secret == "" {
				secret = strings.TrimSpace(fileConfig.Runtime.HostNotify.LegacyWebhook.Secret)
			}
			if secret != "" {
				return secret
			}
		}
	}
	if secret := strings.TrimSpace(os.Getenv(hermesNotifySecretEnv)); secret != "" {
		return secret
	}
	return strings.TrimSpace(os.Getenv(legacyWebhookNotifySecretEnv))
}

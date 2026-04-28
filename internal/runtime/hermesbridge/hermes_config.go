package hermesbridge

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultHermesHomeDir    = ".hermes"
	defaultWebhookPort      = 8644
	defaultWebhookRouteName = "notify"
	defaultNotifyURL        = "http://127.0.0.1:8765/notify/host-event"
	defaultDeliverTarget    = "feishu"
)

const DefaultWebhookRouteName = defaultWebhookRouteName

var supportedDeliverTargets = []string{
	"bluebubbles",
	"discord",
	"email",
	"feishu",
	"log",
	"matrix",
	"mattermost",
	"qqbot",
	"signal",
	"slack",
	"sms",
	"telegram",
	"wecom",
	"weixin",
}

var deliverHomeChannelEnvKeys = map[string]string{
	"bluebubbles": "BLUEBUBBLES_HOME_CHANNEL",
	"discord":     "DISCORD_HOME_CHANNEL",
	"email":       "EMAIL_HOME_ADDRESS",
	"feishu":      "FEISHU_HOME_CHANNEL",
	"matrix":      "MATRIX_HOME_ROOM",
	"mattermost":  "MATTERMOST_HOME_CHANNEL",
	"qqbot":       "QQ_HOME_CHANNEL",
	"signal":      "SIGNAL_HOME_CHANNEL",
	"slack":       "SLACK_HOME_CHANNEL",
	"sms":         "SMS_HOME_CHANNEL",
	"telegram":    "TELEGRAM_HOME_CHANNEL",
	"wecom":       "WECOM_HOME_CHANNEL",
	"weixin":      "WEIXIN_HOME_CHANNEL",
}

const legacyDefaultNotifyPrompt = `You are an awiki external IM notification formatter.

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
`

const defaultNotifyPromptV1 = `你是 awiki 外部消息通知整理助手。

请根据收到的通知 topic 和 data，把它整理成一条简洁、稳定、适合目标 IM 平台阅读的中文消息。
规则：
1. 只输出最终通知正文，不要加解释。
2. 不要提问，不要添加无关寒暄。
3. 时间统一转换为 Asia/Shanghai，格式为 YYYY-MM-DD HH:mm (Asia/Shanghai)。
4. 字段标题统一使用中文。
5. 不存在的字段不要臆造，缺失时直接省略对应行。
6. 摘要控制在 1 到 5 行短句内。
7. 如果有链接，放在最后单独列出。
8. ` + "`topic=mail.message.received`" + ` 时，优先使用邮箱地址字段，如 ` + "`from_addr`" + `、` + "`mailbox_address`" + `、` + "`subject`" + `、` + "`preview`" + `。
9. IM 通知优先使用可读的人名、handle 或显示名；没有时再使用 DID。

如果 topic 是 ` + "`mail.message.received`" + `，建议格式：
收到外部邮件通知
发件人：<邮箱地址或名称>
收件邮箱：<mailbox_address>
收件人 DID：<recipient_did，如存在>
时间：<Asia/Shanghai 时间>
邮件摘要：
主题：<subject，如存在>
<preview 1-5 行>
附件：<有附件时再展示，例如：有>

如果 topic 是 IM 相关事件，例如 ` + "`im.message.received`" + `、` + "`im.group.message.received`" + `、` + "`im.group.state.changed`" + `，建议格式：
收到外部IM消息通知
发送者：<名称或 DID>
发送者 DID：<如存在>
接收者：<名称或 DID>
接收者 DID：<如存在>
类型：<私信/群消息/状态变更/事件>
时间：<Asia/Shanghai 时间>
消息内容摘要：
<1-5 行>

原始通知 JSON：
{notify_payload}
`

const defaultNotifyPrompt = `你是 awiki 外部消息通知整理助手。

请根据收到的通知 topic 和 data，把它整理成一条简洁、稳定、适合目标 IM 平台阅读的中文消息。
规则：
1. 只输出最终通知正文，不要加解释。
2. 不要提问，不要添加无关寒暄。
3. 时间统一转换为 Asia/Shanghai，格式为 YYYY-MM-DD HH:mm (Asia/Shanghai)。
4. 字段标题统一使用中文。
5. 不存在的字段不要臆造，缺失时直接省略对应行。
6. 摘要控制在 1 到 5 行短句内。
7. 如果有链接，放在最后单独列出。
8. 若 data 中 ` + "`source_kind=mail`" + `，或存在 ` + "`mailbox_address`" + `、` + "`from_addr`" + `、` + "`subject`" + `、` + "`preview`" + ` 等邮件字段，则必须按邮件通知处理，不强依赖 topic 名称。
9. IM 通知优先使用可读的人名、handle 或显示名；没有时再使用 DID。
10. 命中邮件通知时，不要使用“收到外部IM消息通知”作为标题，也不要套用 IM 模板。
11. 处理邮件时，如果 ` + "`from_addr`" + ` 存在且能识别出发件人姓名，优先把姓名写到“发件人”，并把邮箱单独写到“发件邮箱”。
12. 如果 ` + "`preview`" + ` 末尾包含类似“姓名 / 邮箱：...”的署名块，应将其从摘要中提取出来，不要原样重复在“邮件摘要”最后。

如果 data 中 ` + "`source_kind=mail`" + `，或存在 ` + "`mailbox_address`" + `、` + "`from_addr`" + `、` + "`subject`" + `、` + "`preview`" + ` 这类邮件字段，必须使用下面这个模板：
收到外部邮件通知
发件人：<姓名；如果没有姓名则用邮箱地址>
发件邮箱：<from_addr，如存在且与发件人不同>
收件邮箱：<mailbox_address>
收件人 DID：<recipient_did，如存在>
时间：<Asia/Shanghai 时间>
邮件摘要：
主题：<subject，如存在>
<preview 1-5 行，去掉重复署名和邮箱签名>
附件：<有附件时再展示，例如：有>

否则，如果 topic 是 IM 相关事件，例如 ` + "`im.message.received`" + `、` + "`im.group.message.received`" + `、` + "`im.group.state.changed`" + `，使用下面这个模板：
收到外部IM消息通知
发送者：<名称或 DID>
发送者 DID：<如存在>
接收者：<名称或 DID>
接收者 DID：<如存在>
类型：<私信/群消息/状态变更/事件>
时间：<Asia/Shanghai 时间>
消息内容摘要：
<1-5 行>

原始通知 JSON：
{notify_payload}
`

type RouteState struct {
	HermesHome                  string   `json:"hermes_home"`
	ConfigFile                  string   `json:"config_file"`
	EnvFile                     string   `json:"env_file"`
	ConfigExists                bool     `json:"config_exists"`
	WebhookEnabled              bool     `json:"webhook_enabled"`
	WebhookPort                 int      `json:"webhook_port"`
	RouteName                   string   `json:"route_name"`
	RouteConfigured             bool     `json:"route_configured"`
	RouteSecret                 string   `json:"-"`
	RouteSecretConfigured       bool     `json:"route_secret_configured"`
	Deliver                     string   `json:"deliver"`
	DeliverUsesHomeChannel      bool     `json:"deliver_uses_home_channel"`
	HomeChannelKey              string   `json:"home_channel_key,omitempty"`
	HomeChannel                 string   `json:"home_channel,omitempty"`
	HomeChannelConfigured       bool     `json:"home_channel_configured"`
	HomeChannelSupported        bool     `json:"home_channel_supported"`
	FeishuCredentialsConfigured bool     `json:"feishu_credentials_configured"`
	NotifyWebhookURL            string   `json:"notify_webhook_url"`
	Warnings                    []string `json:"warnings,omitempty"`
}

type EnsureRouteOptions struct {
	HermesHome  string
	RouteName   string
	Deliver     string
	WebhookPort int
	Prompt      string
}

func ResolveHermesHome() (string, error) {
	if value := strings.TrimSpace(os.Getenv("HERMES_HOME")); value != "" {
		return value, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, defaultHermesHomeDir), nil
}

func InspectRoute(home string, routeName string) (RouteState, error) {
	return inspectOrEnsureRoute(home, routeName, EnsureRouteOptions{}, false)
}

func EnsureRoute(options EnsureRouteOptions) (RouteState, error) {
	home := strings.TrimSpace(options.HermesHome)
	if home == "" {
		var err error
		home, err = ResolveHermesHome()
		if err != nil {
			return RouteState{}, err
		}
		options.HermesHome = home
	}
	if strings.TrimSpace(options.RouteName) == "" {
		options.RouteName = defaultWebhookRouteName
	}
	if strings.TrimSpace(options.Deliver) == "" {
		options.Deliver = defaultDeliverTarget
	}
	if options.WebhookPort <= 0 {
		options.WebhookPort = defaultWebhookPort
	}
	if strings.TrimSpace(options.Prompt) == "" {
		options.Prompt = defaultNotifyPrompt
	}
	return inspectOrEnsureRoute(home, options.RouteName, options, true)
}

func inspectOrEnsureRoute(home string, routeName string, options EnsureRouteOptions, write bool) (RouteState, error) {
	configPath := filepath.Join(home, "config.yaml")
	envPath := filepath.Join(home, ".env")
	state := RouteState{
		HermesHome: home,
		ConfigFile: configPath,
		EnvFile:    envPath,
		RouteName:  routeName,
	}

	config := map[string]any{}
	if raw, err := os.ReadFile(configPath); err == nil {
		state.ConfigExists = true
		if len(raw) > 0 {
			if err := yaml.Unmarshal(raw, &config); err != nil {
				return RouteState{}, fmt.Errorf("parse Hermes config.yaml: %w", err)
			}
		}
	} else if !os.IsNotExist(err) {
		return RouteState{}, fmt.Errorf("read Hermes config.yaml: %w", err)
	}

	if config == nil {
		config = map[string]any{}
	}
	platforms := getMap(config, "platforms", write)
	webhook := getMap(platforms, "webhook", write)
	if write {
		webhook["enabled"] = true
	}
	state.WebhookEnabled = boolValue(webhook["enabled"], false)
	extra := getMap(webhook, "extra", write)
	if write {
		if intValue(extra["port"], 0) <= 0 {
			extra["port"] = options.WebhookPort
		}
	}
	state.WebhookPort = intValue(extra["port"], defaultWebhookPort)

	routes := getMap(extra, "routes", write)
	route := getMap(routes, routeName, write)
	if write {
		if strings.TrimSpace(stringValue(route["secret"])) == "" {
			route["secret"] = generateSecret()
		}
		if _, exists := route["events"]; !exists {
			route["events"] = []any{}
		}
		if shouldReplaceNotifyPrompt(stringValue(route["prompt"])) {
			route["prompt"] = options.Prompt
		}
		cleanupLegacyNotifySkill(route)
		route["deliver"] = options.Deliver
		cleanupDeliverExtra(route)
	}

	state.RouteConfigured = len(route) > 0
	state.RouteSecret = strings.TrimSpace(stringValue(route["secret"]))
	state.RouteSecretConfigured = state.RouteSecret != ""
	state.Deliver = strings.TrimSpace(stringValue(route["deliver"]))
	if state.Deliver == "" {
		state.Deliver = "log"
	}
	extraMap := getMap(route, "deliver_extra", false)
	state.DeliverUsesHomeChannel = true
	if extraMap != nil {
		if strings.TrimSpace(stringValue(extraMap["chat_id"])) != "" {
			state.DeliverUsesHomeChannel = false
		}
	}
	state.HomeChannelKey = HomeChannelEnvKey(state.Deliver)
	state.HomeChannelSupported = state.HomeChannelKey != ""
	if state.HomeChannelSupported {
		state.HomeChannel = strings.TrimSpace(stringValue(config[state.HomeChannelKey]))
		state.HomeChannelConfigured = state.HomeChannel != ""
	}
	envValues, err := readEnvFile(envPath)
	if err != nil {
		state.Warnings = append(state.Warnings, fmt.Sprintf("Failed to read Hermes .env: %v", err))
	}
	if strings.TrimSpace(envValues["FEISHU_APP_ID"]) != "" && strings.TrimSpace(envValues["FEISHU_APP_SECRET"]) != "" {
		state.FeishuCredentialsConfigured = true
	}
	state.NotifyWebhookURL = fmt.Sprintf("http://127.0.0.1:%d/webhooks/%s", state.WebhookPort, routeName)
	if state.Deliver != "log" && !state.DeliverUsesHomeChannel {
		if state.HomeChannelKey != "" {
			state.Warnings = append(state.Warnings, fmt.Sprintf("Hermes notify route still has deliver_extra.chat_id; notifications will not follow %s until that fixed target is removed.", state.HomeChannelKey))
		} else {
			state.Warnings = append(state.Warnings, "Hermes notify route still has deliver_extra.chat_id; notifications will not follow the platform home channel until that fixed target is removed.")
		}
	}
	if state.DeliverUsesHomeChannel && state.Deliver != "log" {
		switch {
		case !state.HomeChannelSupported:
			state.Warnings = append(state.Warnings, fmt.Sprintf("Hermes notify route deliver target %q does not have a known home-channel config key. Use an explicitly supported messaging platform or set deliver_extra.chat_id manually.", state.Deliver))
		case !state.HomeChannelConfigured:
			state.Warnings = append(state.Warnings, fmt.Sprintf("%s is not configured in Hermes yet. Run /sethome from the desired %s chat before expecting auto delivery.", state.HomeChannelKey, DeliverDisplayName(state.Deliver)))
		}
	}
	if write {
		if err := writeYAMLFile(configPath, config); err != nil {
			return RouteState{}, err
		}
	}
	return state, nil
}

func ValidateLocalNotifyURL(rawURL string) (*url.URL, string, int, error) {
	value := strings.TrimSpace(rawURL)
	if value == "" {
		value = defaultNotifyURL
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, "", 0, fmt.Errorf("parse notify URL: %w", err)
	}
	if !strings.EqualFold(parsed.Scheme, "http") {
		return nil, "", 0, fmt.Errorf("notify URL must use http for local Hermes bridge")
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		host = "127.0.0.1"
	}
	switch strings.ToLower(host) {
	case "127.0.0.1", "localhost", "::1":
	default:
		return nil, "", 0, fmt.Errorf("notify URL host %q is not local; full Hermes setup only supports a local bridge", host)
	}
	port := 0
	if parsed.Port() != "" {
		port, err = strconv.Atoi(parsed.Port())
		if err != nil || port <= 0 {
			return nil, "", 0, fmt.Errorf("notify URL has invalid port")
		}
	} else {
		port = 80
	}
	return parsed, host, port, nil
}

func writeYAMLFile(path string, content map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create Hermes config dir: %w", err)
	}
	raw, err := yaml.Marshal(content)
	if err != nil {
		return fmt.Errorf("marshal Hermes config.yaml: %w", err)
	}
	tempFile, err := os.CreateTemp(filepath.Dir(path), ".hermes-config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp Hermes config file: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		_ = tempFile.Close()
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := tempFile.Write(raw); err != nil {
		return fmt.Errorf("write temp Hermes config file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("sync temp Hermes config file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp Hermes config file: %w", err)
	}
	if err := os.Chmod(tempPath, 0o600); err != nil {
		return fmt.Errorf("chmod temp Hermes config file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace Hermes config file: %w", err)
	}
	cleanup = false
	return nil
}

func readEnvFile(path string) (map[string]string, error) {
	values := map[string]string{}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return values, nil
		}
		return nil, err
	}
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "export ") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "export "))
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		values[key] = value
	}
	return values, nil
}

func getMap(parent map[string]any, key string, create bool) map[string]any {
	if parent == nil {
		return nil
	}
	if existing, ok := parent[key]; ok {
		switch typed := existing.(type) {
		case map[string]any:
			return typed
		case map[any]any:
			converted := map[string]any{}
			for k, v := range typed {
				converted[fmt.Sprint(k)] = v
			}
			parent[key] = converted
			return converted
		}
	}
	if !create {
		return nil
	}
	created := map[string]any{}
	parent[key] = created
	return created
}

func cleanupDeliverExtra(route map[string]any) {
	extra := getMap(route, "deliver_extra", false)
	if extra == nil {
		return
	}
	delete(extra, "chat_id")
	delete(extra, "thread_id")
	delete(extra, "message_thread_id")
	if len(extra) == 0 {
		delete(route, "deliver_extra")
		return
	}
	keys := make([]string, 0, len(extra))
	for key := range extra {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	cleaned := map[string]any{}
	for _, key := range keys {
		cleaned[key] = extra[key]
	}
	route["deliver_extra"] = cleaned
}

func cleanupLegacyNotifySkill(route map[string]any) {
	if route == nil {
		return
	}
	skills := getSequence(route["skills"])
	if len(skills) != 1 {
		return
	}
	if strings.TrimSpace(stringValue(skills[0])) != "notify" {
		return
	}
	delete(route, "skills")
}

func shouldReplaceNotifyPrompt(current string) bool {
	normalized := strings.TrimSpace(current)
	switch normalized {
	case "":
		return true
	case strings.TrimSpace(legacyDefaultNotifyPrompt):
		return true
	case strings.TrimSpace(defaultNotifyPromptV1):
		return true
	}
	if isLegacyIMOnlyNotifyPrompt(normalized) {
		return true
	}
	if strings.Contains(normalized, "你是 awiki 外部消息通知整理助手。") &&
		strings.Contains(normalized, "{notify_payload}") &&
		strings.Contains(normalized, "收到外部邮件通知") &&
		(strings.Contains(normalized, "如果 topic 是 mail.message.received") ||
			strings.Contains(normalized, "topic=mail.message.received 时") ||
			strings.Contains(normalized, "不强依赖 topic 名称") ||
			strings.Contains(normalized, "优先按邮件通知处理")) {
		return true
	}
	if strings.Contains(normalized, "你是 awiki 外部消息通知整理助手。") &&
		strings.Contains(normalized, "{notify_payload}") &&
		strings.Contains(normalized, "收到外部邮件通知") &&
		strings.Contains(normalized, "必须按邮件通知处理") &&
		(!strings.Contains(normalized, "不要使用“收到外部IM消息通知”作为标题") ||
			!strings.Contains(normalized, "发件邮箱：<from_addr，如存在且与发件人不同>") ||
			!strings.Contains(normalized, "去掉重复署名和邮箱签名")) {
		return true
	}
	return false
}

func isLegacyIMOnlyNotifyPrompt(normalized string) bool {
	if normalized == "" {
		return false
	}
	return strings.Contains(normalized, "你是 awiki 外部 IM 消息通知整理助手。") &&
		strings.Contains(normalized, "收到外部IM消息通知") &&
		strings.Contains(normalized, "消息内容摘要：") &&
		strings.Contains(normalized, "{notify_payload}") &&
		!strings.Contains(normalized, "收到外部邮件通知") &&
		!strings.Contains(normalized, "source_kind=mail")
}

func getSequence(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []string:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, item)
		}
		return result
	default:
		return nil
	}
}

func hasNonEmptySequence(value any) bool {
	switch typed := value.(type) {
	case []any:
		return len(typed) > 0
	case []string:
		return len(typed) > 0
	default:
		return false
	}
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

func intValue(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return parsed
		}
	}
	return fallback
}

func boolValue(value any, fallback bool) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	}
	return fallback
}

func generateSecret() string {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "awiki-hermes-secret"
	}
	return hex.EncodeToString(buf)
}

func NormalizeDeliverTarget(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return defaultDeliverTarget
	}
	return normalized
}

func SupportedDeliverTargets() []string {
	values := append([]string(nil), supportedDeliverTargets...)
	sort.Strings(values)
	return values
}

func IsSupportedDeliverTarget(value string) bool {
	normalized := NormalizeDeliverTarget(value)
	for _, candidate := range supportedDeliverTargets {
		if normalized == candidate {
			return true
		}
	}
	return false
}

func HomeChannelEnvKey(deliver string) string {
	return deliverHomeChannelEnvKeys[NormalizeDeliverTarget(deliver)]
}

func DeliverDisplayName(deliver string) string {
	switch NormalizeDeliverTarget(deliver) {
	case "bluebubbles":
		return "BlueBubbles"
	case "discord":
		return "Discord"
	case "email":
		return "Email"
	case "feishu":
		return "Feishu"
	case "log":
		return "log"
	case "matrix":
		return "Matrix"
	case "mattermost":
		return "Mattermost"
	case "qqbot":
		return "QQ Bot"
	case "signal":
		return "Signal"
	case "slack":
		return "Slack"
	case "sms":
		return "SMS"
	case "telegram":
		return "Telegram"
	case "wecom":
		return "WeCom"
	case "weixin":
		return "Weixin"
	default:
		normalized := NormalizeDeliverTarget(deliver)
		if normalized == "" {
			return "Unknown"
		}
		return strings.ToUpper(normalized[:1]) + normalized[1:]
	}
}

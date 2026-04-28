package traceutil

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

const timingEnvKey = "AWIKI_CLI_TRACE_TIMING"

type contextKey struct{}

type Phase struct {
	Name       string `json:"name"`
	DurationMS int64  `json:"duration_ms"`
}

type FallbackEvent struct {
	Stage string `json:"stage"`
	Cause string `json:"cause,omitempty"`
}

type Run struct {
	command string
	started time.Time
	enabled bool

	mu        sync.Mutex
	phases    []Phase
	fallbacks []FallbackEvent
}

func Enabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(timingEnvKey)))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func New(command string) *Run {
	return &Run{
		command: strings.TrimSpace(command),
		started: time.Now(),
		enabled: Enabled(),
	}
}

func WithRun(ctx context.Context, run *Run) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, run)
}

func FromContext(ctx context.Context) *Run {
	if ctx == nil {
		return nil
	}
	run, _ := ctx.Value(contextKey{}).(*Run)
	return run
}

func PhaseContext(ctx context.Context, name string) func() {
	run := FromContext(ctx)
	if run == nil {
		return func() {}
	}
	return run.StartPhase(name)
}

func PhaseContextWithDetail(ctx context.Context, name string, detail string) func() {
	return PhaseContext(ctx, phaseName(name, detail))
}

func RPCPhase(ctx context.Context, operation string) func() {
	return PhaseContextWithDetail(ctx, "business_rpc", operation)
}

func LocalDBPhase(ctx context.Context, operation string) func() {
	return PhaseContextWithDetail(ctx, "local_db", operation)
}

func EnsureJWTPhase(ctx context.Context, operation string) func() {
	return PhaseContextWithDetail(ctx, "ensure_jwt", operation)
}

func HandleLookupPhase(ctx context.Context, operation string) func() {
	return PhaseContextWithDetail(ctx, "handle_lookup", operation)
}

func MarkFallback(ctx context.Context, stage string, cause error) {
	run := FromContext(ctx)
	if run == nil {
		return
	}
	run.MarkFallback(stage, cause)
}

func (r *Run) StartPhase(name string) func() {
	if r == nil || !r.enabled {
		return func() {}
	}
	started := time.Now()
	r.mu.Lock()
	index := len(r.phases)
	r.phases = append(r.phases, Phase{
		Name:       strings.TrimSpace(name),
		DurationMS: 0,
	})
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if index < 0 || index >= len(r.phases) {
			return
		}
		r.phases[index].DurationMS = time.Since(started).Milliseconds()
	}
}

func (r *Run) MarkFallback(stage string, cause error) {
	if r == nil || !r.enabled {
		return
	}
	item := FallbackEvent{Stage: strings.TrimSpace(stage)}
	if cause != nil {
		item.Cause = cause.Error()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallbacks = append(r.fallbacks, item)
}

func (r *Run) Emit(w io.Writer) error {
	if r == nil || !r.enabled || w == nil {
		return nil
	}
	r.mu.Lock()
	phases := append([]Phase(nil), r.phases...)
	fallbacks := append([]FallbackEvent(nil), r.fallbacks...)
	r.mu.Unlock()

	return emitPretty(w, r.command, phases, fallbacks, time.Since(r.started).Milliseconds())
}

func phaseName(name string, detail string) string {
	name = strings.TrimSpace(name)
	detail = sanitizePhaseDetail(detail)
	if name == "" {
		return detail
	}
	if detail == "" {
		return name
	}
	return name + ":" + detail
}

func sanitizePhaseDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	detail = strings.Trim(detail, "/")
	detail = strings.ReplaceAll(detail, "/", ".")
	detail = strings.Join(strings.Fields(detail), "_")
	detail = strings.ReplaceAll(detail, "_.", "_")
	detail = strings.ReplaceAll(detail, "._", "_")
	detail = strings.Trim(detail, "._")
	return detail
}

func emitPretty(w io.Writer, command string, phases []Phase, fallbacks []FallbackEvent, totalMS int64) error {
	if _, err := fmt.Fprintln(w, "[awiki-cli 耗时追踪]"); err != nil {
		return err
	}
	if command != "" {
		if _, err := fmt.Fprintf(w, "命令: %s\n", command); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "总耗时: %s\n", formatDurationMS(totalMS)); err != nil {
		return err
	}
	if len(phases) > 0 {
		if _, err := fmt.Fprintln(w, "阶段:"); err != nil {
			return err
		}
		for i, phase := range phases {
			if _, err := fmt.Fprintf(w, "  %2d. %s: %s\n", i+1, humanizePhaseName(phase.Name), formatDurationMS(phase.DurationMS)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w, "说明: 各阶段按开始顺序展示，阶段之间可能重叠，因此不会等于总耗时相加。"); err != nil {
			return err
		}
	}
	if len(fallbacks) > 0 {
		if _, err := fmt.Fprintln(w, "回退:"); err != nil {
			return err
		}
		for _, fallback := range fallbacks {
			line := humanizeText(fallback.Stage)
			if strings.TrimSpace(fallback.Cause) != "" {
				line += " (" + fallback.Cause + ")"
			}
			if _, err := fmt.Fprintf(w, "  - %s\n", line); err != nil {
				return err
			}
		}
	}
	return nil
}

func humanizePhaseName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "未命名阶段"
	}
	parts := strings.SplitN(name, ":", 2)
	group := phaseGroupLabel(parts[0])
	if len(parts) == 1 || strings.TrimSpace(parts[1]) == "" {
		return group
	}
	return group + " / " + humanizeText(parts[1])
}

func phaseGroupLabel(group string) string {
	switch strings.TrimSpace(group) {
	case "business_rpc":
		return "远端 RPC"
	case "local_db":
		return "本地数据库"
	case "ensure_jwt":
		return "JWT 续期"
	case "handle_lookup":
		return "Handle 解析"
	case "resolve_config":
		return "解析配置"
	case "update_check":
		return "检查更新"
	case "update_registry_fetch":
		return "请求更新源"
	case "npm_upgrade_install":
		return "npm 升级安装"
	case "workspace_upgrade":
		return "工作区升级"
	case "bridge_health_probe":
		return "本地桥健康检查"
	case "bridge_call":
		return "调用本地桥"
	case "contact_sync":
		return "联系人同步"
	default:
		return humanizeText(group)
	}
}

func humanizeText(value string) string {
	value = strings.TrimSpace(value)
	if translated, ok := knownHumanizedText[value]; ok {
		return translated
	}
	value = strings.ReplaceAll(value, ".", " ")
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return "空"
	}
	return value
}

func formatDurationMS(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%d 毫秒", ms)
	}
	return fmt.Sprintf("%.3f 秒", float64(ms)/1000)
}

var knownHumanizedText = map[string]string{
	"websocket_to_http":               "WebSocket 降级到 HTTP",
	"handle_to_did":                   "Handle 转 DID",
	"did_to_handle":                   "DID 转 Handle",
	"target_resolve":                  "目标解析",
	"lookup":                          "查询",
	"resolve":                         "解析",
	"get_public_profile":              "获取公开资料",
	"contact_sync_by_did":             "按 DID 同步联系人",
	"handle_cache_lookup_by_handle":   "按 Handle 查询缓存",
	"handle_cache_lookup_by_did":      "按 DID 查询缓存",
	"handle_cache_write":              "写入 Handle 缓存",
	"persist_direct_send":             "写入直聊发送结果",
	"persist_inbox_messages":          "写入收件箱消息",
	"persist_history_messages":        "写入历史消息",
	"read_inbox_cache":                "读取收件箱缓存",
	"read_mail_notification_cache":    "读取邮件通知缓存",
	"read_history_cache":              "读取历史缓存",
	"read_inbox_cache_by_peer_dids":   "按对端 DID 读取收件箱缓存",
	"read_history_cache_by_peer_dids": "按对端 DID 读取历史缓存",
	"message_fallback_refresh":        "消息回退时刷新 JWT",
	"message_service_retry":           "消息服务重试前刷新 JWT",
	"identity_refresh_token":          "身份刷新 Token",
	"identity_bootstrap":              "身份服务启动鉴权",
	"mail_bootstrap":                  "邮件服务启动鉴权",
	"content_bootstrap":               "内容服务启动鉴权",
}

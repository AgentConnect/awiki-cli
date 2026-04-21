package traceutil

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunEmitPrettyFormatIsReadable(t *testing.T) {
	t.Setenv(timingEnvKey, "1")

	run := New("awiki-cli test")
	outerDone := run.StartPhase("outer")
	innerDone := run.StartPhase("inner")
	innerDone()
	outerDone()
	run.MarkFallback("websocket_to_http", nil)

	var buf bytes.Buffer
	if err := run.Emit(&buf); err != nil {
		t.Fatalf("Emit() error = %v", err)
	}

	output := buf.String()
	for _, want := range []string{
		"[awiki-cli 耗时追踪]\n",
		"命令: awiki-cli test\n",
		"总耗时: ",
		"阶段:\n",
		"回退:\n",
		"  - WebSocket 降级到 HTTP\n",
		"说明: 各阶段按开始顺序展示，阶段之间可能重叠，因此不会等于总耗时相加。\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	outerIndex := strings.Index(output, "1. outer:")
	innerIndex := strings.Index(output, "2. inner:")
	if outerIndex < 0 || innerIndex < 0 || outerIndex >= innerIndex {
		t.Fatalf("phase order not preserved in pretty output:\n%s", output)
	}
}

func TestPhaseContextWithDetailPrettyHumanizesPhaseName(t *testing.T) {
	t.Setenv(timingEnvKey, "1")

	run := New("awiki-cli test")
	done := PhaseContextWithDetail(WithRun(nil, run), "business_rpc", "POST /user-service/auth/email-send")
	done()

	var buf bytes.Buffer
	if err := run.Emit(&buf); err != nil {
		t.Fatalf("Emit() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "远端 RPC / POST user-service auth email-send") {
		t.Fatalf("pretty output did not humanize phase name:\n%s", output)
	}
}

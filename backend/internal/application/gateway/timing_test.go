package gateway

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
)

func TestGenerationTimingLogsOnlyPhaseMetadata(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	timing := newGenerationTiming("public-model", accountdomain.ProviderBuild)
	timing.markSelection(10 * time.Millisecond)
	timing.markCredential(20 * time.Millisecond)
	timing.markUpstream(30 * time.Millisecond)
	timing.markUpstream(40 * time.Millisecond)
	body := &firstByteReadCloser{ReadCloser: io.NopCloser(strings.NewReader("ok")), mark: timing.markFirstBody}
	if _, err := io.ReadAll(body); err != nil {
		t.Fatal(err)
	}
	timing.finish(logger, "success")
	logged := output.String()
	for _, expected := range []string{"generation_timing", "route=public-model", "provider=grok_build", "selection_wait_ms=10", "credential_wait_ms=20", "upstream_wait_ms=70", "attempts=2", "retries=1"} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("log missing %q: %s", expected, logged)
		}
	}
}

func TestFirstTokenTimerMarksOnce(t *testing.T) {
	timer := newFirstTokenTimer(time.Now().Add(-25 * time.Millisecond))
	if timer.milliseconds() != nil {
		t.Fatal("unmarked timer returned a value")
	}
	timer.mark()
	first := timer.milliseconds()
	if first == nil || *first < 20 {
		t.Fatalf("first token milliseconds = %v", first)
	}
	time.Sleep(time.Millisecond)
	timer.mark()
	second := timer.milliseconds()
	if second == nil || *second != *first {
		t.Fatalf("timer changed after second mark: first=%v second=%v", first, second)
	}
}

func TestGenerationTimingPreservesRetryAndQualitySequence(t *testing.T) {
	timing := newGenerationTiming("test", accountdomain.ProviderBuild)
	timing.markSelection(12 * time.Millisecond)
	timing.markCredential(8 * time.Millisecond)
	firstStart := timing.started.Add(20 * time.Millisecond)
	timing.markUpstream(100 * time.Millisecond)
	timing.recordCall(accountdomain.Credential{ID: 11, Name: "first"}, firstStart, 100*time.Millisecond, 200, false)
	byteMS := int64(3)
	timing.recordQuality(30*time.Second, &qualityPeekObservation{firstByteMS: &byteMS}, "timeout")
	before := timing.snapshot()
	timing.recordAction(1, "fallback")
	timing.markUpstream(200 * time.Millisecond)
	timing.recordCall(accountdomain.Credential{ID: 22, Name: "second"}, firstStart.Add(31*time.Second), 200*time.Millisecond, 200, false)
	visibleMS := int64(500)
	timing.recordQuality(time.Second, &qualityPeekObservation{firstVisibleMS: &visibleMS}, "deliver")
	got := timing.snapshot()
	if len(before.Calls) != 1 || before.Calls[0].Outcome != "timeout" || *before.Calls[0].QualityMS != 30000 || before.Calls[0].Action != "" || got.Calls[0].Action != "fallback" {
		t.Fatalf("earlier snapshot changed: %+v", before)
	}
	if got.SelectionMS != 12 || got.CredentialMS != 8 || got.UpstreamMS != 300 || got.QualityMS != 31000 || len(got.Calls) != 2 {
		t.Fatalf("incorrect phase totals: %+v", got)
	}
	if got.Calls[0].AccountID != "11" || got.Calls[1].AccountID != "22" || got.Calls[1].StartedOffsetMS != 31020 || got.Calls[1].Outcome != "deliver" || got.Calls[1].FirstByteMS != nil || *got.Calls[1].FirstVisibleMS != 500 {
		t.Fatalf("retry metadata associated with wrong call: %+v", got.Calls)
	}
}

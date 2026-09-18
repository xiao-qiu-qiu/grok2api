package gateway

import (
	"io"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/domain/audit"
	"github.com/chenyme/grok2api/backend/internal/pkg/perfmetrics"
)

// generationTiming 只记录阶段耗时和有限枚举，不保存请求体、凭据或会话键。
type generationTiming struct {
	mu             sync.Mutex
	started        time.Time
	route          string
	provider       accountdomain.Provider
	selectionWait  time.Duration
	credentialWait time.Duration
	upstreamWait   time.Duration
	firstHeaders   time.Duration
	firstBody      time.Duration
	attempts       int
	finished       bool
	calls          []audit.PerformanceCall
	qualityWait    time.Duration
}

func (t *generationTiming) recordCall(credential accountdomain.Credential, started time.Time, duration time.Duration, status int, failed bool) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	outcome := "response"
	if failed || status >= 400 {
		outcome = "error"
	}
	t.calls = append(t.calls, audit.PerformanceCall{Number: len(t.calls) + 1,
		AccountID: strconv.FormatUint(credential.ID, 10), AccountName: credential.Name,
		StartedOffsetMS: max(int64(0), started.Sub(t.started).Milliseconds()),
		UpstreamMS:      duration.Milliseconds(), StatusCode: status, Outcome: outcome})
	return len(t.calls)
}

func (t *generationTiming) recordAction(number int, action string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if number > 0 && number <= len(t.calls) {
		t.calls[number-1].Action = action
	}
}

func (t *generationTiming) recordQuality(duration time.Duration, observation *qualityPeekObservation, outcome string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.qualityWait += duration
	if len(t.calls) == 0 {
		return
	}
	call := &t.calls[len(t.calls)-1]
	ms := duration.Milliseconds()
	call.QualityMS, call.Outcome = &ms, outcome
	call.FirstByteMS, call.FirstThinkingMS, call.FirstVisibleMS = observation.firstByteMS, observation.firstThinkingMS, observation.firstVisibleMS
}

func (t *generationTiming) snapshot() *audit.Performance {
	t.mu.Lock()
	defer t.mu.Unlock()
	return &audit.Performance{SelectionMS: t.selectionWait.Milliseconds(), CredentialMS: t.credentialWait.Milliseconds(),
		UpstreamMS: t.upstreamWait.Milliseconds(), QualityMS: t.qualityWait.Milliseconds(),
		Calls: append([]audit.PerformanceCall{}, t.calls...)}
}

func newGenerationTiming(route string, provider accountdomain.Provider) *generationTiming {
	return &generationTiming{started: time.Now(), route: route, provider: provider}
}

func (t *generationTiming) markSelection(duration time.Duration) {
	t.mu.Lock()
	t.selectionWait += duration
	t.mu.Unlock()
}

func (t *generationTiming) markCredential(duration time.Duration) {
	t.mu.Lock()
	t.credentialWait += duration
	t.mu.Unlock()
}

func (t *generationTiming) markUpstream(duration time.Duration) {
	t.mu.Lock()
	t.attempts++
	t.upstreamWait += duration
	if t.firstHeaders == 0 {
		t.firstHeaders = time.Since(t.started)
	}
	t.mu.Unlock()
}

func (t *generationTiming) markFirstBody() {
	t.mu.Lock()
	if t.firstBody == 0 {
		t.firstBody = time.Since(t.started)
	}
	t.mu.Unlock()
}

func (t *generationTiming) finish(logger *slog.Logger, outcome string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		return
	}
	t.finished = true
	total := time.Since(t.started)
	retries := max(0, t.attempts-1)
	fields := []any{
		"route", t.route, "provider", t.provider, "outcome", outcome, "total_ms", total.Milliseconds(),
		"selection_wait_ms", t.selectionWait.Milliseconds(), "credential_wait_ms", t.credentialWait.Milliseconds(),
		"upstream_wait_ms", t.upstreamWait.Milliseconds(), "first_headers_ms", t.firstHeaders.Milliseconds(),
		"first_body_ms", t.firstBody.Milliseconds(), "attempts", t.attempts, "retries", retries,
	}
	t.mu.Unlock()
	labels := perfmetrics.Labels{Subsystem: "gateway", Provider: string(t.provider), Outcome: outcome}
	perfmetrics.Default.ObserveDuration("request_duration_us", labels, total)
	perfmetrics.Default.ObserveDuration("stage_duration_us", withTimingStage(labels, "selection"), t.selectionWait)
	perfmetrics.Default.ObserveDuration("stage_duration_us", withTimingStage(labels, "credential"), t.credentialWait)
	perfmetrics.Default.ObserveDuration("stage_duration_us", withTimingStage(labels, "upstream"), t.upstreamWait)
	perfmetrics.Default.Add("attempt_count", labels, int64(t.attempts))
	if logger == nil {
		logger = slog.Default()
	}
	logger.Debug("generation_timing", fields...)
}

func withTimingStage(labels perfmetrics.Labels, stage string) perfmetrics.Labels {
	labels.Stage = stage
	return labels
}

type firstByteReadCloser struct {
	io.ReadCloser
	once sync.Once
	mark func()
}

type firstTokenTimer struct {
	started time.Time
	once    sync.Once
	encoded atomic.Int64
}

func newFirstTokenTimer(started time.Time) *firstTokenTimer {
	return &firstTokenTimer{started: started}
}

func (t *firstTokenTimer) mark() {
	if t == nil {
		return
	}
	t.once.Do(func() {
		elapsedMS := max(int64(0), time.Since(t.started).Milliseconds())
		t.encoded.Store(elapsedMS + 1)
	})
}

func (t *firstTokenTimer) milliseconds() *int64 {
	if t == nil {
		return nil
	}
	encoded := t.encoded.Load()
	if encoded == 0 {
		return nil
	}
	value := encoded - 1
	return &value
}

func (r *firstByteReadCloser) Read(buffer []byte) (int, error) {
	n, err := r.ReadCloser.Read(buffer)
	if n > 0 && r.mark != nil {
		r.once.Do(r.mark)
	}
	return n, err
}

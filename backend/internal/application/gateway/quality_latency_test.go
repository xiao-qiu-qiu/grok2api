package gateway

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestQualityHoldEarlyDeliveryRequiresSubstantialSustainedEvidence(t *testing.T) {
	t.Parallel()
	base := QualityStreamSignals{
		HasThinking: true, FirstVisible: true, VisibleTokens: 40,
		VisibleFlushMS: 1100, VisibleSpanMS: 1100, EncryptedBytes: 1024, EncryptedFloor: 256,
	}
	if got := ClassifyQualityHold(base, 8); got != QualityDeliver {
		t.Fatalf("ongoing substantial response without final usage = %s", got)
	}
	for _, mutate := range []func(*QualityStreamSignals){
		func(s *QualityStreamSignals) { s.VisibleSpanMS = 100 },
		func(s *QualityStreamSignals) { s.VisibleSpanMS = 0 },
		func(s *QualityStreamSignals) { s.VisibleTokens = 2 },
		func(s *QualityStreamSignals) { s.EncryptedBytes = 300 },
		func(s *QualityStreamSignals) { s.EncryptedFloor = 800 },
	} {
		sig := base
		mutate(&sig)
		if got := ClassifyQualityHold(sig, 8); got != QualityWait {
			t.Fatalf("inconclusive evidence released early: %+v = %s", sig, got)
		}
	}
}

func TestQualityPeekObservationsAndReplay(t *testing.T) {
	t.Parallel()
	fixture := sse(`data: {"choices":[{"delta":{"reasoning_content":"real thinking","content":"answer"}}]}`, "data: [DONE]")
	observed := &qualityPeekObservation{}
	replay, verdict, _, _, err := peekQualityStream(context.Background(), io.NopCloser(strings.NewReader(fixture)), qualityProtocolChat, QualityRetryRuntime{}, observed)
	if err != nil || verdict != QualityDeliver {
		t.Fatalf("peek verdict=%s err=%v", verdict, err)
	}
	defer replay.Close()
	got, err := io.ReadAll(replay)
	if err != nil || string(got) != fixture {
		t.Fatalf("replay changed stream: err=%v", err)
	}
	if observed.firstByteMS == nil || observed.firstThinkingMS == nil || observed.firstVisibleMS == nil {
		t.Fatalf("missing observations: %+v", observed)
	}
}

func TestQualityPeekStrongCipherAloneStillHasHardDeadline(t *testing.T) {
	t.Parallel()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	go func() {
		_, _ = io.WriteString(writer, sse(`data: {"type":"response.output_item.added","item":{"id":"rs_1","type":"reasoning","encrypted_content":"`+strings.Repeat("x", 1024)+`"}}`))
	}()
	started := time.Now()
	_, verdict, _, _, err := peekQualityStream(context.Background(), reader, qualityProtocolResponses, QualityRetryRuntime{HoldTimeout: 30 * time.Millisecond})
	if err != errQualityHoldTimeout || verdict != QualityWait {
		t.Fatalf("cipher-only stalled response: verdict=%s err=%v", verdict, err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("quality deadline did not interrupt stalled ciphertext stream")
	}
}

func TestQualityPeekDeliversSustainedCipherStreamBeforeUsage(t *testing.T) {
	t.Parallel()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	first := sse(
		`data: {"type":"response.output_item.added","item":{"id":"rs_1","type":"reasoning","encrypted_content":"`+strings.Repeat("x", 1024)+`"}}`,
		`data: {"type":"response.output_text.delta","delta":"`+strings.Repeat("word", 40)+`"}`,
	)
	second := sse(`data: {"type":"response.output_text.delta","delta":" continues"}`)
	tail := sse(`data: {"type":"response.completed","response":{"id":"resp_1","usage":{"output_tokens":90,"output_tokens_details":{"reasoning_tokens":40}}}}`)
	continueWrite := make(chan struct{})
	defer close(continueWrite)
	writeErr := make(chan error, 1)
	go func() {
		if _, err := io.WriteString(writer, first); err != nil {
			writeErr <- err
			return
		}
		time.Sleep(1100 * time.Millisecond)
		if _, err := io.WriteString(writer, second); err != nil {
			writeErr <- err
			return
		}
		<-continueWrite
		_, err := io.WriteString(writer, tail)
		_ = writer.Close()
		writeErr <- err
	}()
	started := time.Now()
	replay, verdict, _, _, err := peekQualityStream(context.Background(), reader, qualityProtocolResponses, QualityRetryRuntime{HoldTimeout: 3 * time.Second})
	if err != nil || verdict != QualityDeliver || time.Since(started) >= 2500*time.Millisecond {
		t.Fatalf("ongoing ciphertext stream was not released early: verdict=%s err=%v elapsed=%s", verdict, err, time.Since(started))
	}
	defer replay.Close()
	continueWrite <- struct{}{}
	got, err := io.ReadAll(replay)
	if err != nil || string(got) != first+second+tail {
		t.Fatalf("early handoff lost stream data: err=%v", err)
	}
	if err := <-writeErr; err != nil {
		t.Fatal(err)
	}
}

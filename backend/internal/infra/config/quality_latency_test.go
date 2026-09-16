package config

import (
	"testing"
	"time"
)

func TestQualityTotalHoldTimeoutDefaultAndBounds(t *testing.T) {
	t.Parallel()
	if got := defaultConfig().QualityGuard.RequestRetry.TotalHoldTimeout.Value(); got != 45*time.Second {
		t.Fatalf("total hold default=%s", got)
	}
	for _, value := range []time.Duration{0, 200 * time.Millisecond, 45 * time.Second, 3 * time.Minute} {
		if err := validateQualityGuardRequestRetry(QualityGuardRequestRetryConfig{Enabled: true, TotalHoldTimeout: Duration(value)}); err != nil {
			t.Fatalf("valid total hold %s: %v", value, err)
		}
	}
	for _, value := range []time.Duration{-time.Second, time.Millisecond, 3*time.Minute + time.Millisecond} {
		if err := validateQualityGuardRequestRetry(QualityGuardRequestRetryConfig{Enabled: true, TotalHoldTimeout: Duration(value)}); err == nil {
			t.Fatalf("accepted invalid total hold %s", value)
		}
	}
}

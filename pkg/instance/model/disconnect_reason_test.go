package instance_model

import (
	"testing"
	"time"
)

func TestTemporaryBanReasonRoundTrip(t *testing.T) {
	until := time.Date(2026, 10, 12, 15, 4, 5, 0, time.UTC)
	reason := TemporaryBanReason(until, "sent too many messages")

	got, ok := TemporaryBanUntil(reason)
	if !ok || !got.Equal(until) {
		t.Fatalf("TemporaryBanUntil(%q) = %s, %v", reason, got, ok)
	}
}

func TestTemporaryBanUntilIgnoresOtherReasons(t *testing.T) {
	for _, reason := range []string{"", "Reconnecting", "401: logged out from another device", "Temporary ban until garbage: x"} {
		if _, ok := TemporaryBanUntil(reason); ok {
			t.Errorf("TemporaryBanUntil(%q) parsed a non-ban reason", reason)
		}
	}
}

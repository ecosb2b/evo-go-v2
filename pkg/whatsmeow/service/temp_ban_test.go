package whatsmeow_service

import (
	"testing"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
)

func TestBanWait(t *testing.T) {
	now := time.Unix(10_000, 0)
	banned := instance_model.TemporaryBanReason(now.Add(90*time.Minute), "spam")
	expired := instance_model.TemporaryBanReason(now.Add(-time.Minute), "spam")

	if got := banWait(banned, now); got != 90*time.Minute {
		t.Fatalf("banWait(active) = %s, want 1h30m", got)
	}
	if got := banWait(expired, now); got != 0 {
		t.Fatalf("banWait(expired) = %s, want 0", got)
	}
	if got := banWait("Reconnecting", now); got != 0 {
		t.Fatalf("banWait(not a ban) = %s, want 0", got)
	}
}

func TestSkipAtBoot(t *testing.T) {
	now := time.Unix(10_000, 0)
	tests := []struct {
		name   string
		reason string
		want   bool
	}{
		{"active ban", instance_model.TemporaryBanReason(now.Add(time.Hour), "spam"), true},
		{"expired ban", instance_model.TemporaryBanReason(now.Add(-time.Minute), "spam"), false},
		{"stream replaced", instance_model.DisconnectReasonStreamReplaced, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &instance_model.Instance{Id: "i1", DisconnectReason: tt.reason}
			if got := skipAtBoot(instance, now); got != tt.want {
				t.Fatalf("skipAtBoot(%q) = %v, want %v", tt.reason, got, tt.want)
			}
		})
	}
}

func TestTemporaryBanUntilUsesExpiry(t *testing.T) {
	now := time.Unix(1000, 0)
	if got := temporaryBanUntil(now, 2*time.Hour); !got.Equal(now.Add(2 * time.Hour)) {
		t.Fatalf("got %s", got)
	}
}

// Sem prazo informado, o supervisor também precisa esperar.
func TestTemporaryBanUntilDefaultsWhenExpiryUnknown(t *testing.T) {
	now := time.Unix(1000, 0)
	if got := temporaryBanUntil(now, 0); !got.Equal(now.Add(tempBanUnknownWait)) {
		t.Fatalf("got %s, want now + %s", got, tempBanUnknownWait)
	}
}

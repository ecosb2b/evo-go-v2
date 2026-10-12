package whatsmeow_service

import (
	"testing"
	"time"
)

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

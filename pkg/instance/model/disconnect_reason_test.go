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

func TestRequiresQR(t *testing.T) {
	const jid = "5511999999999@s.whatsapp.net"
	tests := []struct {
		name   string
		jid    string
		reason string
		want   bool
	}{
		{"never paired", "", "", true},
		{"logged out from the phone", jid, "401: logged out from another device", true},
		{"primary device logged out", jid, "403: primary device was logged out", true},
		{"unknown logout", jid, "406: logged out for unknown reason", true},
		{"logged out through the API", jid, DisconnectReasonLogoutByAPI, true},
		{"QR window ended", jid, "Maximum QR code count (7) reached", true},
		{"QR socket dropped", jid, DisconnectReasonQRSocketLost, true},
		{"reconnecting", jid, "Reconnecting", false},
		{"disconnected through the API", jid, DisconnectReasonByAPI, false},
		{"banned", jid, TemporaryBanReason(time.Now(), "spam"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequiresQR(tt.jid, tt.reason); got != tt.want {
				t.Fatalf("RequiresQR(%q, %q) = %v, want %v", tt.jid, tt.reason, got, tt.want)
			}
		})
	}
}

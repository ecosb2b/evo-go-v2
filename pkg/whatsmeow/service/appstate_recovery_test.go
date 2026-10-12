package whatsmeow_service

import (
	"errors"
	"fmt"
	"testing"

	"go.mau.fi/whatsmeow/appstate"
)

func TestReserveAppStateRecoveryDeduplicatesEachStage(t *testing.T) {
	client := &MyClient{}
	name := appstate.WAPatchRegularLow

	if !client.reserveAppStateRecovery(name, false) {
		t.Fatal("first full-sync attempt should be reserved")
	}
	if client.reserveAppStateRecovery(name, false) {
		t.Fatal("duplicate full-sync attempt should be suppressed during cooldown")
	}
	if !client.reserveAppStateRecovery(name, true) {
		t.Fatal("recovery request is a separate stage and should be reserved")
	}
	if client.reserveAppStateRecovery(name, true) {
		t.Fatal("duplicate recovery request should be suppressed during cooldown")
	}
	if !client.reserveAppStateRecovery(appstate.WAPatchRegularHigh, false) {
		t.Fatal("cooldown for one collection must not suppress another collection")
	}
}

// Pedir recovery ao celular só faz sentido quando o snapshot não verificou;
// erro de rede no fetch também chega como FullSync após o bump do whatsmeow.
func TestIsAppStateVerificationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"LTHash mismatch", fmt.Errorf("decode: %w", appstate.ErrMismatchingLTHash), true},
		{"patch MAC mismatch", appstate.ErrMismatchingPatchMAC, true},
		{"content MAC mismatch", appstate.ErrMismatchingContentMAC, true},
		{"index MAC mismatch", appstate.ErrMismatchingIndexMAC, true},
		{"missing previous SET", appstate.ErrMissingPreviousSetValueOperation, true},
		{"network failure on fetch", errors.New("websocket not connected"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAppStateVerificationError(tt.err); got != tt.want {
				t.Fatalf("isAppStateVerificationError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

package whatsmeow_service

import "testing"

// O teardown do QR encerra o runtime DESTE MyClient, nunca o que estiver no
// mapa killChannel (depois de um restart, ele é o runtime novo).
func TestSignalRuntimeKillUsesOwnChannel(t *testing.T) {
	own := make(chan bool, 1)
	newer := make(chan bool, 1)
	mycli := &MyClient{
		userID:      "inst-1",
		runtimeKill: own,
		killChannel: map[string](chan bool){"inst-1": newer},
	}

	mycli.signalRuntimeKill()
	mycli.signalRuntimeKill() // canal cheio: não pode bloquear

	if len(own) != 1 {
		t.Fatalf("own kill channel has %d signals, want 1", len(own))
	}
	if len(newer) != 0 {
		t.Fatal("teardown signalled the runtime registered in the shared map")
	}
}

func TestSignalRuntimeKillWithoutChannel(t *testing.T) {
	(&MyClient{}).signalRuntimeKill() // não pode entrar em pânico nem bloquear
}

func TestShouldAnnounceLoggedOut(t *testing.T) {
	tests := []struct {
		restarting, qrEnded, want bool
	}{
		{false, false, true},
		{true, false, false},
		{false, true, false},
		{true, true, false},
	}
	for _, tt := range tests {
		if got := shouldAnnounceLoggedOut(tt.restarting, tt.qrEnded); got != tt.want {
			t.Errorf("shouldAnnounceLoggedOut(%v, %v) = %v, want %v", tt.restarting, tt.qrEnded, got, tt.want)
		}
	}
}

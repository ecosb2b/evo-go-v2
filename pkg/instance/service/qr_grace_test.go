package instance_service

import (
	"testing"
	"time"
)

func TestQRRestartAllowed(t *testing.T) {
	tests := []struct {
		name  string
		age   time.Duration
		known bool
		want  bool
	}{
		{"no runtime age (nothing registered)", 0, false, true},
		{"runtime just created, still connecting", 2 * time.Second, true, false},
		{"right at the grace limit", qrConnectGrace, true, true},
		{"old runtime with a dead socket", time.Minute, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := qrRestartAllowed(tt.age, tt.known); got != tt.want {
				t.Fatalf("qrRestartAllowed(%s, %v) = %v, want %v", tt.age, tt.known, got, tt.want)
			}
		})
	}
}

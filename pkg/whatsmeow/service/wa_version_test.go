package whatsmeow_service

import (
	"testing"

	"go.mau.fi/whatsmeow/store"
)

func TestWAVersionToApply(t *testing.T) {
	current := store.WAVersionContainer{2, 3000, 1049558099}

	tests := []struct {
		name     string
		v        clientVersion
		explicit bool
		want     bool
	}{
		{"newer from web", clientVersion{2, 3000, 1049600000}, false, true},
		{"same as compiled", clientVersion{2, 3000, 1049558099}, false, false},
		{"older from web is ignored", clientVersion{2, 3000, 1020000000}, false, false},
		{"older but configured explicitly", clientVersion{2, 3000, 1020000000}, true, true},
		{"zero is ignored", clientVersion{}, true, false},
		{"newer minor", clientVersion{2, 3001, 1}, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldApplyWAVersion(current, tt.v, tt.explicit); got != tt.want {
				t.Fatalf("shouldApplyWAVersion(%v, %v, %v) = %v, want %v", current, tt.v, tt.explicit, got, tt.want)
			}
		})
	}
}

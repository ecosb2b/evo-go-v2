package config

import "testing"

func TestParseQRMaxCount(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{"", defaultQRMaxCount},
		{"0", 0},
		{"3", 3},
		{" 9 ", 9},
		{"abc", defaultQRMaxCount},
		{"-1", defaultQRMaxCount},
	}
	for _, tt := range tests {
		if got := parseQRMaxCount(tt.raw); got != tt.want {
			t.Errorf("parseQRMaxCount(%q) = %d, want %d", tt.raw, got, tt.want)
		}
	}
	if defaultQRMaxCount != 7 {
		t.Fatalf("defaultQRMaxCount = %d, want 7 (6 codes, ~160 s)", defaultQRMaxCount)
	}
}

package user_service

import "testing"

func TestParseLID(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "123456789@lid", want: "123456789@lid"},
		{in: "123456789", want: "123456789@lid"},
		{in: " 123456789@lid ", want: "123456789@lid"},
		{in: "123456789:12@lid", want: "123456789@lid"},
		{in: "5511999999999@s.whatsapp.net", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tt := range tests {
		got, err := parseLID(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseLID(%q) = %s, want error", tt.in, got)
			}
			continue
		}
		if err != nil || got.String() != tt.want {
			t.Errorf("parseLID(%q) = %s, %v; want %s", tt.in, got, err, tt.want)
		}
	}
}

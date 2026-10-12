package utils

import (
	"reflect"
	"testing"
)

func TestWhatsAppNumberCandidates(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "BR mobile with 9, DDD >= 31: also tries without the 9",
			input: "5531987654321",
			want:  []string{"+5531987654321", "+553187654321"},
		},
		{
			name:  "BR mobile already stripped by the middleware: also tries with the 9",
			input: "+553187654321@s.whatsapp.net",
			want:  []string{"+5531987654321", "+553187654321"},
		},
		{
			name:  "device suffix is not part of the number",
			input: "5531987654321:12@s.whatsapp.net",
			want:  []string{"+5531987654321", "+553187654321"},
		},
		{
			name:  "BR mobile with 9, DDD < 31",
			input: "5511987654321",
			want:  []string{"+5511987654321", "+551187654321"},
		},
		{
			name:  "BR mobile without 9, DDD < 31",
			input: "551187654321",
			want:  []string{"+5511987654321", "+551187654321"},
		},
		{
			name:  "BR landline: no variant",
			input: "551133334444",
			want:  []string{"+551133334444"},
		},
		{
			name:  "formatting characters are ignored",
			input: "+55 (31) 98765-4321",
			want:  []string{"+5531987654321", "+553187654321"},
		},
		{
			name:  "non-BR number: no variant",
			input: "14155552671",
			want:  []string{"+14155552671"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WhatsAppNumberCandidates(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("WhatsAppNumberCandidates(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

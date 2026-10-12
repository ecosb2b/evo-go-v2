package utils

import (
	"testing"
	"time"
)

func TestStoredQRRoundTrip(t *testing.T) {
	expires := time.Unix(1_800_000_000, 0)
	raw := EncodeStoredQR("data:image/png;base64,AAA", "2@ref,pub,id,adv", expires)

	q, ok := ParseStoredQR(raw)
	if !ok {
		t.Fatalf("ParseStoredQR(%q) rejected a value written by EncodeStoredQR", raw)
	}
	if q.Image != "data:image/png;base64,AAA" || q.Code != "2@ref,pub,id,adv" || !q.ExpiresAt.Equal(expires) {
		t.Fatalf("round trip mismatch: %+v", q)
	}
}

// Valores gravados antes desta versão (sem validade) continuam servidos.
func TestParseStoredQRLegacyTwoFields(t *testing.T) {
	q, ok := ParseStoredQR("img|code")
	if !ok || q.Image != "img" || q.Code != "code" {
		t.Fatalf("legacy value rejected: %+v ok=%v", q, ok)
	}
	if !q.ExpiresAt.IsZero() || q.Expired(time.Now()) {
		t.Fatalf("legacy value must have unknown validity and never expire, got %+v", q)
	}
}

func TestParseStoredQRRejectsMalformed(t *testing.T) {
	for _, raw := range []string{"", "img", "img|", "|code", "img|code|not-a-number"} {
		if _, ok := ParseStoredQR(raw); ok {
			t.Errorf("ParseStoredQR(%q) accepted a malformed value", raw)
		}
	}
}

func TestStoredQRExpired(t *testing.T) {
	q := StoredQR{Image: "i", Code: "c", ExpiresAt: time.Unix(100, 0)}
	if q.Expired(time.Unix(99, 0)) {
		t.Fatal("expired one second before its deadline")
	}
	if !q.Expired(time.Unix(100, 0)) {
		t.Fatal("still valid at its deadline")
	}
}

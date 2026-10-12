package whatsmeow_service

import (
	"strings"
	"testing"
)

func TestDisconnectReason(t *testing.T) {
	if got := disconnectReason(""); got != disconnectedByServer {
		t.Fatalf("without stream error = %q, want %q", got, disconnectedByServer)
	}

	got := disconnectReason(`stream:error code= <stream:error><ack class="status" type="media"/></stream:error>`)
	if !strings.Contains(got, `<ack class="status" type="media"/>`) {
		t.Fatalf("stream error detail missing: %q", got)
	}

	long := disconnectReason("stream:error " + strings.Repeat("x", 2000))
	if len(long) > maxDisconnectReasonLen {
		t.Fatalf("reason has %d bytes, want at most %d", len(long), maxDisconnectReasonLen)
	}
}

func TestLastStreamErrorIsConsumedOnce(t *testing.T) {
	mycli := &MyClient{}
	mycli.recordStreamError("515", "<stream:error code=\"515\"/>")

	first := mycli.takeStreamError()
	if !strings.Contains(first, "515") {
		t.Fatalf("first take = %q, want the recorded error", first)
	}
	if second := mycli.takeStreamError(); second != "" {
		t.Fatalf("second take = %q, want empty", second)
	}
}

package send_service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebPDetection(t *testing.T) {
	static := append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, 8)...)
	if !isWebP(static) {
		t.Fatal("expected RIFF/WEBP payload to be detected")
	}
	if isAnimatedWebP(static) {
		t.Fatal("static WebP must not be marked animated")
	}

	animated := append([]byte("RIFF\x00\x00\x00\x00WEBPVP8X\x00\x00\x00\x00\x02"), make([]byte, 8)...)
	if !isAnimatedWebP(animated) {
		t.Fatal("VP8X animation flag must be detected")
	}

	animChunk := []byte("RIFF\x00\x00\x00\x00WEBPVP8 ANIM")
	if !isAnimatedWebP(animChunk) {
		t.Fatal("ANIM chunk must be detected")
	}
	if isWebP([]byte("not-webp")) {
		t.Fatal("invalid payload must not be detected as WebP")
	}
}

func TestFetchStickerData(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("sticker-bytes"))
		}))
		defer srv.Close()

		got, err := fetchStickerData(context.Background(), srv.URL)
		if err != nil {
			t.Fatalf("fetchStickerData: %v", err)
		}
		if string(got) != "sticker-bytes" {
			t.Fatalf("unexpected body: %q", got)
		}
	})

	t.Run("rejects invalid URL", func(t *testing.T) {
		if _, err := fetchStickerData(context.Background(), "file:///tmp/sticker.webp"); err == nil {
			t.Fatal("expected invalid URL error")
		}
	})

	t.Run("rejects HTTP error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "nope", http.StatusBadGateway)
		}))
		defer srv.Close()

		if _, err := fetchStickerData(context.Background(), srv.URL); err == nil {
			t.Fatal("expected HTTP status error")
		}
	})
}

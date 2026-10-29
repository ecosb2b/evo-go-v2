package utils

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIsPublicIP(t *testing.T) {
	private := []string{
		"0.0.0.0", "10.1.2.3", "127.0.0.1", "100.64.0.1", "169.254.169.254",
		"172.16.0.1", "172.31.255.255", "192.168.0.10", "192.0.0.1", "198.18.0.1",
		"224.0.0.1", "255.255.255.255",
		"::", "::1", "fc00::1", "fd12::1", "fe80::1", "ff02::1",
		"::ffff:10.0.0.1", "::ffff:127.0.0.1", "64:ff9b::a00:1",
		"::ffff:a9fe:a9fe", "::7f00:1", "64:ff9b:1::1", "100::1", "2001::1",
		"2002:7f00:1::1", "fec0::1", "198.51.100.7", "203.0.113.9",
	}
	for _, s := range private {
		if IsPublicIP(net.ParseIP(s)) {
			t.Errorf("IsPublicIP(%s) = true, want false", s)
		}
	}
	public := []string{"8.8.8.8", "172.32.0.1", "100.128.0.1", "2606:4700::1111", "::ffff:8.8.8.8", "2001:4860:4860::8888"}
	for _, s := range public {
		if !IsPublicIP(net.ParseIP(s)) {
			t.Errorf("IsPublicIP(%s) = false, want true", s)
		}
	}
	if IsPublicIP(nil) {
		t.Error("IsPublicIP(nil) = true, want false")
	}
}

func TestSafeGetRefusesLoopback(t *testing.T) {
	t.Setenv("ALLOW_PRIVATE_URLS", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("segredo interno"))
	}))
	defer srv.Close()

	resp, err := SafeGet(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("SafeGet to 127.0.0.1 succeeded, want ErrPrivateAddress")
	}
	if !errors.Is(err, ErrPrivateAddress) {
		t.Fatalf("err = %v, want ErrPrivateAddress", err)
	}
}

// dialControl é o que roda em toda conexão, inclusive a de cada redirect.
func TestDialControl(t *testing.T) {
	t.Setenv("ALLOW_PRIVATE_URLS", "")
	for _, addr := range []string{"10.0.0.1:443", "127.0.0.1:80", "[::1]:443", "169.254.169.254:80"} {
		if err := dialControl("tcp", addr, nil); !errors.Is(err, ErrPrivateAddress) {
			t.Errorf("dialControl(%s) = %v, want ErrPrivateAddress", addr, err)
		}
	}
	for _, addr := range []string{"8.8.8.8:443", "[2606:4700::1111]:443"} {
		if err := dialControl("tcp", addr, nil); err != nil {
			t.Errorf("dialControl(%s) = %v, want nil", addr, err)
		}
	}
	t.Setenv("ALLOW_PRIVATE_URLS", "true")
	if err := dialControl("tcp", "10.0.0.1:443", nil); err != nil {
		t.Errorf("ALLOW_PRIVATE_URLS=true: dialControl = %v, want nil", err)
	}
}

func TestSafeGetAllowPrivateURLs(t *testing.T) {
	t.Setenv("ALLOW_PRIVATE_URLS", "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	resp, err := SafeGet(srv.URL)
	if err != nil {
		t.Fatalf("SafeGet with ALLOW_PRIVATE_URLS=true: %v", err)
	}
	resp.Body.Close()
}

func TestDownloadImage(t *testing.T) {
	t.Setenv("ALLOW_PRIVATE_URLS", "true") // o servidor de teste é local
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpegdata"))
		case "/page":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>"))
		case "/big.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte(strings.Repeat("x", 2048)))
		case "/slow.jpg":
			time.Sleep(500 * time.Millisecond)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	ctx := context.Background()

	data, err := DownloadImage(ctx, srv.URL+"/ok.jpg", 1024, 5*time.Second)
	if err != nil || string(data) != "jpegdata" {
		t.Fatalf("ok.jpg: data=%q err=%v", data, err)
	}
	if _, err := DownloadImage(ctx, srv.URL+"/page", 1024, 5*time.Second); err == nil {
		t.Error("text/html accepted as image")
	}
	if _, err := DownloadImage(ctx, srv.URL+"/big.jpg", 1024, 5*time.Second); err == nil {
		t.Error("image over the limit accepted")
	}
	if _, err := DownloadImage(ctx, srv.URL+"/missing.jpg", 1024, 5*time.Second); err == nil {
		t.Error("404 accepted")
	}
	if _, err := DownloadImage(ctx, srv.URL+"/slow.jpg", 1024, 100*time.Millisecond); err == nil {
		t.Error("slow download did not time out")
	}
}

func TestCheckRedirect(t *testing.T) {
	req := func(raw string) *http.Request {
		r, _ := http.NewRequest(http.MethodGet, raw, nil)
		return r
	}
	https := req("https://a.example/")
	via := func(n int) []*http.Request {
		out := make([]*http.Request, n)
		for i := range out {
			out[i] = https
		}
		return out
	}
	if err := checkRedirect(req("https://b.example/"), via(3)); err != nil {
		t.Errorf("3º redirect: %v, want nil", err)
	}
	if err := checkRedirect(req("https://b.example/"), via(4)); !errors.Is(err, ErrTooManyRedirects) {
		t.Errorf("4º redirect: %v, want ErrTooManyRedirects", err)
	}
	if err := checkRedirect(req("http://169.254.169.254/"), via(1)); !errors.Is(err, ErrRedirectScheme) {
		t.Errorf("https → http: %v, want ErrRedirectScheme", err)
	}
	if err := checkRedirect(req("file:///etc/passwd"), via(1)); !errors.Is(err, ErrRedirectScheme) {
		t.Errorf("file://: %v, want ErrRedirectScheme", err)
	}
	if err := checkRedirect(req("http://b.example/"), []*http.Request{req("http://a.example/")}); err != nil {
		t.Errorf("http → http: %v, want nil", err)
	}
}

func TestMediaDownloadMaxBytes(t *testing.T) {
	t.Setenv("MEDIA_DOWNLOAD_MAX_MB", "")
	if got := MediaDownloadMaxBytes(); got != 100<<20 {
		t.Errorf("padrão = %d, want %d", got, 100<<20)
	}
	t.Setenv("MEDIA_DOWNLOAD_MAX_MB", "16")
	if got := MediaDownloadMaxBytes(); got != 16<<20 {
		t.Errorf("16 MB = %d", got)
	}
	t.Setenv("MEDIA_DOWNLOAD_MAX_MB", "abc")
	if got := MediaDownloadMaxBytes(); got != 100<<20 {
		t.Errorf("inválido = %d, want padrão", got)
	}
}

func TestDownloadLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.pdf":
			_, _ = w.Write([]byte("%PDF-1.4"))
		case "/big": // sem Content-Length (chunked): o limite conta o que chega
			_, _ = w.Write([]byte(strings.Repeat("x", 512)))
			w.(http.Flusher).Flush()
			_, _ = w.Write([]byte(strings.Repeat("x", 2048)))
		case "/small":
			_, _ = w.Write([]byte(strings.Repeat("x", 512)))
			w.(http.Flusher).Flush()
			_, _ = w.Write([]byte(strings.Repeat("x", 512)))
		case "/declared":
			w.Header().Set("Content-Length", "4096")
			_, _ = w.Write([]byte(strings.Repeat("x", 4096)))
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/two":
			http.Redirect(w, r, "/one", http.StatusFound)
		case "/one":
			http.Redirect(w, r, "/ok.pdf", http.StatusFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	ctx := context.Background()

	t.Run("recusa endereço interno sem repetir a URL", func(t *testing.T) {
		t.Setenv("ALLOW_PRIVATE_URLS", "")
		_, err := DownloadLimited(ctx, srv.URL+"/ok.pdf?token=segredo", 1024)
		if !errors.Is(err, ErrPrivateAddress) || !IsMediaURLRefused(err) {
			t.Fatalf("err = %v, want ErrPrivateAddress", err)
		}
		if strings.Contains(err.Error(), "segredo") || strings.Contains(err.Error(), "ok.pdf") {
			t.Fatalf("erro repete a URL: %v", err)
		}
		if !strings.HasPrefix(err.Error(), "media download failed: ") {
			t.Fatalf("prefixo: %v", err)
		}
	})

	t.Setenv("ALLOW_PRIVATE_URLS", "true") // o servidor de teste é local

	if data, err := DownloadLimited(ctx, srv.URL+"/ok.pdf", 1024); err != nil || string(data) != "%PDF-1.4" {
		t.Fatalf("ok.pdf: data=%q err=%v", data, err)
	}
	if _, err := DownloadLimited(ctx, srv.URL+"/two", 1024); err != nil {
		t.Errorf("2 redirects: %v, want nil", err)
	}
	for _, path := range []string{"/big", "/declared"} {
		if _, err := DownloadLimited(ctx, srv.URL+path, 1024); !errors.Is(err, ErrMediaTooLarge) {
			t.Errorf("%s: %v, want ErrMediaTooLarge", path, err)
		}
	}
	if data, err := DownloadLimited(ctx, srv.URL+"/small", 1024); err != nil || len(data) != 1024 {
		t.Errorf("/small no limite exato: len=%d err=%v", len(data), err)
	}
	if _, err := DownloadLimited(ctx, srv.URL+"/loop", 1024); !errors.Is(err, ErrTooManyRedirects) {
		t.Errorf("/loop: %v, want ErrTooManyRedirects", err)
	}
	_, err := DownloadLimited(ctx, srv.URL+"/missing", 1024)
	if err == nil || err.Error() != "media download failed: HTTP status 404" || IsMediaURLRefused(err) {
		t.Errorf("/missing: %v", err)
	}
}

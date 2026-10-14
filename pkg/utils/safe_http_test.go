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
	}
	for _, s := range private {
		if IsPublicIP(net.ParseIP(s)) {
			t.Errorf("IsPublicIP(%s) = true, want false", s)
		}
	}
	public := []string{"8.8.8.8", "172.32.0.1", "100.128.0.1", "2606:4700::1111", "::ffff:8.8.8.8"}
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

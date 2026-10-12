package instance_service

import (
	"testing"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/utils"
)

func TestReadFreshQR(t *testing.T) {
	fresh := utils.EncodeStoredQR("img", "code", time.Now().Add(time.Minute))
	stale := utils.EncodeStoredQR("img", "code", time.Now().Add(-time.Second))

	if q, ok := readFreshQR(fresh); !ok || q.Code != "code" {
		t.Fatalf("fresh QR rejected: %+v ok=%v", q, ok)
	}
	if _, ok := readFreshQR(stale); ok {
		t.Fatal("expired QR was served")
	}
	if _, ok := readFreshQR(""); ok {
		t.Fatal("empty column was served")
	}
	if q, ok := readFreshQR("img|code"); !ok || q.Code != "code" {
		t.Fatalf("legacy QR rejected: %+v ok=%v", q, ok)
	}
}

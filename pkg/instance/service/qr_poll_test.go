package instance_service

import (
	"errors"
	"testing"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/utils"
)

// O primeiro código costuma chegar 1-3 s depois do /connect: a rota relê o
// banco até ele aparecer, em vez de desistir na primeira leitura vazia.
func TestPollFreshQRWaitsForTheFirstCode(t *testing.T) {
	fresh := utils.EncodeStoredQR("img", "code", time.Now().Add(time.Minute))
	reads := 0
	read := func() (string, error) {
		reads++
		if reads < 3 {
			return "", nil
		}
		return fresh, nil
	}

	q, ok, err := pollFreshQR(read, time.Second, 5*time.Millisecond)
	if err != nil || !ok || q.Code != "code" {
		t.Fatalf("q=%+v ok=%v err=%v, want the code from the third read", q, ok, err)
	}
	if reads != 3 {
		t.Fatalf("reads = %d, want 3", reads)
	}
}

func TestPollFreshQRGivesUpAfterTheBudget(t *testing.T) {
	stale := utils.EncodeStoredQR("img", "code", time.Now().Add(-time.Second))
	start := time.Now()

	_, ok, err := pollFreshQR(func() (string, error) { return stale, nil }, 40*time.Millisecond, 5*time.Millisecond)
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want no code and no error", ok, err)
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond || elapsed > time.Second {
		t.Fatalf("gave up after %s, want about the 40ms budget", elapsed)
	}
}

func TestPollFreshQRReturnsReadErrors(t *testing.T) {
	dbDown := errors.New("connection refused")
	_, ok, err := pollFreshQR(func() (string, error) { return "", dbDown }, time.Second, 5*time.Millisecond)
	if !errors.Is(err, dbDown) || ok {
		t.Fatalf("ok=%v err=%v, want the read error", ok, err)
	}
}

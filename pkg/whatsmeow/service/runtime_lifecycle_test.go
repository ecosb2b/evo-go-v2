package whatsmeow_service

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestReserveRuntimeAllowsOnlyOneOwner(t *testing.T) {
	service := &whatsmeowService{}
	const workers = 64
	var owners atomic.Int32
	var ownerToken atomic.Uint64

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, _, ok := service.reserveRuntime("instance-1")
			if ok {
				owners.Add(1)
				ownerToken.Store(token)
			}
		}()
	}
	wg.Wait()

	if got := owners.Load(); got != 1 {
		t.Fatalf("expected one runtime owner, got %d", got)
	}
	service.finishRuntime("instance-1", ownerToken.Load())
	if _, _, ok := service.reserveRuntime("instance-1"); !ok {
		t.Fatal("expected a new runtime reservation after teardown")
	}
}

func TestFinishRuntimeIsIdempotentAndTokenSafe(t *testing.T) {
	service := &whatsmeowService{}
	token, done, ok := service.reserveRuntime("instance-1")
	if !ok {
		t.Fatal("expected runtime reservation")
	}

	service.finishRuntime("instance-1", token+1)
	select {
	case <-done:
		t.Fatal("stale token finished the active runtime")
	default:
	}

	service.finishRuntime("instance-1", token)
	service.finishRuntime("instance-1", token)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runtime completion channel was not closed")
	}
}

func TestBeginReconnectDeduplicatesConcurrentRequests(t *testing.T) {
	service := &whatsmeowService{}
	const workers = 64
	var accepted atomic.Int32

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := service.beginReconnect("instance-1"); ok {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := accepted.Load(); got != 1 {
		t.Fatalf("expected one reconnect owner, got %d", got)
	}

	service.finishReconnect("instance-1", false)
	failures, ok := service.beginReconnect("instance-1")
	if !ok || failures != 1 {
		t.Fatalf("expected retry with one failure, got failures=%d ok=%v", failures, ok)
	}
	service.finishReconnect("instance-1", true)
	if failures, ok = service.beginReconnect("instance-1"); !ok || failures != 0 {
		t.Fatalf("successful reconnect should reset state, got failures=%d ok=%v", failures, ok)
	}
}

func TestReconnectDelayBackoffAndCap(t *testing.T) {
	tests := []struct {
		failures int
		want     time.Duration
	}{
		{failures: 0, want: time.Second},
		{failures: 1, want: 2 * time.Second},
		{failures: 4, want: 16 * time.Second},
		{failures: 5, want: 30 * time.Second},
		{failures: 20, want: 30 * time.Second},
	}
	for _, test := range tests {
		if got := reconnectDelay(test.failures, 0); got != test.want {
			t.Errorf("failures=%d: got %s, want %s", test.failures, got, test.want)
		}
	}
	if got := reconnectDelay(0, 1); got != 1250*time.Millisecond {
		t.Fatalf("expected 25%% jitter, got %s", got)
	}
}

// Restart do supervisor não pode chegar ao cliente como LoggedOut: o ramo de
// kill do StartClient consulta esta marca antes de emitir o evento.
func TestRuntimeRestartMarkIsPerRuntime(t *testing.T) {
	service := &whatsmeowService{}
	if service.runtimeRestarting("instance-1") {
		t.Fatal("unknown instance reported as restarting")
	}

	token, _, ok := service.reserveRuntime("instance-1")
	if !ok {
		t.Fatal("expected runtime reservation")
	}
	if service.runtimeRestarting("instance-1") {
		t.Fatal("fresh runtime reported as restarting")
	}

	service.markRuntimeRestart("instance-1")
	if !service.runtimeRestarting("instance-1") {
		t.Fatal("restart mark not visible")
	}

	service.finishRuntime("instance-1", token)
	if _, _, ok := service.reserveRuntime("instance-1"); !ok {
		t.Fatal("expected a new runtime reservation")
	}
	if service.runtimeRestarting("instance-1") {
		t.Fatal("restart mark leaked into the next runtime")
	}
}

package whatsmeow_service

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
)

type publishedQR struct {
	code      string
	count     int
	expiresAt time.Time
}

type teardownCall struct {
	reason      string
	forceLogout bool
}

// fakeQREnv simula o runtime para o emissor, sem WhatsApp.
type fakeQREnv struct {
	mu        sync.Mutex
	stopped   chan struct{}
	connected atomic.Bool
	paired    atomic.Bool
	passkey   atomic.Bool
	maxCount  int
	now       time.Time
	published []publishedQR
	teardowns []teardownCall
}

func newFakeQREnv() *fakeQREnv {
	env := &fakeQREnv{stopped: make(chan struct{}), now: time.Unix(1000, 0)}
	env.connected.Store(true)
	return env
}

func (e *fakeQREnv) Stopped() <-chan struct{} { return e.stopped }
func (e *fakeQREnv) Connected() bool          { return e.connected.Load() }
func (e *fakeQREnv) Paired() bool             { return e.paired.Load() }
func (e *fakeQREnv) PasskeyActive() bool      { return e.passkey.Load() }
func (e *fakeQREnv) MaxCount() int            { return e.maxCount }
func (e *fakeQREnv) Now() time.Time           { return e.now }

func (e *fakeQREnv) Publish(code string, count int, expiresAt time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.published = append(e.published, publishedQR{code, count, expiresAt})
}

func (e *fakeQREnv) Teardown(reason string, forceLogout bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.teardowns = append(e.teardowns, teardownCall{reason, forceLogout})
}

func (e *fakeQREnv) snapshot() ([]publishedQR, []teardownCall) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]publishedQR(nil), e.published...), append([]teardownCall(nil), e.teardowns...)
}

// waitPublished espera o emissor publicar n códigos (ou falha o teste).
func (e *fakeQREnv) waitPublished(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if published, _ := e.snapshot(); len(published) >= n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("emitter did not publish %d codes in time", n)
}

var fastQRTiming = qrTiming{First: 60 * time.Millisecond, Next: 30 * time.Millisecond, Poll: 2 * time.Millisecond}

type emitterResult struct {
	end   qrEmitterEnd
	count int
}

func runAsync(env *fakeQREnv, codes []string, count int) <-chan emitterResult {
	out := make(chan emitterResult, 1)
	go func() {
		end, n := runQREmitter(env, codes, count, fastQRTiming)
		out <- emitterResult{end, n}
	}()
	return out
}

func waitResult(t *testing.T, ch <-chan emitterResult) emitterResult {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(2 * time.Second):
		t.Fatal("emitter did not return in time")
		return emitterResult{}
	}
}

func TestQREmitterPublishesAllCodesThenTearsDown(t *testing.T) {
	env := newFakeQREnv()
	r := waitResult(t, runAsync(env, []string{"a", "b", "c"}, 0))

	if r.end != qrEndExhausted || r.count != 3 {
		t.Fatalf("end=%v count=%d, want exhausted/3", r.end, r.count)
	}
	published, teardowns := env.snapshot()
	if len(published) != 3 || published[0].code != "a" || published[2].code != "c" || published[2].count != 3 {
		t.Fatalf("published = %+v", published)
	}
	if !published[0].expiresAt.Equal(env.now.Add(fastQRTiming.First)) || !published[1].expiresAt.Equal(env.now.Add(fastQRTiming.Next)) {
		t.Fatalf("first code must live First and the next ones Next: %+v", published)
	}
	if len(teardowns) != 1 || teardowns[0] != (teardownCall{"", false}) {
		t.Fatalf("teardowns = %+v, want one plain QR timeout", teardowns)
	}
}

// Runtime encerrado (restart, logout, reconnect): a rotina antiga para em
// silêncio — não sobrescreve o QR do runtime novo nem o derruba.
func TestQREmitterStopsWithRuntimeWithoutTeardown(t *testing.T) {
	env := newFakeQREnv()
	result := runAsync(env, []string{"a", "b", "c"}, 0)
	env.waitPublished(t, 1)
	close(env.stopped)

	r := waitResult(t, result)
	published, teardowns := env.snapshot()
	if r.end != qrEndStopped || len(published) != 1 || len(teardowns) != 0 {
		t.Fatalf("end=%v published=%d teardowns=%+v, want stopped/1/none", r.end, len(published), teardowns)
	}
}

// Socket do QR caiu: o código na tela morreu; derruba a sessão para o próximo
// GET /qr subir uma nova em vez de servir códigos de um socket morto.
func TestQREmitterTearsDownWhenSocketDrops(t *testing.T) {
	env := newFakeQREnv()
	result := runAsync(env, []string{"a", "b", "c"}, 0)
	env.waitPublished(t, 1)
	env.connected.Store(false)

	r := waitResult(t, result)
	published, teardowns := env.snapshot()
	want := teardownCall{instance_model.DisconnectReasonQRSocketLost, false}
	if r.end != qrEndSocketLost || len(published) != 1 || len(teardowns) != 1 || teardowns[0] != want {
		t.Fatalf("end=%v published=%d teardowns=%+v", r.end, len(published), teardowns)
	}
}

// Depois do PairSuccess o WhatsApp derruba o socket (515) e reconecta: isso
// não é queda do QR.
func TestQREmitterLeavesPairedSessionAlone(t *testing.T) {
	env := newFakeQREnv()
	result := runAsync(env, []string{"a", "b"}, 0)
	env.waitPublished(t, 1)
	env.paired.Store(true)
	env.connected.Store(false)

	r := waitResult(t, result)
	if _, teardowns := env.snapshot(); r.end != qrEndPaired || len(teardowns) != 0 {
		t.Fatalf("end=%v teardowns=%+v, want paired/none", r.end, teardowns)
	}
}

func TestQREmitterKeepsSocketForPasskey(t *testing.T) {
	env := newFakeQREnv()
	result := runAsync(env, []string{"a", "b"}, 0)
	env.waitPublished(t, 1)
	env.passkey.Store(true)

	r := waitResult(t, result)
	if _, teardowns := env.snapshot(); r.end != qrEndPaired || len(teardowns) != 0 {
		t.Fatalf("end=%v teardowns=%+v, want paired/none", r.end, teardowns)
	}
}

func TestQREmitterMaxCount(t *testing.T) {
	env := newFakeQREnv()
	env.maxCount = 2
	r := waitResult(t, runAsync(env, []string{"a", "b", "c"}, 0))

	published, teardowns := env.snapshot()
	want := teardownCall{"Maximum QR code count (2) reached", true}
	if r.end != qrEndMaxCount || len(published) != 1 || len(teardowns) != 1 || teardowns[0] != want {
		t.Fatalf("end=%v published=%+v teardowns=%+v", r.end, published, teardowns)
	}
}

// A contagem continua entre lotes de códigos do mesmo runtime.
func TestQREmitterCountContinuesAcrossBatches(t *testing.T) {
	env := newFakeQREnv()
	env.maxCount = 5
	r := waitResult(t, runAsync(env, []string{"a", "b"}, 3))

	published, _ := env.snapshot()
	if r.end != qrEndMaxCount || r.count != 5 || len(published) != 1 || published[0].count != 4 {
		t.Fatalf("end=%v count=%d published=%+v", r.end, r.count, published)
	}
}

func TestMarkRuntimeStoppingIsIdempotent(t *testing.T) {
	mycli := &MyClient{runtimeDone: make(chan struct{})}

	mycli.markRuntimeStopping()
	select {
	case <-mycli.runtimeDone:
	default:
		t.Fatal("runtimeDone must be closed after the first call")
	}
	mycli.markRuntimeStopping() // não pode dar panic (close duplo)
}

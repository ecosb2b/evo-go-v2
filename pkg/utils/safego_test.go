package utils

import (
	"testing"
	"time"
)

// Um panic em goroutine não recuperado mata o processo inteiro, derrubando
// todas as instâncias. Go precisa conter o panic e seguir.
func TestGoRecoversPanic(t *testing.T) {
	done := make(chan struct{})
	Go("test-panic", func() {
		defer close(done)
		panic("boom")
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("goroutine did not run")
	}
	// Se o panic escapasse, o binário de teste já teria abortado.
}

func TestGoRunsFunction(t *testing.T) {
	ran := make(chan struct{})
	Go("test-run", func() { close(ran) })
	select {
	case <-ran:
	case <-time.After(time.Second):
		t.Fatal("function did not run")
	}
}

func TestRecoverInsideAnonymousGoroutine(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer Recover("test-anon")
		panic("boom")
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("goroutine did not finish")
	}
}

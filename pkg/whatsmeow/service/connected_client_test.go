package whatsmeow_service

import (
	"testing"
	"time"
)

func TestAwaitSession(t *testing.T) {
	const wait = 200 * time.Millisecond
	const poll = 10 * time.Millisecond

	t.Run("connected returns at once", func(t *testing.T) {
		start := time.Now()
		ok := awaitSession(func() sessionState { return sessionConnected }, wait, poll)
		if !ok {
			t.Fatal("want ready")
		}
		if time.Since(start) > poll {
			t.Fatalf("took %s, want no wait", time.Since(start))
		}
	})

	t.Run("no session fails at once", func(t *testing.T) {
		start := time.Now()
		ok := awaitSession(func() sessionState { return sessionAbsent }, wait, poll)
		if ok {
			t.Fatal("want not ready")
		}
		if time.Since(start) > poll {
			t.Fatalf("took %s, want immediate failure", time.Since(start))
		}
	})

	t.Run("reconnecting waits until connected", func(t *testing.T) {
		calls := 0
		ok := awaitSession(func() sessionState {
			calls++
			if calls < 3 {
				return sessionReconnecting
			}
			return sessionConnected
		}, wait, poll)
		if !ok {
			t.Fatal("want ready after reconnect")
		}
	})

	t.Run("reconnecting gives up after wait", func(t *testing.T) {
		start := time.Now()
		ok := awaitSession(func() sessionState { return sessionReconnecting }, wait, poll)
		if ok {
			t.Fatal("want not ready")
		}
		// Desiste quando a próxima espera passaria do prazo, então até um
		// intervalo de poll antes de wait.
		if elapsed := time.Since(start); elapsed < wait-2*poll || elapsed > wait+100*time.Millisecond {
			t.Fatalf("took %s, want about %s", elapsed, wait)
		}
	})
}

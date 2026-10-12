package whatsmeow_service

import (
	"sync"
	"testing"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
)

// A goroutine de presença precisa parar pelo próprio canal, sem disputar o
// killChannel com o StartClient.
func TestSchedulePresenceUpdatesStopsOnStopChannel(t *testing.T) {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		schedulePresenceUpdates(&MyClient{userID: "inst-a"}, stop)
		close(done)
	}()

	close(stop)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("presence goroutine did not stop")
	}
}

func TestPresenceGenerationInvalidatedByStop(t *testing.T) {
	mycli := &MyClient{}
	mycli.presenceStop = make(chan struct{})
	mycli.presenceGen = 1

	if !mycli.presenceCurrent(1) {
		t.Fatal("generation 1 should be current while running")
	}
	if mycli.presenceCurrent(0) {
		t.Fatal("older generation reported as current")
	}

	mycli.stopPresence()
	mycli.stopPresence() // idempotente
	if mycli.presenceCurrent(1) {
		t.Fatal("generation still current after stopPresence")
	}
}

func TestCompanionActivityInterval(t *testing.T) {
	for range 1000 {
		d := companionActivityInterval()
		if d < 20*time.Hour || d >= 24*time.Hour {
			t.Fatalf("interval %s outside [20h, 24h)", d)
		}
	}
}

// Rodado com -race: troca de settings em runtime concorrendo com o handler.
func TestInstanceAccessorConcurrentSwap(t *testing.T) {
	mycli := &MyClient{instance: &instance_model.Instance{Id: "inst-a"}}

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			mycli.setInstance(&instance_model.Instance{Id: "inst-a", AlwaysOnline: i%2 == 0})
		}()
		go func() {
			defer wg.Done()
			if mycli.currentInstance().Id != "inst-a" {
				t.Error("unexpected instance")
			}
		}()
	}
	wg.Wait()
}

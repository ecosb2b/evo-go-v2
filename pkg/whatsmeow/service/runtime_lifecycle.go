package whatsmeow_service

import (
	"fmt"
	"math/rand"
	"time"
)

const (
	runtimeStopTimeout = 10 * time.Second
	reconnectBaseDelay = time.Second
	reconnectMaxDelay  = 30 * time.Second
	reconnectProbeTime = 30 * time.Second
)

type instanceRuntime struct {
	token uint64
	done  chan struct{}
}

type reconnectRuntime struct {
	inFlight bool
	failures int
}

func (w *whatsmeowService) reserveRuntime(instanceID string) (uint64, <-chan struct{}, bool) {
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	if w.runtimes == nil {
		w.runtimes = make(map[string]*instanceRuntime)
	}

	if runtime, exists := w.runtimes[instanceID]; exists {
		return runtime.token, runtime.done, false
	}

	w.runtimeSequence++
	runtime := &instanceRuntime{token: w.runtimeSequence, done: make(chan struct{})}
	w.runtimes[instanceID] = runtime
	return runtime.token, runtime.done, true
}

func (w *whatsmeowService) finishRuntime(instanceID string, token uint64) {
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()

	runtime, exists := w.runtimes[instanceID]
	if !exists || runtime.token != token {
		return
	}
	delete(w.runtimes, instanceID)
	close(runtime.done)
}

func (w *whatsmeowService) runtimeDone(instanceID string) <-chan struct{} {
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	if runtime := w.runtimes[instanceID]; runtime != nil {
		return runtime.done
	}
	return nil
}

func (w *whatsmeowService) signalRuntimeStop(instanceID string) <-chan struct{} {
	done := w.runtimeDone(instanceID)

	ClientMapsMu.RLock()
	killChan := w.killChannel[instanceID]
	ClientMapsMu.RUnlock()
	if killChan != nil {
		select {
		case killChan <- true:
		default:
		}
	}
	return done
}

func waitRuntimeDone(done <-chan struct{}) bool {
	if done == nil {
		return true
	}
	select {
	case <-done:
		return true
	case <-time.After(runtimeStopTimeout):
		return false
	}
}

func (w *whatsmeowService) beginReconnect(instanceID string) (int, bool) {
	w.reconnectMu.Lock()
	defer w.reconnectMu.Unlock()
	if w.reconnects == nil {
		w.reconnects = make(map[string]*reconnectRuntime)
	}
	state := w.reconnects[instanceID]
	if state == nil {
		state = &reconnectRuntime{}
		w.reconnects[instanceID] = state
	}
	if state.inFlight {
		return state.failures, false
	}
	state.inFlight = true
	return state.failures, true
}

func (w *whatsmeowService) finishReconnect(instanceID string, success bool) {
	w.reconnectMu.Lock()
	defer w.reconnectMu.Unlock()
	state := w.reconnects[instanceID]
	if state == nil {
		return
	}
	state.inFlight = false
	if success {
		delete(w.reconnects, instanceID)
	} else {
		state.failures++
	}
}

func reconnectDelay(failures int, jitter float64) time.Duration {
	if failures < 0 {
		failures = 0
	}
	delay := reconnectBaseDelay
	for attempt := 0; attempt < failures && delay < reconnectMaxDelay; attempt++ {
		delay *= 2
		if delay > reconnectMaxDelay {
			delay = reconnectMaxDelay
		}
	}
	if jitter < 0 {
		jitter = 0
	}
	if jitter > 1 {
		jitter = 1
	}
	return delay + time.Duration(float64(delay)*0.25*jitter)
}

func (w *whatsmeowService) restartInstance(instanceID string) error {
	w.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] Restarting instance runtime", instanceID)
	done := w.signalRuntimeStop(instanceID)
	if !waitRuntimeDone(done) {
		return fmt.Errorf("timed out stopping runtime for instance %s", instanceID)
	}

	instance, err := w.instanceRepository.GetInstanceByID(instanceID)
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}
	w.userInfoCache.Delete(instance.Token)
	if err := w.instanceRepository.UpdateConnected(instanceID, false, "Reconnecting"); err != nil {
		w.loggerWrapper.GetLogger(instanceID).LogWarn("[%s] Failed to update reconnecting status: %v", instanceID, err)
	}
	return w.StartInstance(instanceID)
}

func (w *whatsmeowService) runtimeConnected(instanceID string) bool {
	ClientMapsMu.RLock()
	client := w.clientPointer[instanceID]
	ClientMapsMu.RUnlock()
	return client != nil && client.IsConnected() && client.IsLoggedIn()
}

func (w *whatsmeowService) ScheduleReconnect(instanceID string) {
	instance, err := w.instanceRepository.GetInstanceByID(instanceID)
	if err != nil || instance.Jid == "" {
		return
	}
	failures, ok := w.beginReconnect(instanceID)
	if !ok {
		return
	}

	go func() {
		delay := reconnectDelay(failures, rand.Float64())
		w.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] Reconnection scheduled in %s", instanceID, delay)
		timer := time.NewTimer(delay)
		defer timer.Stop()
		<-timer.C

		err := w.restartInstance(instanceID)
		connected := false
		if err == nil {
			deadline := time.NewTimer(reconnectProbeTime)
			ticker := time.NewTicker(500 * time.Millisecond)
		probe:
			for {
				select {
				case <-ticker.C:
					if w.runtimeConnected(instanceID) {
						connected = true
						break probe
					}
				case <-deadline.C:
					break probe
				}
			}
			ticker.Stop()
			if !deadline.Stop() {
				select {
				case <-deadline.C:
				default:
				}
			}
		}

		w.finishReconnect(instanceID, connected)
		if !connected {
			if err != nil {
				w.loggerWrapper.GetLogger(instanceID).LogError("[%s] Reconnection failed: %v", instanceID, err)
			}
			w.ScheduleReconnect(instanceID)
		}
	}()
}

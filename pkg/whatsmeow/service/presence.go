package whatsmeow_service

import (
	"context"
	"math/rand"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// Presença do companion. Duas regras opostas precisam conviver:
//   - ficar "available" faz o WhatsApp entregar na sessão vinculada e
//     suprimir a notificação no celular;
//   - ficar sem atividade faz o celular desvincular o companion (~14 dias) e
//     o WhatsApp derrubá-lo (~30 dias), mesmo conectado.
//
// alwaysOnline=true mantém available com o ciclo periódico de sempre.
// alwaysOnline=false manda um pulso curto de available a cada ~20h e volta a
// unavailable. [upstream#213]
//
// Nenhuma goroutine daqui lê o killChannel: ele tem um único receptor, o
// StartClient, e um segundo receptor roubava o sinal de parada do runtime.

// reconcilePresence aplica o alwaysOnline atual. É idempotente: chamado em
// todo Connected/PushNameSetting e quando as settings mudam em runtime.
func (mycli *MyClient) reconcilePresence() {
	if mycli == nil || mycli.WAClient == nil || !mycli.WAClient.IsLoggedIn() {
		return
	}
	online := mycli.currentInstance().AlwaysOnline

	mycli.presenceMu.Lock()
	if mycli.presenceStop != nil && mycli.presenceOnline == online {
		mycli.presenceMu.Unlock()
		return
	}
	if mycli.presenceStop != nil {
		close(mycli.presenceStop)
	}
	stop := make(chan struct{})
	mycli.presenceStop = stop
	mycli.presenceOnline = online
	mycli.presenceGen++
	gen := mycli.presenceGen
	mycli.presenceMu.Unlock()

	logger := mycli.loggerWrapper.GetLogger(mycli.userID)
	if online {
		go schedulePresenceUpdates(mycli, stop)
		if err := mycli.WAClient.SendPresence(context.Background(), types.PresenceAvailable); err != nil {
			logger.LogWarn("[%s] Failed to send available presence %v", mycli.userID, err)
		} else {
			logger.LogInfo("[%s] Marked self as available (alwaysOnline=true)", mycli.userID)
		}
		return
	}

	if err := mycli.WAClient.SendPresence(context.Background(), types.PresenceUnavailable); err != nil {
		logger.LogWarn("[%s] Failed to send unavailable presence %v", mycli.userID, err)
	} else {
		logger.LogInfo("[%s] Marked self as unavailable (alwaysOnline=false)", mycli.userID)
	}
	go scheduleCompanionActivityPulse(mycli, stop, gen)
}

// stopPresence encerra as goroutines de presença do runtime.
func (mycli *MyClient) stopPresence() {
	if mycli == nil {
		return
	}
	mycli.presenceMu.Lock()
	defer mycli.presenceMu.Unlock()
	if mycli.presenceStop != nil {
		close(mycli.presenceStop)
		mycli.presenceStop = nil
	}
}

func (mycli *MyClient) presenceCurrent(gen uint64) bool {
	mycli.presenceMu.Lock()
	defer mycli.presenceMu.Unlock()
	return mycli.presenceStop != nil && mycli.presenceGen == gen
}

// schedulePresenceUpdates mantém o ciclo de alwaysOnline=true: a cada 1-3h
// alterna unavailable e available, terminando em available.
func schedulePresenceUpdates(mycli *MyClient, stop <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if _, err := mycli.instanceRepository.GetInstanceByID(mycli.userID); err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Instance no longer exists, stopping presence updates", mycli.userID)
				return
			}

			processPresenceUpdates(mycli)

			ticker.Reset(time.Duration(1+rand.Intn(3)) * time.Hour)
		}
	}
}

func processPresenceUpdates(mycli *MyClient) {
	logger := mycli.loggerWrapper.GetLogger(mycli.userID)
	if err := mycli.WAClient.SendPresence(context.Background(), types.PresenceUnavailable); err != nil {
		logger.LogError("[%s] Failed to set presence as unavailable %v", mycli.userID, err)
	}

	time.Sleep(time.Duration(1+rand.Intn(5)) * time.Second)

	if err := mycli.WAClient.SendPresence(context.Background(), types.PresenceAvailable); err != nil {
		logger.LogError("[%s] Failed to set presence as available %v", mycli.userID, err)
	} else {
		logger.LogInfo("[%s] Marked self as available", mycli.userID)
	}
}

// scheduleCompanionActivityPulse dá o pulso de atividade de alwaysOnline=false.
func scheduleCompanionActivityPulse(mycli *MyClient, stop <-chan struct{}, gen uint64) {
	timer := time.NewTimer(companionActivityInterval())
	defer timer.Stop()
	for {
		select {
		case <-stop:
			return
		case <-timer.C:
			if !mycli.presenceCurrent(gen) || !mycli.WAClient.IsLoggedIn() {
				return
			}
			pulseCompanionActivity(mycli, gen)
			timer.Reset(companionActivityInterval())
		}
	}
}

// pulseCompanionActivity registra uso do companion e volta a unavailable.
func pulseCompanionActivity(mycli *MyClient, gen uint64) {
	logger := mycli.loggerWrapper.GetLogger(mycli.userID)
	if err := mycli.WAClient.SendPresence(context.Background(), types.PresenceAvailable); err != nil {
		logger.LogWarn("[%s] Failed to send activity presence: %v", mycli.userID, err)
		return
	}
	time.Sleep(time.Duration(3+rand.Intn(5)) * time.Second)

	var err error
	for attempt := 0; attempt < 3; attempt++ {
		// Se a configuração mudou no meio do pulso, quem manda é ela.
		if !mycli.presenceCurrent(gen) || !mycli.WAClient.IsLoggedIn() {
			return
		}
		if err = mycli.WAClient.SendPresence(context.Background(), types.PresenceUnavailable); err == nil {
			logger.LogInfo("[%s] Companion activity pulsed; presence restored to unavailable", mycli.userID)
			return
		}
		time.Sleep(time.Second)
	}
	logger.LogWarn("[%s] Failed to restore unavailable presence: %v", mycli.userID, err)
}

func companionActivityInterval() time.Duration {
	return 20*time.Hour + time.Duration(rand.Intn(4*60))*time.Minute
}

package whatsmeow_service

import (
	"context"
	"errors"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/evolution-foundation/evolution-go/pkg/utils"
)

// Recuperação de app state (mute, pin, arquivo, contatos, etiquetas) quando o
// sync falha. Sem isso a coleção fica travada até alguém reparear. [upstream#144]
//
// Sequência documentada pelo whatsmeow: sync incremental que falha vira um
// full sync; se o snapshot completo também não verificar, pede ao celular um
// snapshot de recuperação. Nunca usa a recuperação "fatal", que desvincula
// todos os companions e exige novo QR.

const appStateRecoveryCooldown = 15 * time.Minute

type appStateRecoveryAttempt struct {
	fullSyncAt        time.Time
	recoveryRequestAt time.Time
}

// reserveAppStateRecovery limita cada estágio a uma tentativa por coleção a
// cada appStateRecoveryCooldown.
func (mycli *MyClient) reserveAppStateRecovery(name appstate.WAPatchName, recoveryRequest bool) bool {
	mycli.appStateRecoveryMu.Lock()
	defer mycli.appStateRecoveryMu.Unlock()

	if mycli.appStateRecovery == nil {
		mycli.appStateRecovery = make(map[appstate.WAPatchName]appStateRecoveryAttempt)
	}

	now := time.Now()
	attempt := mycli.appStateRecovery[name]
	last := attempt.fullSyncAt
	if recoveryRequest {
		last = attempt.recoveryRequestAt
	}
	if !last.IsZero() && now.Sub(last) < appStateRecoveryCooldown {
		return false
	}

	if recoveryRequest {
		attempt.recoveryRequestAt = now
	} else {
		attempt.fullSyncAt = now
	}
	mycli.appStateRecovery[name] = attempt
	return true
}

// isAppStateVerificationError indica um snapshot que não verificou. A partir
// do whatsmeow de 2026-10 o AppStateSyncError com FullSync também sai quando o
// fetch falha por rede; nesse caso pedir recovery ao celular é desperdício.
func isAppStateVerificationError(err error) bool {
	return errors.Is(err, appstate.ErrMismatchingLTHash) ||
		errors.Is(err, appstate.ErrMismatchingPatchMAC) ||
		errors.Is(err, appstate.ErrMismatchingContentMAC) ||
		errors.Is(err, appstate.ErrMismatchingIndexMAC) ||
		errors.Is(err, appstate.ErrMissingPreviousSetValueOperation)
}

func (mycli *MyClient) handleAppStateSyncError(evt *events.AppStateSyncError) {
	if evt == nil || mycli.WAClient == nil {
		return
	}
	logger := mycli.loggerWrapper.GetLogger(mycli.userID)

	recoveryRequest := evt.FullSync
	if recoveryRequest && !isAppStateVerificationError(evt.Error) {
		logger.LogWarn("[%s] App-state full sync of %s failed without a verification error; not requesting recovery: %v", mycli.userID, evt.Name, evt.Error)
		return
	}
	if !mycli.reserveAppStateRecovery(evt.Name, recoveryRequest) {
		logger.LogInfo("[%s] App-state recovery already attempted recently for %s (fullSync=%t)", mycli.userID, evt.Name, evt.FullSync)
		return
	}

	utils.Go("appStateRecovery", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if !recoveryRequest {
			logger.LogWarn("[%s] App-state incremental sync failed for %s; starting full sync: %v", mycli.userID, evt.Name, evt.Error)
			if err := mycli.WAClient.FetchAppState(ctx, evt.Name, true, false); err != nil {
				// Se o snapshot não verificar, o whatsmeow emite outro
				// AppStateSyncError com FullSync e o próximo estágio assume.
				logger.LogWarn("[%s] App-state full sync did not complete for %s: %v", mycli.userID, evt.Name, err)
			}
			return
		}

		logger.LogWarn("[%s] App-state full sync failed verification for %s; requesting recovery snapshot from the phone", mycli.userID, evt.Name)
		if _, err := mycli.WAClient.SendPeerMessage(ctx, whatsmeow.BuildAppStateRecoveryRequest(evt.Name)); err != nil {
			logger.LogError("[%s] Failed to send app-state recovery request for %s: %v", mycli.userID, evt.Name, err)
			return
		}
		logger.LogInfo("[%s] App-state recovery request sent for %s", mycli.userID, evt.Name)
	})
}

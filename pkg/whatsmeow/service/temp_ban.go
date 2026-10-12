package whatsmeow_service

import (
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
)

// banWait diz quanto falta para o ban gravado no motivo terminar (0 se não há
// ban ou se ele já passou).
func banWait(reason string, now time.Time) time.Duration {
	until, ok := instance_model.TemporaryBanUntil(reason)
	if !ok || !now.Before(until) {
		return 0
	}
	return until.Sub(now)
}

// pendingBanWait lê o motivo gravado da instância e devolve a espera do ban.
func (w *whatsmeowService) pendingBanWait(instanceID string) time.Duration {
	instance, err := w.instanceRepository.GetInstanceByID(instanceID)
	if err != nil || instance == nil {
		return 0
	}
	return banWait(instance.DisconnectReason, time.Now())
}

// tempBanUnknownWait: espera usada quando o WhatsApp não informa o prazo do ban.
const tempBanUnknownWait = time.Hour

// temporaryBanUntil calcula quando o ban termina.
func temporaryBanUntil(now time.Time, expire time.Duration) time.Time {
	if expire <= 0 {
		expire = tempBanUnknownWait
	}
	return now.Add(expire)
}

// recordDisconnect grava a instância como desconectada com o motivo dado.
func (mycli *MyClient) recordDisconnect(reason string) {
	instance := mycli.currentInstance()
	instance.Connected = false
	instance.DisconnectReason = reason
	if err := mycli.instanceRepository.UpdateConnected(instance.Id, false, reason); err != nil {
		mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Error updating instance: %s", instance.Id, err)
	}
}

package whatsmeow_service

import "time"

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

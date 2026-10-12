package whatsmeow_service

import (
	"errors"
	"time"

	"go.mau.fi/whatsmeow"
)

// ErrInstanceNotConnected indica que a instância não tem sessão pronta para
// enviar: nunca foi pareada, foi desconectada ou continua caída após a espera.
var ErrInstanceNotConnected = errors.New("instance is not connected to WhatsApp")

// sessionWait é quanto um envio espera o supervisor terminar uma reconexão
// antes de desistir.
const sessionWait = 5 * time.Second

type sessionState int

const (
	// sessionAbsent: sem cliente ou sem pareamento. Esperar não adianta.
	sessionAbsent sessionState = iota
	// sessionReconnecting: pareada, com o socket caído; o supervisor religa.
	sessionReconnecting
	sessionConnected
)

// awaitSession consulta o estado até a sessão ficar pronta. Desiste na hora se
// não houver sessão e após wait se ela continuar reconectando.
func awaitSession(state func() sessionState, wait, poll time.Duration) bool {
	deadline := time.Now().Add(wait)
	for {
		switch state() {
		case sessionConnected:
			return true
		case sessionAbsent:
			return false
		}
		if time.Now().Add(poll).After(deadline) {
			return false
		}
		time.Sleep(poll)
	}
}

func (w *whatsmeowService) clientSession(instanceID string) (*whatsmeow.Client, sessionState) {
	ClientMapsMu.RLock()
	client := w.clientPointer[instanceID]
	ClientMapsMu.RUnlock()

	switch {
	case client == nil || client.Store == nil || client.Store.ID == nil:
		return client, sessionAbsent
	case client.IsConnected() && client.IsLoggedIn():
		return client, sessionConnected
	default:
		return client, sessionReconnecting
	}
}

// ConnectedClient devolve o cliente pronto para enviar, sem iniciar nem
// reiniciar nada: quem reconecta é o supervisor. Se a instância está pareada
// mas reconectando, espera até sessionWait; se não há sessão, falha na hora.
func (w *whatsmeowService) ConnectedClient(instanceID string) (*whatsmeow.Client, error) {
	var client *whatsmeow.Client
	reviving := false
	ready := awaitSession(func() sessionState {
		var state sessionState
		client, state = w.clientSession(instanceID)
		if state != sessionAbsent || client != nil {
			return state
		}
		// Sem runtime: instância pareada cujo start falhou ou que está entre
		// o stop e o start de uma reconexão. Se ela deve estar no ar, pede a
		// reconexão ao supervisor (deduplicada) e espera em vez de falhar.
		if !reviving {
			if w.reconnectDecision(instanceID) == reconnectStop {
				return sessionAbsent
			}
			w.ScheduleReconnect(instanceID)
			reviving = true
		}
		return sessionReconnecting
	}, sessionWait, 250*time.Millisecond)
	if !ready {
		return nil, ErrInstanceNotConnected
	}
	return client, nil
}

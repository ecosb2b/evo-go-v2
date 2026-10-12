package whatsmeow_service

import (
	"sync"

	"go.mau.fi/whatsmeow/store"
)

// store.DeviceProps.Version só descreve o device anunciado no pareamento. O
// handshake usa a versão global do pacote store, que só muda via
// store.SetWAVersion, e isso nunca era chamado: a versão baixada da web era
// descartada e a conexão seguia com a compilada no whatsmeow até o WhatsApp
// recusar com "Client outdated (405)". [upstream#199]
//
// A versão é global e o whatsmeow a lê sem lock, então só a gravamos quando
// muda de fato; o mutex serializa as instâncias que sobem em paralelo.
var waVersionMu sync.Mutex

// applyWAVersion leva a versão ao handshake. explicit indica versão
// configurada por env, aplicada mesmo se for mais antiga que a compilada; a
// baixada da web só entra se for mais nova.
func applyWAVersion(v clientVersion, explicit bool) bool {
	waVersionMu.Lock()
	defer waVersionMu.Unlock()

	if !shouldApplyWAVersion(store.GetWAVersion(), v, explicit) {
		return false
	}
	store.SetWAVersion(store.WAVersionContainer{uint32(v.Major), uint32(v.Minor), uint32(v.Patch)})
	return true
}

func shouldApplyWAVersion(current store.WAVersionContainer, v clientVersion, explicit bool) bool {
	next := store.WAVersionContainer{uint32(v.Major), uint32(v.Minor), uint32(v.Patch)}
	if next.IsZero() || next == current {
		return false
	}
	if explicit {
		return true
	}
	for i := range next {
		if next[i] != current[i] {
			return next[i] > current[i]
		}
	}
	return false
}

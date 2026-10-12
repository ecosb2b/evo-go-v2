package instance_model

import (
	"strings"
	"time"
)

// DisconnectReasonQRSocketLost: o socket da sessão de QR caiu antes do scan. O
// código na tela morreu junto; é preciso gerar outro.
const DisconnectReasonQRSocketLost = "QR socket closed before pairing"

const (
	// DisconnectReasonStreamReplaced: outro processo conectou com as mesmas
	// chaves. Religar sozinho brigaria com ele.
	DisconnectReasonStreamReplaced = "Stream replaced by another connection"
	// DisconnectReasonClientOutdated: o WhatsApp recusou a versão do cliente (405).
	DisconnectReasonClientOutdated = "405: client outdated"

	temporaryBanPrefix = "Temporary ban until "
)

// TemporaryBanReason monta o disconnect_reason de um ban temporário, com a data
// em que ele termina (o supervisor espera até lá antes de religar).
func TemporaryBanReason(until time.Time, code string) string {
	return temporaryBanPrefix + until.UTC().Format(time.RFC3339) + ": " + code
}

// TemporaryBanUntil lê a data de fim de um motivo montado por TemporaryBanReason.
func TemporaryBanUntil(reason string) (time.Time, bool) {
	if !strings.HasPrefix(reason, temporaryBanPrefix) {
		return time.Time{}, false
	}
	rest := reason[len(temporaryBanPrefix):]
	end := strings.Index(rest, ": ")
	if end < 0 {
		return time.Time{}, false
	}
	until, err := time.Parse(time.RFC3339, rest[:end])
	if err != nil {
		return time.Time{}, false
	}
	return until, true
}

// requiresQRPrefixes: motivos em que o aparelho foi desvinculado (ou nunca
// chegou a ser) e só um QR novo resolve. Nesses casos o jid continua gravado,
// então "tem número" não basta para saber se dá para só reconectar.
var requiresQRPrefixes = []string{
	"401:", "403:", "406:",
	DisconnectReasonLogoutByAPI,
	"Maximum QR code count",
	DisconnectReasonQRSocketLost,
}

// RequiresQR diz se a instância (desconectada) precisa de um QR novo.
func RequiresQR(jid, reason string) bool {
	if jid == "" {
		return true
	}
	for _, prefix := range requiresQRPrefixes {
		if strings.HasPrefix(reason, prefix) {
			return true
		}
	}
	return false
}

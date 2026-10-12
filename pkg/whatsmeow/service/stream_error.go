package whatsmeow_service

import "fmt"

// O WhatsApp fecha o socket com <stream:error> por motivos que o whatsmeow
// não trata (ex.: <ack class="status" type="media"/>, issue #211). O
// Disconnected que vem logo depois gravava só uma frase genérica, sem dizer o
// porquê. O stream:error fica guardado no runtime e entra no motivo.

const (
	disconnectedByServer   = "Disconnected emitted because the websocket is closed by the server."
	maxDisconnectReasonLen = 500
)

func (mycli *MyClient) recordStreamError(code, rawXML string) {
	mycli.streamErrorMu.Lock()
	mycli.lastStreamError = fmt.Sprintf("stream:error code=%s %s", code, rawXML)
	mycli.streamErrorMu.Unlock()
}

// takeStreamError devolve e limpa o último stream:error, para não ser
// atribuído a uma desconexão posterior.
func (mycli *MyClient) takeStreamError() string {
	mycli.streamErrorMu.Lock()
	defer mycli.streamErrorMu.Unlock()
	s := mycli.lastStreamError
	mycli.lastStreamError = ""
	return s
}

func disconnectReason(streamError string) string {
	if streamError == "" {
		return disconnectedByServer
	}
	reason := "Disconnected by the server after " + streamError
	if len(reason) > maxDisconnectReasonLen {
		reason = reason[:maxDisconnectReasonLen]
	}
	return reason
}

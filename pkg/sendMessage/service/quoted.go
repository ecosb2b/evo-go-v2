package send_service

import (
	"encoding/json"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// QuotedKey aceita o formato da Evolution API v2 ({"key":{"id","participant"}}).
type QuotedKey struct {
	ID          string `json:"id"`
	Participant string `json:"participant"`
}

type QuotedStruct struct {
	MessageID   string `json:"messageId"`
	Participant string `json:"participant"`
	// Conteúdo da mensagem citada. Sem ele o WhatsApp mostra um card de
	// citação vazio e que não navega até a original (#189): o servidor não
	// guarda o conteúdo das mensagens, então quem cita precisa enviá-lo.
	// Text cobre o caso simples; Message aceita o objeto "message" recebido
	// no webhook e tem prioridade.
	Text    string          `json:"text,omitempty"`
	Message json.RawMessage `json:"message,omitempty"`
	Key     *QuotedKey      `json:"key,omitempty"`
}

// normalized copia key.id/key.participant quando messageId/participant não
// vieram; os campos explícitos têm prioridade.
func (q QuotedStruct) normalized() QuotedStruct {
	if q.Key != nil {
		if q.MessageID == "" {
			q.MessageID = q.Key.ID
		}
		if q.Participant == "" {
			q.Participant = q.Key.Participant
		}
	}
	return q
}

// buildQuotedMessage monta o QuotedMessage do ContextInfo a partir do que o
// cliente enviou. Sem conteúdo, mantém o card vazio de antes. Se quoted.message
// não decodificar, usa quoted.text e devolve o erro para log: tipos com
// campos oneof (botões, interativos, carrossel) não passam por encoding/json.
func buildQuotedMessage(q QuotedStruct) (*waE2E.Message, error) {
	if len(q.Message) > 0 {
		var msg waE2E.Message
		err := json.Unmarshal(q.Message, &msg)
		if err == nil {
			return &msg, nil
		}
		return &waE2E.Message{Conversation: proto.String(q.Text)}, err
	}
	return &waE2E.Message{Conversation: proto.String(q.Text)}, nil
}

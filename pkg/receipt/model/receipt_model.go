package receipt_model

import (
	"time"
)

// MessageReceipt guarda o andamento de uma mensagem ENVIADA pela instância:
// quando saiu, quando chegou ao celular do contato, quando foi lida e quando
// o áudio/vídeo foi reproduzido. Só conversas individuais (grupos ficam de
// fora). Guarda ids e datas — nunca o conteúdo da mensagem.
//
// Cada coluna *_at guarda a PRIMEIRA vez que o estado foi visto, e o status
// sai das colunas preenchidas (ver Status), então recibos fora de ordem nunca
// fazem a mensagem "voltar" de lida para entregue.
type MessageReceipt struct {
	ID          uint       `json:"-" gorm:"primaryKey"`
	InstanceID  string     `json:"-" gorm:"size:64;not null;uniqueIndex:idx_receipt_instance_message"`
	MessageID   string     `json:"messageId" gorm:"size:128;not null;uniqueIndex:idx_receipt_instance_message"`
	Chat        string     `json:"chat"`
	SentAt      *time.Time `json:"sentAt"`
	DeliveredAt *time.Time `json:"deliveredAt"`
	ReadAt      *time.Time `json:"readAt"`
	PlayedAt    *time.Time `json:"playedAt"`
	CreatedAt   time.Time  `json:"-"`
	UpdatedAt   time.Time  `json:"-" gorm:"index"`
}

// State é um evento que muda o andamento da mensagem.
type State string

const (
	StateSent      State = "sent"
	StateDelivered State = "delivered"
	StateRead      State = "read"
	StatePlayed    State = "played"
)

// Status públicos, do menos para o mais avançado.
const (
	StatusUnknown   = "unknown" // sem registro (mensagem antiga, de grupo ou de outra instância)
	StatusSent      = "sent"
	StatusDelivered = "delivered"
	StatusRead      = "read"
	StatusPlayed    = "played"
)

// Apply marca o estado na linha. Lida e reproduzida implicam entregue: se o
// recibo de entrega se perdeu, a entrega recebe a mesma data.
func Apply(r *MessageReceipt, state State, at time.Time) {
	t := at.UTC()
	switch state {
	case StateSent:
		r.SentAt = &t
	case StateDelivered:
		r.DeliveredAt = &t
	case StateRead:
		r.ReadAt = &t
		r.DeliveredAt = &t
	case StatePlayed:
		r.PlayedAt = &t
		r.DeliveredAt = &t
	}
}

// Status devolve o estado mais avançado já registrado.
func Status(r *MessageReceipt) string {
	switch {
	case r == nil:
		return StatusUnknown
	case r.PlayedAt != nil:
		return StatusPlayed
	case r.ReadAt != nil:
		return StatusRead
	case r.DeliveredAt != nil:
		return StatusDelivered
	case r.SentAt != nil:
		return StatusSent
	default:
		return StatusUnknown
	}
}

// View é o formato da API: o registro mais o status calculado.
type View struct {
	MessageID   string     `json:"messageId"`
	Chat        string     `json:"chat"`
	Status      string     `json:"status"`
	SentAt      *time.Time `json:"sentAt"`
	DeliveredAt *time.Time `json:"deliveredAt"`
	ReadAt      *time.Time `json:"readAt"`
	PlayedAt    *time.Time `json:"playedAt"`
}

// ToView monta a resposta de um id; r == nil vira "unknown" com datas nulas.
func ToView(messageID string, r *MessageReceipt) View {
	v := View{MessageID: messageID, Status: Status(r)}
	if r != nil {
		v.Chat = r.Chat
		v.SentAt = utc(r.SentAt)
		v.DeliveredAt = utc(r.DeliveredAt)
		v.ReadAt = utc(r.ReadAt)
		v.PlayedAt = utc(r.PlayedAt)
	}
	return v
}

// utc: o driver devolve as datas no fuso da sessão do banco; a API responde
// sempre em UTC.
func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

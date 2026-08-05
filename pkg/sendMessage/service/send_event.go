package send_service

import (
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// ============================================================================
// [Athene] Envio de evento/agenda do WhatsApp (waE2E.EventMessage).
// Portado do PR #90 do upstream, com import path adaptado (evolution-foundation).
// ============================================================================

// EventTime aceita epoch em segundos (número ou string) OU um timestamp ISO 8601
// (RFC3339) com timezone, normalizando tudo para epoch em segundos.
type EventTime int64

func (t *EventTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		*t = EventTime(n)
		return nil
	}
	if tm, err := time.Parse(time.RFC3339, s); err == nil {
		*t = EventTime(tm.Unix())
		return nil
	}
	return fmt.Errorf("invalid time %q: use ISO 8601 (RFC3339) or epoch seconds", s)
}

// Unix devolve o horário em epoch (segundos).
func (t EventTime) Unix() int64 { return int64(t) }

// EventLocationStruct é o local opcional anexado ao evento.
type EventLocationStruct struct {
	Name      string  `json:"name,omitempty" example:"Sede Grupo Mirandas"`
	Latitude  float64 `json:"latitude,omitempty" example:"-16.6869"`
	Longitude float64 `json:"longitude,omitempty" example:"-49.2648"`
	Address   string  `json:"address,omitempty" example:"Av. Principal, 1000"`
}

// EventStruct é o body de POST /send/event.
//
// Envia um evento/agenda do WhatsApp. `startTime`/`endTime` aceitam string ISO 8601
// (RFC3339) com timezone ou epoch em segundos. Só `number`, `name` e `startTime`
// são obrigatórios. Normalmente enviado a um JID de grupo (…@g.us).
type EventStruct struct {
	Number string `json:"number" example:"120363000000000000@g.us"`
	Name   string `json:"name" example:"Reuniao de vendas"`

	Description string `json:"description,omitempty"`
	// Texto opcional enviado ANTES do card do evento (funciona como legenda; o
	// evento não tem campo de legenda). Respeita mentionAll/mentionedJid/delay.
	Text string `json:"text,omitempty"`

	StartTime EventTime `json:"startTime" swaggertype:"string" example:"2026-06-25T20:00:00-03:00"`
	EndTime   EventTime `json:"endTime,omitempty" swaggertype:"string"`

	Location *EventLocationStruct `json:"location,omitempty"`
	// Link de chamada (apenas call.whatsapp.com; links externos vão em description).
	JoinLink string `json:"joinLink,omitempty"`

	ExtraGuestsAllowed bool  `json:"extraGuestsAllowed,omitempty"`
	IsScheduleCall     bool  `json:"isScheduleCall,omitempty"`
	HasReminder        bool  `json:"hasReminder,omitempty"`
	ReminderOffsetSec  int64 `json:"reminderOffsetSec,omitempty"`
	IsCanceled         bool  `json:"isCanceled,omitempty"`

	Id           string       `json:"id,omitempty"`
	Delay        int32        `json:"delay,omitempty"`
	MentionedJID []string     `json:"mentionedJid,omitempty"`
	MentionAll   bool         `json:"mentionAll,omitempty"`
	FormatJid    *bool        `json:"formatJid,omitempty"`
	Quoted       QuotedStruct `json:"quoted,omitempty"`
}

func (s *sendService) SendEvent(data *EventStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	_, err := s.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	// Texto-legenda opcional: o EventMessage não tem legenda, então quando `text`
	// é informado ele vai como uma mensagem de texto separada, logo antes do card.
	if data.Text != "" {
		if _, err := s.SendText(&TextStruct{
			Number:       data.Number,
			Text:         data.Text,
			Delay:        data.Delay,
			MentionAll:   data.MentionAll,
			MentionedJID: data.MentionedJID,
			FormatJid:    data.FormatJid,
		}, instance); err != nil {
			return nil, fmt.Errorf("failed to send event text: %w", err)
		}
	}

	event := &waE2E.EventMessage{
		Name:      proto.String(data.Name),
		StartTime: proto.Int64(data.StartTime.Unix()),
	}
	if data.Description != "" {
		event.Description = proto.String(data.Description)
	}
	if data.EndTime.Unix() > 0 {
		event.EndTime = proto.Int64(data.EndTime.Unix())
	}
	if data.JoinLink != "" {
		event.JoinLink = proto.String(data.JoinLink)
	}
	// O cliente oficial sempre envia esses booleans explicitamente na criação;
	// o servidor espera que estejam presentes (um *bool nil é descartado do fio e
	// o evento é silenciosamente ignorado), então setamos sem condição.
	event.IsCanceled = proto.Bool(data.IsCanceled)
	event.IsScheduleCall = proto.Bool(data.IsScheduleCall)
	event.ExtraGuestsAllowed = proto.Bool(data.ExtraGuestsAllowed)
	if data.HasReminder {
		event.HasReminder = proto.Bool(true)
		if data.ReminderOffsetSec > 0 {
			event.ReminderOffsetSec = proto.Int64(data.ReminderOffsetSec)
		}
	}
	if data.Location != nil {
		event.Location = &waE2E.LocationMessage{
			DegreesLatitude:  proto.Float64(data.Location.Latitude),
			DegreesLongitude: proto.Float64(data.Location.Longitude),
			Name:             proto.String(data.Location.Name),
			Address:          proto.String(data.Location.Address),
		}
	}

	// MessageSecret (32 bytes) é necessário pra que as respostas do evento
	// (vou/não vou) possam ser descriptografadas — mesmo padrão do BuildPollCreation.
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("failed to generate event message secret: %w", err)
	}

	msg := &waE2E.Message{
		EventMessage:       event,
		MessageContextInfo: &waE2E.MessageContextInfo{MessageSecret: secret},
	}

	return s.SendMessage(instance, msg, "EventMessage", &SendDataStruct{
		Id:           data.Id,
		Number:       data.Number,
		Quoted:       data.Quoted,
		Delay:        data.Delay,
		MentionAll:   data.MentionAll,
		MentionedJID: data.MentionedJID,
		FormatJid:    data.FormatJid,
	})
}

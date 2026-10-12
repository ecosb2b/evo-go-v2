package message_service

import (
	"context"
	"errors"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
)

type SubscribePresenceStruct struct {
	Number string `json:"number"`
}

type SubscribePresenceResult struct {
	Subscribed bool `json:"subscribed"`
	// Warning explica por que os eventos podem não chegar (ver presenceWarning).
	Warning string `json:"warning,omitempty"`
}

// presenceWarning: o WhatsApp só entrega a presença de terceiros a quem está
// online. Com alwaysOnline=false a instância fica offline de propósito, para
// não suprimir as notificações no celular, e por isso não forçamos available
// aqui como o PR original fazia.
const presenceWarning = "alwaysOnline is false: WhatsApp only delivers contact presence while the instance is online, so Presence events will not arrive until alwaysOnline is enabled"

// SubscribePresence assina a presença (online / visto por último) de um
// contato; os eventos Presence passam a chegar no webhook. A assinatura vale
// até a próxima reconexão. [upstream#152]
func (m *messageService) SubscribePresence(data *SubscribePresenceStruct, instance *instance_model.Instance) (*SubscribePresenceResult, error) {
	client, err := m.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	recipient, ok := utils.ParseJID(data.Number)
	if !ok {
		return nil, errors.New("invalid phone number")
	}
	// Nós de protocolo saem sem usync: o JID precisa estar sem o "+".
	recipient = utils.CanonicalJID(recipient)

	if err := client.SubscribePresence(context.Background(), recipient); err != nil {
		return nil, err
	}
	m.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Subscribed to presence of %s", instance.Id, recipient)

	result := &SubscribePresenceResult{Subscribed: true}
	if !instance.AlwaysOnline {
		result.Warning = presenceWarning
	}
	return result, nil
}

package instance_service

import (
	"strings"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	event_types "github.com/evolution-foundation/evolution-go/pkg/internal/event_types"
)

// [Athene/PR#136] applyConnectSettings altera a instância APENAS nos campos
// explicitamente informados. Subscribe vazio mantém os Events atuais; só cai
// para MESSAGE quando não há Events salvos. Strings de produtor vazias mantêm o
// valor atual; mande "disabled"/"false" para desligar. Devolve o mapa de colunas
// a atualizar no banco (update parcial).
func applyConnectSettings(instance *instance_model.Instance, data *ConnectStruct) map[string]interface{} {
	updates := map[string]interface{}{}
	if instance == nil || data == nil {
		return updates
	}

	if len(data.Subscribe) > 0 {
		var subscribedEvents []string
		if data.Subscribe[0] == "ALL" {
			subscribedEvents = append(subscribedEvents, event_types.AllEventTypes...)
		} else {
			for _, arg := range data.Subscribe {
				if !event_types.IsEventType(arg) {
					continue
				}
				subscribedEvents = append(subscribedEvents, arg)
			}
		}
		eventString := strings.Join(subscribedEvents, ",")
		instance.Events = eventString
		updates["events"] = eventString
	} else if instance.Events == "" {
		instance.Events = event_types.MESSAGE
		updates["events"] = event_types.MESSAGE
	}

	if data.WebhookUrl != "" {
		instance.Webhook = data.WebhookUrl
		updates["webhook"] = data.WebhookUrl
	}
	if data.RabbitmqEnable != "" {
		instance.RabbitmqEnable = data.RabbitmqEnable
		updates["rabbitmq_enable"] = data.RabbitmqEnable
	}
	if data.NatsEnable != "" {
		instance.NatsEnable = data.NatsEnable
		updates["nats_enable"] = data.NatsEnable
	}
	if data.WebSocketEnable != "" {
		instance.WebSocketEnable = data.WebSocketEnable
		updates["web_socket_enable"] = data.WebSocketEnable
	}

	return updates
}

// splitSubscribedEvents deriva a lista de eventos assinados a partir do campo
// Events já persistido na instância (em vez do body da requisição).
func splitSubscribedEvents(events string) []string {
	if events == "" {
		return []string{event_types.MESSAGE}
	}
	parts := strings.Split(events, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{event_types.MESSAGE}
	}
	return out
}

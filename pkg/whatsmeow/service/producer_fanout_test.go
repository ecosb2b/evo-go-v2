package whatsmeow_service

import (
	"errors"
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
)

type fakeProducer struct {
	err   error
	calls int
}

func (f *fakeProducer) Produce(string, []byte, string, string) error {
	f.calls++
	return f.err
}

func (f *fakeProducer) CreateGlobalQueues() error { return nil }

// Uma falha num produtor não pode impedir a entrega pelos outros: antes, um
// RabbitMQ fora do ar fazia o webhook da mesma mensagem nunca ser chamado.
func TestSendToQueueOrWebhookContinuesAfterProducerFailure(t *testing.T) {
	failing := errors.New("broker down")

	tests := []struct {
		name                      string
		rabbit, nats, ws, webhook *fakeProducer
	}{
		{"rabbitmq fails", &fakeProducer{err: failing}, &fakeProducer{}, &fakeProducer{}, &fakeProducer{}},
		{"nats fails", &fakeProducer{}, &fakeProducer{err: failing}, &fakeProducer{}, &fakeProducer{}},
		{"websocket fails", &fakeProducer{}, &fakeProducer{}, &fakeProducer{err: failing}, &fakeProducer{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &whatsmeowService{
				rabbitmqProducer:  tt.rabbit,
				natsProducer:      tt.nats,
				websocketProducer: tt.ws,
				webhookProducer:   tt.webhook,
				loggerWrapper:     logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()}),
			}
			instance := &instance_model.Instance{
				Id:              "inst-a",
				RabbitmqEnable:  "enabled",
				NatsEnable:      "enabled",
				WebSocketEnable: "enabled",
				Webhook:         "http://example.invalid/hook",
			}

			w.sendToQueueOrWebhook(instance, "inst-a.message", []byte(`{}`))

			for name, p := range map[string]*fakeProducer{
				"rabbitmq": tt.rabbit, "nats": tt.nats, "websocket": tt.ws, "webhook": tt.webhook,
			} {
				if p.calls != 1 {
					t.Errorf("%s producer called %d times, want 1", name, p.calls)
				}
			}
		})
	}
}

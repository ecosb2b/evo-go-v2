package instance_service

import (
	"testing"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
)

// ALL precisa ser gravado literal: expandido na lista de grupos conhecidos,
// o CallWebhook nunca entrava no atalho de ALL e eventos fora dos grupos
// (passkey, offline sync) não chegavam. (#105)
func TestApplyConnectSettingsKeepsAllLiteral(t *testing.T) {
	tests := []struct {
		name      string
		subscribe []string
		want      string
	}{
		{"ALL alone", []string{"ALL"}, "ALL"},
		{"ALL among others", []string{"MESSAGE", "ALL"}, "ALL"},
		{"explicit list", []string{"MESSAGE", "CONNECTION"}, "MESSAGE,CONNECTION"},
		{"unknown entries dropped", []string{"MESSAGE", "NOPE"}, "MESSAGE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &instance_model.Instance{}
			updates := applyConnectSettings(instance, &ConnectStruct{Subscribe: tt.subscribe})
			if instance.Events != tt.want || updates["events"] != tt.want {
				t.Fatalf("events = %q (update %v), want %q", instance.Events, updates["events"], tt.want)
			}
		})
	}
}

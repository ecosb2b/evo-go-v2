package whatsmeow_service

import (
	"errors"
	"testing"

	"gorm.io/gorm"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
)

func TestDecideReconnect(t *testing.T) {
	paired := &instance_model.Instance{Jid: "5531987654321@s.whatsapp.net"}
	dbDown := errors.New("connection refused")

	tests := []struct {
		name      string
		instance  *instance_model.Instance
		lookupErr error
		hasDevice bool
		deviceErr error
		want      reconnectDecision
	}{
		{name: "paired with session", instance: paired, hasDevice: true, want: reconnectProceed},
		{name: "never paired", instance: &instance_model.Instance{}, want: reconnectStop},
		{
			name:     "disconnected through the API",
			instance: &instance_model.Instance{Jid: paired.Jid, DisconnectReason: instance_model.DisconnectReasonByAPI},
			want:     reconnectStop,
		},
		{
			name:     "logged out through the API",
			instance: &instance_model.Instance{Jid: paired.Jid, DisconnectReason: instance_model.DisconnectReasonLogoutByAPI},
			want:     reconnectStop,
		},
		{
			name:      "session replaced by another process",
			instance:  &instance_model.Instance{Jid: paired.Jid, DisconnectReason: instance_model.DisconnectReasonStreamReplaced},
			hasDevice: true,
			want:      reconnectStop,
		},
		{name: "session deleted by a logout from the phone", instance: paired, hasDevice: false, want: reconnectStop},
		{name: "instance deleted", lookupErr: gorm.ErrRecordNotFound, want: reconnectStop},
		{name: "database unavailable on lookup", lookupErr: dbDown, want: reconnectRetryLater},
		{name: "database unavailable on device check", instance: paired, deviceErr: dbDown, want: reconnectRetryLater},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decideReconnect(tt.instance, tt.lookupErr, func() (bool, error) {
				return tt.hasDevice, tt.deviceErr
			})
			if got != tt.want {
				t.Fatalf("decideReconnect = %v, want %v", got, tt.want)
			}
		})
	}
}

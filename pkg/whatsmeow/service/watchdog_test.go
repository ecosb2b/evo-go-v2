package whatsmeow_service

import (
	"reflect"
	"testing"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
)

func ageOf(ages map[string]time.Duration) func(string) (time.Duration, bool) {
	return func(id string) (time.Duration, bool) {
		age, ok := ages[id]
		return age, ok
	}
}

func TestWatchdogTargets(t *testing.T) {
	instances := []*instance_model.Instance{
		{Id: "down", Jid: "1@s.whatsapp.net", DisconnectReason: "Reconnecting"},
		{Id: "online", Jid: "2@s.whatsapp.net"},
		{Id: "never-paired"},
		{Id: "by-api", Jid: "3@s.whatsapp.net", DisconnectReason: instance_model.DisconnectReasonByAPI},
		{Id: "replaced", Jid: "4@s.whatsapp.net", DisconnectReason: instance_model.DisconnectReasonStreamReplaced},
		nil,
	}
	connected := func(id string) bool { return id == "online" }

	got := watchdogTargets(instances, connected, ageOf(nil))
	if want := []string{"down"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("watchdogTargets = %v, want %v", got, want)
	}
}

// Um runtime recém-criado (conectando ou mostrando QR) não pode ser derrubado.
func TestWatchdogTargetsSkipsYoungRuntime(t *testing.T) {
	instances := []*instance_model.Instance{
		{Id: "young", Jid: "1@s.whatsapp.net"},
		{Id: "old", Jid: "2@s.whatsapp.net"},
	}
	ages := map[string]time.Duration{"young": 10 * time.Second, "old": 10 * time.Minute}

	got := watchdogTargets(instances, func(string) bool { return false }, ageOf(ages))
	if want := []string{"old"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("watchdogTargets = %v, want %v", got, want)
	}
}

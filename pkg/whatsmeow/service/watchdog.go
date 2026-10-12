package whatsmeow_service

import (
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
)

const (
	// watchdogInterval: a cada quanto tempo a verificação procura instâncias
	// que deveriam estar no ar e não estão.
	watchdogInterval = 90 * time.Second
	// watchdogRuntimeGrace: runtime mais novo que isso ainda está conectando ou
	// mostrando QR, e não pode ser derrubado pela verificação.
	watchdogRuntimeGrace = 2 * time.Minute
)

// watchdogTargets escolhe as instâncias pareadas que não estão conectadas. O
// ScheduleReconnect ainda confere o aparelho, o ban e a deduplicação.
func watchdogTargets(
	instances []*instance_model.Instance,
	connected func(id string) bool,
	runtimeAge func(id string) (time.Duration, bool),
) []string {
	var ids []string
	for _, instance := range instances {
		if instance == nil || instance.Jid == "" {
			continue
		}
		switch instance.DisconnectReason {
		case instance_model.DisconnectReasonByAPI, instance_model.DisconnectReasonLogoutByAPI,
			instance_model.DisconnectReasonStreamReplaced:
			continue
		}
		if connected(instance.Id) {
			continue
		}
		if age, ok := runtimeAge(instance.Id); ok && age < watchdogRuntimeGrace {
			continue
		}
		ids = append(ids, instance.Id)
	}
	return ids
}

// startReconnectWatchdog religa, periodicamente, as instâncias que deveriam
// estar no ar e ficaram paradas fora do supervisor.
func (w *whatsmeowService) startReconnectWatchdog(clientName string) {
	go func() {
		defer utils.Recover("reconnectWatchdog")
		ticker := time.NewTicker(watchdogInterval)
		defer ticker.Stop()
		for range ticker.C {
			instances, err := w.instanceRepository.GetInstancesToRestore(clientName)
			if err != nil {
				w.loggerWrapper.GetLogger(clientName).LogWarn("[%s] Watchdog could not list instances: %v", clientName, err)
				continue
			}
			for _, id := range watchdogTargets(instances, w.runtimeConnected, w.runtimeAge) {
				w.loggerWrapper.GetLogger(id).LogInfo("[%s] Watchdog: instance should be online but is not — scheduling reconnection", id)
				w.ScheduleReconnect(id)
			}
		}
	}()
}

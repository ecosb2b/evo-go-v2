package whatsmeow_service

import instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"

// currentInstance devolve a instância vigente do runtime. O ponteiro é
// trocado por UpdateInstanceSettings/UpdateInstanceAdvancedSettings enquanto o
// handler de eventos o lê em outra goroutine.
func (mycli *MyClient) currentInstance() *instance_model.Instance {
	mycli.instanceMu.RLock()
	defer mycli.instanceMu.RUnlock()
	return mycli.instance
}

func (mycli *MyClient) setInstance(instance *instance_model.Instance) {
	mycli.instanceMu.Lock()
	mycli.instance = instance
	mycli.instanceMu.Unlock()
}

package whatsmeow_service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"

	"github.com/evolution-foundation/evolution-go/pkg/utils"
)

// qrEnv liga o emissor de QR (qr_emitter.go) a um runtime real.
type qrEnv struct {
	mycli *MyClient
}

func (e qrEnv) Stopped() <-chan struct{} { return e.mycli.runtimeDone }

func (e qrEnv) Connected() bool {
	client := e.mycli.WAClient
	return client != nil && client.IsConnected()
}

// Paired também é true sem cliente: não há mais o que emitir.
func (e qrEnv) Paired() bool {
	client := e.mycli.WAClient
	return client == nil || client.Store.ID != nil
}

func (e qrEnv) PasskeyActive() bool {
	ceremony := e.mycli.passkeyCeremony
	return ceremony != nil && ceremony.HasActiveByInstance(e.mycli.userID)
}

func (e qrEnv) MaxCount() int  { return e.mycli.config.QrcodeMaxCount }
func (e qrEnv) Now() time.Time { return time.Now() }

func (e qrEnv) Teardown(reason string, forceLogout bool) {
	mycli := e.mycli
	if forceLogout && mycli.WAClient != nil && mycli.WAClient.IsConnected() {
		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] %s, forcing logout and QRTimeout", mycli.userID, reason)
		if err := mycli.WAClient.Logout(context.Background()); err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Error during forced logout: %v", mycli.userID, err)
		}
	}
	mycli.teardownQR(reason, forceLogout)
}

func (e qrEnv) Publish(code string, count int, expiresAt time.Time) {
	mycli := e.mycli
	instanceID := mycli.userID

	if mycli.config.QrcodeMaxCount > 0 {
		mycli.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] QR code generated #%d (max: %d)", instanceID, count, mycli.config.QrcodeMaxCount)
	} else {
		mycli.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] QR code generated #%d (limit disabled)", instanceID, count)
	}
	if mycli.config.LogType != "json" {
		fmt.Println("QR code:\n", code)
	}

	image, _ := qrcode.Encode(code, qrcode.Medium, 256)
	base64qrcode := "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)
	if err := mycli.instanceRepository.UpdateQrcode(instanceID, utils.EncodeStoredQR(base64qrcode, code, expiresAt)); err != nil {
		mycli.loggerWrapper.GetLogger(instanceID).LogError("[%s] Error updating instance: %s", instanceID, err)
	}

	postMap := map[string]interface{}{
		"event": "QRCode",
		"data": map[string]interface{}{
			"qrcode":    base64qrcode,
			"code":      code,
			"count":     count,
			"maxCount":  mycli.config.QrcodeMaxCount,
			"expiresAt": expiresAt.Unix(),
		},
		"instanceToken": mycli.token,
		"instanceId":    instanceID,
		"instanceName":  mycli.currentInstance().Name,
	}
	queueName := strings.ToLower(fmt.Sprintf("%s.%s", instanceID, "QRCode"))
	values, err := json.Marshal(postMap)
	if err != nil {
		mycli.loggerWrapper.GetLogger(instanceID).LogError("[%s] Failed to marshal JSON for queue", instanceID)
		return
	}
	go mycli.service.CallWebhook(mycli.currentInstance(), queueName, values)
	if mycli.config.AmqpGlobalEnabled || mycli.config.NatsGlobalEnabled {
		go mycli.service.SendToGlobalQueues("QRCode", values, instanceID)
	}
}

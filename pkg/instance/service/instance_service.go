package instance_service

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_repository "github.com/evolution-foundation/evolution-go/pkg/instance/repository"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type InstanceService interface {
	Create(data *CreateStruct) (*instance_model.Instance, error)
	Connect(data *ConnectStruct, instance *instance_model.Instance) (*instance_model.Instance, string, string, error)
	Reconnect(instance *instance_model.Instance) error
	Disconnect(instance *instance_model.Instance) (*instance_model.Instance, error)
	Logout(instance *instance_model.Instance) (*instance_model.Instance, error)
	Status(instance *instance_model.Instance) (*StatusStruct, error)
	GetQr(instance *instance_model.Instance) (*QrcodeStruct, error)
	Pair(data *PairStruct, instance *instance_model.Instance) (*PairReturnStruct, error)
	GetAll() ([]*instance_model.Instance, error)
	Info(instanceId string) (*instance_model.Instance, error)
	Rename(instanceId string, name string) (*instance_model.Instance, error)
	Delete(id string) error
	SetProxy(id string, proxyConfig *ProxyConfig) error
	SetProxyFromStruct(id string, data *SetProxyStruct) error
	RemoveProxy(id string) error
	ForceReconnect(instanceId string, number string) error
	GetInstanceByToken(token string) (*instance_model.Instance, error)
	GetLogs(instanceId string, startDate, endDate time.Time, level string, limit int) ([]logger_wrapper.LogEntry, error)
	GetAdvancedSettings(instanceId string) (*instance_model.AdvancedSettings, error)
	UpdateAdvancedSettings(instanceId string, settings *instance_model.AdvancedSettings) error
}

type instances struct {
	instanceRepository instance_repository.InstanceRepository
	config             *config.Config
	killChannel        map[string](chan bool)
	clientPointer      map[string]*whatsmeow.Client
	whatsmeowService   whatsmeow_service.WhatsmeowService
	loggerWrapper      *logger_wrapper.LoggerManager
}

type ProxyConfig struct {
	Protocol string `json:"protocol,omitempty"`
	Port     string `json:"port"`
	Password string `json:"password"`
	Username string `json:"username"`
	Host     string `json:"host"`
}

type CreateStruct struct {
	InstanceId       string                           `json:"instanceId"`
	Name             string                           `json:"name"`
	Token            string                           `json:"token"`
	Proxy            *ProxyConfig                     `json:"proxy"`
	AdvancedSettings *instance_model.AdvancedSettings `json:"advancedSettings"`
}

type ConnectStruct struct {
	WebhookUrl      string   `json:"webhookUrl"`
	Subscribe       []string `json:"subscribe"`
	Immediate       bool     `json:"immediate"`
	Phone           string   `json:"phone"`
	RabbitmqEnable  string   `json:"rabbitmqEnable"`
	WebSocketEnable string   `json:"websocketEnable"`
	NatsEnable      string   `json:"natsEnable"`
}

type StatusStruct struct {
	Connected bool
	LoggedIn  bool
	myJid     *types.JID
	Name      string
}

type QrcodeStruct struct {
	Qrcode string `json:"qrcode"`
	Code   string `json:"code"`
	// Validade do código (unix, segundos) e quantos segundos faltam, quando a
	// EvGo sabe. O primeiro código vale ~60 s e os seguintes ~20 s.
	ExpiresAt int64 `json:"expiresAt,omitempty"`
	ExpiresIn int   `json:"expiresIn,omitempty"`
	// Passkey ceremony fields. Populated when the account requires a WebAuthn
	// passkey to finish linking (no QR to scan at that point). The manager uses
	// PasskeyStage to switch its UI and PasskeyOpenUrl for the
	// "Abrir WhatsApp Web" button that launches the passkey ceremony.
	PasskeyStage   string `json:"passkeyStage,omitempty"`
	PasskeyOpenURL string `json:"passkeyOpenUrl,omitempty"`
	PasskeyCode    string `json:"passkeyCode,omitempty"`
}

type PairStruct struct {
	Subscribe []string `json:"subscribe"`
	Phone     string   `json:"phone"`
}

type PairReturnStruct struct {
	PairingCode string
}

type SetProxyStruct struct {
	Protocol string `json:"protocol,omitempty"`
	Host     string `json:"host" validate:"required"`
	Port     string `json:"port" validate:"required"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type ForceReconnectStruct struct {
	Number string `json:"number"`
}

type RenameStruct struct {
	Name string `json:"name"`
}

func (i *instances) ensureClientConnected(instanceId string) (*whatsmeow.Client, error) {
	logger := i.loggerWrapper.GetLogger(instanceId)
	whatsmeow_service.ClientMapsMu.RLock()
	client := i.clientPointer[instanceId]
	whatsmeow_service.ClientMapsMu.RUnlock()
	logger.LogInfo("[%s] Checking client connection status - Client exists: %v", instanceId, client != nil)

	if client == nil {
		logger.LogInfo("[%s] No client found, attempting to start new instance", instanceId)
		err := i.whatsmeowService.StartInstance(instanceId)
		if err != nil {
			logger.LogError("[%s] Failed to start instance: %v", instanceId, err)
			return nil, errors.New("no active session found")
		}

		logger.LogInfo("[%s] Instance started, waiting 2 seconds...", instanceId)
		time.Sleep(2 * time.Second)

		whatsmeow_service.ClientMapsMu.RLock()
		client = i.clientPointer[instanceId]
		whatsmeow_service.ClientMapsMu.RUnlock()
		logger.LogInfo("[%s] Checking new client - Exists: %v, Connected: %v",
			instanceId,
			client != nil,
			client != nil && client.IsConnected())

		if client == nil || !client.IsConnected() {
			logger.LogError("[%s] New client validation failed - Exists: %v, Connected: %v",
				instanceId,
				client != nil,
				client != nil && client.IsConnected())
			return nil, errors.New("no active session found")
		}
	} else if !client.IsConnected() {
		logger.LogError("[%s] Existing client is disconnected - Connected status: %v",
			instanceId,
			client.IsConnected())
		return nil, errors.New("client disconnected")
	}

	logger.LogInfo("[%s] Client successfully validated - Connected: %v", instanceId, client.IsConnected())
	return client, nil
}

func (i instances) Create(data *CreateStruct) (*instance_model.Instance, error) {
	if data.Proxy != nil {
		data.Proxy.Protocol = utils.NormalizeProxyProtocol(data.Proxy.Protocol, data.Proxy.Port)
	}

	proxyJson, err := json.Marshal(data.Proxy)
	if err != nil {
		return nil, err
	}

	findInstance, _ := i.instanceRepository.GetInstanceByName(data.Name)

	if findInstance != nil {
		return nil, fmt.Errorf("instance already exists")
	}

	instance := instance_model.Instance{
		Id:         data.InstanceId,
		Name:       data.Name,
		Token:      data.Token,
		OsName:     i.config.OsName,
		Proxy:      string(proxyJson),
		Connected:  false,
		ClientName: i.config.ClientName,
	}

	// Set advanced settings if provided (ponteiros nil ficam com o default). [Athene/PR#136]
	if data.AdvancedSettings != nil {
		if data.AdvancedSettings.AlwaysOnline != nil {
			instance.AlwaysOnline = *data.AdvancedSettings.AlwaysOnline
		}
		if data.AdvancedSettings.RejectCall != nil {
			instance.RejectCall = *data.AdvancedSettings.RejectCall
		}
		instance.MsgRejectCall = data.AdvancedSettings.MsgRejectCall
		if data.AdvancedSettings.ReadMessages != nil {
			instance.ReadMessages = *data.AdvancedSettings.ReadMessages
		}
		if data.AdvancedSettings.IgnoreGroups != nil {
			instance.IgnoreGroups = *data.AdvancedSettings.IgnoreGroups
		}
		if data.AdvancedSettings.IgnoreStatus != nil {
			instance.IgnoreStatus = *data.AdvancedSettings.IgnoreStatus
		}
	}

	createdInstance, err := i.instanceRepository.Create(instance)
	if err != nil {
		return nil, err
	}

	return createdInstance, nil
}

func (i instances) Connect(data *ConnectStruct, instance *instance_model.Instance) (*instance_model.Instance, string, string, error) {
	i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Processing subscribe events: %v", instance.Id, data.Subscribe)

	// [Athene/PR#136] Update PARCIAL: campos vazios mantêm o valor persistido.
	// MESSAGE só vira padrão quando não há Events salvos. Para desligar webhook/
	// integrações, mande "disabled"/"false" (string vazia preserva o atual).
	oldEvents := instance.Events
	oldRabbitmq := instance.RabbitmqEnable

	updates := applyConnectSettings(instance, data)

	if instance.Events != oldEvents {
		i.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] events changed: %q -> %q", instance.Id, oldEvents, instance.Events)
	}
	if instance.RabbitmqEnable != oldRabbitmq {
		i.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] rabbitmqEnable changed: %q -> %q", instance.Id, oldRabbitmq, instance.RabbitmqEnable)
	}

	if len(updates) > 0 {
		if err := i.instanceRepository.UpdateConnectSettings(instance.Id, updates); err != nil {
			i.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error updating instance: %s", instance.Id, err)
			return nil, "", "", err
		}
	}

	subscribedEvents := splitSubscribedEvents(instance.Events)
	eventString := instance.Events

	// Verifica se a instância já está rodando
	whatsmeow_service.ClientMapsMu.RLock()
	isInstanceRunning := i.clientPointer[instance.Id] != nil
	whatsmeow_service.ClientMapsMu.RUnlock()

	// Sincroniza as configurações na instância em execução (se já estiver conectada)
	err := i.whatsmeowService.UpdateInstanceSettings(instance.Id)
	if err != nil {
		i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Instance not in runtime yet, will be updated when connected", instance.Id)
		isInstanceRunning = false
	} else {
		i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Instance settings updated successfully in runtime", instance.Id)
		isInstanceRunning = true
	}

	// Se a instância não estiver rodando, inicia uma nova
	if !isInstanceRunning {
		i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Starting new client instance", instance.Id)

		clientData := &whatsmeow_service.ClientData{
			Instance:      instance,
			Subscriptions: subscribedEvents,
			Phone:         data.Phone,
			IsProxy:       false,
		}

		if instance.Proxy != "" || i.config.ProxyHost != "" {
			var proxyConfig ProxyConfig
			err := json.Unmarshal([]byte(instance.Proxy), &proxyConfig)
			if err != nil {
				i.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error unmarshalling proxy config: %v", instance.Id, err)
				return nil, "", "", err
			}

			if proxyConfig.Host != "" || i.config.ProxyHost != "" {
				clientData.IsProxy = true
			}
		}

		go i.whatsmeowService.StartClient(clientData)
	} else {
		whatsmeow_service.ClientMapsMu.RLock()
		client := i.clientPointer[instance.Id]
		whatsmeow_service.ClientMapsMu.RUnlock()

		// Runtime registrado com o socket morto (ex.: após stream:error):
		// antes o /connect só atualizava settings e virava no-op. Uma sessão
		// pareada é reiniciada; o QR em andamento de uma não pareada fica.
		if client != nil && !client.IsConnected() && client.Store != nil && client.Store.ID != nil {
			i.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Instance registered but disconnected, restarting runtime", instance.Id)
			go func() {
				defer utils.Recover("connectRestart")
				if err := i.whatsmeowService.ReconnectClient(instance.Id); err != nil {
					i.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Restart from /connect failed: %v", instance.Id, err)
				}
			}()
		} else {
			i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Instance already running, settings updated without restarting client", instance.Id)
		}
	}

	// logger.LogInfo("Waiting 1 seconds")
	// time.Sleep(1000 * time.Millisecond)

	// if i.clientPointer[instance.Id] != nil {
	// 	if !i.clientPointer[instance.Id].IsConnected() {
	// 		return instance, "", "", fmt.Errorf("failed to connect")
	// 	}
	// } else {
	// 	return instance, "", "", fmt.Errorf("failed to connect")
	// }

	return instance, instance.Jid, eventString, nil
}

func (i instances) Reconnect(instance *instance_model.Instance) error {
	_, err := i.ensureClientConnected(instance.Id)
	if err != nil {
		return err
	}

	return i.whatsmeowService.ReconnectClient(instance.Id)
}

func (i instances) Disconnect(instance *instance_model.Instance) (*instance_model.Instance, error) {
	client, err := i.ensureClientConnected(instance.Id)
	if err != nil {
		return instance, err
	}

	if client.IsConnected() {
		if client.IsLoggedIn() {
			// Não apagar instance.Events: com a string vazia, depois do
			// /reconnect o filtro de eventos descarta todo webhook
			// (strings.Split("", ",") devolve [""]). Só o estado de conexão
			// muda, sem regravar a linha inteira. [upstream#187]
			//
			// O motivo vai antes de parar o runtime: uma reconexão do
			// supervisor que esteja esperando esse mesmo stop relê o motivo
			// e desiste, em vez de religar a instância.
			instance.Connected = false
			instance.DisconnectReason = instance_model.DisconnectReasonByAPI
			if err := i.instanceRepository.UpdateConnected(instance.Id, false, instance.DisconnectReason); err != nil {
				return instance, err
			}

			if err := i.whatsmeowService.ClearInstanceCache(instance.Id, instance.Token); err != nil {
				return instance, err
			}

			i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Disconnection successful", instance.Id)
			return instance, nil
		}
	}

	i.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Ignoring disconnect as it was not connected", instance.Id)
	return instance, nil
}

func (i instances) Logout(instance *instance_model.Instance) (*instance_model.Instance, error) {
	client, err := i.ensureClientConnected(instance.Id)
	if err != nil {
		return instance, err
	}

	if client.IsLoggedIn() && client.IsConnected() {
		err := client.Logout(context.Background())
		if err != nil {
			return instance, err
		}

		// Motivo antes do stop, como no Disconnect: o ramo de kill só grava a
		// coluna connected, e o supervisor relê o motivo após o stop.
		instance.Connected = false
		instance.DisconnectReason = instance_model.DisconnectReasonLogoutByAPI
		err = i.instanceRepository.UpdateConnected(instance.Id, false, instance.DisconnectReason)
		if err != nil {
			return instance, err
		}

		if err := i.whatsmeowService.ClearInstanceCache(instance.Id, instance.Token); err != nil {
			return instance, err
		}

		i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Logout successful", instance.Id)
		return instance, nil
	}

	if client.IsConnected() {
		client.Disconnect()

		if err := i.whatsmeowService.ClearInstanceCache(instance.Id, instance.Token); err != nil {
			return instance, err
		}

		i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Disconnection successful", instance.Id)
		return instance, nil
	}

	i.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Ignoring logout as it was not connected", instance.Id)
	return instance, fmt.Errorf("ignoring logout as it was not connected")
}

func (i instances) Status(instance *instance_model.Instance) (*StatusStruct, error) {
	whatsmeow_service.ClientMapsMu.RLock()
	client := i.clientPointer[instance.Id]
	whatsmeow_service.ClientMapsMu.RUnlock()

	if client == nil {
		return &StatusStruct{
			Connected: false,
			LoggedIn:  false,
		}, nil
	}

	isConnected := client.IsConnected()
	isLoggedIn := client.IsLoggedIn()

	var myJid *types.JID
	var name string
	if isLoggedIn {
		myJid = client.Store.ID
		name = client.Store.PushName
	}

	return &StatusStruct{
		Connected: isConnected,
		LoggedIn:  isLoggedIn,
		myJid:     myJid,
		Name:      name,
	}, nil
}

func (i instances) GetQr(instance *instance_model.Instance) (*QrcodeStruct, error) {
	logger := i.loggerWrapper.GetLogger(instance.Id)
	whatsmeow_service.ClientMapsMu.RLock()
	client := i.clientPointer[instance.Id]
	whatsmeow_service.ClientMapsMu.RUnlock()

	// Se o cliente já está logado, NÃO reiniciar — apenas informar. Chamar StartInstance
	// numa sessão logada derruba o whatsmeow (Disconnected → novo QR), o que pode ser
	// disparado por polling do frontend logo após PairSuccess, quebrando a conexão.
	if client != nil && client.IsLoggedIn() {
		logger.LogInfo("[%s] Client is already logged in — returning 'session already logged in' (StartInstance skipped to avoid disconnect)", instance.Id)
		return nil, fmt.Errorf("session already logged in")
	}

	// started: esta chamada subiu a sessão e já esperou por ela (ver abaixo).
	started := false

	// Se não há cliente OU cliente sem sessão logada, iniciar/reiniciar para gerar QR
	if client == nil || (!client.IsConnected() && !client.IsLoggedIn()) {
		start := true
		if client == nil {
			logger.LogInfo("[%s] No client found, starting new instance for QR code", instance.Id)
		} else {
			// Pareada mas fora do ar (conectando ou após stream:error): QR não
			// se aplica e derrubar o runtime atrapalharia a reconexão.
			if client.Store != nil && client.Store.ID != nil {
				logger.LogInfo("[%s] Paired client is reconnecting — no QR to return", instance.Id)
				return nil, fmt.Errorf("session already logged in")
			}
			if age, known := i.whatsmeowService.RuntimeAge(instance.Id); !qrRestartAllowed(age, known) {
				logger.LogInfo("[%s] Runtime started %s ago and is still connecting — waiting for its QR", instance.Id, age.Round(time.Second))
				start = false
			} else {
				logger.LogInfo("[%s] Client exists but not connected/logged, restarting for QR code", instance.Id)
				// O runtime antigo continua reservado mesmo com o socket morto, e o
				// StartInstance abaixo viraria no-op ("already running"). Para ele primeiro.
				if err := i.whatsmeowService.ClearInstanceCache(instance.Id, instance.Token); err != nil {
					logger.LogError("[%s] Failed to stop stale runtime: %v", instance.Id, err)
					return nil, fmt.Errorf("failed to stop stale runtime: %w", err)
				}
			}
		}

		if start {
			started = true
			err := i.whatsmeowService.StartInstance(instance.Id)
			if err != nil {
				logger.LogError("[%s] Failed to start instance: %v", instance.Id, err)
				return nil, fmt.Errorf("failed to start instance: %w", err)
			}

			// Aguardar um pouco para o cliente iniciar e gerar QR code
			logger.LogInfo("[%s] Waiting for QR code generation...", instance.Id)
			time.Sleep(3 * time.Second)

			whatsmeow_service.ClientMapsMu.RLock()
			client = i.clientPointer[instance.Id]
			whatsmeow_service.ClientMapsMu.RUnlock()
			if client != nil && client.IsLoggedIn() {
				return nil, fmt.Errorf("session already logged in")
			}
		}
	} else if !client.IsConnected() {
		// Se o cliente existe mas não está conectado, pode estar aguardando QR code
		logger.LogInfo("[%s] Client exists but not connected, checking for existing QR code", instance.Id)
	}

	// Buscar instância atualizada do banco para pegar o QR code mais recente
	instance, err := i.instanceRepository.GetInstanceByID(instance.Id)
	if err != nil {
		return nil, err
	}

	// If a passkey ceremony is in progress, there is no QR to scan — return the
	// passkey stage + the #wapk openUrl so the manager can render the
	// "Abrir WhatsApp Web" button. Checked before the empty-QR branch because
	// during a passkey ceremony instance.Qrcode is empty.
	if store := i.whatsmeowService.PasskeyCeremonyStore(); store != nil {
		if token, state, ok := store.StateByInstance(instance.Id); ok {
			logger.LogInfo("[%s] Passkey ceremony active (stage=%s) — returning passkey info instead of QR", instance.Id, state.Stage)
			return &QrcodeStruct{
				PasskeyStage:   state.Stage,
				PasskeyCode:    state.Code,
				PasskeyOpenURL: buildPasskeyOpenURL(token),
			}, nil
		}
	}

	// Sessão recém-aberta por outra chamada (ex.: o /connect de um instante
	// atrás): o primeiro código ainda pode estar a caminho, então espera mais.
	// Se foi esta chamada que subiu a sessão, os 3 s acima já contam.
	qrWait := qrRetryWait
	if age, known := i.whatsmeowService.RuntimeAge(instance.Id); !started && known && age < qrConnectGrace {
		qrWait = qrFirstCodeWait
	}

	stored, ok := readFreshQR(instance.Qrcode)
	if !ok {
		// Sem QR válido ainda (sessão subindo ou código recém-vencido): relê o
		// banco até o código aparecer ou o prazo acabar.
		logger.LogInfo("[%s] No valid QR code yet, waiting up to %s...", instance.Id, qrWait)
		instanceID := instance.Id
		stored, ok, err = pollFreshQR(func() (string, error) {
			current, err := i.instanceRepository.GetInstanceByID(instanceID)
			if err != nil {
				return "", err
			}
			return current.Qrcode, nil
		}, qrWait, qrPollInterval)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("no QR code available. Please wait a moment and try again")
		}
	}

	qr := &QrcodeStruct{
		Qrcode: stored.Image,
		Code:   stored.Code,
	}
	if !stored.ExpiresAt.IsZero() {
		qr.ExpiresAt = stored.ExpiresAt.Unix()
		qr.ExpiresIn = int(time.Until(stored.ExpiresAt).Seconds())
	}

	return qr, nil
}

// qrConnectGrace: tempo que um runtime recém-criado tem para abrir o socket
// antes de o GET /qr considerá-lo morto. Sem isso, um GET logo depois do
// /connect derrubava a sessão que ainda conectava e gerava um aparelho novo.
const qrConnectGrace = 15 * time.Second

func qrRestartAllowed(age time.Duration, known bool) bool {
	return !known || age >= qrConnectGrace
}

const (
	// qrPollInterval: intervalo entre as releituras do QR guardado.
	qrPollInterval = 500 * time.Millisecond
	// qrRetryWait: espera por um código novo quando o atual acabou de vencer.
	qrRetryWait = 2 * time.Second
	// qrFirstCodeWait: espera pelo PRIMEIRO código de uma sessão que acabou de
	// abrir (o WhatsApp o entrega 1-3 s depois do /connect). Frontends que
	// pedem o QR junto com o /connect recebiam erro antes do código chegar.
	// Fica abaixo dos 8 s que os frontends esperam por esta rota.
	qrFirstCodeWait = 6 * time.Second
)

// pollFreshQR relê o QR guardado a cada interval até achar um código válido
// ou o prazo (budget) acabar. Erro de leitura encerra na hora.
func pollFreshQR(read func() (string, error), budget, interval time.Duration) (utils.StoredQR, bool, error) {
	deadline := time.Now().Add(budget)
	for {
		time.Sleep(interval)
		raw, err := read()
		if err != nil {
			return utils.StoredQR{}, false, err
		}
		if q, ok := readFreshQR(raw); ok {
			return q, true, nil
		}
		if !time.Now().Before(deadline) {
			return utils.StoredQR{}, false, nil
		}
	}
}

// readFreshQR devolve o QR guardado se ele existir e ainda não tiver vencido.
func readFreshQR(raw string) (utils.StoredQR, bool) {
	q, ok := utils.ParseStoredQR(raw)
	if !ok || q.Expired(time.Now()) {
		return utils.StoredQR{}, false
	}
	return q, true
}

// buildPasskeyOpenURL builds the URL the manager opens to start the passkey
// ceremony: https://web.whatsapp.com/#wapk=<base64url({t:token,b:publicBase})>.
// publicBase must be the PUBLICLY reachable API base the browser can hit; set it
// via PASSKEY_PUBLIC_URL. Kept in sync with the event handler in whatsmeow.go.
func buildPasskeyOpenURL(token string) string {
	publicBase := os.Getenv("PASSKEY_PUBLIC_URL")
	if publicBase == "" {
		publicBase = "<SET_PASSKEY_PUBLIC_URL>"
	}
	payload := fmt.Sprintf(`{"t":%q,"b":%q}`, token, publicBase)
	wapk := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return "https://web.whatsapp.com/#wapk=" + wapk
}

func (i instances) Pair(data *PairStruct, instance *instance_model.Instance) (*PairReturnStruct, error) {
	logger := i.loggerWrapper.GetLogger(instance.Id)
	whatsmeow_service.ClientMapsMu.RLock()
	client := i.clientPointer[instance.Id]
	whatsmeow_service.ClientMapsMu.RUnlock()

	if client == nil || !client.IsConnected() {
		if client != nil && client.IsLoggedIn() {
			return nil, fmt.Errorf("instance is already authenticated")
		}
		logger.LogInfo("[%s] No active connection, starting instance for phone pairing", instance.Id)
		if err := i.whatsmeowService.StartInstance(instance.Id); err != nil {
			logger.LogError("[%s] Failed to start instance for pairing: %v", instance.Id, err)
			return nil, fmt.Errorf("failed to start instance: %w", err)
		}
		// Wait for the WA websocket connection and initial QR generation to establish.
		// PairPhone must be called after the QR event is received per whatsmeow docs.
		time.Sleep(3 * time.Second)
		whatsmeow_service.ClientMapsMu.RLock()
		client = i.clientPointer[instance.Id]
		whatsmeow_service.ClientMapsMu.RUnlock()
		if client == nil {
			return nil, fmt.Errorf("failed to initialize client for pairing")
		}
	}

	if client.IsLoggedIn() {
		return nil, fmt.Errorf("instance is already authenticated")
	}

	code, err := client.PairPhone(context.Background(), data.Phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		logger.LogError("[%s] PairPhone failed: %v", instance.Id, err)
		return nil, fmt.Errorf("pairing failed: %w", err)
	}

	return &PairReturnStruct{PairingCode: code}, nil
}

func (i instances) GetAll() ([]*instance_model.Instance, error) {
	instances, err := i.instanceRepository.GetAll(i.config.ClientName)
	if err != nil {
		return nil, err
	}

	for _, instance := range instances {
		whatsmeow_service.ClientMapsMu.RLock()
		client := i.clientPointer[instance.Id]
		whatsmeow_service.ClientMapsMu.RUnlock()
		if client != nil {
			instance.Connected = client.IsLoggedIn()
		} else {
			instance.Connected = false
		}
		instance.NeedsQR = !instance.Connected && instance_model.RequiresQR(instance.Jid, instance.DisconnectReason)

		instance.Proxy = ""
	}

	return instances, nil
}

func (i instances) Info(instanceId string) (*instance_model.Instance, error) {
	instance, err := i.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		return nil, err
	}

	// Atualiza o status connected com base no estado real do cliente
	whatsmeow_service.ClientMapsMu.RLock()
	client := i.clientPointer[instance.Id]
	whatsmeow_service.ClientMapsMu.RUnlock()
	if client != nil {
		instance.Connected = client.IsLoggedIn()
	} else {
		instance.Connected = false
	}
	instance.NeedsQR = !instance.Connected && instance_model.RequiresQR(instance.Jid, instance.DisconnectReason)

	instance.Proxy = ""

	return instance, nil
}

// Rename troca o rótulo da instância. Id e token não mudam, mas o nome sai como
// "instanceName" nos payloads de webhook — por isso o runtime é sincronizado,
// para os próximos eventos já carregarem o nome novo sem reconectar.
func (i instances) Rename(instanceId string, name string) (*instance_model.Instance, error) {
	instance, err := i.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		return nil, err
	}

	if existing, _ := i.instanceRepository.GetInstanceByName(name); existing != nil && existing.Id != instance.Id {
		return nil, fmt.Errorf("an instance with this name already exists")
	}

	if err := i.instanceRepository.UpdateName(instance.Id, name); err != nil {
		return nil, err
	}
	instance.Name = name

	if err := i.whatsmeowService.UpdateInstanceSettings(instance.Id); err != nil {
		// Instância desconectada não tem runtime para atualizar; não falha a operação.
		i.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Error syncing renamed instance to runtime: %v", instance.Id, err)
	}

	instance.Proxy = ""

	return instance, nil
}

func (i instances) Delete(id string) error {
	instance, err := i.instanceRepository.GetInstanceByID(id)
	if err != nil {
		return err
	}

	whatsmeow_service.ClientMapsMu.RLock()
	deleteClient := i.clientPointer[instance.Id]
	whatsmeow_service.ClientMapsMu.RUnlock()

	if deleteClient != nil && deleteClient.IsConnected() {
		if deleteClient.IsLoggedIn() {
			deleteClient.Logout(context.Background())
		}
		deleteClient.Disconnect()
	}

	// O serviço coordena o teardown e aguarda a goroutine proprietária terminar.
	err = i.whatsmeowService.ClearInstanceCache(instance.Id, instance.Token)
	if err != nil {
		i.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Failed to clear instance cache: %v", instance.Id, err)
	}

	err = i.instanceRepository.Delete(id)
	if err != nil {
		return err
	}

	return nil
}

func (i instances) SetProxy(id string, proxyConfig *ProxyConfig) error {
	instance, err := i.instanceRepository.GetInstanceByID(id)
	if err != nil {
		return err
	}

	// Validate proxy configuration
	if proxyConfig == nil {
		return fmt.Errorf("proxy configuration cannot be nil")
	}

	if proxyConfig.Host == "" {
		return fmt.Errorf("proxy host is required")
	}

	if proxyConfig.Port == "" {
		return fmt.Errorf("proxy port is required")
	}

	proxyConfig.Protocol = utils.NormalizeProxyProtocol(proxyConfig.Protocol, proxyConfig.Port)

	// Convert proxy config to JSON
	proxyJSON, err := json.Marshal(proxyConfig)
	if err != nil {
		i.loggerWrapper.GetLogger(id).LogError("[%s] Failed to marshal proxy config: %v", id, err)
		return fmt.Errorf("failed to marshal proxy configuration: %v", err)
	}

	instance.Proxy = string(proxyJSON)

	// Só a coluna proxy: regravar a linha inteira sobrescreveria settings
	// alterados em paralelo.
	err = i.instanceRepository.UpdateProxy(id, instance.Proxy)
	if err != nil {
		i.loggerWrapper.GetLogger(id).LogError("[%s] Failed to update instance with proxy: %v", id, err)
		return err
	}

	i.loggerWrapper.GetLogger(id).LogInfo("[%s] Proxy configuration updated: %s://%s:%s", id, proxyConfig.Protocol, proxyConfig.Host, proxyConfig.Port)

	// Reconnect to apply proxy changes
	utils.Go("proxyReconnect", func() { _ = i.Reconnect(instance) })

	return nil
}

func (i instances) SetProxyFromStruct(id string, data *SetProxyStruct) error {
	if data == nil {
		return fmt.Errorf("proxy data cannot be nil")
	}

	proxyConfig := &ProxyConfig{
		Protocol: data.Protocol,
		Host:     data.Host,
		Port:     data.Port,
		Username: data.Username,
		Password: data.Password,
	}

	return i.SetProxy(id, proxyConfig)
}

func (i instances) RemoveProxy(id string) error {
	instance, err := i.instanceRepository.GetInstanceByID(id)
	if err != nil {
		return err
	}

	instance.Proxy = ""

	err = i.instanceRepository.UpdateProxy(id, "")
	if err != nil {
		return err
	}

	i.loggerWrapper.GetLogger(id).LogInfo("[%s] Proxy configuration removed", id)

	utils.Go("proxyReconnect", func() { _ = i.Reconnect(instance) })

	return nil
}

func (i instances) ForceReconnect(instanceId string, number string) error {
	whatsmeow_service.ClientMapsMu.RLock()
	client := i.clientPointer[instanceId]
	whatsmeow_service.ClientMapsMu.RUnlock()
	if client != nil && client.IsConnected() && client.IsLoggedIn() {
		return fmt.Errorf("client already connected")
	}
	if err := i.whatsmeowService.ForceUpdateJid(instanceId, number); err != nil {
		return err
	}
	return i.whatsmeowService.ReconnectClient(instanceId)
}

func (i instances) GetInstanceByToken(token string) (*instance_model.Instance, error) {
	return i.instanceRepository.GetInstanceByToken(token)
}

func (i instances) GetLogs(instanceId string, startDate, endDate time.Time, level string, limit int) ([]logger_wrapper.LogEntry, error) {
	// Inicializa o slice vazio para garantir que nunca retorne null
	logs := make([]logger_wrapper.LogEntry, 0)

	// Define valores padrão
	if limit <= 0 {
		limit = 100 // Limite padrão de 100 registros
	}

	// Se não foi fornecida data inicial, usa 7 dias atrás
	if startDate.IsZero() {
		startDate = time.Now().AddDate(0, 0, -7)
	}

	// Se não foi fornecida data final, usa data atual
	if endDate.IsZero() {
		endDate = time.Now()
	}

	// Ajusta as datas para início e fim do dia
	startDate = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, time.UTC)
	endDate = time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 999999999, time.UTC)

	// Garante que a data inicial não seja posterior à data final
	if startDate.After(endDate) {
		return logs, fmt.Errorf("data inicial não pode ser posterior à data final")
	}

	// Níveis de log válidos
	validLevels := map[string]bool{
		"INFO":  true,
		"ERROR": true,
		"WARN":  true,
		"DEBUG": true,
	}

	var levelArray []string
	if level == "" {
		// Se nenhum nível foi especificado, usa todos
		levelArray = []string{"INFO", "ERROR", "WARN", "DEBUG"}
	} else {
		// Divide e normaliza os níveis fornecidos
		for _, l := range strings.Split(level, ",") {
			l = strings.TrimSpace(strings.ToUpper(l))
			if !validLevels[l] {
				return logs, fmt.Errorf("nível de log inválido: %s", l)
			}
			levelArray = append(levelArray, l)
		}
	}

	// Lê os logs do arquivo
	logPath := filepath.Join(i.config.LogDirectory, instanceId, "instance.log")
	file, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return logs, nil // Retorna array vazio se arquivo não existir
		}
		return logs, fmt.Errorf("erro ao abrir arquivo de log: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Aumenta o buffer do scanner para lidar com linhas grandes
	const maxCapacity = 1024 * 1024 // 1MB
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		var entry logger_wrapper.LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // Ignora linhas inválidas
		}

		// Ajusta o timestamp da entrada para UTC para comparação correta
		entry.Timestamp = entry.Timestamp.UTC()

		// Aplica os filtros
		if entry.Timestamp.Before(startDate) || entry.Timestamp.After(endDate) {
			continue
		}

		if !slices.Contains(levelArray, entry.Level) {
			continue
		}

		logs = append(logs, entry)

		// Verifica o limite
		if len(logs) >= limit {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return logs, fmt.Errorf("erro ao ler arquivo de log: %v", err)
	}

	// Ordena os logs por timestamp em ordem decrescente
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].Timestamp.After(logs[j].Timestamp)
	})

	return logs, nil
}

func (i instances) GetAdvancedSettings(instanceId string) (*instance_model.AdvancedSettings, error) {
	i.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Getting advanced settings", instanceId)

	settings, err := i.instanceRepository.GetAdvancedSettings(instanceId)
	if err != nil {
		i.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error getting advanced settings: %v", instanceId, err)
		return nil, err
	}

	return settings, nil
}

func (i instances) UpdateAdvancedSettings(instanceId string, settings *instance_model.AdvancedSettings) error {
	i.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Updating advanced settings", instanceId)

	err := i.instanceRepository.UpdateAdvancedSettings(instanceId, settings)
	if err != nil {
		i.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error updating advanced settings: %v", instanceId, err)
		return err
	}

	// Sincroniza as configurações na instância em execução
	err = i.whatsmeowService.UpdateInstanceAdvancedSettings(instanceId)
	if err != nil {
		i.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] Error syncing advanced settings to runtime: %v", instanceId, err)
		// Não falha a operação, apenas loga o warning
	}

	i.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Advanced settings updated successfully", instanceId)
	return nil
}

func NewInstanceService(
	instanceRepository instance_repository.InstanceRepository,
	killChannel map[string](chan bool),
	clientPointer map[string]*whatsmeow.Client,
	whatsmeowService whatsmeow_service.WhatsmeowService,
	config *config.Config,
	loggerWrapper *logger_wrapper.LoggerManager,
) InstanceService {
	return &instances{
		instanceRepository: instanceRepository,
		killChannel:        killChannel,
		clientPointer:      clientPointer,
		whatsmeowService:   whatsmeowService,
		config:             config,
		loggerWrapper:      loggerWrapper,
	}
}

package instance_model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Motivos gravados em disconnect_reason quando a desconexão foi pedida pela
// API. O boot usa isso para não religar instâncias desligadas de propósito.
const (
	DisconnectReasonByAPI       = "Disconnected by API"
	DisconnectReasonLogoutByAPI = "Logged out by API"
)

type Instance struct {
	Id               string    `json:"id" gorm:"type:uuid;primaryKey"`
	Name             string    `json:"name"`
	Token            string    `json:"token" gorm:"unique"`
	Webhook          string    `json:"webhook"`
	RabbitmqEnable   string    `json:"rabbitmqEnable"`
	WebSocketEnable  string    `json:"websocketEnable"`
	NatsEnable       string    `json:"natsEnable"`
	Jid              string    `json:"jid" gorm:"column:jid"`
	Qrcode           string    `json:"qrcode" gorm:"type:text"`
	Connected        bool      `json:"connected"`
	Expiration       int64     `json:"expiration"`
	DisconnectReason string    `json:"disconnect_reason"`
	Events           string    `json:"events"`
	OsName           string    `json:"os_name"`
	Proxy            string    `json:"proxy"`
	ClientName       string    `json:"client_name"`
	CreatedAt        time.Time `json:"createdAt" gorm:"autoCreateTime"`

	// NeedsQR é calculado na leitura (não é coluna): desconectada e sem sessão
	// para retomar — só um QR novo resolve.
	NeedsQR bool `json:"needsQr" gorm:"-"`

	// Advanced Settings
	AlwaysOnline  bool   `json:"alwaysOnline" gorm:"default:false"`
	RejectCall    bool   `json:"rejectCall" gorm:"default:false"`
	MsgRejectCall string `json:"msgRejectCall" gorm:"default:''"`
	ReadMessages  bool   `json:"readMessages" gorm:"default:false"`
	IgnoreGroups  bool   `json:"ignoreGroups" gorm:"default:false"`
	IgnoreStatus  bool   `json:"ignoreStatus" gorm:"default:false"`
}

// AdvancedSettings representa as configurações avançadas de uma instância.
// [Athene/PR#136] Os bools são ponteiros para que chaves JSON omitidas no PUT
// não sejam gravadas como false (update parcial).
type AdvancedSettings struct {
	AlwaysOnline  *bool  `json:"alwaysOnline"`
	RejectCall    *bool  `json:"rejectCall"`
	MsgRejectCall string `json:"msgRejectCall"`
	ReadMessages  *bool  `json:"readMessages"`
	IgnoreGroups  *bool  `json:"ignoreGroups"`
	IgnoreStatus  *bool  `json:"ignoreStatus"`
}

func (m *Instance) BeforeCreate(tx *gorm.DB) (err error) {
	if m.Id == "" {
		m.Id = uuid.New().String()
	}
	return
}

// BoolPtr devolve um ponteiro para v (helper para respostas/updates de AdvancedSettings). [Athene/PR#136]
func BoolPtr(v bool) *bool {
	return &v
}

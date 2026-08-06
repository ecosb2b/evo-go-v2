package typebot_model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Status de uma sessão de conversa com o bot.
const (
	SessionOpened = "opened"
	SessionPaused = "paused"
	SessionClosed = "closed"
)

// Typebot é a configuração de um bot para uma instância.
//
// A tabela aceita mais de uma linha por instância — é só uma FK, não custa nada
// — mas a seleção hoje pega o primeiro habilitado. Isso evita ter que migrar o
// schema se um dia for preciso rotear entre vários bots por keyword ou regex.
//
// Portado de evolution-foundation/evolution-api
// (src/api/integrations/chatbot/typebot), Apache 2.0. Lá a configuração é
// genérica para sete integrações; aqui é só Typebot, e os campos que aquela
// base tem e não usamos ficaram de fora: debounceTime, keepOpen, fallback,
// triggerType/triggerOperator e ignoreJids.
type Typebot struct {
	Id         string `json:"id" gorm:"type:uuid;primaryKey"`
	InstanceID string `json:"instanceId" gorm:"index;not null"`

	Enabled     bool   `json:"enabled" gorm:"default:true"`
	Description string `json:"description"`

	// URL base do Typebot (ex.: https://viewer.exemplo.net) e o nome público do
	// fluxo, que é o {typebot} da rota /api/v1/typebots/{typebot}/startChat.
	URL     string `json:"url" gorm:"not null"`
	Typebot string `json:"typebot" gorm:"not null"`

	// Expire é o tempo em minutos sem interação após o qual a sessão é
	// encerrada e a próxima mensagem começa um fluxo novo. Zero desliga a
	// expiração.
	Expire int `json:"expire" gorm:"default:0"`

	// KeywordFinish é a palavra que o contato manda para encerrar (ex.: "#sair").
	KeywordFinish string `json:"keywordFinish"`

	// UnknownMessage é enviado quando o Typebot responde sem nenhum texto.
	UnknownMessage string `json:"unknownMessage"`

	// DelayMessage é a pausa em milissegundos antes de cada mensagem enviada,
	// para a resposta não parecer instantânea demais.
	DelayMessage int `json:"delayMessage" gorm:"default:0"`

	// ListeningFromMe faz o bot reagir também às mensagens enviadas pela própria
	// instância. StopBotFromMe encerra a sessão quando o operador escreve
	// manualmente na conversa — é o que permite assumir um atendimento.
	ListeningFromMe bool `json:"listeningFromMe" gorm:"default:false"`
	StopBotFromMe   bool `json:"stopBotFromMe" gorm:"default:true"`

	CreatedAt time.Time `json:"createdAt" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updatedAt" gorm:"autoUpdateTime"`
}

func (m *Typebot) BeforeCreate(tx *gorm.DB) (err error) {
	if m.Id == "" {
		m.Id = uuid.New().String()
	}
	return
}

// TypebotSession é a conversa em andamento entre um contato e um bot.
//
// SessionID guarda o identificador devolvido pelo Typebot no startChat, usado
// depois no continueChat. Ele fica num campo próprio: o projeto de origem
// concatena "{id}-{sessionId}" numa string só e recupera com split('-'), o que
// quebra se o identificador contiver hífen.
type TypebotSession struct {
	Id         string `json:"id" gorm:"type:uuid;primaryKey"`
	InstanceID string `json:"instanceId" gorm:"index;not null"`
	TypebotID  string `json:"typebotId" gorm:"index;not null"`

	// RemoteJid identifica o contato (ex.: 5588999999999@s.whatsapp.net).
	RemoteJid string `json:"remoteJid" gorm:"index;not null"`
	PushName  string `json:"pushName"`

	SessionID string `json:"sessionId"`
	Status    string `json:"status" gorm:"default:'opened'"`

	// AwaitUser indica que a última mensagem foi do bot e estamos esperando o
	// contato responder.
	AwaitUser bool `json:"awaitUser" gorm:"default:false"`

	CreatedAt time.Time `json:"createdAt" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updatedAt" gorm:"autoUpdateTime"`
}

func (m *TypebotSession) BeforeCreate(tx *gorm.DB) (err error) {
	if m.Id == "" {
		m.Id = uuid.New().String()
	}
	return
}

// IsExpired informa se a sessão passou do tempo de inatividade configurado.
// expireMinutes igual a zero desliga a expiração.
func (m *TypebotSession) IsExpired(expireMinutes int, now time.Time) bool {
	if expireMinutes <= 0 {
		return false
	}
	return now.Sub(m.UpdatedAt) > time.Duration(expireMinutes)*time.Minute
}

// TypebotRequest é o corpo aceito na criação e na atualização de um bot.
// Os bools são ponteiros para que chaves omitidas num PUT não sejam gravadas
// como false — mesmo padrão de AdvancedSettings em instance_model.
type TypebotRequest struct {
	Enabled         *bool  `json:"enabled"`
	Description     string `json:"description"`
	URL             string `json:"url"`
	Typebot         string `json:"typebot"`
	Expire          *int   `json:"expire"`
	KeywordFinish   string `json:"keywordFinish"`
	UnknownMessage  string `json:"unknownMessage"`
	DelayMessage    *int   `json:"delayMessage"`
	ListeningFromMe *bool  `json:"listeningFromMe"`
	StopBotFromMe   *bool  `json:"stopBotFromMe"`
}

package typebot_service

import (
	"net/http"
	"strings"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	send_service "github.com/evolution-foundation/evolution-go/pkg/sendMessage/service"
	typebot_model "github.com/evolution-foundation/evolution-go/pkg/typebot/model"
	typebot_repository "github.com/evolution-foundation/evolution-go/pkg/typebot/repository"
)

// typebotHTTPTimeout limita as chamadas ao Typebot. Sem isso, um servidor
// pendurado seguraria a goroutine de eventos do whatsmeow da instância.
const typebotHTTPTimeout = 30 * time.Second

type TypebotService interface {
	// ProcessMessage é o ponto de entrada: recebe uma mensagem já normalizada e
	// conduz a conversa com o bot. Não retorna erro porque é chamado do handler
	// de eventos — falhas são registradas em log, nunca propagadas para o
	// processamento da mensagem.
	ProcessMessage(instance *instance_model.Instance, remoteJid, pushName, content string, fromMe bool)
}

type typebotService struct {
	typebotRepository typebot_repository.TypebotRepository
	sendService       send_service.SendService
	config            *config.Config
	loggerWrapper     *logger_wrapper.LoggerManager
	httpClient        *http.Client
}

func NewTypebotService(
	typebotRepository typebot_repository.TypebotRepository,
	sendService send_service.SendService,
	config *config.Config,
	loggerWrapper *logger_wrapper.LoggerManager,
) TypebotService {
	return &typebotService{
		typebotRepository: typebotRepository,
		sendService:       sendService,
		config:            config,
		loggerWrapper:     loggerWrapper,
		httpClient:        &http.Client{Timeout: typebotHTTPTimeout},
	}
}

// ProcessMessage implementa a máquina de estados. A ordem das checagens importa:
// a de fromMe vem antes da sessão porque encerrar um atendimento assumido pelo
// operador não deve depender de haver sessão aberta.
func (t *typebotService) ProcessMessage(instance *instance_model.Instance, remoteJid, pushName, content string, fromMe bool) {
	log := t.loggerWrapper.GetLogger(instance.Id)

	// Status broadcast não é conversa.
	if remoteJid == "status@broadcast" || remoteJid == "" {
		return
	}

	bot, err := t.typebotRepository.GetActiveBot(instance.Id)
	if err != nil {
		log.LogError("[%s] typebot: erro ao buscar bot: %v", instance.Id, err)
		return
	}
	if bot == nil {
		return
	}

	session, err := t.typebotRepository.GetSessionByRemoteJid(instance.Id, remoteJid)
	if err != nil {
		log.LogError("[%s] typebot: erro ao buscar sessão de %s: %v", instance.Id, remoteJid, err)
		return
	}

	if fromMe {
		// O operador escreveu na conversa. StopBotFromMe encerra a sessão para
		// ele assumir o atendimento sem o bot respondendo por cima.
		if bot.StopBotFromMe && session != nil && session.Status == typebot_model.SessionOpened {
			session.Status = typebot_model.SessionClosed
			if err := t.typebotRepository.UpdateSession(session); err != nil {
				log.LogError("[%s] typebot: erro ao encerrar sessão de %s: %v", instance.Id, remoteJid, err)
				return
			}
			log.LogInfo("[%s] typebot: sessão de %s encerrada porque a instância enviou uma mensagem", instance.Id, remoteJid)
			return
		}
		if !bot.ListeningFromMe {
			return
		}
	}

	if session != nil {
		switch session.Status {
		case typebot_model.SessionPaused:
			return
		case typebot_model.SessionClosed:
			// Sessão encerrada começa um fluxo novo.
			session = nil
		default:
			if session.IsExpired(bot.Expire, time.Now()) {
				log.LogInfo("[%s] typebot: sessão de %s expirou após %d min de inatividade", instance.Id, remoteJid, bot.Expire)
				session.Status = typebot_model.SessionClosed
				if err := t.typebotRepository.UpdateSession(session); err != nil {
					log.LogError("[%s] typebot: erro ao expirar sessão de %s: %v", instance.Id, remoteJid, err)
				}
				session = nil
			}
		}
	}

	// A palavra de encerramento só faz sentido com sessão em andamento.
	if session != nil && bot.KeywordFinish != "" &&
		strings.EqualFold(strings.TrimSpace(content), strings.TrimSpace(bot.KeywordFinish)) {
		session.Status = typebot_model.SessionClosed
		if err := t.typebotRepository.UpdateSession(session); err != nil {
			log.LogError("[%s] typebot: erro ao encerrar sessão de %s: %v", instance.Id, remoteJid, err)
		}
		log.LogInfo("[%s] typebot: sessão de %s encerrada pela palavra-chave", instance.Id, remoteJid)
		return
	}

	var reply *typebotResponse
	if session == nil {
		reply, err = t.startChat(bot, instance, remoteJid, pushName)
		if err != nil {
			log.LogError("[%s] typebot: startChat falhou para %s: %v", instance.Id, remoteJid, err)
			return
		}
		session = &typebot_model.TypebotSession{
			InstanceID: instance.Id,
			TypebotID:  bot.Id,
			RemoteJid:  remoteJid,
			PushName:   pushName,
			SessionID:  reply.SessionID,
			Status:     typebot_model.SessionOpened,
		}
		if err := t.typebotRepository.CreateSession(session); err != nil {
			log.LogError("[%s] typebot: erro ao criar sessão de %s: %v", instance.Id, remoteJid, err)
			return
		}
	} else {
		reply, err = t.continueChat(bot, session.SessionID, content)
		if err != nil {
			log.LogError("[%s] typebot: continueChat falhou para %s: %v", instance.Id, remoteJid, err)
			return
		}
	}

	t.deliver(bot, instance, session, reply)

	session.AwaitUser = true
	session.Status = typebot_model.SessionOpened
	if err := t.typebotRepository.UpdateSession(session); err != nil {
		log.LogError("[%s] typebot: erro ao atualizar sessão de %s: %v", instance.Id, remoteJid, err)
	}
}

// deliver envia ao contato o que o Typebot devolveu. Quando não vem nenhum
// texto aproveitável, cai no UnknownMessage — é o que a tela do Evolution API
// chama de "Unknown Message".
func (t *typebotService) deliver(
	bot *typebot_model.Typebot,
	instance *instance_model.Instance,
	session *typebot_model.TypebotSession,
	reply *typebotResponse,
) {
	log := t.loggerWrapper.GetLogger(instance.Id)
	sent := false

	// Um "wait" pedido pelo fluxo é honrado antes de entregar as mensagens.
	for _, action := range reply.ClientSideActions {
		if action.Type == "wait" && action.Wait != nil && action.Wait.Seconds > 0 {
			seconds := action.Wait.Seconds
			// Teto para que um fluxo mal configurado não prenda a goroutine.
			if seconds > 60 {
				seconds = 60
			}
			time.Sleep(time.Duration(seconds) * time.Second)
		}
	}

	for _, message := range reply.Messages {
		switch message.Type {
		case "text":
			text := flattenRichText(message.Content.RichText)
			if strings.TrimSpace(text) == "" {
				continue
			}
			if err := t.sendText(bot, instance, session.RemoteJid, text); err != nil {
				log.LogError("[%s] typebot: erro ao enviar resposta para %s: %v", instance.Id, session.RemoteJid, err)
				continue
			}
			sent = true
		case "image", "video", "audio":
			url := strings.TrimSpace(message.Content.URL)
			if url == "" {
				log.LogWarn("[%s] typebot: bloco '%s' sem url, descartado", instance.Id, message.Type)
				continue
			}
			if err := t.sendMedia(bot, instance, session.RemoteJid, message.Type, url); err != nil {
				// O envio de mídia tem mais formas de falhar que o de texto: a URL
				// pode estar fora do ar, e o SendMediaUrl recusa formatos que o
				// WhatsApp não aceita (só jpeg/png/webp em imagem, só mp4 em
				// vídeo). Nesses casos o link ainda é melhor que silêncio.
				log.LogWarn("[%s] typebot: envio de %s falhou para %s (%v), enviando como link", instance.Id, message.Type, session.RemoteJid, err)
				if err := t.sendText(bot, instance, session.RemoteJid, url); err != nil {
					log.LogError("[%s] typebot: erro ao enviar link para %s: %v", instance.Id, session.RemoteJid, err)
					continue
				}
			}
			sent = true

		default:
			// "embed" e qualquer bloco novo do Typebot: não há como renderizar no
			// WhatsApp, então vai o link.
			if url := strings.TrimSpace(message.Content.URL); url != "" {
				if err := t.sendText(bot, instance, session.RemoteJid, url); err != nil {
					log.LogError("[%s] typebot: erro ao enviar link para %s: %v", instance.Id, session.RemoteJid, err)
					continue
				}
				sent = true
				continue
			}
			log.LogWarn("[%s] typebot: tipo de bloco '%s' desconhecido e sem url, descartado", instance.Id, message.Type)
		}
	}

	if sent {
		return
	}

	// Resposta sem mensagem não é sinônimo de "não entendi". Quando o fluxo pede
	// uma ação de cliente, ele está apenas num passo que espera algo do cliente —
	// mandar o unknownMessage aqui faria o contato receber um erro logo no
	// primeiro "oi".
	if blocking := unsupportedActions(reply); len(blocking) > 0 {
		log.LogWarn(
			"[%s] typebot: o fluxo pediu ação de cliente não suportada (%s) para %s. "+
				"Não há interpretador JavaScript em Go: mova essa lógica para o fluxo usando as "+
				"prefilledVariables já enviadas (normalizedUserId, userPhone, userLid, jidType), "+
				"ou a conversa vai travar neste passo.",
			instance.Id, strings.Join(blocking, ", "), session.RemoteJid,
		)
		return
	}

	if len(reply.Messages) == 0 {
		log.LogWarn("[%s] typebot: fluxo respondeu sem mensagens para %s", instance.Id, session.RemoteJid)
		return
	}

	if bot.UnknownMessage != "" {
		if err := t.sendText(bot, instance, session.RemoteJid, bot.UnknownMessage); err != nil {
			log.LogError("[%s] typebot: erro ao enviar unknownMessage para %s: %v", instance.Id, session.RemoteJid, err)
		}
	}
}

// unsupportedActions lista as ações de cliente que este runtime não consegue
// executar e que, por isso, deixariam a conversa parada.
func unsupportedActions(reply *typebotResponse) []string {
	var blocking []string
	for _, action := range reply.ClientSideActions {
		if action.Type == "wait" {
			continue
		}
		blocking = append(blocking, action.Type)
	}
	return blocking
}

func (t *typebotService) sendText(
	bot *typebot_model.Typebot,
	instance *instance_model.Instance,
	remoteJid, text string,
) error {
	_, err := t.sendService.SendText(&send_service.TextStruct{
		Number: numberFromJid(remoteJid),
		Text:   text,
		Delay:  int32(bot.DelayMessage),
	}, instance)
	return err
}

// sendMedia baixa e envia a mídia devolvida pelo Typebot. O SendMediaUrl cuida
// do download, da detecção de mime e — no caso de áudio — da conversão para
// opus, que é o formato que o WhatsApp aceita como mensagem de voz.
func (t *typebotService) sendMedia(
	bot *typebot_model.Typebot,
	instance *instance_model.Instance,
	remoteJid, mediaType, url string,
) error {
	_, err := t.sendService.SendMediaUrl(&send_service.MediaStruct{
		Number: numberFromJid(remoteJid),
		Url:    url,
		Type:   mediaType,
		Delay:  int32(bot.DelayMessage),
	}, instance)
	return err
}

// numberFromJid extrai o número do JID. JIDs @lid não têm número utilizável e
// são repassados inteiros, como faz a implementação de referência.
func numberFromJid(remoteJid string) string {
	if strings.Contains(remoteJid, "@lid") {
		return remoteJid
	}
	if i := strings.Index(remoteJid, "@"); i >= 0 {
		return remoteJid[:i]
	}
	return remoteJid
}

package typebot_service

import (
	"net/http"
	"strings"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_repository "github.com/evolution-foundation/evolution-go/pkg/instance/repository"
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

	selfJids *selfJidCache
	buckets  *bucketRegistry
	alerts   AlertEmitter
}

func NewTypebotService(
	typebotRepository typebot_repository.TypebotRepository,
	instanceRepository instance_repository.InstanceRepository,
	sendService send_service.SendService,
	alerts AlertEmitter,
	config *config.Config,
	loggerWrapper *logger_wrapper.LoggerManager,
) TypebotService {
	return &typebotService{
		typebotRepository: typebotRepository,
		sendService:       sendService,
		config:            config,
		loggerWrapper:     loggerWrapper,
		httpClient:        &http.Client{Timeout: typebotHTTPTimeout},

		selfJids: newSelfJidCache(instanceRepository),
		buckets:  newBucketRegistry(config.TypebotSendRateLimit, config.TypebotSendRateBurst),
		alerts:   alerts,
	}
}

// pauseSession encerra o atendimento automático de um contato e avisa quem
// opera. A pausa é reversível pelo endpoint /typebot/changeStatus.
//
// O alerta importa tanto quanto a pausa: sem ele a proteção age em silêncio, e
// um contato legítimo pausado por engano só seria descoberto quando alguém
// reclamasse.
func (t *typebotService) pauseSession(
	instance *instance_model.Instance,
	session *typebot_model.TypebotSession,
	reason string,
	detail map[string]any,
) {
	log := t.loggerWrapper.GetLogger(instance.Id)

	session.Status = typebot_model.SessionPaused
	session.PausedReason = reason
	if err := t.typebotRepository.UpdateSession(session); err != nil {
		log.LogError("[%s] typebot: erro ao pausar sessão de %s: %v", instance.Id, session.RemoteJid, err)
		return
	}

	log.LogWarn("[%s] typebot: sessão de %s pausada automaticamente (%s) %v",
		instance.Id, session.RemoteJid, reason, detail)

	if t.alerts == nil {
		return
	}
	payload := map[string]any{
		"remoteJid": session.RemoteJid,
		"sessionId": session.Id,
		"reason":    reason,
	}
	for k, v := range detail {
		payload[k] = v
	}
	t.alerts.SendOperationalEvent(instance, "TypebotAutoPaused", payload)
}

// withinContactRate conta as mensagens do contato numa janela deslizante e
// devolve false quando o limite estoura.
//
// A contagem vive na sessão para sobreviver a um restart: zerar o contador
// quando o processo reinicia daria ao contato em flood exatamente a folga que a
// proteção existe para negar.
func (t *typebotService) withinContactRate(session *typebot_model.TypebotSession) (bool, int) {
	limit := t.config.TypebotContactRateLimit
	window := time.Duration(t.config.TypebotContactRateWindow) * time.Second

	if limit <= 0 || window <= 0 {
		return true, 0
	}

	now := time.Now()
	if session.WindowStart.IsZero() || now.Sub(session.WindowStart) > window {
		session.WindowStart = now
		session.MsgCount = 0
	}

	session.MsgCount++
	return session.MsgCount <= limit, session.MsgCount
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

	// Antes de qualquer coisa: se o remetente é outra instância deste servidor,
	// responder cria um laço em que os dois lados se alimentam sem parar. Vem
	// antes até da busca do bot porque é a única checagem que não admite
	// exceção.
	if t.selfJids.contains(remoteJid) {
		log.LogWarn("[%s] typebot: %s é uma instância deste servidor, ignorado para evitar laço", instance.Id, remoteJid)
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

	// O limite por contato é avaliado antes de falar com o Typebot: o objetivo é
	// cortar no gateway, sem gastar a chamada externa nem produzir resposta.
	if session != nil {
		if ok, count := t.withinContactRate(session); !ok {
			t.pauseSession(instance, session, ReasonRateLimit, map[string]any{
				"messages":      count,
				"windowSeconds": t.config.TypebotContactRateWindow,
				"limit":         t.config.TypebotContactRateLimit,
			})
			return
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

	// Fim de fluxo: sem input e sem ação pendente, o Typebot não tem mais passos.
	// Manter a sessão aberta faria a próxima mensagem ir para continueChat com um
	// sessionId que o Typebot já descartou — respondendo vazio, o que acabaria
	// disparando o unknownMessage sem motivo.
	//
	// Encerrando aqui, a próxima mensagem começa um fluxo novo pela saudação, que
	// é o comportamento esperado de quem volta a falar depois de terminar.
	if reply.awaitsUser() {
		session.AwaitUser = true
		session.Status = typebot_model.SessionOpened
	} else {
		session.AwaitUser = false
		session.Status = typebot_model.SessionClosed
		log.LogInfo("[%s] typebot: fluxo concluído para %s, sessão encerrada", instance.Id, remoteJid)
	}

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
	// O teto por instância é aplicado no envio, não na entrada: o que o WhatsApp
	// mede é quanto o número manda, e uma resposta pode conter várias mensagens.
	if waited := t.buckets.wait(instance.Id); waited > 0 {
		t.loggerWrapper.GetLogger(instance.Id).LogInfo(
			"[%s] typebot: envio para %s aguardou %s pelo teto da instância", instance.Id, remoteJid, waited)
	}

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
	t.buckets.wait(instance.Id)

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

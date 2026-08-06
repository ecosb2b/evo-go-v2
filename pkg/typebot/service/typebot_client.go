package typebot_service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	typebot_model "github.com/evolution-foundation/evolution-go/pkg/typebot/model"
)

// typebotResponse é o corpo devolvido tanto pelo startChat quanto pelo
// continueChat. Só os campos que usamos são declarados; o Typebot manda
// bastante coisa a mais (input, clientSideActions, dynamicTheme) que não
// interfere no envio para o WhatsApp.
type typebotResponse struct {
	SessionID string           `json:"sessionId"`
	Messages  []typebotMessage `json:"messages"`
	// Input presente significa que o fluxo parou num passo esperando o contato
	// responder. Ausente, o fluxo chegou ao fim — é o único sinal que o Typebot
	// dá disso, e é o que permite encerrar a sessão em vez de deixá-la aberta
	// apontando para um sessionId que o Typebot já descartou.
	Input *typebotInput `json:"input"`
	// ClientSideActions são passos que o Typebot espera que o CLIENTE execute
	// (rodar um script, esperar N segundos) antes de continuar. O Evolution API
	// consegue rodar os scripts porque é Node; em Go não há motor de JavaScript,
	// então só o "wait" é honrado — ver handleClientSideActions.
	ClientSideActions []clientSideAction `json:"clientSideActions"`
}

// typebotInput só precisa existir ou não; o tipo é registrado para diagnóstico.
type typebotInput struct {
	Type string `json:"type"`
}

// awaitsUser informa se a conversa continua — porque o fluxo espera uma resposta
// do contato, ou porque pediu uma ação de cliente e aguarda o retorno dela.
func (r *typebotResponse) awaitsUser() bool {
	if r.Input != nil {
		return true
	}
	for _, action := range r.ClientSideActions {
		if action.ExpectsDedicatedReply {
			return true
		}
	}
	return false
}

type clientSideAction struct {
	Type string `json:"type"`
	Wait *struct {
		Seconds int `json:"secondsToWaitFor"`
	} `json:"wait"`
	ExpectsDedicatedReply bool `json:"expectsDedicatedReply"`
}

type typebotMessage struct {
	Type    string         `json:"type"`
	Content typebotContent `json:"content"`
}

type typebotContent struct {
	RichText []richTextBlock `json:"richText"`
	// URL cobre os blocos de mídia (image, video, audio, embed), que hoje são
	// entregues como link.
	URL string `json:"url"`
}

type richTextBlock struct {
	Children []richTextElement `json:"children"`
}

// richTextElement é recursivo: o Typebot aninha elementos para compor
// formatação e links.
type richTextElement struct {
	Type          string            `json:"type"`
	Text          string            `json:"text"`
	Bold          bool              `json:"bold"`
	Italic        bool              `json:"italic"`
	Underline     bool              `json:"underline"`
	Strikethrough bool              `json:"strikethrough"`
	URL           string            `json:"url"`
	Children      []richTextElement `json:"children"`
}

func (t *typebotService) startChat(
	bot *typebot_model.Typebot,
	instance *instance_model.Instance,
	remoteJid, pushName string,
) (*typebotResponse, error) {
	url := fmt.Sprintf("%s/api/v1/typebots/%s/startChat",
		strings.TrimRight(bot.URL, "/"), bot.Typebot)

	// As prefilledVariables ficam disponíveis dentro do fluxo do Typebot.
	//
	// A implementação de referência inclui aqui a GLOBAL_API_KEY. Mandamos o
	// token da instância no lugar: ele basta para o fluxo chamar /send/* daquela
	// instância, e não entrega a chave que administra o servidor inteiro a um
	// Typebot que pode estar hospedado por terceiros.
	// O JID já vai normalizado. O fluxo não precisa de um bloco de script para
	// separar telefone de LID — em Go isso é trivial, e evita depender de
	// clientSideActions/setVariable, que exigiriam um interpretador JavaScript.
	jid := normalizeJid(remoteJid)

	body := map[string]any{
		"prefilledVariables": map[string]any{
			"remoteJid":    remoteJid,
			"pushName":     pushName,
			"instanceName": instance.Name,
			"instanceId":   instance.Id,
			"ownerJid":     instance.Jid,
			"apiKey":       instance.Token,

			"normalizedUserId": jid.NormalizedID,
			"userPhone":        jid.Phone,
			"userLid":          jid.Lid,
			"jidType":          jid.Type,
		},
	}

	return t.post(url, body)
}

func (t *typebotService) continueChat(
	bot *typebot_model.Typebot,
	sessionID, content string,
) (*typebotResponse, error) {
	url := fmt.Sprintf("%s/api/v1/sessions/%s/continueChat",
		strings.TrimRight(bot.URL, "/"), sessionID)

	return t.post(url, map[string]any{"message": content})
}

func (t *typebotService) post(url string, body any) (*typebotResponse, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar corpo: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("erro ao montar requisição: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro na chamada ao typebot: %w", err)
	}
	defer resp.Body.Close()

	// Limite de leitura para que uma resposta anormalmente grande não consuma a
	// memória do processo.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("typebot respondeu %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var parsed typebotResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("erro ao interpretar resposta: %w", err)
	}

	return &parsed, nil
}

// normalizedJid é o JID do contato decomposto, entregue ao fluxo do Typebot em
// prefilledVariables.
type normalizedJid struct {
	NormalizedID string
	Phone        string
	Lid          string
	Type         string
}

// normalizeJid separa telefone de LID. Reproduz a lógica que os fluxos costumam
// fazer num bloco de script: o LID é preservado inteiro (não dá para converter
// em telefone), enquanto o @s.whatsapp.net vira só o número.
func normalizeJid(remoteJid string) normalizedJid {
	switch {
	case remoteJid == "":
		return normalizedJid{Type: "unknown"}

	case strings.HasSuffix(remoteJid, "@lid"):
		return normalizedJid{
			Type:         "lid",
			Lid:          strings.TrimSuffix(remoteJid, "@lid"),
			NormalizedID: remoteJid,
		}

	case strings.HasSuffix(remoteJid, "@s.whatsapp.net"):
		phone := strings.TrimSuffix(remoteJid, "@s.whatsapp.net")
		return normalizedJid{
			Type:         "phone",
			Phone:        phone,
			NormalizedID: phone,
		}

	default:
		return normalizedJid{Type: "other", NormalizedID: remoteJid}
	}
}

// flattenRichText converte o rich text do Typebot em texto com a formatação do
// WhatsApp. Cada bloco vira uma linha.
func flattenRichText(blocks []richTextBlock) string {
	var out strings.Builder
	for i, block := range blocks {
		for _, element := range block.Children {
			out.WriteString(renderElement(element))
		}
		if i < len(blocks)-1 {
			out.WriteString("\n")
		}
	}
	return strings.TrimRight(out.String(), "\n")
}

func renderElement(element richTextElement) string {
	text := element.Text

	if len(element.Children) > 0 {
		var nested strings.Builder
		for _, child := range element.Children {
			nested.WriteString(renderElement(child))
		}
		text = nested.String()
	}

	if text == "" {
		return ""
	}

	// Sem marcação a aplicar, o texto sai exatamente como veio.
	//
	// Isso importa: os fluxos costumam já escrever a marcação do WhatsApp
	// diretamente no texto ("*negrito*") e usar quebras de linha no início do
	// bloco para espaçar a mensagem. Normalizar o espaçamento aqui apagaria
	// essas quebras — sublinhado, aliás, nem existe no WhatsApp.
	if !element.Bold && !element.Italic && !element.Strikethrough {
		return appendLinkURL(element, text)
	}

	// Para envolver com marcadores, o espaço em branco das bordas precisa ficar
	// de fora: "*texto *" não é renderizado como negrito pelo WhatsApp.
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return text
	}

	start := strings.Index(text, trimmed)
	leading, trailing := text[:start], text[start+len(trimmed):]

	if element.Bold {
		trimmed = "*" + trimmed + "*"
	}
	if element.Italic {
		trimmed = "_" + trimmed + "_"
	}
	if element.Strikethrough {
		trimmed = "~" + trimmed + "~"
	}

	text = leading + trimmed + trailing

	return appendLinkURL(element, text)
}

// appendLinkURL mantém o destino de um link visível. O WhatsApp não tem âncora,
// então um rótulo diferente da URL perderia o endereço se ela não fosse anexada.
func appendLinkURL(element richTextElement, text string) string {
	if element.Type == "a" && element.URL != "" && !strings.Contains(text, element.URL) {
		return fmt.Sprintf("%s (%s)", text, element.URL)
	}
	return text
}

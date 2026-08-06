package typebot_service

import (
	"encoding/json"
	"strings"
	"testing"
)

// realStartChatResponse é a resposta de verdade de um startChat, capturada de
// um Typebot em produção. Serve de âncora: a estrutura do rich text foi inferida
// lendo a implementação em TypeScript do evolution-api, e só uma resposta real
// prova que a leitura estava certa.
//
// Dois detalhes deste payload já quebraram o parser uma vez:
//   - a marcação do WhatsApp vem literal no texto ("*negrito*"), não em atributos
//     bold/italic;
//   - blocos começam com "\n" para espaçar a mensagem, e um TrimSpace ingênuo
//     apagava essas quebras.
const realStartChatResponse = `{
  "sessionId": "h7pyi59xxa3ws1evg8h6pt8v",
  "typebot": {"id": "p1zb3pu9beruhg5iu5kwuvk1", "version": "6"},
  "messages": [
    {
      "id": "hvb8czilhtjlfhq9bn0cktxo",
      "type": "text",
      "content": {
        "type": "richText",
        "richText": [
          {"type": "p", "children": [{"text": "Olá! Seja bem-vindo ao"}]},
          {"type": "p", "children": [{"text": "*Grupo ECOSTI!*"}]},
          {"type": "p", "children": [{"text": "\n🏭 *Fábrica de Móveis de Alto Padrão*"}]},
          {"type": "p", "children": [{"text": ""}]},
          {"type": "p", "children": [{"text": "🚨 *ATENÇÃO, VENDAS APENAS PARA LOGISTAS*"}]}
        ]
      }
    },
    {
      "id": "lbxmbv5ou5x1jkj7y9ptqc9l",
      "type": "text",
      "content": {
        "type": "richText",
        "richText": [
          {"type": "p", "children": [{"text": "💬 *Em que podemos ajudar?*"}]},
          {"type": "p", "children": [{"text": "1️⃣ - *Sou Cliente*"}]},
          {"type": "p", "children": [{"text": "0️⃣ - *SAIR*"}]}
        ]
      }
    }
  ],
  "clientSideActions": []
}`

func TestParseRealStartChatResponse(t *testing.T) {
	var parsed typebotResponse
	if err := json.Unmarshal([]byte(realStartChatResponse), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed.SessionID != "h7pyi59xxa3ws1evg8h6pt8v" {
		t.Errorf("sessionId = %q", parsed.SessionID)
	}
	if len(parsed.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(parsed.Messages))
	}

	first := flattenRichText(parsed.Messages[0].Content.RichText)

	// A quebra de linha que abre o terceiro bloco tem de sobreviver: ela é o que
	// separa o cabeçalho do corpo na mensagem que o contato recebe.
	want := "Olá! Seja bem-vindo ao\n*Grupo ECOSTI!*\n\n🏭 *Fábrica de Móveis de Alto Padrão*\n\n🚨 *ATENÇÃO, VENDAS APENAS PARA LOGISTAS*"
	if first != want {
		t.Errorf("primeira mensagem:\n--- obtido ---\n%q\n--- esperado ---\n%q", first, want)
	}

	// A marcação escrita à mão no fluxo precisa chegar intacta ao WhatsApp.
	if !strings.Contains(first, "*Grupo ECOSTI!*") {
		t.Error("marcação literal de negrito foi alterada")
	}

	second := flattenRichText(parsed.Messages[1].Content.RichText)
	if second != "💬 *Em que podemos ajudar?*\n1️⃣ - *Sou Cliente*\n0️⃣ - *SAIR*" {
		t.Errorf("segunda mensagem = %q", second)
	}
}

// TestAwaitsUser cobre a decisão de encerrar ou manter a sessão. Errar aqui tem
// consequência visível: manter aberta uma sessão que o Typebot já descartou faz a
// próxima mensagem cair no unknownMessage, e encerrar cedo demais faz o contato
// receber a saudação no meio da conversa.
func TestAwaitsUser(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{
			"input presente: fluxo espera resposta",
			`{"messages":[],"input":{"type":"text input"}}`,
			true,
		},
		{
			"sem input: fluxo terminou",
			`{"messages":[{"type":"text"}]}`,
			false,
		},
		{
			"input null equivale a ausente",
			`{"messages":[],"input":null}`,
			false,
		},
		{
			// O passo de script devolve messages e input vazios, mas a conversa
			// não acabou — o Typebot aguarda o retorno da ação.
			"acao de cliente aguardando retorno",
			`{"messages":[],"clientSideActions":[{"type":"setVariable","expectsDedicatedReply":true}]}`,
			true,
		},
		{
			// Um wait não espera retorno: se não há input, o fluxo acabou mesmo.
			"wait nao segura a sessao",
			`{"messages":[],"clientSideActions":[{"type":"wait","wait":{"secondsToWaitFor":2}}]}`,
			false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var parsed typebotResponse
			if err := json.Unmarshal([]byte(tc.body), &parsed); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := parsed.awaitsUser(); got != tc.want {
				t.Errorf("awaitsUser() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRenderElementFormatting cobre o caminho em que o Typebot manda a formatação
// como atributo, em vez de marcação escrita no texto.
func TestRenderElementFormatting(t *testing.T) {
	cases := []struct {
		name    string
		element richTextElement
		want    string
	}{
		{"texto puro", richTextElement{Text: "oi"}, "oi"},
		{"negrito", richTextElement{Text: "oi", Bold: true}, "*oi*"},
		{"italico", richTextElement{Text: "oi", Italic: true}, "_oi_"},
		{"tachado", richTextElement{Text: "oi", Strikethrough: true}, "~oi~"},
		// Marcador colado em espaço não é renderizado pelo WhatsApp, então o
		// espaço fica de fora.
		{"negrito com espaco nas bordas", richTextElement{Text: " oi ", Bold: true}, " *oi* "},
		// Sublinhado não existe no WhatsApp.
		{"sublinhado ignorado", richTextElement{Text: "oi", Underline: true}, "oi"},
		// Quebra de linha de espaçamento não pode ser normalizada.
		{"quebra preservada", richTextElement{Text: "\noi"}, "\noi"},
		{
			"link com rotulo",
			richTextElement{Type: "a", URL: "https://ecosti.net", Children: []richTextElement{{Text: "nosso site"}}},
			"nosso site (https://ecosti.net)",
		},
		{
			"link com a url no texto nao duplica",
			richTextElement{Type: "a", URL: "https://ecosti.net", Children: []richTextElement{{Text: "https://ecosti.net"}}},
			"https://ecosti.net",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderElement(tc.element); got != tc.want {
				t.Errorf("renderElement() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeJid(t *testing.T) {
	cases := []struct {
		remoteJid string
		want      normalizedJid
	}{
		{"5588999999999@s.whatsapp.net", normalizedJid{Type: "phone", Phone: "5588999999999", NormalizedID: "5588999999999"}},
		// LID não tem telefone recuperável, então vai inteiro.
		{"216754450071619@lid", normalizedJid{Type: "lid", Lid: "216754450071619", NormalizedID: "216754450071619@lid"}},
		{"120363000000000000@g.us", normalizedJid{Type: "other", NormalizedID: "120363000000000000@g.us"}},
		{"", normalizedJid{Type: "unknown"}},
	}

	for _, tc := range cases {
		t.Run(tc.remoteJid, func(t *testing.T) {
			if got := normalizeJid(tc.remoteJid); got != tc.want {
				t.Errorf("normalizeJid(%q) = %+v, want %+v", tc.remoteJid, got, tc.want)
			}
		})
	}
}

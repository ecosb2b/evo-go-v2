# Customizações do fork (Athene) — EvolutionGO

Documento de rastreio das alterações feitas neste fork em relação ao upstream
`evolution-foundation/evolution-go`. Serve para facilitar upgrades futuros e
merges: ao sincronizar com o upstream ou atualizar o `whatsmeow`, revise cada
item abaixo para garantir que a customização continua aplicada e compatível.

Base: whatsmeow `v0.0.0-20260713112832-d8960d9575d2` (commit de 13/07/2026).
Última atualização deste doc: 2026-07-16.

> Atualizado de `v0.0.0-20260630180629-b572e5bcb92b` (30/06) para
> `v0.0.0-20260713112832-d8960d9575d2` (13/07) em 16/07/2026. Build compilou sem
> erro e todas as customizações abaixo continuaram válidas (nenhuma assinatura de
> API mudou). Motivo do bump: acompanhar o rollout de LID/username do WhatsApp e
> evitar falhas de descriptografia futuras.
>
> **Username/apelido do WhatsApp:** ainda NÃO suportado — o whatsmeow não expõe
> `SetUsername`/resolução de nickname nesta versão. Reavaliar em upgrades futuros.

---

## 1. Envio de botões interativos (`POST /send/button`) — CORE

**Arquivos:**
- `pkg/sendMessage/service/send_service.go` (função `SendButton`)
- `pkg/sendMessage/handler/send_handler.go` (validação)

**Problema original:** botões (principalmente `copy` / copia-e-cola) eram
entregues ao aparelho mas não renderizavam ("Aguardando mensagem" / "Não foi
possível carregar"). O whatsmeow oficial não tem suporte a envio de botões, então
foi preciso reproduzir byte a byte o molde de um Baileys que funciona
(imagem `mfilype/evolution-buttons`, Baileys 7.0.0-rc.9).

**O que foi feito (todas as peças são necessárias em conjunto):**

1. **Node `<biz>` no stanza XMPP (a peça-chave)** — injetado via `AdditionalNodes`
   em `SendMessage`. Formato extraído do fork que renderiza:
   - CTA/reply: `<biz><interactive type="native_flow" v="1"><native_flow v="2" name="mixed"/></interactive></biz>`
   - PIX: `<biz><interactive type="native_flow" v="1"><native_flow name="payment_info"/></interactive></biz>`
   - Detalhes que só descobrimos inspecionando o fork: `native_flow` leva
     `v="2"` e `name="mixed"` (NÃO o nome do tipo do botão). Nenhum node `<bot>`.

2. **`InteractiveMessage` no topo da mensagem** (sem wrapper `viewOnceMessage`
   nem `documentWithCaptionMessage`). Tentativas com esses wrappers não renderizavam.

3. **`cta_copy` no molde exato:** `{"fix":true,"display_text":...,"copy_code":...}`
   (com `fix:true`, sem `id`).

4. **`messageParamsJson`:** `{"from":"api","templateId":"<uuid v4>"}` — usa
   `github.com/google/uuid` (import adicionado no arquivo).

5. **Reply unificado com os CTAs:** botões `reply` (quick_reply) passaram a usar o
   MESMO caminho `InteractiveMessage`. O caminho antigo (`ButtonsMessage` dentro de
   `DocumentWithCaptionMessage` + node `<bot biz_bot="1"/>`) foi REMOVIDO — ele
   causava erro `405` do servidor.

6. **Footer opcional:** removida a validação `footer is required` no handler; o
   `Footer` só é incluído no `InteractiveMessage` quando preenchido.

**Contrato da API (inalterado para o cliente):**
```
POST /send/button
Header: apikey: <token da instância>
{ "number", "title", "description", "footer"(opcional),
  "buttons": [ { "type": "reply|copy|url|call|pix", "displayText", "id"|"copyCode"|"url"|"phoneNumber"... } ] }
```

**Ao fazer upgrade:** se o upstream reescrever `SendButton`, reaplique os 6 pontos
acima. Se o WhatsApp mudar o molde, capture novamente o payload de um cliente que
funcione (ver seção "Debug" abaixo) e ajuste `cta_copy`/`native_flow`.

---

## 2. `SendReportingTokens = false`

**Arquivo:** `pkg/whatsmeow/service/whatsmeow.go` (logo após `whatsmeow.NewClient`).

O whatsmeow desliga o node `<reporting>` por padrão. Durante a investigação foi
testado ligado (`true`), mas o token gerado fazia o cliente rejeitar mensagens
`nativeFlow` (sintoma: recibo `retry` seguido de "Não foi possível carregar").
Ficou **desligado** (`false`) — comportamento histórico do EvoGO, com o qual
texto/mídia/carrossel/pix/botões funcionam.

**Ao fazer upgrade:** manter `client.SendReportingTokens = false` a menos que se
comprove que ligar volta a funcionar.

---

## 3. Novo endpoint `POST /user/savecontact`

**Arquivos:**
- `pkg/user/service/user_service.go` (`SaveContact`, struct `SaveContactStruct`)
- `pkg/user/handler/user_handler.go` (`SaveContact`)
- `pkg/routes/routes.go` (rota `POST /user/savecontact`)

**O que faz:** adiciona/atualiza um contato na lista de contatos do WhatsApp da
instância, via app state patch (coleção `critical_unblock_low`, índice `contact`).
É o mesmo mecanismo da tela "Novo contato" do WhatsApp Web. Com `saveOnPhone:true`
(padrão) seta `ContactAction.SaveOnPrimaryAddressbook`, que faz o celular primário
gravar também na agenda do sistema.

**Contrato:**
```
POST /user/savecontact
Header: apikey: <token da instância>
{ "number": "5582988898565", "fullName": "Fulano de Tal",
  "firstName": "Fulano"(opcional), "saveOnPhone": true(opcional, padrão true) }
```

**Requisitos de runtime:** as app state keys precisam já estar sincronizadas
(acontece após o pareamento) e o celular primário precisa estar online para
propagar. Se faltar chave, o whatsmeow retorna `no app state keys found`.

**Imports adicionados** em `user_service.go`: `strings`,
`go.mau.fi/whatsmeow/appstate`, `go.mau.fi/whatsmeow/proto/waSyncAction`,
`google.golang.org/protobuf/proto`.

**Ao fazer upgrade do whatsmeow:** confirmar que continuam existindo
`appstate.WAPatchCriticalUnblockLow`, `appstate.IndexContact`,
`waSyncAction.ContactAction{FullName,FirstName,SaveOnPrimaryAddressbook}` e
`client.SendAppState(ctx, patch)`.

---

## 4. Fix do vazamento de conexão no Postgres (container único do sqlstore)

**Arquivos:**
- `pkg/whatsmeow/service/whatsmeow.go` (`getAuthContainer`, `sharedAuthContainer`)
- `pkg/whatsmeow/service/auth_container_retry_test.go` (teste de regressão)

**Origem:** PR #117 do upstream (`fix(whatsmeow): reuse a single capped sqlstore
container`), ainda **não mergeado** quando trouxemos. Traga como customização
temporária e **remova quando o upstream mergear** (aí o código vem do próprio
upstream e evita conflito).

**Problema:** cada `StartClient` — na conexão inicial E em toda reconexão —
chamava `sqlstore.New()`, abrindo um `*sql.DB` novo, sem cap de pool e nunca
fechado. Isso vaza conexões e vai saturando o Postgres `evogo_auth` com o tempo
(sintoma: "too many connections" / instabilidade após horas no ar).

**O que foi feito:** introduzido `getAuthContainer()` que cria **um único**
container compartilhado (`sharedAuthContainer`, protegido por mutex), com pool
capado (`SetMaxOpenConns(20)`, `SetMaxIdleConns(5)`, `ConnMaxLifetime 5min`,
`ConnMaxIdleTime 2min` no Postgres; `SetMaxOpenConns(1)` no SQLite) via
`sqlstore.NewWithDB` + `container.Upgrade`. Só o **sucesso** é memorizado — se o
Postgres estiver fora no boot, a próxima chamada tenta de novo em vez de cachear
o erro pra sempre. O `StartClient` agora só chama `w.getAuthContainer()`.

**Adaptação ao aplicar:** o teste do PR usava o import antigo
`github.com/EvolutionAPI/...`; aqui foi corrigido para
`github.com/evolution-foundation/...`. Usa o driver `modernc.org/sqlite` (já no
go.mod) e os campos `config`/`exPath` da struct `whatsmeowService`.

**Ao fazer upgrade do whatsmeow:** confirmar que `sqlstore.NewWithDB(db, dialect,
log) *Container` e `(*Container).Upgrade(ctx)` continuam existindo com essa
assinatura (validado na v0.0.0-20260713112832).

---

## Debug de botões (como reinvestigar se voltar a quebrar)

1. Ligar `WADEBUG=DEBUG` no `.env` para ver os stanzas XMPP (`--> <message ...>`).
2. Instrumentação temporária (já removida): dump do `RawMessage` de mensagens
   interativas RECEBIDAS e do payload protobuf ENVIADO no `/send/button`, ambos via
   `protojson.Marshal`. Reintroduzir se necessário para comparar com um cliente que
   funcione.
3. Comparar o payload/stanza contra um botão real que renderiza (ex.: receber um
   copia-e-cola de uma instância evolution-api v2 no mesmo número).

## Referências

- Molde dos botões: fork `mfilype/evolution-buttons` (Baileys 7.0.0-rc.9),
  `node_modules/baileys/lib/Socket/messages-send.js` (função `getButtonArgs`).
- Contatos: `whatsmeow/appstate` (encode.go, keys.go) e `appstate.go`
  (`SendAppState`, handler do índice `contact`).
- Relatos do comportamento de botões: tulir/whatsmeow discussions #650, #711.

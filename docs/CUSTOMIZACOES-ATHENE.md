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

## 5. Dashboard customizado (`GET /dashboard`)

**Arquivos:**
- `manager/dist/dashboard.html` (página estática do dashboard)
- `pkg/routes/routes.go` (rota `GET /dashboard`)

**O que faz:** serve uma página de dashboard self-hosted, no mesmo domínio da API
(sem CORS), fora do catch-all `/manager/*any` (que sempre devolve o `index.html`
do SPA React). A página consome endpoints já existentes:
- `GET /instance/all` → KPIs (total/conectadas/desconectadas/disponibilidade),
  donut de status, gráfico de instâncias criadas por dia, tabela de instâncias.
- `GET /server/ok` → indicador de saúde do servidor.
- `GET /instance/logs/:id` → painel de logs recentes ao clicar numa instância.

Autenticação: a página pede a **Global API Key** no próprio navegador e guarda em
`localStorage` (é uma página do próprio servidor do usuário, não um artifact).
Todas as chamadas usam o header `apikey`. Auto-refresh configurável (10/15/30/60s).
Chart.js via CDN (cdnjs).

Acesso direto: `https://<seu-dominio>/dashboard`.

**Exibição dentro da aba "Dashboard" do manager:** como só temos o build do front
(sem fonte React), `manager/dist/index.html` recebeu um `<script>` que detecta o
placeholder "Dashboard content will be implemented here..." e o substitui por um
`<iframe src="/dashboard?embed=1">` (via MutationObserver, sem tocar no JS
minificado). O `?embed=1` esconde o cabeçalho próprio do dashboard pra ficar nativo.
Se o EvolutionGO mudar o texto do placeholder ou publicar um novo `manager/dist`,
reaplicar o script e/ou ajustar a `PHRASE`.

**Seção "Salvar contato" no modal de teste:** o `index.html` também injeta (via
segundo `<script>`) um bloco `POST /user/savecontact` dentro do modal "Testar
mensagens" do manager (aberto pelo ícone de frasco no card da instância). Detecta
o modal pelo `<h2>` "Testar mensagens", insere o bloco acima do rodapé e usa o campo
"Número de destino" já existente como contato. A **apikey da instância é detectada
automaticamente**: um patch em `window.fetch` captura o `token` de cada instância do
`/instance/all` (e o header `apikey` das chamadas `/send`/`/user`); o token certo é
escolhido pelo nome da instância no título do modal. Faz `fetch('/user/savecontact')`
same-origin.
Não toca no React minificado — se o SPA re-renderizar o modal, o MutationObserver
reinjeta. Ao publicar novo `manager/dist`, reaplicar.

### Fase 2 — endpoint `GET /server/stats` (métricas de sistema + mensagens)

**Arquivos:**
- `pkg/server/handler/server_handler.go` (`Stats`, lê `/proc/meminfo` e `/proc/loadavg`)
- `pkg/message/repository/message_repository.go` (`GetStats`, tipos `MessageStats`/`StatKV`)
- `pkg/routes/routes.go` (rota `GET /server/stats`, auth `AuthAdmin`)
- `cmd/evolution-go/main.go` (injeta `messageRepository` no `NewServerHandler`)

Retorna JSON `{ system, messages }`:
- **system**: `goroutines`, `numCpu`, `goVersion`, `uptimeSeconds`, `memAllocMB`,
  `memSysMB`, `heapInuseMB`, `numGC` (runtime Go) + `loadAvg1/5/15` (`/proc/loadavg`)
  + `hostMemTotalMB`/`hostMemAvailableMB`/`hostMemUsedPct` (`/proc/meminfo`). As
  leituras de `/proc` são Linux-only e degradam graciosamente se ausentes.
- **messages**: `total`, `byStatus` (ex.: Received/Read), `byDay` (últimos 14 dias)
  e `topSources` (top 8 contatos por volume), via `GetStats`.

O dashboard consome isso: cards de RAM/Load/Goroutines/Uptime/Mensagens, gráfico
"Mensagens por dia" e lista "Conversas mais ativas".

**Limitações (sem mudança de schema):** a tabela `message` não tem `instance_id`
nem direção; e só mensagens **recebidas** são persistidas hoje (`Status="Received"`,
atualizado para `Read` em receipts). Logo, não há "por instância" nem "enviadas vs
recebidas". Também depende de `DATABASE_SAVE_MESSAGES=true` — sem isso a tabela fica
vazia e os widgets de mensagem mostram zero. Para evoluir: adicionar `instance_id`
+ direção ao modelo + migração e capturá-los no `InsertMessage`.

**Ao fazer upgrade:** confirmar `NewServerHandler(messageRepository)` no `main.go`,
a rota `GET /server/stats` com `AuthAdmin`, e que `runtime.ReadMemStats` continua
disponível (stdlib — estável).

**Ao fazer upgrade:** a rota é independente; só garantir que `manager/dist/` continua
sendo copiado no Dockerfile (`COPY --from=build /build/manager/dist ./manager/dist`)
e que o `dashboard.html` não seja sobrescrito por um rebuild do front oficial.

---

## 6. Catálogo de produtos (WhatsApp Business) — `POST/GET/DELETE /catalog/...`

**Arquivos:**
- `pkg/user/service/catalog.go` (`CreateProduct`, `GetCatalog`, `DeleteProducts`, upload manual)
- `pkg/user/handler/catalog_handler.go` (handlers)
- `pkg/user/service/user_service.go` e `pkg/user/handler/user_handler.go` (métodos nas interfaces)
- `pkg/routes/routes.go` (grupo `/catalog`, auth por token da instância)

**Origem:** engenharia reversa dos IQs `w:biz:catalog` do Baileys
(`src/Socket/business.ts`, `src/Utils/business.ts`), reproduzidos no Go via
`client.DangerousInternals().SendIQ(...)` + `waBinary.Node`.

**Endpoints:**
- `POST /catalog/product` — cria produto (`product_catalog_add`). Body:
  `{ name, description?, price (miliunidades: R$10,00=10000), currency, retailerId?,
  imageBase64? | imageUrl?, isHidden? }`.
- `GET /catalog/products` — lista (`product_catalog` get).
- `DELETE /catalog/product` — remove (`product_catalog_delete`). Body: `{ productIds: [...] }`.

**Upload de imagem (o ponto não-trivial):** o whatsmeow **não** tem MediaType para
catálogo. No Baileys, `product-catalog-image` sobe em `/product/image` **sem
criptografia** (HKDF vazio). Então o upload é feito à mão em `uploadCatalogImage`:
`DangerousInternals().RefreshMediaConn` pega `auth`+`host`, e um `POST` direto envia
os bytes crus; a resposta traz `direct_path`, que vira a URL referenciada no node
`<media><image><url>`. URLs `*.whatsapp.net` são reusadas sem re-upload.

**Requisitos:** conta **Business** com catálogo habilitado; instância conectada.

**Chaves da API do whatsmeow usadas (validar em upgrades):**
`client.DangerousInternals()` → `SendIQ`, `RefreshMediaConn`;
`whatsmeow.DangerousInfoQuery`/`DangerousInfoQueryType`; `types.ServerJID`;
`waBinary.Node`/`Attrs`; `client.Store.ID`. São APIs "Dangerous" (internas expostas)
— podem mudar entre versões; se o build quebrar aqui, reconferir `internals.go`.

### ⚠️ STATUS: INATIVO — o namespace `w:biz:catalog` não responde mais

**Não remova sem ler.** O código está completo e compila, mas **nenhum** dos IQs
funciona: o WhatsApp não responde nada (timeout de 75s), inclusive no
`GET /catalog/products`, que é o IQ mais simples (read-only) e idêntico ao Baileys.

Evidências levantadas em 18/07/2026:
1. O read-only também dá timeout, com produto existente no catálogo — logo **não é**
   o formato do node de produto nem o campo de país (`compliance_info`).
2. Timeout (silêncio) e não `<iq type="error">` = o servidor **descarta** o stanza,
   comportamento típico de namespace desativado.
3. O **evolution-api (Baileys)** teve rotas `/chat/fetchCatalogs` e
   `/chat/fetchCollections` no changelog e **as removeu**: não existem mais no
   `src/api/routes/chat.router.ts` do main, e na v2.3.7 retornam 404. Ou seja, o
   próprio projeto Baileys-based abandonou o recurso.
4. Demais IQs da instância funcionam normalmente (mensagens, contatos, grupos),
   então não é problema de sessão/socket.

**Conclusão:** a Meta migrou o catálogo para a **API MEX (GraphQL)**. Reimplementar
exigiria `client.DangerousInternals().SendMexIQ(ctx, queryID, variables)` com os
`query_id`/`doc_id` extraídos do bundle do WhatsApp Web — esforço alto e frágil
(quebra a cada rotação de IDs da Meta). Mantido como referência caso a Meta reative
o namespace ou caso se decida partir pro MEX.

**Alternativa em uso:** ver seção 7 (envio de card de produto), que funciona porque
é mensagem, não IQ de negócio.

---

## 7. Envio de card de produto (`POST /send/product`)

**Arquivos:**
- `pkg/sendMessage/service/send_product.go` (`SendProduct`, `ProductStruct`)
- `pkg/sendMessage/handler/send_product_handler.go`
- `pkg/sendMessage/service/send_service.go` e `handler/send_handler.go` (interfaces)
- `pkg/routes/routes.go` (rota `POST /send/product`)

**Por que funciona (e o catálogo não):** aqui é `waE2E.ProductMessage` — uma
**mensagem**, que trafega pelo mesmo caminho de texto/mídia — e não um IQ
`w:biz:catalog`. A imagem usa o upload padrão (`client.Upload` + `MediaImage`),
plenamente suportado pelo whatsmeow.

**Contrato:**
```
POST /send/product
Header: apikey: <token da instância>
{ "number", "productId", "title", "price" (miliunidades: R$10,00=10000),
  "currency", "description"?, "retailerId"?, "url"?,
  "imageBase64" | "imageUrl", "businessOwnerJid"? (padrão: JID da instância),
  "body"?, "footer"?, "delay"?, "quoted"? }
```

**Fluxo de uso:** os produtos são criados/gerenciados no **app oficial** ou no
**Meta Commerce Manager** (que sincroniza com o catálogo da conta); a API só dispara
o card na conversa, informando o `productId` do catálogo.

---

## 8. Múltiplos webhooks por instância — fan-out

**Arquivo:** `pkg/events/webhook/webhook_producer.go` (`Produce`, `splitWebhookURLs`)

O modelo `Instance` continua com **um único** campo `Webhook string` (sem migração
de banco). A mudança está só no producer: `Produce` agora chama `splitWebhookURLs`,
que quebra a string em **N URLs** e envia a **mesma** requisição para cada uma
(cada uma com seu próprio retry). É 100% retrocompatível — uma única URL continua
funcionando igual.

Formatos aceitos no campo `Webhook`:
- lista separada por **quebra de linha**, vírgula ou ponto-e-vírgula
  (ex.: `https://a/webhook\nhttps://b/webhook`);
- array JSON (`["https://a/webhook","https://b/webhook"]`).

Duplicatas, entradas vazias e o marcador `disabled` são ignorados. O endpoint que
grava (`POST /instance/connect`, campo `webhookUrl`) não mudou — o manager só passa
a mandar as URLs juntadas por `\n`. O webhook global (`config.WebhookUrl`) segue
sendo enviado em paralelo, como antes.

**UI:** ver seção 9.

---

## 9. Página de Configurações da instância no manager — Proxy + Webhooks

**Arquivo:** `manager/dist/index.html` (3º bloco `<script>` injetado, via DOM —
o manager é build React compilado, então não tocamos no JS minificado).

Roda na rota `/manager/instances/{id}/settings`. Chave admin (apikey global) e
`apiUrl` vêm de `localStorage["evolution-auth"].state`. O `instanceId` sai da URL.

**(1) Card "Configurações de Proxy"** — injetado logo acima do card de webhook
(clona o `className` do card existente para manter a aparência). Lê o proxy atual
de `GET /instance/info/{id}` (campo `proxy`, um JSON `{protocol,host,port,username,
password}`), permite definir (`POST /instance/proxy/{id}`) e remover
(`DELETE /instance/proxy/{id}`). Campos: host, porta, protocolo (http/https/socks5),
usuário e senha (opcionais). Setar/remover já dispara reconexão no backend.

**(2) Múltiplos webhooks** — o campo único "URL do Webhook" é escondido e
substituído por uma lista de N inputs (adicionar/remover). A cada edição, o valor
das linhas é juntado por `\n` e **escrito de volta no input controlado do React**
(via o setter nativo de `value` + disparo do evento `input`), de modo que o botão
**"Salvar Webhook" original** (que já manda `webhookUrl` + eventos + integrações
para `/instance/connect`) continua sendo a única fonte de gravação. O backend faz o
fan-out (seção 8). Não há `sync()` inicial — o valor vindo do React não é
sobrescrito até o usuário editar.

**Rebuild:** ambos exigem novo build da imagem (o `manager/dist` é copiado no
Docker) + deploy no Dokploy, além do backend recompilado por causa da seção 8.

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

# Changelog do fork — ecosb2b/evo-go-v2

Este arquivo registra **apenas as versões deste fork**. O [`CHANGELOG.md`](./CHANGELOG.md)
é mantido pelo upstream (`evolution-foundation/evolution-go`) e não deve ser
editado aqui — editá-lo geraria conflito a cada sincronização.

Imagens: `ghcr.io/ecosb2b/evo-go-v2`

> **Fixe sempre a tag de versão** (`:0.7.3`), nunca `:latest`. A `latest` é
> reescrita a cada push na `main`, então um `docker compose pull` futuro pode
> trocar a imagem embaixo de um servidor em produção sem aviso.

---

## 0.9.1 — 2026-10-29

**Imagem:** `ghcr.io/ecosb2b/evo-go-v2:0.9.1`
**Digest:** `sha256:0bc662bf894732826f326ee3847cd1492e0ebb92e1f5d6eedafa0adb415e5422`
**Commit:** `9a4ff2e`

### Segurança: download de mídia (SSRF)

- `/send/sticker` e `/send/status/media` por URL passam a baixar pelo
  `utils.SafeHTTPClient` (endereço interno recusado na conexão). A 0.8.1
  listava o sticker como protegido, mas ele e o status usavam um cliente HTTP
  comum, sem checagem nenhuma (e o status, sem prazo).
- Download de mídia: no máximo **3** redirects (antes 5) e nunca de `https` para
  `http`.
- Faixas recusadas novas: `::/96` (IPv4-compatível), `64:ff9b:1::/48`,
  `100::/64`, Teredo `2001::/32`, 6to4 `2002::/16` e `fec0::/10`.
- `/send/media` e `/send/status/media` por URL: teto de tamanho
  (`MEDIA_DOWNLOAD_MAX_MB`, padrão 100), conferido no `Content-Length` e nos
  bytes recebidos. Antes não havia teto: uma URL de arquivo enorme esgotava a
  memória. Resposta não 2xx do servidor da mídia agora é erro
  (`media download failed: HTTP status 404`); antes a página de erro era enviada
  como mídia.
- Recusa por endereço interno, redirect ou tamanho responde **422** (antes 500).
- O erro do download não repete mais a URL (podia ter token na query) e começa
  com `media download failed:`.

### Variável nova

| Variável | Padrão | Uso |
|---|---|---|
| `MEDIA_DOWNLOAD_MAX_MB` | `100` | Tamanho máximo (MB) de mídia baixada por URL |

---

## 0.9.0 — 2026-10-17

**Imagem:** `ghcr.io/ecosb2b/evo-go-v2:0.9.0`
**Digest:** `sha256:cdf8737a92f8135535cec745476fb71986b7306e137e6cd4b4a1ecab254841f0`
**Commit:** `10ef339`

Andamento real de cada mensagem enviada: enviada, entregue, lida e reproduzida.

### Recibos de mensagem

- Tabela nova `message_receipts` (criada pela migração no boot): uma linha por
  mensagem enviada, com `sentAt`, `deliveredAt`, `readAt` e `playedAt`. Guarda só
  ids e datas, nunca o conteúdo, e funciona independente de
  `DATABASE_SAVE_MESSAGES`. Cada data é a **primeira** ocorrência: recibos fora
  de ordem não fazem a mensagem voltar de lida para entregue. Grupos ficam de fora.
- Todo `/send/*` grava a mensagem como enviada já na saída.
- Rota nova `POST /message/receipts` `{ "ids": [...] }` (até 200, token da
  instância): `status` (`sent`, `delivered`, `read`, `played` ou `unknown`) e as
  datas de cada id, só da própria instância. Não exige a instância conectada.

### Webhook `Receipt`

- O recibo **Played** (o contato ouviu o áudio ou viu o vídeo) deixou de ser
  descartado: sai com `state: "Played"` (e `"PlayedSelf"` para outro aparelho da
  própria conta).
- Recibos de entrega com vários ids: todos são processados (antes, só o primeiro
  do lote). Recibo repetido (mesmo id e estado) gera um webhook só.

### Variável nova

| Variável | Padrão | Uso |
|---|---|---|
| `MESSAGE_RECEIPTS_RETENTION_DAYS` | `30` | Dias que o andamento fica guardado (`0` = sempre). Limpeza a cada 6 h. |

---

## 0.8.1 — 2026-10-14

**Imagem:** `ghcr.io/ecosb2b/evo-go-v2:0.8.1`
**Digest:** `sha256:41495503fdae84ffef70a7c948cd1e54376967b886f36d6539f195a1d3faed79`
**Commit:** `d5f68ca`

Correções de segurança e de contrato apontadas na validação real do
Mobilium-Frontend-V2.

### Segurança: downloads de URL só para endereço público (SSRF)

- Toda URL que a API baixa por conta de quem chama — mídia em `/send/*`
  (`media`, `sticker`, `link`, `carousel`, `product`, botões com mídia), foto do
  grupo e foto do perfil — passa por `utils.SafeHTTPClient`. Ele recusa
  localhost, redes privadas, link-local (inclusive metadados da nuvem) e faixas
  reservadas **na hora da conexão**: depois do DNS e em cada redirect.
- Os downloads ganharam timeout (120 s; antes não havia limite e um IP que não
  respondia prendia a chamada).
- `POST /user/profilePicture`: limite de 5 MB, prazo de 15 s e `Content-Type`
  de imagem obrigatório.
- **Mudança de comportamento:** URL de mídia na rede interna (ex.:
  `http://minio:9000/...`) passa a falhar. Quem serve mídia de storage interno
  define `ALLOW_PRIVATE_URLS=true` (desliga a checagem).

### Typebot

- `DELETE /typebot/:id`, `DELETE /typebot/sessions/:id` e
  `PUT /typebot/sessions/:id/status` respondem `404` (`typebot not found` /
  `session not found`) quando o item não existe — antes, `200`.

### Variável nova

| Variável | Padrão | Uso |
|---|---|---|
| `ALLOW_PRIVATE_URLS` | `false` | `true` permite baixar mídia de endereço interno |

---

## 0.8.0 — 2026-10-12

**Imagem:** `ghcr.io/ecosb2b/evo-go-v2:0.8.0`
**Digest:** `sha256:68c7a126c59baf848a142fa2447d402aefcf6dbd6e524e983325cb8ddce9d449`
**Commit:** `a47a71e`

Sincronização com o upstream e correções do QR "Não foi possível conectar" e das
desconexões. Lista completa de rotas, campos e eventos para integradores:
[`docs/CHANGELOG-0.8.0-E-CATALOGO-DE-ROTAS.md`](docs/CHANGELOG-0.8.0-E-CATALOGO-DE-ROTAS.md).
Diagnóstico das causas: [`docs/DIAGNOSTICO-QR-E-DESCONEXAO.md`](docs/DIAGNOSTICO-QR-E-DESCONEXAO.md).

### QR e pareamento

- A rotina que troca o QR fica presa à sessão que a criou: não serve mais código
  de uma conexão morta nem derruba a sessão nova quando a antiga termina.
- O QR é guardado com validade: `GET /instance/qr` traz `expiresAt`/`expiresIn` e
  nunca devolve código vencido. Logo depois do `/instance/connect`, espera até
  ~6 s pelo primeiro código.
- Troca da chave do QR pelo WhatsApp durante o pareamento (`RotateADVSecret`) é
  tratada: os códigos pendentes são reescritos.
- `GET /instance/qr` não reinicia mais uma sessão que ainda está abrindo.
- Janela de pareamento de ~160 s: `QRCODE_MAX_COUNT` passa a ter padrão `7`.
- Webhook novo `PairError`; o fim de uma janela de QR sem scan não emite mais
  `LoggedOut`.

### Reconexão

- Ban temporário, sessão substituída e cliente desatualizado ficam registrados em
  `disconnect_reason` e saem por webhook (`TemporaryBan`, `StreamReplaced`,
  `ClientOutdated`).
- O supervisor espera o fim do ban (também no boot) e não briga com uma sessão
  assumida por outro processo.
- Verificação a cada 90 s religa instâncias pareadas que ficaram paradas (requer
  `CONNECT_ON_STARTUP=true`). Depois de 10 falhas seguidas, a espera entre
  tentativas cresce até 5 min.
- Campo novo `needsQr` em `/instance/all` e `/instance/info`.

### Atenção ao atualizar

- **`CONNECT_ON_STARTUP=true`** no ambiente: o padrão é `false`, e sem ele nenhuma
  instância volta sozinha depois de um restart.
- Remova `QRCODE_MAX_COUNT=5` se estiver fixado.
- `PASSKEY_PUBLIC_URL` deve ser a URL pública do EvoProxy (a extensão de passkey
  chama o proxy, que repassa para cá).
- Os nomes certos das variáveis de log são `DEBUG_ENABLED` e `LOG_TYPE`
  (`WADEBUG`/`LOGTYPE`, usados em alguns compose, eram ignorados).
- `/send/*` com a instância desconectada responde `409` na hora (antes `500`
  depois de até ~80 s).

---

## 0.7.6 — 2026-08-20

**Imagem:** `ghcr.io/ecosb2b/evo-go-v2:0.7.6`
**Digest:** `sha256:12bd9a512d9aebd0256775ce5e080eb70da5d79d40beabaf54d8309c04c81321`
**Commit:** `e14612f`

### Renomear instância

```
PUT /instance/name/:instanceId   { "name": "novo nome" }
```

Troca o rótulo da instância sem recriá-la. Autenticação pela Global API Key
(mesmo grupo do create/delete). Nome já usado por outra instância é rejeitado;
renomear para o próprio nome atual é aceito.

O `id` e o `token` não mudam — nenhuma integração precisa ser reconfigurada. O
runtime é sincronizado na hora, então os webhooks passam a enviar o
`instanceName` novo imediatamente, sem reconectar.

**Atenção:** automações que filtram eventos pelo campo `instanceName` do payload
passam a receber o nome novo após o rename. Quem filtra por `instanceId` ou
token não é afetado.

### Manager

O campo "Nome da Instância" na página de configurações virou um input editável —
o botão Salvar aparece quando o valor difere do atual.

---

## 0.7.5 — 2026-08-06

**Imagem:** `ghcr.io/ecosb2b/evo-go-v2:0.7.5`
**Digest:** `sha256:7d664fcff9249d9752324af1b36fbb1867d56bcbbeb97c0681160ff067464683`
**Commit:** `9f9b911`

### Proteções contra laço e flood

Nada na integração agia **durante** um flood: `expire`, `keywordFinish` e
`stopBotFromMe` dependem de inatividade ou de alguém intervir — as duas coisas
que um laço nunca dá. O ativo em risco é o número: o WhatsApp bane por volume e
padrão, e bot respondendo bot produz os dois.

**Guarda de auto-laço.** O remetente é comparado com os JIDs das instâncias
deste servidor. Duas instâncias suas conversando não têm freio natural, e é o
caminho mais rápido para perder um número. A comparação descarta o sufixo de
aparelho: o JID gravado traz `:7@s.whatsapp.net` e o remetente chega sem ele.

**Limite por contato.** 10 mensagens em 60s por padrão, avaliado antes de chamar
o Typebot. Ao estourar, a sessão vai para `paused`.

**Teto de envio por instância.** Balde de fichas, **desligado por padrão**. Cobre
o caso que o limite por contato não pega: cem contatos com nove mensagens cada
não estouram nenhum limite individual, mas produzem novecentos envios em rajada.

**Alerta.** Toda pausa automática emite `TypebotAutoPaused` pelo webhook,
ignorando o filtro de assinaturas — quem não configurou o evento é exatamente
quem precisa saber que algo deu errado.

O caminho do webhook não é afetado: as proteções rodam na goroutine de despacho e
só impedem o bot de responder. As mensagens continuam chegando, e um contato
pausado pode ser roteado para atendimento humano.

### Novo endpoint

```
POST /typebot/changeStatus   { "remoteJid": "...", "status": "paused" }
```

Encerra, pausa ou reabre pelo JID do contato — a chave que uma automação externa
conhece. Responde `404` quando não há sessão, em vez de `200` silencioso.

**Atenção:** `closed` **não** silencia um contato; ele limpa a sessão e a próxima
mensagem recomeça pela saudação. Para parar de responder a alguém, use `paused`.

### Fim de fluxo detectado

A sessão era marcada como `opened` após cada resposta, incondicionalmente, e o
fim do fluxo nunca era percebido. A mensagem seguinte ia para `continueChat` com
um `sessionId` que o Typebot já havia descartado — respondendo vazio, o que
disparava o `unknownMessage` sem motivo.

O campo `input` passou a ser lido: ausente significa que não há mais passos. Um
passo de script também devolve `input` vazio sem ter terminado, então
`expectsDedicatedReply` é verificado junto.

### Configuração

```yaml
TYPEBOT_CONTACT_RATE_LIMIT: "10"     # 0 desliga
TYPEBOT_CONTACT_RATE_WINDOW: "60"
TYPEBOT_SEND_RATE_LIMIT: "0"         # 20 liga o teto por instância
TYPEBOT_SEND_RATE_BURST: "20"
```

Valor inválido não impede o boot: cai no padrão com aviso no log. As colunas
novas da sessão são criadas pelo `AutoMigrate`.

---

## 0.7.4 — 2026-08-06

**Imagem:** `ghcr.io/ecosb2b/evo-go-v2:0.7.4`
**Digest:** `sha256:2388fb3d8f50a12c162b967d67283029c1c351446a3a876ff58fa39b718981e5`
**Commit:** `7b7401e`

### Integração com Typebot

O Evolution GO não tem nenhuma integração de chatbot — ele emite eventos e para
por aí. Esta versão traz a de Typebot, e apenas ela.

Uma mensagem de texto recebida resolve o bot habilitado da instância, abre ou
recupera a sessão daquele contato, e conversa com o Typebot pelo `startChat` /
`continueChat`, respondendo pela própria instância. A sessão expira por
inatividade, encerra por palavra-chave, e `stopBotFromMe` a encerra quando o
operador escreve na conversa — é assim que um humano assume o atendimento.

**Endpoints** (autenticados pelo token da instância):

```
POST   /typebot                      criar
GET    /typebot                      listar
PUT    /typebot/:id                  atualizar
DELETE /typebot/:id                  remover (leva as sessões junto)
GET    /typebot/sessions             listar sessões
PUT    /typebot/sessions/:id/status  pause / close / reopen
DELETE /typebot/sessions/:id         remover sessão
```

**Limitações conhecidas.** Scripts dentro de `clientSideActions` não são
executados: não há motor de JavaScript em Go. Em vez disso o JID chega
decomposto em `prefilledVariables` (`normalizedUserId`, `userPhone`, `userLid`,
`jidType`), que é o que esses scripts normalmente calculam — se o seu fluxo tiver
um bloco de script, remova-o e use essas variáveis. Quando ainda assim houver um,
o log diz exatamente isso em vez de falhar em silêncio.

Não implementados: `debounceTime`, `keepOpen`, bot de fallback e roteamento por
keyword ou regex.

### Gate próprio, sem servidor de licenciamento

O upstream bloqueia todas as rotas com 503 até a licença ser ativada, e o manager
redireciona para `license.evolutionfoundation.com.br` antes mesmo do login. Este
fork passa a decidir isso sozinho, em `pkg/gate` — o pacote `core` **não foi
alterado**, e `EVOLUTION_GATE_MODE=license` devolve o comportamento original sem
mexer em código.

O `core.InitializeRuntime` deixou de ser chamado: além de montar o contexto de
licença, ele contatava o servidor a cada boot. **Nenhuma chamada externa de
licenciamento acontece mais**, nem no boot, nem por heartbeat, nem por rota.

A autenticação por apikey continua intacta — o gate era a trava comercial de
ativação, nunca a camada de autenticação.

### Manager

O fonte do painel passou a ser versionado em `evolution-go-manager/`, o que
tornou possível alterá-lo de verdade. Novidades:

- **Typebot** na página de configurações da instância: formulário e lista de
  sessões com pausar, encerrar e remover
- **Dashboard** deixou de ser um placeholder e passa a exibir o painel real

Três correções, todas da mesma família — contratos e estado que o TypeScript não
verifica em tempo de execução, e por isso invisíveis ao `tsc`:

- o QR não renderizava: o código lia `data.Qrcode` capitalizado enquanto a API
  responde `data.qrcode`
- o QR não se renovava sozinho: dependências instáveis reiniciavam o timer antes
  dele completar
- a conexão era detectada um ciclo atrasada, por ler a lista de instâncias do
  closure após o fetch

### Ao atualizar

```bash
docker compose pull && docker compose up -d
```

Fixe `:0.7.4`. Instâncias em `connected = false` continuam precisando de um
`POST /instance/connect` uma vez — o `ConnectOnStartup` filtra por
`connected = true`.

---

## 0.7.3 — 2026-08-05

**Imagem:** `ghcr.io/ecosb2b/evo-go-v2:0.7.3`
**Digest:** `sha256:0f25d268f39eaf403e555492460b2887940bbd00ec47716fd9bde37b9f9e7ee0`
**Commit:** `a6b0bae`

> A 0.7.3 foi construída duas vezes. A primeira build (commit `7b24690`,
> `sha256:72c7138f…`) foi republicada ao adicionar este changelog, porque o
> `COPY . .` do Dockerfile faz qualquer arquivo do repositório entrar no contexto
> de build. O digest acima é o vigente. O `paths-ignore` adicionado ao workflow
> em `a6b0bae` impede que isso volte a acontecer com commits de documentação.

Primeira release própria do fork. Adota o fork
[samuelpc7/evolution-go](https://github.com/samuelpc7/evolution-go) e fecha os
três problemas de concorrência investigados nesta base.

### Correções de concorrência

- **Vazamento de conexões no PostgreSQL.** `StartClient` criava um
  `sqlstore.Container` novo a cada conexão *e a cada reconexão*. Como
  `sqlstore.New` abre o próprio `*sql.DB` e nenhum era fechado, as conexões
  cresciam linearmente até `FATAL: sorry, too many clients already`. Medido em
  laboratório: 92 containers esgotavam um PostgreSQL com `max_connections=100`.
  Passa a existir um único container compartilhado, com pool limitado (20/5/5min/2min
  no Postgres; `MaxOpenConns(1)` no SQLite). Backport do PR #117 do upstream.

- **Loop de desconexão.** Cada `events.Disconnected` disparava `ReconnectClient`,
  que produzia **dois** clientes para o mesmo device — um pelo canal de kill,
  outro por `StartInstance`. Dois sockets no mesmo device fazem o WhatsApp fechar
  um, gerando novo `Disconnected`: o loop se auto-alimentava, sem backoff.
  Substituído por um supervisor de runtime com token de propriedade, espera pela
  finalização da goroutine anterior, deduplicação de reconexões concorrentes,
  backoff exponencial com jitter (teto de 30s) e sonda de confirmação.

- **Escrita concorrente em mapa.** `clientPointer`, `myClientPointer` e
  `killChannel` eram mapas Go sem sincronização, criados uma vez e compartilhados
  por referência com 11 serviços — lidos por praticamente toda requisição HTTP e
  escritos por goroutines de fundo. Isso produz `fatal error: concurrent map
  writes`, que **não** é panic recuperável: mata o processo e derruba todas as
  instâncias de uma vez. Todos os acessos passaram a ser guardados por
  `ClientMapsMu` (103 pontos, 13 arquivos), com a regra de nunca segurar o lock
  durante operação de canal ou chamada bloqueante.

### PRs do upstream incorporados

#149 (QR de sessão já autenticada) · #135 (escrita concorrente no WebSocket) ·
#150 (history sync) · #34 (LID em grupos) · #100 (participantes em array) ·
#143 (NATS opcional) · #137 (menções) · #120 (foto de perfil e resolução LID→PN) ·
#130 (canonicalização de JID em edição/remoção) · #151 (stickers WebP animados) ·
#122 (edição de mensagens com secret)

### Novidades

- `POST /send/product` — envio de card de produto do catálogo
- `POST /user/savecontact` — grava contato via app state
- `GET /server/stats` — métricas de sistema e de mensagens
- `GET /dashboard` — dashboard self-hosted
- Múltiplos webhooks por instância, com fan-out no producer (retrocompatível, sem migração de banco)
- Botões interativos corrigidos (molde `native_flow`)

### Removidos

- **Endpoints `/catalog/*`.** Os IQs `w:biz:catalog` deixaram de responder (a Meta
  migrou o catálogo para a API MEX/GraphQL). Não era apenas código morto: cada
  chamada prendia a requisição por 75s antes de falhar. `POST /send/product` não
  depende disso e continua funcionando, porque trafega como mensagem e não como
  IQ de negócio.
- Binário compilado de 61 MB que estava versionado na raiz do repositório.

### Dependências

`go.mau.fi/whatsmeow` de `20260630180629` para `20260721154117`. É a mudança de
maior risco desta release, por afetar comportamento de protocolo — e o primeiro
lugar a investigar se algo estranho aparecer em runtime.

### Ao atualizar

```bash
docker compose pull && docker compose up -d
```

Instâncias que já estejam com `connected = false` **não** são reconectadas no
boot: o `ConnectOnStartup` filtra por `connected = true`. Elas precisam de um
`POST /instance/connect` uma vez; depois disso voltam a ser recuperadas
automaticamente.

Vale acompanhar após o deploy:

```sql
SELECT count(*), state FROM pg_stat_activity
WHERE datname = 'evogo_auth' GROUP BY state;
```

---

## Antes da 0.7.3 — sem versionamento próprio

Duas imagens foram publicadas por este fork **reusando a tag `0.7.2`** do
upstream, antes de adotarmos versão própria. Registrado aqui porque a tag
`0.7.2` significa coisas diferentes conforme quando foi puxada:

| Digest | Commit | Conteúdo |
|---|---|---|
| `sha256:a1c744f1…` | `3ffd53d` | correção do vazamento de conexões + workflow apontado para o GHCR |
| `sha256:0c9745eb…` | `e046de5` | acrescenta a primeira correção do loop de desconexão (`EnableAutoReconnect = true`), substituída na 0.7.3 pelo supervisor de runtime |

A tag `0.7.2` no registry aponta hoje para `sha256:0c9745eb…`, e é o alvo de
rollback a partir da 0.7.3.

Note que as tags git `0.6.1`, `0.7.0`, `0.7.1` e `0.7.2` foram herdadas do
upstream no clone e apontam para commits **dele**, não deste fork. A `0.7.3` é a
primeira tag git a apontar para código próprio.

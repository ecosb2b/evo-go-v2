# Changelog do fork — ecosb2b/evo-go-v2

Este arquivo registra **apenas as versões deste fork**. O [`CHANGELOG.md`](./CHANGELOG.md)
é mantido pelo upstream (`evolution-foundation/evolution-go`) e não deve ser
editado aqui — editá-lo geraria conflito a cada sincronização.

Imagens: `ghcr.io/ecosb2b/evo-go-v2`

> **Fixe sempre a tag de versão** (`:0.7.3`), nunca `:latest`. A `latest` é
> reescrita a cada push na `main`, então um `docker compose pull` futuro pode
> trocar a imagem embaixo de um servidor em produção sem aviso.

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

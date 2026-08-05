# Changelog do fork — ecosb2b/evo-go-v2

Este arquivo registra **apenas as versões deste fork**. O [`CHANGELOG.md`](./CHANGELOG.md)
é mantido pelo upstream (`evolution-foundation/evolution-go`) e não deve ser
editado aqui — editá-lo geraria conflito a cada sincronização.

Imagens: `ghcr.io/ecosb2b/evo-go-v2`

> **Fixe sempre a tag de versão** (`:0.7.3`), nunca `:latest`. A `latest` é
> reescrita a cada push na `main`, então um `docker compose pull` futuro pode
> trocar a imagem embaixo de um servidor em produção sem aviso.

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

# Sincronização com o upstream: roteiro de correções

**Objetivo:** trazer ao fork as correções e features da comunidade (`evolution-foundation/evolution-go`) **desde a 0.7.2** que se aplicam a nós, corrigir os bugs que a triagem achou no nosso próprio código e atualizar o whatsmeow.

**Branch:** `feat/upstream-sync-2026-10`, criada a partir de `main@d6bd42e`. Local, sem push até aprovação.

**Abordagem:** onde existe PR da comunidade, **portamos o trecho que resolve, adaptado** ao nosso supervisor de runtime, ao `ClientMapsMu` e ao container compartilhado, e citamos o PR no commit (`[upstream#NNN]`). Os PRs não entram como patch, porque a maioria traz o repositório inteiro no diff. O que já está corrigido no fork **não é refeito**.

**Fonte:** triagem de 12/10/2026 de 41 issues e ~85 PRs úteis (corte em 03/07/2026), cruzados com `main@d6bd42e`.

## Restrições globais

- Sem Go local. Compilação: `docker build --target build .`. Testes: alvo `test` do Dockerfile (Fase 0).
- Não alterar `pkg/core` nem `CHANGELOG.md`. As releases vão em `CHANGELOG-FORK.md`.
- Todo acesso a `clientPointer`/`myClientPointer`/`killChannel` passa por `ClientMapsMu`, e o lock nunca fica preso durante operação de canal ou chamada bloqueante.
- A reconexão pertence ao supervisor (`runtime_lifecycle.go`). Nenhum caminho novo chama `Disconnect()`/`Connect()` direto no cliente.
- Contrato da API retrocompatível. Quando não for possível, a mudança vai destacada no changelog.
- Um commit por item, com teste quando o item tiver lógica testável.

## Pontos de atenção na revisão

1. Instância deslogada no meio de uma reconexão: para e espera um QR pedido pelo usuário, sem laço de QR.
2. RabbitMQ ou NATS fora do ar: o webhook e o websocket continuam recebendo.
3. Número BR com e sem o 9, DDD ≥ 31: os dois recebem.
4. Token da instância A em rota da instância B: 403.
5. Redeploy com instância em reconexão (`connected=false`, `jid` preenchido): volta sozinha.

---

## Fase 0 — Infraestrutura de teste
| # | Item |
|---|---|
| 0.1 | Alvo `test` no Dockerfile (`go test ./pkg/...` com CGO) |
| 0.2 | Versionar `pkg/whatsmeow/service/pg_version_compat_test.go`, que está fora do controle de versão (se passar) |

## Fase 1 — Segurança e perda silenciosa de mensagens
| # | Problema | Origem | Arquivos |
|---|---|---|---|
| 1.1 | **IDOR em `advanced-settings`**: o token de uma instância lê e altera outra, e a global key recebe 401 | PR #136 (escopo) | `routes.go:119`, `instance_handler.go:646,676` |
| 1.2 | `Disconnect` apaga `Events` (os webhooks param), e o Save da linha inteira reseta settings | PR #187, #111 | `instance_service.go:335,363`, `instance_repository.go` |
| 1.3 | Falha em um produtor (Rabbit, NATS ou websocket) dá `return` e pula os seguintes, inclusive o webhook | PR #181 (parte), nosso | `whatsmeow.go:~2496-2539` |
| 1.4 | Mensagem indecifrável descartada (lead perdido). O ramo 66/67 contorna o supervisor | PR #156, nosso | `whatsmeow.go:~424, 2233-2258` |
| 1.5 | Boot restaura só `connected=true` | PR #154 (parte) | `instance_repository.go:140-157`, `whatsmeow.go:2635` |
| 1.6 | Envio para instância deslogada leva cerca de 80 s e reinicia instância sem pareamento | #111, nosso | `send_service.go:386-466, 612-650` |
| 1.7 | 9º dígito BR removido antes da checagem | #214 | `utils.go:181`, `jid_validation_middleware.go:375,411`, `send_service.go:541-557` |

## Fase 2 — whatsmeow e ciclo de vida
| # | Problema | Origem | Arquivos |
|---|---|---|---|
| 2.1 | Bump do whatsmeow + Go 1.26; `SetStatusMessage`/`GetBlocklist`; `/user/blocklist` mantém o formato `jids` | PR #190, #211 | `go.mod`, `Dockerfile`, `update-whatsmeow.sh`, `user_service.go:628,683` |
| 2.2 | `store.SetWAVersion` (405 "client outdated") | PR #199 (parte) | `whatsmeow.go:426-451` |
| 2.3 | Supervisor: laço de QR após logout; desiste da instância em erro de banco; `/connect` não revive cliente morto; `GetQr` fica preso | ideia do PR #197, #185, nosso | `runtime_lifecycle.go:169-222`, `instance_service.go:256-297` |
| 2.4 | Presença rouba o `killChannel` e duplica a cada PushNameSetting; pulso a cada ~20 h quando `alwaysOnline=false` | nosso, PR #213 | `whatsmeow.go:700-760, 947-1023` |
| 2.5 | Webhook `LoggedOut` falso a cada restart | nosso | `whatsmeow.go:668-698`, `runtime_lifecycle.go:64-75` |
| 2.6 | Nenhuma goroutine nossa tem `recover()` | #188, nosso | `pkg/whatsmeow/service/` |
| 2.7 | Tratar `AppStateSyncError` sem pedir recovery em erro de rede | PR #144 | `whatsmeow.go` |
| 2.8 | `StreamError` sem diagnóstico | #211 | `whatsmeow.go` |
| 2.9 | Escrita sem lock em `myClient.Instance` e `subscriptions` | nosso | `whatsmeow.go:2912,2973` |

## Fase 3 — Envio de mensagens
| # | Problema | Origem | Arquivos |
|---|---|---|---|
| 3.1 | `quoted` vira card vazio; participant recebe o MessageID; `ContextInfo` é zerado | #189 | `send_service.go` |
| 3.2 | Lista dá 405 (`ListMessage` top-level, sem wrapper e sem nó `bot`) | #170, #204 | `send_service.go:2348-2400` |
| 3.3 | `/user/profileName` trava ~75 s | #176 | `user_service.go:669` |
| 3.4 | `viewOnce` em mídia | PR #147 | `send_service.go:111` |
| 3.5 | Menções somem em Interactive, List e ViewOnce | resto do PR #137 | `mention.go` |
| 3.6 | `/send/event` sai como `type=text`: conferir no código e corrigir o tipo do stanza (validação final em aparelho real fica com o usuário) | PR #90 | `send_event.go` |

## Fase 4 — Eventos e endpoints
| # | Problema | Origem | Arquivos |
|---|---|---|---|
| 4.1 | Picture, UserAbout, ButtonClick, Passkey* e OfflineSyncCompleted não chegam ao NATS, ao AMQP ou ao webhook | #193, #105 | `whatsmeow.go:2369-2806`, `event_types.go` |
| 4.2 | `ALL` expandido no connect | #105 | `connect_settings.go:23` |
| 4.3 | Websocket aceita um só assinante e escreve segurando o RLock | PR #181 | `websocket_producer.go` |
| 4.4 | `POST /message/subscribe` (SubscribePresence) | #146, PR #152 | `pkg/message` |
| 4.5 | `POST /user/lid` (LID → número) | PR #179 | `pkg/user` |

## Fase 5 — Manager e passkey
| # | Problema | Origem | Arquivos |
|---|---|---|---|
| 5.1 | Sidebar some no mobile; botões invisíveis continuam clicáveis | PR #184 | `evolution-go-manager/src` |
| 5.2 | Cerimônia de passkey velha continua viva após restart | #107 | `whatsmeow.go:1107` |
| 5.3 | Passkey helper falha com 1Password | #173 | `passkey-helper/` |

## Fora de escopo, por decisão do usuário (12/10)
#217 (áudio com `ptt:false`), #123 (status do proxy), #103/PR #207 (link preview), #104/PR #212 (largura e altura da imagem), #124/#115 (erro 463), #148, #172/#171, #116/#108, tudo o que é anterior a 03/07, e tudo o que já está corrigido no fork (vazamento de pool, mapas, Archive, websocket, participantes, menções básicas, settings de grupo etc.).

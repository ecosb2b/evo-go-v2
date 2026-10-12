# Diagnóstico — QR "Não foi possível conectar" e desconexões

12/10/2026 · Análise de código, sem alteração e sem teste contra o WhatsApp real.

Cobre a Evolution GO em produção (**0.7.6**, branch `main`), a próxima versão (**0.8.0**, branch `feat/upstream-sync-2026-10`, ainda não testada), a Evolution GO original (`upstream/main`, 0.7.2), o **evoproxy-backend** e o frontend **Mobilium-Frontend-V2** (`whattsapp-card.tsx`).

## Resumo

- **O "Não foi possível conectar" acontece na produção por defeito da EvGo, não do proxy.** A rotina que gera o QR a cada 20s não está presa à sessão que a criou. Quando o socket do QR cai, ou a sessão é reiniciada no meio do pareamento, essa rotina continua gravando no banco QRs de um socket morto. O `GET /instance/qr` devolve esses QRs e o celular recusa o scan. No fim da janela, a rotina ainda pode derrubar a sessão nova. A Evolution GO original tem o mesmo defeito.
- **A 0.8.0 herda tudo isso e acrescenta mais uma causa:** a biblioteca nova do WhatsApp pode trocar a chave do QR no meio do pareamento, e a EvGo ignora o aviso. Precisa ser corrigido antes de lançar.
- **Desconexões:** a nossa EvGo já reconecta sozinha nas quedas comuns e é bem melhor que a original, mas tem lacunas (ban, sessão substituída, instâncias paradas sem verificação periódica). As regras de 14 e 30 dias do WhatsApp não têm solução no servidor: só um QR novo resolve, então o que dá para fazer é detectar e avisar cedo.
- **Proxy:** marca "só reconectar" quando o celular deslogou, e o usuário fica preso sem QR.
- **Frontend:** mostra "QR expirado" por volta de 30s, com o primeiro QR ainda válido até 60s. Na 0.8.0 também aciona o reinício indevido da sessão.

---

## 1. QR code: por que dá "Não foi possível conectar"

### Como o QR funciona hoje (0.7.6 e 0.8.0)

1. `POST /instance/connect` inicia a sessão em segundo plano.
2. O WhatsApp entrega um lote de códigos. A função `handleQRCodes` (`pkg/whatsmeow/service/whatsmeow.go`) abre uma rotina que, para cada código, grava `"<imagem>|<código>"` na coluna `instance.qrcode`, emite o webhook `QRCode` e espera 60s (primeiro código) ou 20s (demais).
3. `GET /instance/qr` **não gera nada**: lê a coluna do banco.
4. No fim dos códigos, `teardownQR` limpa a coluna, emite `QRTimeout` e manda o sinal de encerramento pelo canal `killChannel[ID da instância]`.

### Causa A — QR de socket morto servido pela API (produção, alta confiança)

- A rotina de `handleQRCodes` só para quando a sessão foi pareada (`Store.ID != nil`) ou há passkey em andamento. **Ela não verifica se o socket ainda está conectado.**
- Se o socket cai no meio do QR (oscilação de rede, servidor do WhatsApp fecha a conexão), a rotina segue gravando no banco, a cada 20s, códigos daquele socket morto, até o fim da janela.
- Na 0.7.6, `GetQr` (`pkg/instance/service/instance_service.go`) vê "cliente existe, não conectado" e chama `StartInstance`, que **não faz nada** porque a sessão continua reservada. Em seguida devolve o que está no banco: o QR morto.
- Para uma instância nova (sem número gravado), o supervisor não reconecta (`ScheduleReconnect` só age se houver `Jid`). Nada recupera a sessão até o fim da janela.
- O frontend vê um código diferente do anterior, entende que é um QR novo e mostra. O scan falha com "Não foi possível conectar".

### Causa B — Rotina órfã derruba a sessão nova (produção e 0.8.0, alta confiança)

- Quando a sessão de QR é reiniciada (supervisor, `logout`, `reconnect`, ou o `GET /qr` na 0.8.0), a rotina da sessão anterior **não é cancelada**.
- Ela continua sobrescrevendo a coluna com códigos mortos por cima dos códigos da sessão nova.
- No fim, `teardownQR` manda o sinal por `killChannel[ID da instância]`. Esse mapa é compartilhado e indexado pelo ID da instância, então o sinal **encerra a sessão nova**, possivelmente com o usuário escaneando.
- **Gatilho típico na produção:** instância repareada ou deslogada pelo celular **mantém o número antigo** (`Jid`) gravado. Uma oscilação durante o QR gera um `Disconnected`, e o supervisor, que só olha se existe `Jid`, reinicia a sessão. Nasce uma sessão nova com QR novo, e a rotina antiga fica viva.

### Causa C — QR antigo não é apagado ao reiniciar (produção e 0.8.0, média confiança)

O encerramento da sessão (`StartClient` ao receber o sinal) e o `ClearInstanceCache` não limpam a coluna `qrcode`. Logo depois de um reinício (inclusive restart do container), o `GET /qr` pode devolver o QR da sessão anterior antes de a nova gerar o próprio.

### Causa D — Só na 0.8.0: troca da chave do QR ignorada (alta confiança no mecanismo)

- A 0.8.0 atualizou a biblioteca whatsmeow (`go.mod`: `c386243a72ba`, de 07/10/2026; a produção usa `8b4a8ba0d318`, de 21/07/2026).
- A versão nova suporta `companion_reg_refresh`: o WhatsApp pode trocar a chave usada no QR durante o pareamento, e a biblioteca emite `events.RotateADVSecret`.
- A EvGo não trata esse evento (cai em "Unhandled event"). Os códigos no banco e no webhook continuam com a chave antiga, e **todo scan seguinte falha**.
- O canal de QR nativo da biblioteca (`qrchan.go`) já faz a troca; a EvGo usa um caminho próprio e não faz.
- Não se sabe com que frequência o WhatsApp manda essa troca (o recurso foi marcado como "não testado" pelo autor da biblioteca).

### Causa E — Só na 0.8.0: `GET /qr` reinicia a sessão enquanto o socket ainda abre (média-alta confiança)

- A 0.8.0 passou a derrubar e recriar a sessão no `GET /qr` quando ela "existe, mas não está conectada e não tem pareamento" (`GetQr`, trecho do `ClearInstanceCache`).
- Logo após o `connect`, o socket ainda está abrindo e cai exatamente nessa condição. Cada recriação gera um aparelho novo (chaves novas) e pode deixar uma rotina órfã (causa B).
- O nosso frontend pede o QR **imediatamente** após o `connect`; o painel da própria EvGo espera 2s.

### Outras causas possíveis (baixa confiança)

- **Conta que exige passkey:** sem a extensão Evolution Passkey Helper e sem `PASSKEY_PUBLIC_URL` configurada, o pareamento não termina.
- **Erro durante o pareamento** (verificação da chave, falha ao gravar o aparelho no Postgres): a biblioteca emite `events.PairError`, que a EvGo não trata nem avisa.
- **Scan no último instante da janela:** o encerramento pode derrubar o socket no meio do handshake.

### O que **não** é causa

- O `POST /instance/connect` que o frontend repete a cada 20s: durante o QR (sem pareamento), ele só atualiza configurações e não reinicia a sessão (`Connect`, ramo "settings updated without restarting client").
- O evoproxy: o `GET /instances/:id/qr` só repassa a resposta da EvGo.
- Versão da biblioteca ou identificação do navegador: nenhuma das duas é antiga a ponto de o WhatsApp recusar.

### Como confirmar nos logs da EvGo

| Causa | O que procurar |
|---|---|
| A | `Client exists but not connected, checking for existing QR code` seguido de novos `QR code generated #N` sem `Connected` |
| B | Duas sequências `QR code generated #1` intercaladas na mesma instância; `QR timeout — signaling kill channel` logo após um reinício |
| D | `Unhandled event *events.RotateADVSecret`; `Invalid HMAC from pair success message` |
| E | `Client exists but not connected/logged, restarting for QR code` logo após `Starting new client instance` |
| Erro de pareamento | `Failed to pair device`; `Unhandled event *events.PairError` |
| Passkey | `Passkey required` |

---

## 2. Validade do QR

| | WhatsApp | Nossa EvGo hoje (0.7.6 e 0.8.0) |
|---|---|---|
| 1º código | 60s | 60s |
| Códigos seguintes | 20s cada | 20s cada |
| Janela total | ~160s (6 códigos) | **~120s (4 códigos)**: o padrão `QRCODE_MAX_COUNT=5` encerra no 5º |

- O `GET /qr` devolve o código da janela corrente, que pode ter só alguns segundos de validade restante. A resposta não informa quando o código vence.
- O código numérico (`/instance/pair`) morre junto com a janela de QR: ~120s contados do **primeiro QR**, não de quando o código foi pedido.
- Quando o celular recebe o código numérico, os QRs exibidos ao lado deixam de valer.
- A FAQ da wiki (`docs/wiki/referencia/faq.md`) fala em "~40s", o que está errado.

### Frontend (Mobilium-Frontend-V2, `whattsapp-card.tsx` e `constants/timing.ts`)

- `WHATSAPP_QR_TTL_MS = 20_000` vale para todos os códigos, inclusive o primeiro (60s). Aos 20s, o card tenta renovar (`connect` + `GET /qr`), recebe **o mesmo código** (ainda válido), conta como falha e, com `WHATSAPP_QR_MAX_REFRESH_FAILS = 3` a cada 5s, mostra **"QR expirado" por volta de 30s**, com o QR ainda bom por mais 30s.
- `handlePairQr` chama `connect` (ou `repair`) e pede o QR em seguida, sem espera. Na 0.8.0 isso aciona a causa E.
- A janela de polling (`48 × 2,5s = 120s`) bate com a janela atual da EvGo; se a EvGo passar a usar 160s, o frontend precisa acompanhar.

---

## 3. Desconexões

### O que a nossa EvGo faz hoje (0.7.6 e 0.8.0, praticamente igual)

- A reconexão automática da biblioteca (`EnableAutoReconnect`) está **desligada**. Quem reconecta é um supervisor próprio (`ScheduleReconnect`, `runtime_lifecycle.go`), com espera crescente (1s, 2s, 4s… até 30s) e verificação de 30s depois de cada tentativa, sem limite de tentativas.
- Conexão morta em silêncio é detectada no 3º keepalive perdido (~1 a 1,5 min) e reconectada.

| Situação | Volta sozinha? | O que precisa |
|---|---|---|
| Queda de rede, servidor fechou a conexão, erro de stream | **Sim** | — |
| Conexão morta em silêncio | **Sim** (~1-1,5 min) | — |
| Logout pelo celular, aparelho removido, regra de 14/30 dias (`401:`, `403:`, `406:`) | Não | **QR novo** |
| `disconnect` ou `logout` pela API | Não (de propósito) | `connect` / QR |
| Outro processo conectou com a mesma sessão (`StreamReplaced`) | Não | `connect` |
| Falha de conexão não tratada (`409: client user agent was rejected` etc.) | Não | `connect` |
| Ban temporário (`TemporaryBan`) | Ver problema abaixo | Esperar o ban |
| Cliente desatualizado (`ClientOutdated`, 405) | Não | Restart do container |
| Restart do container | **Sim**, inclusive as que estavam reconectando (0.8.0) | — |

### Problemas encontrados

1. **O supervisor insiste durante ban.** Ban, `ClientOutdated` e falhas de conexão não gravam motivo no banco. Se acontecem dentro de um ciclo do supervisor, ele tenta de novo a cada ~30-70s enquanto durar o ban, o que pode agravá-lo. Fora do supervisor, o contrário: ninguém religa.
2. **Sem verificação periódica.** Nada encontra instâncias paradas fora do supervisor (sessão substituída, falha ao iniciar a sessão, ban terminado).
3. **Sinalização incompleta.** Ban, sessão substituída e 405 não aparecem em `disconnect_reason` nem em webhook. O `LoggedOut` real sai até 3 vezes. O fim de uma janela de QR sem scan emite `LoggedOut` (enganoso).
4. **Não existe campo "pareada / precisa de QR".** O número (`jid`) continua gravado depois do logout pelo celular; só o prefixo do `disconnect_reason` diferencia.
5. **O pulso de presença da 0.8.0** (fica online alguns segundos a cada ~20h com `alwaysOnline=false`) zera a cada reconexão. Uma instância que reconecta com intervalo menor que 20h nunca dá o pulso.
6. **Só na produção:** o supervisor reinicia instâncias que têm número gravado mesmo sem aparelho (após logout pelo celular), o que gera sessões de QR que ninguém está vendo e alimenta a causa B. A 0.8.0 já verifica o aparelho (`decideReconnect` + `hasDeviceStore`).

### Comparação com a Evolution GO original (0.7.2)

A original é pior em todos esses pontos. Sem supervisor, ela tenta reconectar uma vez e, se falhar, fica parada. Não detecta conexão morta em silêncio (aparece "conectada" para sempre). No boot, só religa as que estavam conectadas. O `disconnect` pela API apaga os eventos assinados. E reinicia a sessão sempre que ela é encerrada, o que gera ciclos de QR depois de um logout. Nada que valha copiar dela nesses pontos.

### As regras de 14 e 30 dias do WhatsApp

- **14 dias sem usar o celular principal:** depende do celular. **Nenhum servidor consegue evitar.**
- **30 dias sem atividade do aparelho vinculado:** o WhatsApp não documenta o que conta como atividade. Enviar e receber mensagens é o sinal mais provável. Não há confirmação de que o pulso de presença ou um login contem.
- Quando o WhatsApp remove o aparelho por essas regras, **só um QR novo resolve**. A reconexão automática não ajuda nesse caso.
- O que dá para fazer: avisar quando a instância cai por logout (precisa de QR) e, como heurística, avisar quando o celular principal está sem uso há ~10 dias.

### evoproxy-backend

- `precisaParear` é calculado como "não conectada **e** sem número" (`src/lib/instanceLive.js`, `connectionStatus`). Depois de um logout pelo celular o número continua gravado, então o proxy responde `temDevice: true, precisaParear: false`. O frontend mostra "Reconectar", o `connect` abre uma sessão de QR que ninguém vê, e o usuário fica preso.
- O proxy não recebe webhooks da EvGo nem lê `disconnect_reason`; não há alerta de desconexão.

---

## 4. Plano de correção (não aplicado)

### EvGo — QR (produção e 0.8.0)

| # | Correção | Resolve |
|---|---|---|
| Q1 | Amarrar a rotina de QR à sessão que a criou: um contexto cancelado no encerramento de `StartClient`; sair também quando `!WAClient.IsConnected()`; trocar `time.Sleep` por `select` com o cancelamento | A, B |
| Q2 | `teardownQR` sinaliza o canal de encerramento **da própria sessão** (guardado no `MyClient`), nunca o do mapa por ID | B |
| Q3 | Limpar a coluna `qrcode` ao encerrar a sessão, em `ClearInstanceCache` e ao iniciar uma sessão nova sem aparelho, antes do `Connect` | C |
| Q4 | Gravar o QR com horário de expiração (ou identificador da sessão) e fazer o `GetQr` recusar código vencido ou de outra sessão; devolver o tempo restante na resposta | A, C e o frontend |
| Q5 | **Só 0.8.0:** tratar `events.RotateADVSecret` (trocar a chave nos códigos atual e restantes, regravar, reemitir `QRCode`), como faz `qrchan.go` | D |
| Q6 | **Só 0.8.0:** não reiniciar a sessão no `GetQr` enquanto o `Connect` está em andamento (registrar o cliente no mapa só depois do `Connect`, ou uma marca "conectando") | E |
| Q7 | Tratar `events.PairError` (log + webhook no grupo QRCODE); emitir `QRSuccess`; não emitir `LoggedOut` no fim de uma janela de QR | Diagnóstico |
| Q8 | Opcional: `QRCODE_MAX_COUNT=0` (ou 7) para usar os ~160s inteiros; corrigir a FAQ | Janela curta |

Na produção, Q1 a Q4 resolvem o problema atual. Q5 e Q6 são obrigatórios antes de lançar a 0.8.0.

### EvGo — desconexão

| # | Correção | Resolve |
|---|---|---|
| D1 | Gravar motivo e emitir webhook para `TemporaryBan` (com a expiração), `StreamReplaced` e `ClientOutdated` | Problema 3 |
| D2 | O supervisor respeita esses motivos: espera o ban expirar, tenta devagar no 405, não briga com `StreamReplaced` | Problema 1 |
| D3 | Verificação periódica (1-2 min): instâncias que deveriam estar no ar (mesmo critério do boot + aparelho existente) e não estão → `ScheduleReconnect`; espera crescente até 5-15 min após várias falhas | Problema 2 |
| D4 | Expor `paired` / `needsQr` em `/instance/all` e `/instance/info` | Problema 4 |
| D5 | Só na produção: o supervisor verifica se o aparelho existe antes de reiniciar (já feito na 0.8.0) | Problema 6 |
| D6 | Guardar o horário do último pulso de presença no banco, para reconexões não zerarem o temporizador | Problema 5 |
| D7 | Emitir `LoggedOut` uma vez só | Problema 3 |

**Não recomendado:** religar `EnableAutoReconnect` (competiria com o supervisor) e "atividade artificial" mandando mensagens para o próprio número (aparece no celular e não há garantia de que conte).

### evoproxy-backend

| # | Correção |
|---|---|
| P1 | Calcular `precisaParear` também pelo `disconnect_reason` do `/instance/all`: prefixos `401:`, `403:`, `406:`, `Logged out by API` e `Maximum QR code count` → `precisaParear: true`, `temDevice: false`. Quando a EvGo expuser `needsQr` (D4), usar esse campo. |
| P2 | Repassar `disconnect_reason` (ou uma versão traduzida) na resposta de `GET /instances` e `/status`, para o frontend mostrar o motivo |

### Frontend (Mobilium-Frontend-V2)

| # | Correção |
|---|---|
| F1 | Validade do primeiro QR em 60s e dos seguintes em 20s (ou, com Q4, usar o tempo restante que a API devolver) |
| F2 | Esperar 1-2s entre `connect`/`repair` e o primeiro `GET /qr` |
| F3 | Só mostrar "QR expirado" no fim da janela real (~120s hoje), não após 3 renovações sem código novo |
| F4 | Com P1, mostrar o fluxo de QR (e não "Reconectar") quando o celular deslogou |

### Alertas de desconexão

| # | O quê | Como |
|---|---|---|
| A1 | Fonte do estado | Assinar o grupo `CONNECTION` num webhook do proxy (ou polling de `/instance/all` a cada 1-2 min) |
| A2 | "Precisa de QR" | `LoggedOut` ou `disconnect_reason` com `401:`/`403:`/`406:` → alerta imediato ao responsável da instância |
| A3 | "Desconectada há X min" | `connected=false` por mais de N minutos sem motivo de API → alerta |
| A4 | Ban | Webhook `TemporaryBan` (depois de D1) → alerta com a expiração |
| A5 | Risco dos 14 dias (heurística) | Registrar a última mensagem enviada pelo celular principal (`IsFromMe` vinda do aparelho 0) e avisar ao passar de ~10 dias. É uma estimativa: o WhatsApp não documenta o critério. |
| A6 | Exibição | Badge no painel (frontend/mobpainel) e notificação pelo serviço de notificações existente |

### Ordem sugerida

1. **Produção:** Q1-Q4 + D5 numa 0.7.7; P1 no proxy; F1-F4 no frontend.
2. **0.8.0:** portar o item 1, mais Q5, Q6 e Q7, antes de lançar.
3. **Em seguida:** D1-D4, D6, D7 e os alertas A1-A6.

### Como validar (antes de publicar)

- **Causas A e B:** iniciar um pareamento, derrubar a rede do container por ~10s no meio do QR (`docker network disconnect`/`connect`) e conferir que o `GET /qr` deixa de devolver código do socket morto e que nenhuma rotina antiga derruba a sessão nova.
- **Causa B (gatilho da produção):** instância com número gravado e sem aparelho; forçar um `Disconnected` durante o QR e verificar que não sobram duas sequências de `QR code generated #1`.
- **Causa E:** chamar `connect` e `GET /qr` sem espera, 20 vezes seguidas, e conferir que a sessão não é recriada.
- **Janela:** cronometrar do primeiro QR até o `QRTimeout` com o `QRCODE_MAX_COUNT` escolhido.
- **Frontend:** o primeiro QR fica na tela até ~60s sem "expirado".
- **Proxy:** após logout pelo celular, `/status` responde `precisaParear: true`.

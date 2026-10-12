# Evolution GO 0.8.0 — Changelog e Catálogo de Rotas

12/10/2026 · Ecos

A versão 0.8.0 é compatível com as integrações atuais: nenhuma rota foi removida e nenhum campo deixou de existir. Mudam alguns códigos de erro e o comportamento de eventos — veja o que fazer na primeira seção.

## O que sua aplicação precisa fazer

Nada quebra para quem já integra, mas revise estes 7 pontos antes de atualizar.

1. **Trate HTTP 409 nos envios.** Todo `/send/*` para uma instância desconectada agora responde `409` com `{"error":"... instance is not connected to WhatsApp"}` em menos de 6 segundos. Antes era `500` depois de até \~80 segundos. Se sua aplicação re-tenta todo 500, passe a tratar 409 como "reconecte a instância", sem retry imediato.
2. **`/instance/:instanceId/advanced-settings` agora confere o dono.** O token de uma instância só lê e altera a própria instância (outra → `403`). A Global API Key passou a ser aceita (antes recebia `401`).
3. **Não espere mais `LoggedOut` em reconexões automáticas.** O webhook `LoggedOut` agora só sai em logout real ou desconexão pela API. Se sua aplicação usava esse evento para marcar a instância como caída durante reconexões, use `Disconnected` e `Connected`.
4. **Assinatura `ALL` fica gravada como `ALL`.** Quem assina `ALL` passa a receber também eventos que antes não chegavam (passkey, `OfflineSyncCompleted`, `SendStatus`). Ignore tipos de evento que sua aplicação não conhece, em vez de falhar.
5. **Para a citação mostrar a mensagem original, envie o conteúdo dela.** No objeto `quoted`, mande `text` ou o `message` recebido no webhook. Só com `messageId` a citação funciona, mas o card aparece vazio, como antes.
6. **O `/user/blocklist` ganhou o campo `Items`.** `DHash` e `JIDs` continuam iguais; `Items` traz LID e número separados.
7. **Listas mudaram de formato no envio.** O payload de `/send/list` é o mesmo, mas a mensagem sai em outro formato para voltar a ser entregue. Teste uma lista antes de liberar para os seus clientes.

## Impacto do deploy nas sessões conectadas

Atualizar não desconecta nenhum WhatsApp nem pede QR de novo: as sessões ficam no banco e voltam sozinhas depois do restart.

| Ponto | O que acontece |
| --- | --- |
| Restart do container | Todas as instâncias caem por alguns segundos e reconectam sozinhas, como em qualquer deploy. Mensagens recebidas nesse intervalo chegam na reconexão (fila offline do WhatsApp). |
| Banco de sessões (whatsmeow) | Duas migrações automáticas que só **adicionam colunas com valor padrão** (`companion_meta_nonce`, `wasa_root_secret_id`). Nenhum dado de sessão é alterado ou apagado. |
| Banco da aplicação | Sem mudança de schema. |
| Instâncias que estavam em reconexão | Agora voltam sozinhas no boot. Antes só voltavam as marcadas como conectadas, e as outras precisavam de um `POST /instance/connect`. |
| Instâncias desconectadas pela API | Continuam paradas. |
| Instâncias deslogadas pelo celular | Continuam paradas; precisam de QR novo, como antes. |
| Rollback para 0.7.6 | Seguro: as migrações são marcadas como compatíveis com versões anteriores, e a 0.7.6 roda sobre o banco já migrado. |

Recomendações para o deploy:

- Faça o deploy num horário de pouco tráfego, como em qualquer restart.
- Fixe a tag da imagem (`ghcr.io/ecosb2b/evo-go-v2:0.8.0`), nunca `latest`.
- Depois do deploy, confira `GET /instance/all`: as instâncias devem voltar a `connected: true` em até 1–2 minutos.
- Nova variável `REREQUEST_FROM_PHONE` vem ligada por padrão (pede ao celular mensagens que não puderam ser decifradas). Não é preciso configurar nada.

## Changelog — rotas novas

Duas rotas novas, as duas com o token da instância no header `apikey`.

| Rota | Para que serve | Body | Resposta |
| --- | --- | --- | --- |
| `POST /message/subscribe` | Passar a receber a presença (online / visto por último) de um contato no webhook `Presence`. Vale até a próxima reconexão da instância. | `number` (string, obrigatório): número do contato | `{"message":"success","data":{"subscribed":true,"warning":"..."}}` |
| `POST /user/lid` | Descobrir o número de telefone por trás de um LID (`123...@lid`), formato que o WhatsApp usa cada vez mais para remetentes e participantes de grupo. | `lid` (string, obrigatório): `"123@lid"` ou só os dígitos. `groupJid` (string, opcional): grupo onde o LID aparece, usado para buscar o número se ele ainda não for conhecido | `{"message":"success","data":{"lid":"...@lid","phoneNumber":"5511...","jid":"5511...@s.whatsapp.net"}}` · `404` se o número ainda não é conhecido · `400` se o LID for inválido |

Cuidados:

- **`/message/subscribe`**: o WhatsApp só envia a presença de contatos enquanto a instância está online. Com `alwaysOnline=false` a assinatura é aceita, mas os eventos não chegam, e a resposta traz `warning` explicando isso. A rota não liga a presença sozinha, porque ficar online corta as notificações no celular do dono do número.
- **`/user/lid`**: o WhatsApp não tem consulta de LID para número. O mapeamento só é conhecido depois que chega uma mensagem desse contato ou depois que um grupo com ele é lido. Um `404` significa "ainda não sei", não "não existe".

## Changelog — campos e comportamento das rotas existentes

Nenhum campo foi removido; todos os campos novos são opcionais.

| Rota | Tipo | Campo / comportamento | Explicação |
| --- | --- | --- | --- |
| Todas as `/send/*` (campo `quoted`) | Campo novo | `quoted.text` (string) | Texto da mensagem citada. Faz o card da citação mostrar a prévia e abrir a original. |
| Todas as `/send/*` (campo `quoted`) | Campo novo | `quoted.message` (objeto) | O objeto `message` recebido no webhook da mensagem citada (ex.: `{"conversation":"oi"}` ou `{"imageMessage":{...}}`). Tem prioridade sobre `text`. Botões, listas e carrossel não são aceitos aqui; nesse caso vale o `text`. |
| Todas as `/send/*` (campo `quoted`) | Campo novo | `quoted.key.id`, `quoted.key.participant` | Formato da Evolution API v2, aceito como alternativa a `messageId` e `participant`. |
| `POST /send/media` | Campo novo | `viewOnce` (bool, padrão `false`) | Envia como visualização única (imagem, vídeo, áudio). Documento e figurinha ignoram. No multipart: campo de formulário `viewOnce=true`. |
| `POST /send/media` (multipart) | Campo novo | `quoted.text` | Mesmo significado do JSON. |
| `POST /send/carousel` | Campo novo | `mentionAll` (bool), `mentionedJid` (lista de strings) | Menções no carrossel. Antes não existiam. |
| `POST /send/carousel` | Comportamento | `formatJid` | Era aceito e ignorado; agora é respeitado. |
| `/send/button`, `/send/list`, `/send/carousel`, `/send/product` | Comportamento | `mentionAll`, `mentionedJid` | Eram descartados sem aviso; agora as menções saem. |
| Todas as `/send/*` | Comportamento | resposta de erro | Instância desconectada → `409` imediato (antes `500` após até \~80s). |
| Todas as `/send/*` | Comportamento | `number` (celular BR) | Com `CHECK_USER_EXISTS=true` (padrão), o número é procurado com e sem o 9 depois do DDD numa consulta só, e a mensagem vai para a forma registrada. Antes falhava para quem está registrado com o 9 em DDD ≥ 31. |
| `POST /send/list` | Comportamento | formato de envio | A lista sai como `ListMessage` direta, formato que o WhatsApp volta a entregar. Payload igual. |
| `POST /user/block`, `POST /user/unblock`, `GET /user/blocklist` | Campo novo na resposta | `Items` (lista de `{LID, PN, Active}`) | Cada contato bloqueado com LID e número separados. `DHash` e `JIDs` continuam (em `JIDs` vem o número quando conhecido, senão o LID). |
| `POST /user/profileName` | Comportamento | `name` | Agora altera o nome de fato. Antes travava \~75s e não mudava nada. |
| `GET/PUT /instance/:instanceId/advanced-settings` | Autenticação | `apikey` | Aceita a Global API Key ou o token **da própria** instância; token de outra → `403`. |
| `POST /instance/connect` | Comportamento | `subscribe: ["ALL"]` | Gravado como `ALL` (antes virava a lista de grupos conhecidos). Se o runtime existe com a conexão morta, o `/connect` agora reinicia em vez de só salvar settings. |
| `POST /instance/disconnect` | Comportamento | eventos assinados | Não apaga mais os eventos assinados (antes os webhooks paravam depois do `/reconnect`). `disconnect_reason` = `"Disconnected by API"`. |
| `DELETE /instance/logout` | Comportamento | `disconnect_reason` | Passa a ser `"Logged out by API"`. |
| `GET /instance/qr` | Comportamento | instância pareada reconectando | Responde `"session already logged in"` em vez de derrubar a conexão para gerar QR. |
| `GET /instance/qr` | Campo novo | `expiresAt` (unix), `expiresIn` (segundos) | Quando o código vence. O primeiro vale ~60 s e os seguintes ~20 s. Código vencido não é mais devolvido: a rota espera um código válido (até ~6 s logo depois do `/instance/connect`, que é quando o primeiro código ainda está a caminho; até ~2 s nos demais casos) ou responde `no QR code available`. |
| `GET /ws` (websocket) | Comportamento | conexões por instância | Várias conexões da mesma instância recebem todos os eventos. Antes a última substituía as anteriores. |
| `GET /instance/all`, `GET /instance/info/:instanceId` | Campo novo | `needsQr` (bool) | `true` quando a instância está desconectada e só um QR novo resolve (logout pelo celular, logout pela API, janela de QR encerrada, nunca pareada). `false` quando ela volta sozinha ou só precisa de `/instance/connect`. |
| `GET /instance/all`, `GET /instance/info/:instanceId` | Formato | `qrcode` | O valor gravado agora é `"<imagem>\|<código>\|<expira em unix>"` (o terceiro campo é novo). Para obter o QR, use `GET /instance/qr` (`qrcode`, `code`, `expiresAt`) em vez de separar esta coluna. |

## Changelog — webhooks, eventos e variáveis de ambiente

Os eventos ficaram mais completos. Nenhum campo existente mudou de nome ou formato.

| Evento | Mudança | O que sua aplicação vê |
| --- | --- | --- |
| `Presence` | Campos novos | `from` (JID de quem mudou de estado) e `lastSeen` (Unix em segundos, só quando `state` = `offline` e o contato compartilha o visto por último). `state` continua `online` / `offline`. |
| `LoggedOut` | Menos disparos | Só em logout real ou desconexão pela API. Não sai mais a cada reconexão automática. |
| `Disconnected` | `disconnect_reason` mais detalhado | Quando o WhatsApp fecha a conexão com `stream:error`, o motivo traz o detalhe (ex.: `Disconnected by the server after stream:error code=...`). |
| `PasskeyRequest`, `PasskeyConfirmation`, `PasskeyError` | Passam a ser entregues | Grupo `QRCODE`. Antes não chegavam a nenhum destino. |
| `OfflineSyncCompleted` | Passa a ser entregue | Grupo `CONNECTION`: o WhatsApp terminou de entregar as mensagens que chegaram com a instância fora. |
| `SendStatus` | Passa a ser entregue | Grupo `SEND_MESSAGE`: envio por `/send/status/*`. Antes só chegava a quem assinava `ALL`. |
| `ButtonClick`, `Picture`, `UserAbout` | Filas globais | Agora chegam também nas filas globais do RabbitMQ e do NATS (antes só no webhook). |
| Todos | Entrega independente | Uma falha no RabbitMQ, NATS ou websocket não impede mais o webhook do mesmo evento. |
| `PairError` | Novo | Grupo `QRCODE`: o QR foi escaneado, mas o pareamento falhou (o celular mostra "Não foi possível conectar"). `data.error` traz o motivo. Gere um QR novo. |
| `QRTimeout` / `LoggedOut` | Menos disparos | O fim de uma janela de QR sem scan emite só `QRTimeout`; antes também saía um `LoggedOut` enganoso. |
| `QRCode` | Campo novo | `data.expiresAt` (unix): quando o código vence. Se o WhatsApp trocar a chave durante o pareamento, o código atual é reemitido com a chave nova. |
| `QRTimeout` | Novo motivo | `data.reason = "QR socket closed before pairing"`: a conexão do QR caiu antes do scan; peça um QR novo. |
| `StreamReplaced` | Novo | Grupo `CONNECTION`: outro processo conectou com a mesma sessão. A instância não religa sozinha; `disconnect_reason = "Stream replaced by another connection"`. |
| `ClientOutdated` | Novo | Grupo `CONNECTION`: o WhatsApp recusou a versão do cliente (405). `disconnect_reason = "405: client outdated"`. |
| `TemporaryBan` | Campo novo e registro | `data.expiresAt` (unix). O ban agora fica em `disconnect_reason` (`"Temporary ban until <data>: <motivo>"`) e a reconexão espera o fim do prazo. |

Variáveis de ambiente (para quem opera o servidor):

| Variável | Padrão | Para que serve |
| --- | --- | --- |
| `REREQUEST_FROM_PHONE` | `true` | Mensagem recebida que não pôde ser decifrada é pedida de novo ao celular, em vez de descartada. Comum no primeiro contato de um número novo (lead de anúncio). `false` desliga. |
| `QRCODE_MAX_COUNT` | `7` (antes `5`) | Quantas emissões de QR antes de encerrar a sessão de pareamento. Com 7, a janela vai a ~160 s (antes ~120 s). Valor inválido agora usa o padrão em vez de desligar o limite. Quem fixou `QRCODE_MAX_COUNT=5` no ambiente deve remover a variável ou trocar para `7`, senão continua com a janela de ~120 s. |
| `CONNECT_ON_STARTUP` | `false` | Além de religar as instâncias no boot, agora liga a verificação periódica de reconexão (a cada 90 s), que religa instâncias que caíram e ficaram paradas. Com `false`, essa verificação não roda. |

Comportamento novo de presença: com `alwaysOnline=false`, a instância fica online por alguns segundos a cada \~20 horas. Isso evita que o celular desvincule o aparelho por inatividade, sem cortar as notificações.

## Autenticação

Toda chamada leva a chave no header `apikey`; o tipo de chave depende da rota.

| Tipo | Valor do header `apikey` | Onde é usada | Sem chave ou chave errada |
| --- | --- | --- | --- |
| **Global API Key** | O `GLOBAL_API_KEY` configurado no servidor | Administração: criar, listar, renomear e apagar instâncias, proxy, logs, `/server/stats` | `401` |
| **Token da instância** | O `token` da instância (devolvido no `/instance/create` e no `/instance/all`) | Tudo que age sobre uma instância: conectar, QR, enviar, usuário, grupos, chats, etiquetas, Typebot... A instância é identificada pelo token, não pela URL | `401` |
| **Global ou token da própria instância** | Qualquer um dos dois | `GET/PUT /instance/:instanceId/advanced-settings` | `401`; token de outra instância → `403` |
| **Pública** | Nenhum | `/server/ok`, `/passkey-ceremony/*` (usada pela extensão de passkey), `/manager`, `/swagger` | — |

Nos exemplos do catálogo, `{{global}}` é a Global API Key e `{{token}}` o token da instância. As respostas de sucesso seguem, em geral, o formato `{"message":"success","data":...}`, e os erros `{"error":"..."}`.

## Catálogo — Instância, servidor, websocket, passkey e Typebot

Rotas de ciclo de vida da instância (criar, conectar, QR, desconectar), saúde do servidor, eventos em tempo real e o bot Typebot. Erros seguem o formato `{"error": "mensagem"}`; sem chave ou com chave inválida a resposta é `401 {"error":"not authorized"}`.

### POST /instance/create

**Autenticação:** Global API Key | **Para que serve:** cria (cadastra) uma nova instância de WhatsApp no servidor. Não conecta: depois use `/instance/connect` + `/instance/qr` (ou `/instance/pair`) com o token da instância.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| name | string | Sim | Nome único da instância (se já existir, retorna 500 `instance already exists`). |
| token | string | Sim | Token da instância, usado depois no header `apikey` das rotas da instância. Deve ser único. |
| instanceId | string (UUID) | Não | Id desejado. Se omitido, é gerado um UUID. |
| proxy | objeto | Não | `{protocol?, host, port, username, password}`. Se enviado, `host`, `port`, `username` e `password` são obrigatórios. `protocol` aceita `http`, `https`, `socks5`; se ausente é inferido pela porta (1080, 2080 e 42000–43000 = socks5, demais = http). Se omitido e o servidor tiver proxy global (env), ele é aplicado. |
| advancedSettings | objeto | Não | `{alwaysOnline, rejectCall, msgRejectCall, readMessages, ignoreGroups, ignoreStatus}`; campos omitidos ficam `false`/vazio. |

**Exemplo de requisição:**

```json
{ "name": "loja-centro", "token": "a1b2c3-token-secreto", "advancedSettings": { "rejectCall": true, "msgRejectCall": "Não atendemos ligações" } }
```

**Resposta (200):**

```json
{ "message": "success", "data": { "id": "uuid", "name": "loja-centro", "token": "a1b2c3-token-secreto", "webhook": "", "jid": "", "connected": false, "disconnect_reason": "", "events": "", "alwaysOnline": false, "rejectCall": true, "msgRejectCall": "Não atendemos ligações", "readMessages": false, "ignoreGroups": false, "ignoreStatus": false } }
```

**Erros comuns:** 400 JSON inválido / `name is required` / `token is required` / campo de proxy faltando; 401 chave não é a Global API Key; 500 `instance already exists` ou erro de banco (ex.: token duplicado).

### GET /instance/all

**Autenticação:** Global API Key | **Para que serve:** lista todas as instâncias deste servidor (filtradas pelo `CLIENT_NAME` do servidor), com o status de login real no momento.

**Exemplo de requisição:** `GET /instance/all`

**Resposta (200):**

```json
{ "message": "success", "data": [ { "id": "uuid", "name": "loja-centro", "token": "...", "jid": "5511999999999:12@s.whatsapp.net", "connected": true, "events": "MESSAGE,CONNECTION", "webhook": "https://...", "disconnect_reason": "", "proxy": "" } ] }
```

`connected` reflete se o cliente está logado agora; `proxy` sempre vem vazio (credenciais ocultadas).

**Erros comuns:** 401 chave inválida; 500 erro de banco.

### GET /instance/info/:instanceId

**Autenticação:** Global API Key | **Para que serve:** retorna os dados de uma instância específica (mesmo formato de um item de `/instance/all`).

**Path:** `instanceId` (UUID, obrigatório).

**Resposta (200):**

```json
{ "message": "success", "data": { "id": "uuid", "name": "loja-centro", "connected": true, "jid": "...", "events": "ALL", "proxy": "" } }
```

**Erros comuns:** 400 `instanceId is required`; 401 chave inválida; 500 instância não encontrada (`record not found`) ou erro de banco.

### PUT /instance/name/:instanceId

**Autenticação:** Global API Key | **Para que serve:** renomeia a instância (id e token não mudam). O novo nome sai em `instanceName` nos próximos webhooks sem precisar reconectar.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| name | string | Sim | Novo nome (espaços nas pontas são removidos). Precisa ser único. |

**Exemplo de requisição:** `{ "name": "loja-centro-2" }`

**Resposta (200):** `{ "message": "success", "data": { "id": "uuid", "name": "loja-centro-2", ... } }`

**Erros comuns:** 400 `name is required` / JSON inválido; 401 chave inválida; 500 `an instance with this name already exists` ou instância não encontrada.

### DELETE /instance/delete/:instanceId

**Autenticação:** Global API Key | **Para que serve:** remove a instância definitivamente: faz logout do WhatsApp (se logada), para o runtime e apaga o registro. Será necessário novo pareamento para usar o número de novo.

**Resposta (200):** `{ "message": "success" }`

**Erros comuns:** 400 `instanceId is required`; 401 chave inválida; 500 instância não encontrada ou erro ao apagar.

### POST /instance/proxy/:instanceId

**Autenticação:** Global API Key | **Para que serve:** define ou troca o proxy de saída da instância. A instância é reconectada em segundo plano para aplicar o proxy.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| host | string | Sim | Host do proxy. |
| port | string | Sim | Porta (string, ex. `"1080"`). |
| protocol | string | Não | `http`, `https` ou `socks5`; se omitido é inferido pela porta. |
| username | string | Não | Usuário do proxy. |
| password | string | Não | Senha do proxy. |

**Exemplo de requisição:**

```json
{ "host": "proxy.exemplo.com", "port": "1080", "protocol": "socks5", "username": "user", "password": "pass" }
```

**Resposta (200):** `{ "message": "success", "data": { "protocol": "socks5", "host": "proxy.exemplo.com", "port": "1080", "hasAuth": true } }`

**Erros comuns:** 400 `host is required` / `port is required` / JSON inválido; 401 chave inválida; 500 instância não encontrada ou erro ao salvar.

### DELETE /instance/proxy/:instanceId

**Autenticação:** Global API Key | **Para que serve:** remove o proxy da instância (volta a conectar direto) e reconecta em segundo plano.

**Resposta (200):** `{ "message": "success" }`

**Erros comuns:** 400 `id is required`; 401 chave inválida; 500 instância não encontrada ou erro ao salvar.

### POST /instance/forcereconnect/:instanceId

**Autenticação:** Global API Key | **Para que serve:** recuperação de emergência: grava o JID (número) da instância e reinicia o cliente. Útil quando a instância perdeu o vínculo com o número mas a sessão ainda existe.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| number | string | Sim | Número do WhatsApp da instância (ex. `5511999999999`). |

**Resposta (200):** `{ "message": "success" }`

**Erros comuns:** 400 `number is required`; 401 chave inválida; 500 `client already connected` (já está conectada e logada) ou falha ao reconectar.

### GET /instance/logs/:instanceId

**Autenticação:** Global API Key | **Para que serve:** consulta o log interno da instância (conexão, envios, erros) para diagnóstico.

| Query | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| start\_date | string `YYYY-MM-DD` | Não | Data inicial. Padrão: 7 dias atrás. |
| end\_date | string `YYYY-MM-DD` | Não | Data final (até 23:59:59 UTC). Padrão: hoje. |
| level | string | Não | `INFO`, `ERROR`, `WARN`, `DEBUG` ou vários separados por vírgula. Padrão: todos. |
| limit | int | Não | Máximo de linhas. Padrão: 100. |

**Exemplo de requisição:** `GET /instance/logs/3f2a...-uuid?level=ERROR,WARN&limit=50`

**Resposta (200):** array puro, do mais recente para o mais antigo (vazio `[]` se não houver log):

```json
[ { "timestamp": "2026-10-12T13:00:00Z", "level": "ERROR", "instance_id": "uuid", "message": "[uuid] Failed to send message to webhook: ...", "metadata": {} } ]
```

**Erros comuns:** 400 query inválida; 401 chave inválida; 500 nível inválido, data inicial depois da final ou erro de leitura.

### POST /instance/connect

✏️ **Alterado:** `subscribe: ["ALL"]` agora é gravado como `"ALL"` (antes virava a lista de eventos); se o runtime existir com a conexão morta, o `/connect` reinicia a conexão (antes não fazia nada).

**Autenticação:** Token da instância | **Para que serve:** inicia o cliente WhatsApp da instância e configura para onde os eventos vão (webhook, RabbitMQ, NATS, websocket) e quais eventos assinar. É chamado antes de pegar o QR, e pode ser chamado de novo a qualquer momento só para atualizar configurações (atualização parcial).

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| webhookUrl | string | Não | URL que recebe os eventos via POST. `"disabled"` desliga; string vazia mantém a atual. |
| subscribe | string\[\] | Não | Eventos: `ALL`, `MESSAGE`, `SEND_MESSAGE`, `READ_RECEIPT`, `PRESENCE`, `HISTORY_SYNC`, `CHAT_PRESENCE`, `CALL`, `CONNECTION`, `LABEL`, `CONTACT`, `GROUP`, `NEWSLETTER`, `QRCODE`, `BUTTON_CLICK`, `PICTURE`, `USER_ABOUT`. Desconhecidos são ignorados. Com `ALL` na lista, grava só `ALL`. Omitido: mantém os atuais (ou `MESSAGE` se não houver). |
| rabbitmqEnable | string | Não | `"enabled"`/`"true"` liga; `"disabled"`/`"false"` desliga. |
| natsEnable | string | Não | Idem, para NATS. |
| websocketEnable | string | Não | Idem, para o websocket `/ws`. |
| phone | string | Não | Número a parear (usado pelo cliente ao iniciar). |
| immediate | bool | Não | Aceito, sem efeito. |

**Exemplo de requisição:**

```json
{ "webhookUrl": "https://meusistema.com/webhook/evo", "subscribe": ["MESSAGE", "CONNECTION", "QRCODE"], "websocketEnable": "false" }
```

**Resposta (200):**

```json
{ "message": "success", "data": { "jid": "5511999999999:12@s.whatsapp.net", "webhookUrl": "https://meusistema.com/webhook/evo", "eventString": "MESSAGE,CONNECTION,QRCODE" } }
```

`jid` vem vazio se a instância ainda não foi pareada. A conexão é assíncrona: o 200 não significa que já está online; acompanhe por `/instance/status` ou pelo evento `CONNECTION`.

**Erros comuns:** 400 JSON inválido; 401 token inválido; 500 erro ao salvar ou configuração de proxy corrompida.

### GET /instance/status

**Autenticação:** Token da instância | **Para que serve:** diz se a instância está com a conexão aberta e se está logada (pareada). Ideal para polling de saúde.

**Resposta (200)** (chaves em PascalCase):

```json
{ "message": "success", "data": { "Connected": true, "LoggedIn": true, "Name": "Loja Centro" } }
```

Sem cliente rodando: `{"Connected": false, "LoggedIn": false, "Name": ""}`. `Name` é o nome de perfil quando logado.

**Erros comuns:** 401 token inválido; 404 `instance not found`.

### GET /instance/qr

✏️ **Alterado:** se a instância já está pareada mas reconectando, retorna `session already logged in` em vez de reiniciar a sessão (antes podia derrubar a reconexão).

**Autenticação:** Token da instância | **Para que serve:** obtém o QR code para parear o número. Se não houver cliente rodando, ele é iniciado; a chamada pode levar até \~5s esperando o QR. Quando a conta exige passkey, retorna os dados da cerimônia em vez do QR.

**Resposta (200) — QR:**

```json
{ "message": "success", "data": { "qrcode": "data:image/png;base64,iVBOR...", "code": "2@AbCd...,...", "expiresAt": 1792000000, "expiresIn": 54 } }
```

`qrcode` = imagem em base64; `code` = string bruta do QR (para gerar a imagem você mesmo); `expiresAt` = validade do código (unix, segundos) e `expiresIn` = segundos que faltam.

**Resposta (200) — passkey em andamento:**

```json
{ "message": "success", "data": { "qrcode": "", "code": "", "passkeyStage": "challenge", "passkeyOpenUrl": "https://web.whatsapp.com/#wapk=...", "passkeyCode": "" } }
```

`passkeyStage`: `challenge`, `awaiting_confirmation`, `confirmation`, `confirmed` ou `error`. Abra `passkeyOpenUrl` no navegador com a extensão Evolution Passkey Helper; `passkeyCode` é o código de confirmação quando houver.

**Erros comuns:** 400 `session already logged in` (já pareada — não é falha, não precisa de QR), `no QR code available. Please wait a moment and try again`, `failed to start instance: ...`; 401 token inválido; 404 `instance not found`.

### POST /instance/pair

**Autenticação:** Token da instância | **Para que serve:** pareia por código numérico (alternativa ao QR): o usuário digita o código em WhatsApp > Aparelhos conectados > Conectar com número de telefone.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| phone | string | Sim | Número com DDI e DDD, só dígitos (ex. `5511999999999`). |
| subscribe | string\[\] | Não | Aceito, mas não usado aqui (configure eventos via `/instance/connect`). |

**Resposta (200):** `{ "message": "success", "data": { "PairingCode": "ABCD-EFGH" } }`

**Erros comuns:** 400 `phone is required` / número inválido; 401 token inválido; 500 `instance is already authenticated`, `failed to start instance: ...`, `pairing failed: ...`.

### POST /instance/disconnect

✏️ **Alterado:** não apaga mais os eventos assinados (antes, depois de disconnect + reconnect nenhum webhook chegava); grava `disconnect_reason = "Disconnected by API"`.

**Autenticação:** Token da instância | **Para que serve:** desliga a conexão sem desparear o número (a sessão continua válida; volte com `/instance/connect`). Instâncias desligadas por esta rota não são religadas automaticamente no restart do servidor.

**Resposta (200):** `{ "message": "success" }` (também quando a instância já não estava conectada).

**Erros comuns:** 401 token inválido; 500 `no active session found` / `client disconnected` ou erro de banco.

### POST /instance/reconnect

**Autenticação:** Token da instância | **Para que serve:** reinicia a conexão de uma instância com cliente ativo (para aplicar mudanças ou destravar a sessão).

**Resposta (200):** `{ "message": "success" }`

**Erros comuns:** 401 token inválido; 500 `no active session found` / `client disconnected` (para instância desconectada use `/instance/connect`).

### DELETE /instance/logout

✏️ **Alterado:** grava `disconnect_reason = "Logged out by API"`.

**Autenticação:** Token da instância | **Para que serve:** faz logout do WhatsApp (desparea; o aparelho some de "Aparelhos conectados"). Para usar de novo é preciso novo QR ou código. A instância continua cadastrada.

**Resposta (200):** `{ "message": "success" }`

**Erros comuns:** 400 `ignoring logout as it was not connected`, `no active session found`, `client disconnected` ou erro do WhatsApp; 401 token inválido; 404 `instance not found`.

### GET /instance/:instanceId/advanced-settings

✏️ **Alterado:** aceita a Global API Key **ou** o token **daquela** instância. Token de outra instância → 403 (antes qualquer token funcionava e a Global API Key recebia 401).

**Autenticação:** Global API Key ou token da própria instância | **Para que serve:** lê as configurações de comportamento (sempre online, rejeitar chamadas, marcar como lida, ignorar grupos ou status).

**Resposta (200)** (objeto puro, sem envelope):

```json
{ "alwaysOnline": false, "rejectCall": true, "msgRejectCall": "Não atendemos ligações", "readMessages": false, "ignoreGroups": false, "ignoreStatus": true }
```

**Erros comuns:** 400 `instanceId is required`; 401 sem chave ou chave inválida; 403 `token does not grant access to this instance`; 500 UUID inválido ou instância não encontrada.

### PUT /instance/:instanceId/advanced-settings

✏️ **Alterado:** mesma mudança de autenticação do GET.

**Autenticação:** Global API Key ou token da própria instância | **Para que serve:** altera as configurações avançadas. É parcial: só os campos enviados são gravados, e a instância em execução é atualizada na hora, sem reconectar.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| alwaysOnline | bool | Não | Mantém a presença "online". Com `false`, a instância fica offline (o celular continua recebendo notificações) e dá um pulso de alguns segundos online a cada \~20h para não ser desvinculada por inatividade. Agora vale na hora, sem reconectar. |
| rejectCall | bool | Não | Rejeita chamadas de voz e vídeo automaticamente. |
| msgRejectCall | string | Não | Mensagem enviada ao rejeitar. Só é gravada se não vazia. |
| readMessages | bool | Não | Marca as mensagens recebidas como lidas automaticamente. |
| ignoreGroups | bool | Não | Ignora mensagens de grupos (não geram eventos). |
| ignoreStatus | bool | Não | Ignora atualizações de status. |

**Exemplo de requisição:** `{ "rejectCall": true, "msgRejectCall": "Por favor, envie mensagem de texto." }`

**Resposta (200)** (sem envelope `data`):

```json
{ "message": "Advanced settings updated successfully", "settings": { "alwaysOnline": false, "rejectCall": true, "msgRejectCall": "Por favor, envie mensagem de texto.", "readMessages": false, "ignoreGroups": false, "ignoreStatus": false } }
```

**Erros comuns:** 400 `instanceId is required` / JSON inválido; 401; 403 `token does not grant access to this instance`; 500 UUID inválido ou erro de banco.

### GET /server/ok

**Autenticação:** pública | **Para que serve:** health check do servidor (load balancer, monitor de uptime).

**Resposta (200):** `{ "status": "ok" }`

### GET /server/stats

**Autenticação:** Global API Key | **Para que serve:** métricas do processo e do host e contadores de mensagens, usados pelo dashboard (`/dashboard`) e por monitoramento.

**Resposta (200)** (sem envelope):

```json
{ "system": { "goroutines": 120, "numCpu": 4, "uptimeSeconds": 86400, "memAllocMB": 85.2, "heapInuseMB": 90.3, "loadAvg1": 0.5, "hostMemUsedPct": 46.8 }, "messages": { "total": 15230, "byStatus": "...", "byDay": "...", "topSources": "..." } }
```

`loadAvg*` e `hostMem*` só em host Linux. Se as estatísticas de mensagens falharem, `messages` vem `{"total": 0}`.

**Erros comuns:** 401 chave inválida.

### GET /ws

✏️ **Alterado:** várias conexões abertas para a mesma instância (várias abas ou serviços) agora **todas** recebem os eventos (antes só a última recebia).

**Autenticação:** Global API Key na query string `token` (não usa header) | **Para que serve:** receber os eventos das instâncias em tempo real por WebSocket, como alternativa ao webhook.

| Query | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| token | string | Sim | A `GLOBAL_API_KEY`. |
| instanceId | string (UUID) | Não | Só os eventos desta instância. Omitido = eventos de todas as instâncias. |

A instância precisa estar com `websocketEnable: "enabled"` via `/instance/connect`, e só chegam os eventos assinados.

**Exemplo:** `wss://api.exemplo.com/ws?token=GLOBAL_API_KEY&instanceId=3f2a...-uuid`

**Mensagens recebidas:**

```json
{ "queue": "message", "payload": "{\"event\":\"Message\",\"instanceId\":\"...\",\"data\":{...}}" }
```

`payload` é uma **string** com o mesmo JSON enviado ao webhook (faça `JSON.parse(payload)`).

**Erros comuns:** 401 `{"error":"Token inválido"}` antes do upgrade.

### GET /passkey-ceremony/:token

**Autenticação:** pública, controlada pelo token opaco e temporário (\~5 min) da cerimônia | **Para que serve:** usada pela extensão Evolution Passkey Helper para acompanhar o pareamento por passkey. Integrações normalmente não chamam esta rota; basta abrir o `passkeyOpenUrl` de `/instance/qr`.

**Resposta (200):**

```json
{ "stage": "challenge", "skipHandoffUX": false, "publicKey": { "...": "opções WebAuthn" }, "code": "123456", "error": "..." }
```

**Erros comuns:** 400 `token is required`; 404 `ceremony not found or expired` (também quando o pareamento terminou); 503 `passkey ceremony unavailable`.

### POST /passkey-ceremony/:token/response

**Autenticação:** pública (token da cerimônia) | **Para que serve:** a extensão envia a asserção WebAuthn gerada pelo navegador ou gerenciador de senhas, que é repassada ao WhatsApp.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| id | string | Sim | Id da credencial. |
| rawId | string | Sim | Id bruto (base64url). |
| type | string | Sim | `public-key`. |
| response | objeto | Sim | `{clientDataJSON, authenticatorData, signature, userHandle?}` em base64url. |

**Resposta (200):** `{ "ok": true }`

**Erros comuns:** 400 token ausente ou body inválido; 404 `ceremony not found or expired`; 500 erro ao enviar ao WhatsApp; 503.

### POST /passkey-ceremony/:token/confirm

**Autenticação:** pública (token da cerimônia) | **Para que serve:** finaliza o pareamento por passkey depois que o usuário conferiu o código de confirmação. Sem body.

**Resposta (200):** `{ "ok": true }`

**Erros comuns:** 400 `token is required`; 404 `ceremony not found or expired`; 500 falha ao confirmar; 503.

### POST /typebot

**Autenticação:** Token da instância | **Para que serve:** cria a configuração de um bot Typebot para a instância; mensagens recebidas passam a ser respondidas pelo fluxo. É usado o primeiro bot habilitado da instância.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| url | string | Sim | URL base do viewer do Typebot (ex. `https://viewer.exemplo.net`). |
| typebot | string | Sim | Nome público (slug) do fluxo. |
| description | string | Não | Descrição livre. |
| enabled | bool | Não | Padrão `true`. |
| expire | int | Não | Minutos sem interação para encerrar a sessão. `0` = nunca (padrão). |
| keywordFinish | string | Não | Palavra que o contato envia para encerrar (ex. `#sair`). |
| unknownMessage | string | Não | Mensagem enviada quando o Typebot responde sem texto. |
| delayMessage | int | Não | Pausa em ms antes de cada mensagem enviada. Padrão `0`. |
| listeningFromMe | bool | Não | Bot reage também a mensagens enviadas pela própria instância. Padrão `false`. |
| stopBotFromMe | bool | Não | Encerra a sessão quando o operador escreve manualmente (permite assumir o atendimento). Padrão `true`. |

**Exemplo de requisição:**

```json
{ "url": "https://viewer.exemplo.net", "typebot": "atendimento-v1", "expire": 30, "keywordFinish": "#sair", "delayMessage": 1000 }
```

**Resposta (201)** (objeto puro): o bot criado, com `id`, `instanceId`, os campos acima, `createdAt` e `updatedAt`.

**Erros comuns:** 400 `url and typebot are required` / JSON inválido; 401; 500 erro de banco.

### GET /typebot

**Autenticação:** Token da instância | **Para que serve:** lista os bots Typebot da instância.

**Resposta (200):** array puro de bots (vazio `[]` se não houver).

**Erros comuns:** 401; 500 erro de banco.

### PUT /typebot/:id

**Autenticação:** Token da instância | **Para que serve:** altera um bot. Parcial: só os campos enviados mudam (strings vazias são ignoradas; bools e ints podem ser `false`/`0`).

**Body:** mesmos campos de `POST /typebot`, todos opcionais. Ex.: `{ "enabled": false }`

**Resposta (200):** o bot atualizado (objeto puro).

**Erros comuns:** 400 JSON inválido; 401; 404 `typebot not found` (inexistente ou de outra instância); 500.

### DELETE /typebot/:id

**Autenticação:** Token da instância | **Para que serve:** remove o bot e suas sessões.

**Resposta (200):** `{ "success": true }`

### GET /typebot/sessions

**Autenticação:** Token da instância | **Para que serve:** lista as conversas em andamento entre contatos e o bot (painel de atendimento, auditoria, pausas automáticas).

**Resposta (200):** array puro:

```json
[ { "id": "uuid", "typebotId": "uuid", "remoteJid": "5511999999999@s.whatsapp.net", "pushName": "Maria", "sessionId": "...", "status": "opened", "awaitUser": true, "msgCount": 3, "pausedReason": "" } ]
```

`status`: `opened`, `paused` ou `closed`. `pausedReason`: `rate_limit` (contato excedeu o limite de mensagens) ou `self_loop` (conversa entre duas instâncias do próprio servidor).

### POST /typebot/changeStatus

**Autenticação:** Token da instância | **Para que serve:** encerra, pausa ou reabre a sessão de um contato pelo JID, sem listar sessões. Ex.: pausar o bot quando um atendente humano assume.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| remoteJid | string | Sim | JID do contato (ex. `5511999999999@s.whatsapp.net`). |
| status | string | Sim | `opened`, `paused` ou `closed`. |

**Resposta (200):** `{ "success": true, "remoteJid": "5511999999999@s.whatsapp.net", "status": "paused" }`

**Erros comuns:** 400 `remoteJid is required` / `status must be opened, paused or closed`; 401; 404 `no session for this contact`; 500.

### PUT /typebot/sessions/:id/status

**Autenticação:** Token da instância | **Para que serve:** pausa, encerra ou reabre uma sessão pelo id (de `/typebot/sessions`).

**Body:** `{ "status": "closed" }` (`opened`, `paused` ou `closed`).

**Resposta (200):** `{ "success": true }` (também se o id não existir).

**Erros comuns:** 400 status inválido; 401; 500.

### DELETE /typebot/sessions/:id

**Autenticação:** Token da instância | **Para que serve:** apaga uma sessão; a próxima mensagem do contato começa o fluxo do zero.

**Resposta (200):** `{ "success": true }`

## Catálogo — Envio de mensagens (/send)

Todas as rotas são `POST` com JSON (exceto os modos multipart de `/send/media` e `/send/status/media`) e usam o **token da instância** no header `apikey`; a Global API Key não serve aqui. Sucesso é sempre `200 {"message":"success","data":{...}}`, com `data` no formato abaixo.

```json
{
  "message": "success",
  "data": {
    "Info": { "Chat": "5511999999999@s.whatsapp.net", "Sender": "5511888888888:12@s.whatsapp.net", "IsFromMe": true, "IsGroup": false, "ID": "3EB0ABC123...", "Timestamp": "2026-10-12T10:00:00Z", "Type": "ExtendedTextMessage" },
    "Message": { "extendedTextMessage": { "text": "Olá!" } },
    "MessageContextInfo": { "stanzaID": "", "participant": "", "quotedMessage": { "conversation": "" } }
  }
}
```

Use `data.Info.ID` como ID da mensagem enviada para editar, apagar, reagir ou citar depois. `Message` é a mensagem enviada (estrutura do whatsmeow, varia por tipo).

### Mudanças desta versão que valem para todo o grupo

- ✏️ **Instância desconectada agora retorna `409`** na hora, com `{"error":"instance is not connected to WhatsApp"}` (pode vir prefixado, ex.: `"failed to connect client: instance is not connected to WhatsApp"`). Antes ficava presa até \~80s e devolvia `500`. Trate `409` como "reconecte a instância", não como erro do payload.
- ✏️ **Celular brasileiro com ou sem o 9º dígito:** com `CHECK_USER_EXISTS=true`, as duas formas são consultadas numa chamada só e a mensagem vai para a registrada.
- ✏️ **Objeto `quoted` ganhou `text`, `message` e `key`** (ver Campos comuns). Sem o conteúdo da citada, o WhatsApp mostra um card de citação vazio.
- ✏️ **Menções** (`mentionAll`, `mentionedJid`) agora funcionam também em `/send/button`, `/send/list`, `/send/carousel` e `/send/product`.

### Campos comuns

Existem na maioria das rotas de `/send` (cada rota indica quais aceita).

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `number` | string | Sim | Destino: telefone com DDI (ex.: `5511999999999`) ou JID completo (`…@s.whatsapp.net`, grupo `…@g.us`, `…@lid`, canal `…@newsletter`). |
| `formatJid` | bool | Não (padrão `true`) | `true`: valida e normaliza o `number`. `false`: usa o `number` exatamente como enviado. |
| `id` | string | Não | ID personalizado para a mensagem. Se omitido, o servidor gera um. |
| `delay` | int (ms) | Não | Mostra "digitando…" (ou "gravando áudio…") por esse tempo antes de enviar. A requisição só responde depois do delay. |
| `mentionAll` | bool | Não | Só em grupos: menciona todos os participantes. |
| `mentionedJid` | string\[\] | Não | Só em grupos: JIDs a mencionar. Inclua `@numero` no texto para o destaque visual. |
| `quoted` | objeto | Não | Responde (cita) uma mensagem. Campos abaixo. |
| `quoted.messageId` | string | Sim, para citar | ID da mensagem citada (`key.id` recebido no webhook). |
| `quoted.participant` | string | Recomendado | JID do autor da mensagem citada (necessário na prática em grupos). |
| `quoted.text` | string | Não | 🆕 Novo — texto da mensagem citada, exibido no card de citação. |
| `quoted.message` | objeto | Não | 🆕 Novo — o objeto `message` recebido no webhook (ex.: `{"conversation":"oi"}`); tem prioridade sobre `quoted.text`. Se não puder ser lido (botões, interativos), vale `quoted.text`. |
| `quoted.key` | objeto `{id, participant}` | Não | 🆕 Novo — formato da Evolution API v2, alternativa a `messageId`/`participant`. |
| `forwardingScore` | int | Não | Só em `/send/text` e `/send/media` (JSON). Valor > 0 marca a mensagem como "Encaminhada". |

Erros comuns a todas as rotas com `number`: `400 Invalid JSON format`, `400 number is required and cannot be empty`, `400 Invalid number format: ...`. Com `CHECK_USER_EXISTS=true`, número sem WhatsApp → `500 number X is not registered on WhatsApp`.

### POST /send/text

**Autenticação:** token da instância | **Para que serve:** envia uma mensagem de texto. É a rota mais usada para respostas de bot e notificações.

**Body:** campos comuns `number`, `id`, `delay`, `mentionAll`, `mentionedJid`, `formatJid`, `quoted`, `forwardingScore`, mais:

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `text` | string | Sim | Texto (aceita `*negrito*`, `_itálico_`, `~riscado~`). |

**Exemplo de requisição:**

```json
{ "number": "5511999999999", "text": "Olá! Seu pedido foi confirmado.", "delay": 1200,
  "quoted": { "messageId": "3EB0ABC123", "participant": "5511999999999@s.whatsapp.net", "text": "Qual o status do pedido?" } }
```

**Resposta (200):** objeto da mensagem enviada, `Info.Type` = `ExtendedTextMessage`.

**Erros comuns:** `400` `phone number is required` / `message body is required`; `401`; `409` instância desconectada; `500` falha no envio.

### POST /send/link

**Autenticação:** token da instância | **Para que serve:** envia um texto com prévia de link (título, descrição e miniatura), montada a partir do primeiro URL do `text`.

**Body:** campos comuns `number`, `id`, `delay`, `mentionAll`, `mentionedJid`, `formatJid`, `quoted`, mais:

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `text` | string | Sim | Texto que contém o link. |
| `title` | string | Não | Título da prévia. Substituído pelo `<title>` da página quando há URL no texto. |
| `description` | string | Não | Descrição. Substituída pela meta `description`/`og:description` da página. |
| `imgUrl` | string | Não | Miniatura. Substituída pela `og:image` da página. |
| `url` | string | Não | Aceito, mas não usado: o link vem do `text`. |

**Exemplo de requisição:** `{ "number": "5511999999999", "text": "Confira nossa loja: https://minhaloja.com.br" }`

**Resposta (200):** objeto da mensagem enviada, com `extendedTextMessage` contendo `title`, `matchedText`, `description` e miniatura.

**Erros comuns:** `400` `phone number is required` / `message body is required`; `401`; `409`; `500` falha ao buscar a página ou ao enviar.

### POST /send/media

**Autenticação:** token da instância | **Para que serve:** envia imagem, vídeo, vídeo-recado (PTV), áudio (convertido para nota de voz) ou documento, por URL, base64 ou upload de arquivo.

**Modo JSON:** campos comuns `number`, `id`, `delay`, `mentionAll`, `mentionedJid`, `formatJid`, `quoted`, `forwardingScore`, mais:

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `type` | string | Sim | `image`, `video`, `ptv`, `audio` ou `document`. |
| `url` | string | Sim | URL `http(s)://` do arquivo **ou** o conteúdo em base64 puro (sem prefixo `data:`). |
| `caption` | string | Não | Legenda (imagem, vídeo, documento). |
| `filename` | string | Não | Nome do arquivo exibido (para `document`). |
| `viewOnce` | bool | Não | 🆕 Novo — visualização única. Vale para `image`, `video`, `audio` e `ptv`; documento ignora. |

Formatos: `image` → jpeg/png/webp; `video`/`ptv` → mp4; `audio` → qualquer formato conversível (vira ogg/opus); `document` → qualquer. `ptv` só funciona via URL.

**Modo multipart/form-data:**

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `file` | arquivo | Sim | O arquivo. |
| `number` | string | Sim | Destino. |
| `type` | string | Sim | `image`, `video`, `audio` ou `document`. |
| `caption`, `filename`, `id` | string | Não | Iguais ao modo JSON. |
| `delay` | int | Não | Em ms. |
| `mentionAll` | `"true"` | Não | Menciona todos (grupos). |
| `mentionedJid` | string | Não | Repita o campo ou separe por vírgula. |
| `quoted.messageId`, `quoted.participant` | string | Não | Citação. |
| `quoted.text` | string | Não | 🆕 Novo — texto da mensagem citada. |
| `viewOnce` | `"true"` | Não | 🆕 Novo — visualização única. |

**Exemplo de requisição (JSON):**

```json
{ "number": "5511999999999", "type": "image", "url": "https://exemplo.com/foto.jpg", "caption": "Seu comprovante", "viewOnce": true }
```

**Resposta (200):** objeto da mensagem enviada; `Info.Type` = `ImageMessage`, `VideoMessage`, `PtvMessage`, `AudioMessage` ou `DocumentMessage`.

**Erros comuns:** `400` `URL is required` / `media type is required` / `invalid base64 encoding` / `file is required` / `invalid delay`; `401`; `409`; `500` formato inválido para o tipo, falha de download, conversão ou upload.

### POST /send/poll

**Autenticação:** token da instância | **Para que serve:** cria uma enquete; os votos chegam por webhook.

**Body:** campos comuns `number`, `id`, `delay`, `mentionAll`, `mentionedJid`, `formatJid`, `quoted`, mais:

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `question` | string | Sim | Pergunta. |
| `options` | string\[\] | Sim (mín. 2) | Opções de resposta. |
| `maxAnswer` | int | Não | Quantas opções cada pessoa pode marcar (`1` = escolha única; `0` = sem limite). |

**Exemplo de requisição:** `{ "number": "120363000000000000@g.us", "question": "Melhor dia?", "options": ["Segunda", "Quarta", "Sexta"], "maxAnswer": 1 }`

**Resposta (200):** objeto da mensagem enviada, `Info.Type` = `PollCreationMessage`.

**Erros comuns:** `400` `question is required` / `minimum 2 options are required`; `401`; `409`; `500`.

### POST /send/sticker

**Autenticação:** token da instância | **Para que serve:** envia uma figurinha a partir de uma URL de imagem. WebP (inclusive animado) vai intacto; jpg/png são convertidos.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `sticker` | string | Sim | URL `http(s)://` da imagem (até 16 MiB, timeout de 30s). Base64 não é aceito. |

Mais os campos comuns `number`, `id`, `delay`, `mentionAll`, `mentionedJid`, `formatJid`, `quoted`.

**Resposta (200):** objeto da mensagem enviada, `Info.Type` = `StickerMessage`.

**Erros comuns:** `400` `sticker is required`; `401`; `409`; `500` URL inválida, falha de download, arquivo acima do limite, falha de conversão.

### POST /send/location

**Autenticação:** token da instância | **Para que serve:** envia um pin de localização com nome e endereço.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `latitude` | number | Sim | Latitude (não pode ser `0`). |
| `longitude` | number | Sim | Longitude (não pode ser `0`). |
| `name` | string | Sim | Nome do local. |
| `address` | string | Sim | Endereço exibido. |

Mais os campos comuns. **Exemplo:** `{ "number": "5511999999999", "latitude": -23.5614, "longitude": -46.6559, "name": "Loja Paulista", "address": "Av. Paulista, 1000" }`

**Resposta (200):** `Info.Type` = `LocationMessage`. **Erros comuns:** `400` campo obrigatório faltando; `401`; `409`; `500`.

### POST /send/event

**Autenticação:** token da instância | **Para que serve:** cria um Evento do WhatsApp (nome, data e hora, local, link de chamada), normalmente em grupos.

> **Limitação conhecida:** a rota aceita e envia o evento (`200`), mas a biblioteca do WhatsApp o transmite como texto e **os aplicativos podem não exibi-lo**. Não dependa desta rota em produção sem testar.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `name` | string | Sim | Nome do evento. |
| `startTime` | string ou int | Sim | Início: ISO 8601 com fuso (`2026-10-20T19:00:00-03:00`) ou epoch em segundos. |
| `endTime` | string ou int | Não | Término, mesmo formato. |
| `description` | string | Não | Descrição. |
| `text` | string | Não | Texto enviado como mensagem separada, antes do card. |
| `location` | objeto | Não | `{name, latitude, longitude, address}`. |
| `joinLink` | string | Não | Link da chamada. |
| `extraGuestsAllowed`, `isScheduleCall`, `hasReminder`, `isCanceled` | bool | Não | Opções do evento. |
| `reminderOffsetSec` | int | Não | Antecedência do lembrete em segundos (com `hasReminder`). |

Mais os campos comuns. **Resposta (200):** `Info.Type` = `EventMessage`.

**Erros comuns:** `400` `event name is required` / `startTime is required` / formato de hora inválido; `401`; `409` instância desconectada; `500` outras falhas.

### POST /send/contact

**Autenticação:** token da instância | **Para que serve:** envia um cartão de contato (vCard).

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `vcard.fullName` | string | Sim | Nome do contato. |
| `vcard.phone` | string | Sim | Telefone do contato. |
| `vcard.organization` | string | Não | Empresa. |

Nesta rota o `number` é sempre normalizado (ignora `formatJid`); um contato por requisição.

**Exemplo:** `{ "number": "5511999999999", "vcard": { "fullName": "Suporte Loja", "phone": "5511988887777" } }`

**Resposta (200):** `Info.Type` = `ContactMessage`. **Erros comuns:** `400` telefone ou nome do contato faltando ou inválido; `401`; `409`; `500`.

### POST /send/button

**Autenticação:** token da instância | **Para que serve:** mensagem interativa com botões de resposta rápida, copiar código, abrir URL, ligar ou pagar via Pix.

✏️ **Alterado:** `mentionAll` e `mentionedJid` agora são aplicados.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `title` | string | Sim | Título. |
| `description` | string | Sim | Corpo do texto. |
| `footer` | string | Não | Rodapé. |
| `buttons` | Button\[\] | Sim | Botões (ver abaixo). |
| `imageUrl` | string | Não | Imagem no cabeçalho. |
| `videoUrl` | string | Não | Vídeo no cabeçalho (se não houver `imageUrl`). |

Cada botão: `type` (`reply`, `copy`, `url`, `call` ou `pix`), `displayText`, `id` (reply: devolvido no webhook ao clicar), `copyCode` (copy), `url` (url), `phoneNumber` (call), `currency`/`name`/`keyType`/`key` (pix). Regras: no máximo 3 `reply`; `reply` não se mistura com outros tipos; `pix` deve ser o único botão.

Mais os campos comuns, exceto `id`.

**Exemplo de requisição:**

```json
{ "number": "5511999999999", "title": "Oferta especial", "description": "Confira as condições", "buttons": [
  { "type": "reply", "displayText": "Quero saber mais", "id": "btn_info" },
  { "type": "reply", "displayText": "Não, obrigado", "id": "btn_no" } ] }
```

**Resposta (200):** `Info.Type` = `InteractiveMessage`. O clique chega no webhook como evento `ButtonClick`.

**Erros comuns:** `400` `title is required` / `description is required`; `401`; `409`; `500` combinação de botões inválida ou falha de envio.

### POST /send/list

**Autenticação:** token da instância | **Para que serve:** menu de lista (seleção única) com seções e itens; a escolha chega no webhook com o `rowId`.

✏️ **Alterado:** enviada como `ListMessage` direta, formato que o WhatsApp volta a entregar. O payload não mudou. Menções agora são aplicadas.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `title` | string | Sim | Título. |
| `description` | string | Sim | Corpo. |
| `footerText` | string | Sim | Rodapé. |
| `buttonText` | string | Sim | Texto do botão que abre o menu. |
| `sections` | Section\[\] | Sim | `{title?, rows:[{title, description?, rowId?}]}`. Sem `rowId`, o servidor gera um. |

Mais os campos comuns, exceto `id`.

**Exemplo de requisição:**

```json
{ "number": "5511999999999", "title": "Nossos planos", "description": "Escolha o ideal", "footerText": "Minha Loja", "buttonText": "Ver planos",
  "sections": [ { "title": "Planos", "rows": [ { "title": "Básico", "description": "R$ 29,90/mês", "rowId": "plan_basic" }, { "title": "Pro", "rowId": "plan_pro" } ] } ] }
```

**Resposta (200):** `Info.Type` = `ListMessage`.

**Erros comuns:** `400` campo obrigatório faltando; `401`; `409`; `500`.

### POST /send/carousel

**Autenticação:** token da instância | **Para que serve:** carrossel de cartões deslizáveis, cada um com imagem ou vídeo, texto e botões (vitrine de produtos, ofertas).

🆕 **Novo:** `mentionAll` e `mentionedJid`. ✏️ **Alterado:** `formatJid` agora é respeitado.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `body` | string | Não | Texto acima do carrossel. |
| `footer` | string | Não | Texto abaixo. |
| `cards` | Card\[\] | Sim (mín. 1) | Cartões. |
| `cards[].header` | objeto | Sim | `{title?, subtitle?, imageUrl?, videoUrl?}`. |
| `cards[].body.text` | string | Sim | Texto do cartão. |
| `cards[].footer` | string | Não | Rodapé do cartão. |
| `cards[].buttons` | objeto\[\] | Não | `{type, displayText, id, copyCode?}`. `type`: `REPLY` (padrão), `URL`, `CALL`, `COPY`. URL e CALL usam o campo `id` para o link ou telefone; COPY usa `copyCode`. |

Mais `number`, `delay`, `formatJid`, `quoted`, `mentionAll`, `mentionedJid`.

**Exemplo de requisição:**

```json
{ "number": "5511999999999", "body": "Novidades da semana", "cards": [
  { "header": { "title": "Tênis X", "imageUrl": "https://exemplo.com/tenis.jpg" }, "body": { "text": "R$ 199,90" },
    "buttons": [ { "type": "URL", "displayText": "Comprar", "id": "https://loja.com/tenis-x" } ] } ] }
```

**Resposta (200):** `Info.Type` = `InteractiveMessage`.

**Erros comuns:** `400` `at least one card is required`; `401`; `409`; `500`.

### POST /send/product

**Autenticação:** token da instância | **Para que serve:** envia o card de um produto do catálogo (foto, título, preço e botão). O produto precisa existir no catálogo do WhatsApp Business.

✏️ **Alterado:** menções aplicadas; instância desconectada → `409`.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `productId` | string | Sim | ID do produto no catálogo. |
| `title` | string | Sim | Nome exibido. |
| `currency` | string | Sim | Moeda ISO (ex.: `BRL`). |
| `price` | int | Não | Preço em milésimos: R$ 10,00 = `10000`. |
| `imageBase64` / `imageUrl` | string | Um dos dois | Imagem do produto. |
| `description` | string | Não | Descrição. |
| `retailerId` | string | Não | SKU do lojista. |
| `url` | string | Não | Link do produto. |
| `businessOwnerJid` | string | Não | JID do dono do catálogo; padrão: a própria instância. |
| `body` / `footer` | string | Não | Texto e rodapé. |

**Exemplo:** `{ "number": "5511999999999", "productId": "1234567890", "title": "Camiseta Azul", "price": 59900, "currency": "BRL", "imageUrl": "https://exemplo.com/camiseta.jpg" }`

**Resposta (200):** `Info.Type` = `ProductMessage`. **Erros comuns:** `400` campo obrigatório faltando; `401`; `409`; `500`.

### POST /send/status/text

**Autenticação:** token da instância | **Para que serve:** publica um Status (stories) de texto na conta conectada.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `text` | string | Sim | Texto do status. |
| `id` | string | Não | ID personalizado. |

**Resposta (200):** objeto da mensagem, `Info.Chat` = `status@broadcast`. O webhook `SendStatus` agora chega a quem assina `SEND_MESSAGE`.

**Erros comuns:** `400` `text is required`; `401`; `409`; `500`.

### POST /send/status/media

**Autenticação:** token da instância | **Para que serve:** publica um Status de imagem ou vídeo, por URL (JSON) ou upload (multipart).

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `type` | string | Sim | `image` ou `video`. |
| `url` | string | Sim (JSON) | URL do arquivo (jpeg/png/webp; mp4). |
| `caption` | string | Não | Legenda. |
| `id` | string | Não | ID personalizado. |

Multipart: `file` (obrigatório), `type`, `caption`, `id`.

**Resposta (200):** objeto da mensagem, `Info.Chat` = `status@broadcast`.

**Erros comuns:** `400` `url is required` / `type must be 'image' or 'video'` / `file is required`; `401`; `409`; `500`.

## Catálogo — Usuário, mensagens, chats e chamadas

Todas as rotas usam o **token da instância** no header `apikey` (a Global API Key não é aceita). Sucesso: `{"message":"success","data":...}` (algumas só `{"message":"success"}`); erro: `{"error":"..."}`.

- **Instância desconectada:** diferente de `/send/*`, estas rotas continuam respondendo `500` com `"client disconnected"` ou `"no active session found"`.
- **Campo `number`:** aceita número com DDI (`5511999998888`) ou JID completo (`…@s.whatsapp.net`, `…@g.us`, `…@lid`). Vazio ou mal formado → `400`.
- **`timestamp` zerado:** várias rotas devolvem `timestamp` como `"0001-01-01 00:00:00 +0000 UTC"`. Não use como horário real.

### POST /user/info

**Autenticação:** token da instância | **Para que serve:** consulta dados públicos de um ou mais contatos (recado, ID da foto, dispositivos, nome verificado de empresa e LID).

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| number | string\[\] | Sim | Números ou JIDs a consultar (mínimo 1). |

**Exemplo:** `{ "number": ["5511999998888"] }`

**Resposta (200):**

```json
{ "message": "success", "data": { "Users": { "5511999998888@s.whatsapp.net": { "VerifiedName": null, "Status": "Disponível", "PictureID": "1234567890", "Devices": ["5511999998888@s.whatsapp.net"], "LID": "123456789012345@lid" } } } }
```

`Users` é um mapa com o JID como chave. `LID` é `null` quando o mapeamento não é conhecido.

**Erros comuns:** 400 número ausente ou inválido; 401; 500 instância desconectada ou erro do WhatsApp.

### POST /user/check

**Autenticação:** token da instância | **Para que serve:** verifica se números têm WhatsApp e devolve o JID correto para enviar. Use antes de disparos.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| number | string\[\] | Sim | Números a verificar (mínimo 1). |
| formatJid | bool | Não | **Padrão `false` nesta rota.** Com `true`, normaliza antes de consultar e reconsulta sem normalizar os que não forem achados. |

**Exemplo:** `{ "number": ["5511999998888", "5511988887777"] }`

**Resposta (200):**

```json
{ "message": "success", "data": { "Users": [ { "Query": "5511999998888", "IsInWhatsapp": true, "JID": "5511999998888@s.whatsapp.net", "RemoteJID": "5511999998888@s.whatsapp.net", "LID": "123456789012345@lid", "VerifiedName": "" } ] } }
```

`RemoteJID` é o JID a usar no envio.

**Erros comuns:** 400; 401; 500 instância desconectada.

### POST /user/avatar

**Autenticação:** token da instância | **Para que serve:** URL da foto de perfil de um contato ou grupo (para CRMs exibirem o avatar).

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| number | string | Sim | Número ou JID (`@s.whatsapp.net`, `@g.us` ou `@lid`). |
| preview | bool | Não | `true` devolve a miniatura. Padrão `false`. |

**Resposta (200):** `{ "message": "success", "data": { "url": "https://pps.whatsapp.net/v/...", "id": "1234567890", "type": "image", "direct_path": "/v/..." } }`

**Erros comuns:** 400; 401; 429 limite do WhatsApp; 504 WhatsApp não respondeu em 8s; 500 sem foto ou foto oculta (`no profile picture found`).

### GET /user/contacts

**Autenticação:** token da instância | **Para que serve:** lista os contatos que a instância conhece (agenda sincronizada e contatos que já interagiram).

**Resposta (200):** `{ "message": "success", "data": [ { "Jid": "5511999998888@s.whatsapp.net", "Found": true, "FirstName": "Maria", "FullName": "Maria Silva", "PushName": "Maria", "BusinessName": "" } ] }` (`data` = `null` se não houver).

### POST /user/savecontact

**Autenticação:** token da instância | **Para que serve:** adiciona ou atualiza um contato na lista do WhatsApp da instância, com opção de gravar também na agenda do celular.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| number | string | Sim | Número com DDI. |
| fullName | string | Sim | Nome completo. |
| firstName | string | Não | Primeiro nome; se vazio, usa a primeira palavra de `fullName`. |
| saveOnPhone | bool | Não | Padrão `true`: grava também na agenda do celular. |

**Resposta (200):** `{ "message": "success" }`. O celular precisa estar online para propagar. Se o app state de contatos estiver corrompido, o servidor pede recuperação ao celular e responde 500 pedindo nova tentativa em \~30s.

**Erros comuns:** 400 `fullName is required`; 401; 500.

### GET /user/privacy

**Autenticação:** token da instância | **Para que serve:** lê as configurações de privacidade da conta.

**Resposta (200):** `{ "message": "success", "data": { "GroupAdd": "contacts", "LastSeen": "contacts", "Status": "contacts", "Profile": "all", "ReadReceipts": "all", "CallAdd": "all", "Online": "all" } }`

### POST /user/privacy

**Autenticação:** token da instância | **Para que serve:** altera a privacidade da conta. Todos os campos são obrigatórios (envie também os que não mudam).

| Campo | Tipo | Obrigatório | Valores |
| --- | --- | --- | --- |
| groupAdd | string | Sim | `all`, `contacts`, `contact_blacklist` |
| lastSeen | string | Sim | `all`, `contacts`, `contact_blacklist`, `none` |
| status | string | Sim | `all`, `contacts`, `contact_blacklist`, `none` |
| profile | string | Sim | `all`, `contacts`, `contact_blacklist`, `none` |
| readReceipts | string | Sim | `all`, `none` |
| callAdd | string | Sim | `all`, `known` |
| online | string | Sim | `all`, `match_last_seen` |

**Resposta (200):** mesmo formato do GET. As configurações são aplicadas uma a uma; uma falha no meio pode deixar parte já alterada.

**Erros comuns:** 400 campo faltando; 401; 500 valor recusado pelo WhatsApp.

### POST /user/block

✏️ **Alterado:** a resposta mantém `DHash` e `JIDs` e ganha `Items` (`[{LID, PN, Active}]`).

**Autenticação:** token da instância | **Para que serve:** bloqueia um contato e devolve a lista de bloqueados atualizada.

**Body:** `{ "number": "5511999998888" }`

**Resposta (200):**

```json
{ "message": "success", "data": { "DHash": "1700000000000", "JIDs": ["5511999998888@s.whatsapp.net"], "Items": [ { "LID": "123456789012345@lid", "PN": "5511999998888@s.whatsapp.net", "Active": true } ] } }
```

`JIDs`: um por contato bloqueado, o número quando conhecido, senão o LID.

### POST /user/unblock

✏️ **Alterado:** mesmo campo `Items` na resposta.

**Autenticação:** token da instância | **Para que serve:** desbloqueia um contato. **Body:** `{ "number": "5511999998888" }`. **Resposta:** mesmo formato do `/user/block`.

### GET /user/blocklist

✏️ **Alterado:** inclui `Items`; `DHash` e `JIDs` continuam iguais.

**Autenticação:** token da instância | **Para que serve:** lista os contatos bloqueados. **Resposta:** mesmo formato do `/user/block`.

### POST /user/lid

🆕 **Novo.** **Autenticação:** token da instância | **Para que serve:** converte um LID (`…@lid`) no número de telefone, quando um webhook chega com remetente LID e você precisa do telefone para cruzar com o CRM.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| lid | string | Sim | `123456789012345@lid` ou só os dígitos. |
| groupJid | string | Não | Grupo (`…@g.us`) onde o LID aparece; se o número ainda não for conhecido, o grupo é relido. |

**Resposta (200):** `{ "message": "success", "data": { "lid": "123456789012345@lid", "phoneNumber": "5511999998888", "jid": "5511999998888@s.whatsapp.net" } }`

**Erros comuns:** 400 `lid is required` / `invalid lid` / `invalid groupJid`; 401; 404 `no phone number mapping found for this lid` (ainda não conhecido; costuma aparecer depois que o contato manda mensagem); 429/504; 500.

### POST /user/profilePicture

**Autenticação:** token da instância | **Para que serve:** troca a foto de perfil da conta. **Body:** `{ "image": "https://exemplo.com/logo.jpg" }` (URL pública, JPEG; base64 não é aceito).

**Resposta (200):** `{ "message": "success", "data": { "image": "https://exemplo.com/logo.jpg" } }`

### POST /user/profileName

✏️ **Alterado:** agora troca de fato o nome. Antes ficava preso \~75s e não mudava nada.

**Autenticação:** token da instância | **Para que serve:** altera o nome de exibição da conta. **Body:** `{ "name": "Atendimento Loja X" }`

**Resposta (200):** `{ "message": "success", "data": { "name": "Atendimento Loja X" } }` (limite de 30s).

**Erros comuns:** 400 `name is required`; 401; 500 `failed to update profile name: ...`.

### POST /user/profileStatus

**Autenticação:** token da instância | **Para que serve:** altera o recado ("sobre") do perfil. **Body:** `{ "status": "Atendimento de seg a sex, 8h às 18h" }`

**Resposta (200):** `{ "message": "success", "data": { "status": "..." } }`

### POST /message/react

**Autenticação:** token da instância | **Para que serve:** envia ou remove uma reação (emoji) numa mensagem.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| number | string | Sim | Chat da mensagem (grupo: `…@g.us`). |
| reaction | string | Sim | Emoji; `"remove"` remove a reação. |
| id | string | Sim | ID da mensagem que recebe a reação. |
| fromMe | bool | Não | `true` se a mensagem foi enviada pela instância. |
| participant | string | Não | Em grupos, JID de quem enviou a original. |

**Resposta (200):** objeto da mensagem enviada (`Info.Type` = `ReactionMessage`; `Info.ID` é o ID da reação).

### POST /message/presence

**Autenticação:** token da instância | **Para que serve:** mostra "digitando…" ou "gravando áudio…" no chat do contato.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| number | string | Sim | Chat. |
| state | string | Sim | `composing` ou `paused`. |
| isAudio | bool | Não | Com `composing`, mostra "gravando áudio". |
| delay | int | Não | Em ms (máx. 60000): mantém o indicador e envia `paused` no fim. A requisição só responde depois. |

**Resposta (200):** `{ "message": "success", "data": { "timestamp": "..." } }`

### POST /message/subscribe

🆕 **Novo.** **Autenticação:** token da instância | **Para que serve:** passar a receber os eventos `Presence` (online, offline, visto por último) de um contato. Vale até a próxima reconexão da instância.

**Body:** `{ "number": "5511999998888" }`

**Resposta (200):**

```json
{ "message": "success", "data": { "subscribed": true, "warning": "alwaysOnline is false: WhatsApp only delivers contact presence while the instance is online, ..." } }
```

`warning` só aparece com `alwaysOnline=false`: nesse caso os eventos não chegam.

**Erros comuns:** 400 `number is required`; 401; 500.

### POST /message/markread

**Autenticação:** token da instância | **Para que serve:** marca mensagens como lidas (tiques azuis). **Body:** `{ "number": "5511999998888", "id": ["3EB0ABC123DEF456"] }`

**Resposta (200):** `{ "message": "success", "data": { "timestamp": "..." } }`

### POST /message/markplayed

**Autenticação:** token da instância | **Para que serve:** marca áudio como reproduzido (microfone azul). **Body:** igual ao `/message/markread`.

### POST /message/downloadmedia

**Autenticação:** token da instância | **Para que serve:** baixa e descriptografa a mídia de uma mensagem recebida, devolvendo base64.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| message | objeto | Sim | O objeto `message` como chegou no webhook (ex.: `{"imageMessage":{...}}`), com `url`/`directPath`, `mediaKey`, `mimetype` etc. |

**Resposta (200):** `{ "message": "success", "data": { "base64": "data:image/jpeg;base64,/9j/4AAQ..." } }` (data URL completo).

**Erros comuns:** 400; 401; 500 `invalid media type` ou mídia expirada.

### POST /message/status

**Autenticação:** token da instância | **Para que serve:** último status de entrega registrado de uma mensagem (enviada, entregue, lida). **Body:** `{ "id": "3EB0ABC123DEF456" }`

**Resposta (200):** `{ "message": "success", "data": { "result": { "message_id": "3EB0ABC123DEF456", "status": "read", "referral": {} } } }` (`referral` só em mensagens vindas de anúncio).

### POST /message/delete

**Autenticação:** token da instância | **Para que serve:** apaga "para todos" uma mensagem enviada pela instância.

**Body:** `{ "chat": "5511999998888@s.whatsapp.net", "messageId": "3EB0ABC123DEF456" }`

**Resposta (200):** `{ "message": "success", "data": { "messageId": "3EB0XYZ..." } }` (ID da revogação).

### POST /message/edit

**Autenticação:** token da instância | **Para que serve:** edita o texto de uma mensagem de texto enviada pela instância (dentro da janela de edição do WhatsApp). Não edita legendas.

**Body:** `{ "chat": "5511999998888@s.whatsapp.net", "messageId": "3EB0ABC123DEF456", "message": "Texto corrigido" }`

**Resposta (200):** `{ "message": "success", "data": { "messageId": "3EB0XYZ...", "timestamp": "2026-10-12 14:03:10 -0300 -03" } }`

### POST /chat/pin, /chat/unpin, /chat/archive, /chat/unarchive, /chat/mute, /chat/unmute

**Autenticação:** token da instância | **Para que serve:** fixar, desafixar, arquivar, desarquivar, silenciar (por 1 hora, fixo) e reativar notificações de um chat.

**Body:** `{ "chat": "5511999998888@s.whatsapp.net" }`

**Resposta (200):** `{ "message": "success", "data": { "timestamp": "..." } }`

> Estas seis rotas podem responder `200` sem o efeito aparecer no celular. A sincronização dessas configurações agora se recupera sozinha quando dessincroniza, o que deve reduzir o problema, mas teste antes de depender delas.

### POST /chat/history-sync

**Autenticação:** token da instância | **Para que serve:** pede ao celular mensagens mais antigas de um chat. Elas chegam depois pelos eventos de histórico, não na resposta.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| messageInfo | objeto | Sim | Mensagem de referência (a mais antiga que você tem): `Chat`, `IsFromMe`, `IsGroup`, `ID`, `Timestamp` (ISO 8601). |
| count | int | Sim | Quantidade de mensagens (ex.: 50). |

**Resposta (200):** confirmação de que o pedido foi enviado ao celular.

### POST /call/reject

**Autenticação:** token da instância | **Para que serve:** recusa uma chamada recebida, usando os dados do evento `CallOffer` do webhook.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| callCreator | string (JID) | Sim | Quem ligou, como veio no evento. |
| callId | string | Sim | ID da chamada, como veio no evento. |

**Resposta (200):** `{ "message": "success" }`

## Catálogo — Grupos, comunidades, etiquetas, canais e enquetes

Nenhuma rota deste grupo mudou nesta versão; integrações existentes continuam funcionando sem ajuste.

- **Autenticação:** token da instância no header `apikey` (a Global API Key devolve `401`).
- **Instância desconectada:** diferente de `/send/*`, estas rotas continuam respondendo `500` com `"client disconnected"` ou `"no active session found"`.
- **JIDs:** grupos e comunidades usam `<id>@g.us`; canais, `<id>@newsletter`. Onde o catálogo diz **"JID completo"**, mande o JID inteiro, com o `@servidor`.
- Respostas com metadados de grupo usam a estrutura `GroupInfo` do whatsmeow, com chaves em PascalCase (`JID`, `Name`, `Topic`, `Participants`...).

### GET /group/list

**Autenticação:** token da instância | **Para que serve:** lista todos os grupos de que a instância participa, com metadados completos (participantes, configurações, dono).

**Resposta (200):**

```json
{ "message": "success", "data": [ { "JID": "120363000000000000@g.us", "OwnerJID": "1234567890@lid", "OwnerPN": "5511999999999@s.whatsapp.net", "Name": "Equipe Vendas", "Topic": "Descrição", "IsLocked": false, "IsAnnounce": false, "IsParent": false, "Participants": [ { "JID": "...", "PhoneNumber": "...", "IsAdmin": true, "IsSuperAdmin": false } ] } ] }
```

### GET /group/myall

**Autenticação:** token da instância | **Para que serve:** só os grupos que a instância criou ou em que é administradora (os que sua aplicação pode gerenciar). **Resposta:** mesmo formato de `/group/list`.

### POST /group/info

**Autenticação:** token da instância | **Para que serve:** metadados de um grupo (nome, descrição, participantes, configurações).

**Body:** `{ "groupJid": "120363000000000000@g.us" }`

**Resposta (200):** `{ "message": "success", "data": { "JID": "...", "Name": "...", "Topic": "...", "Participants": [] } }`

**Erros comuns:** 400 `groupJID is required`; 401; 500 grupo inexistente, instância fora do grupo ou desconectada.

### POST /group/invitelink

**Autenticação:** token da instância | **Para que serve:** link de convite do grupo; opcionalmente revoga o atual e gera outro.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| groupJid | string | Sim | JID do grupo. |
| reset | bool | Não | `true` revoga o link atual e gera um novo. |

**Resposta (200):** `{ "message": "success", "data": "https://chat.whatsapp.com/AbCdEfGhIjK" }`

### POST /group/photo

**Autenticação:** token da instância | **Para que serve:** altera a foto do grupo (a instância precisa ser admin).

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| groupJid | string | Sim | JID do grupo. |
| image | string | Sim | URL `http(s)://` ou data URL `data:image/jpeg;base64,...` / `data:image/png;base64,...`. |

**Resposta (200):** `{ "message": "success", "data": "1712345678" }` (ID da nova foto).

### POST /group/name

**Autenticação:** token da instância | **Para que serve:** renomeia o grupo. **Body:** `{ "groupJid": "120363...@g.us", "name": "Equipe Vendas 2026" }` · **Resposta:** `{ "message": "success" }`

### POST /group/description

**Autenticação:** token da instância | **Para que serve:** altera a descrição (vazio apaga). **Body:** `{ "groupJid": "120363...@g.us", "description": "Grupo oficial de vendas" }` · **Resposta:** `{ "message": "success" }`

### POST /group/create

**Autenticação:** token da instância | **Para que serve:** cria um grupo com os participantes informados; a instância vira dona.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| groupName | string | Sim | Nome do grupo. |
| participants | string\[\] | Sim (mín. 1) | Números ou JIDs. |

**Resposta (200):**

```json
{ "message": "success", "data": { "jid": "120363000000000000@g.us", "name": "Projeto X", "owner": "5511900000000@s.whatsapp.net", "added": ["5511999999999@s.whatsapp.net"], "failed": ["5521988888888@s.whatsapp.net"] } }
```

`failed` lista os que o WhatsApp recusou (privacidade, número inexistente).

### POST /group/participant

**Autenticação:** token da instância | **Para que serve:** adiciona, remove, promove a admin ou rebaixa participantes.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| groupJid | string | Sim | **JID completo** do grupo. |
| participants | string\[\] | Sim (mín. 1) | Números ou JIDs. |
| action | string | Sim | `add`, `remove`, `promote` ou `demote`. |

**Resposta (200):** `{ "message": "success" }` (não traz o resultado por participante; confira em `/group/info`).

### POST /group/join

**Autenticação:** token da instância | **Para que serve:** entra num grupo por convite. **Body:** `{ "code": "https://chat.whatsapp.com/AbCdEfGhIjK" }` (código ou link completo). **Resposta:** `{ "message": "success" }` (o JID do grupo não vem).

### POST /group/leave

**Autenticação:** token da instância | **Para que serve:** sai de um grupo. **Body:** `{ "groupJid": "120363...@g.us" }` (JID completo). **Resposta:** `{ "message": "success" }`

### POST /group/settings

**Autenticação:** token da instância | **Para que serve:** muda quem envia mensagens, quem edita os dados, a aprovação de entrada e quem adiciona membros.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| groupJid | string | Sim | JID do grupo. |
| action | string | Sim | `announcement` (só admins enviam), `not_announcement`, `locked` (só admins editam dados), `unlocked`, `approval_on` (entrada exige aprovação), `approval_off`, `admin_add` (só admins adicionam), `all_member_add`. |

**Resposta (200):** `{ "message": "success" }`

### POST /community/create

**Autenticação:** token da instância | **Para que serve:** cria uma comunidade (grupo "pai" que agrupa outros). **Body:** `{ "communityName": "Clientes Premium" }`

**Resposta (200):** `{ "message": "success", "data": { "JID": "120363...@g.us", "Name": "Clientes Premium", "IsParent": true } }`

### POST /community/add

**Autenticação:** token da instância | **Para que serve:** vincula grupos existentes a uma comunidade.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| communityJid | string | Sim | JID da comunidade. |
| groupJid | string\[\] | Sim (mín. 1) | JIDs dos grupos. Aqui o campo é um **array**. |

**Resposta (200):** `{ "message": "success", "data": { "success": [...], "failed": [] } }`

> A lista `success` não é confiável (pode repetir itens de `failed`). Use **só `failed`** para saber o que não foi vinculado.

### POST /community/remove

**Autenticação:** token da instância | **Para que serve:** desvincula grupos de uma comunidade. **Body e resposta:** iguais a `/community/add` (mesmo aviso sobre `success`).

### GET /label/list

**Autenticação:** token da instância | **Para que serve:** lista as etiquetas do WhatsApp Business da instância.

**Resposta (200)** (array puro, sem envelope):

```json
[ { "id": "uuid-interno", "instance_id": "uuid", "label_id": "1", "label_name": "Novo cliente", "label_color": "0", "predefined_id": "" } ]
```

`label_id` é o valor usado em `labelId` nas outras rotas.

### POST /label/chat

**Autenticação:** token da instância | **Para que serve:** aplica uma etiqueta a uma conversa. **Body:** `{ "jid": "5511999999999", "labelId": "1" }` · **Resposta:** `{ "message": "success" }`

### POST /label/message

**Autenticação:** token da instância | **Para que serve:** aplica uma etiqueta a uma mensagem. **Body:** `{ "jid": "5511999999999", "labelId": "1", "messageId": "3EB0ABCDEF123456" }` · **Resposta:** `{ "message": "success" }`

### POST /label/edit

**Autenticação:** token da instância | **Para que serve:** cria, renomeia, recolore ou exclui uma etiqueta.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| labelId | string | Sim | ID da etiqueta (ID novo cria). |
| name | string | Sim | Nome. |
| color | int | Não | Índice da cor. Padrão `0`. |
| deleted | bool | Não | `true` exclui. |

**Resposta (200):** `{ "message": "success" }`

### POST /unlabel/chat e POST /unlabel/message

**Autenticação:** token da instância | **Para que serve:** remove a etiqueta de uma conversa ou de uma mensagem. **Body:** igual a `/label/chat` e `/label/message`. **Resposta:** `{ "message": "success" }`

### POST /newsletter/create

**Autenticação:** token da instância | **Para que serve:** cria um Canal do WhatsApp. **Body:** `{ "name": "Novidades da Loja", "description": "Promoções semanais" }`

**Resposta (200):** `{ "message": "success", "data": { "id": "120363...@newsletter", "state": {}, "thread_metadata": {}, "viewer_metadata": {} } }`

### GET /newsletter/list

**Autenticação:** token da instância | **Para que serve:** canais que a instância segue ou administra. Pode demorar com muitos canais (consulta um a um).

### POST /newsletter/info

**Autenticação:** token da instância | **Para que serve:** metadados de um canal. **Body:** `{ "jid": "120363000000000000@newsletter" }` (JID completo).

### POST /newsletter/link

**Autenticação:** token da instância | **Para que serve:** descobre o canal pelo código do link `https://whatsapp.com/channel/<key>`. **Body:** `{ "key": "0029VaAbCdEfGhIjKlMn" }`

### POST /newsletter/subscribe

**Autenticação:** token da instância | **Para que serve:** assina as atualizações ao vivo de um canal (visualizações, reações). Não é o mesmo que seguir. **Body:** `{ "jid": "120363...@newsletter" }`

### POST /newsletter/messages

**Autenticação:** token da instância | **Para que serve:** mensagens publicadas num canal, com paginação.

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| jid | string | Sim | JID completo do canal. |
| count | int | Não | Quantidade de mensagens. |
| before\_id | int | Não | Mensagens anteriores a este server ID (snake\_case). |

**Resposta (200):** `{ "message": "success", "data": [ { "MessageServerID": 123, "MessageID": "...", "ViewsCount": 10, "ReactionCounts": {}, "Message": {} } ] }`

### GET /polls/:pollMessageId/results

**Autenticação:** token da instância | **Para que serve:** votos recebidos de uma enquete enviada pela instância, com contagem por opção e votantes.

**Resposta (200)** (objeto puro, sem envelope):

```json
{ "pollMessageId": "3EB0ABCDEF123456", "pollChatJid": "5511999999999@s.whatsapp.net", "totalVotes": 2, "optionCounts": { "<sha256-da-opcao>": 2 }, "voters": [ { "jid": "...", "phone": "5511...", "name": "Fulano", "selectedOptions": ["<sha256>"], "votedAt": "..." } ] }
```

As opções vêm como **hash SHA-256 do texto da opção**; calcule `sha256(textoDaOpcao)` do seu lado para mapear.

**Erros comuns:** 400; 401; **404** `No votes found for this poll` (sem votos ainda **ou** ID errado); 500.

# Integração de correções do upstream — 2026-08-01

Este documento resume as correções selecionadas do repositório
`evolution-foundation/evolution-go` e adaptadas para o fork Athene. A integração
foi feita em lotes para preservar as implementações próprias e permitir
reversão/bisect por assunto.

## Referências da integração

- Branch: `codex/integrate-upstream-fixes`
- Checkpoint anterior à integração: `127990d`
- Correções independentes: `527f76a`
- Mensagens e JIDs: `4a6b3fa`
- Ciclo de vida e reconexão: `d1e6803`
- Imagem de validação: `samuelpc7/evolution-go:test-d1e6803`
- Digest publicado: `sha256:ab1b17fb6c2f8ac043d4d0bd57f957e02d17c11305bc15e053457395ba0b3591`
- Plataforma publicada: `linux/amd64`

## Lote 1 — correções pequenas e independentes

### PR #149 — QR de sessão já autenticada

Evita que a consulta do QR Code reinicie uma sessão que já está autenticada. A
API informa que a sessão já está conectada e mantém o cliente atual.

### PR #135 — escrita concorrente no WebSocket

Serializa as escritas no produtor WebSocket para impedir chamadas concorrentes
ao mesmo socket. Foram adicionados testes para transmissões simultâneas.

### PR #150 — sincronização de histórico

Passa a enviar as mensagens de sincronização de histórico pelo caminho de peer
message do whatsmeow, adequado a esse tipo de operação.

### PR #34 — grupos e identificadores LID

O endpoint de grupos considera o identificador LID da própria conta ao calcular
participação e permissões, mantendo compatibilidade com contas no modelo PN.

### PR #100 — participantes de grupo no middleware

O middleware de `/group/participant` aceita corretamente operações com arrays de
participantes, sem tratar a coleção como um único JID.

### PR #143 — NATS opcional

Não tenta inicializar uma conexão NATS quando a URL está vazia. Inclui teste para
garantir que a configuração opcional não cause falha na inicialização.

## Lote 2 — mensagens e JIDs

### PR #137 — menções

O processamento de menções reconhece mensagens embrulhadas em
`DocumentWithCaptionMessage` e destinatários LID. A adaptação preserva o tipo
próprio `EventMessage` usado pelo fork.

### PR #120 — foto de perfil e resolução de contato

- Normaliza JIDs antes de consultar avatar.
- Converte LID para PN quando o mapeamento está disponível.
- Aplica timeout às consultas externas.
- Trata respostas HTTP 429 e 504.
- Preserva o mutex local dos mapas de clientes e a rotina própria `SaveContact`.

### PR #130 — edição e remoção de mensagens

Foi portada apenas a parte compatível e necessária: canonicalização do JID em
`DeleteMessageEveryone` e `EditMessage`. As demais alterações do PR não foram
aplicadas para não substituir comportamentos próprios do fork.

### PR #151 — stickers WebP animados

Stickers WebP animados são enviados sem conversão estática. A implementação foi
reforçada com:

- detecção da assinatura por `bytes.Equal`;
- aceitação exclusiva de URLs HTTP/HTTPS;
- timeout de 30 segundos no download;
- limite de 16 MiB;
- validação do status HTTP;
- testes dos cenários de WebP e download.

### PR #122 — edição de mensagens com secret

Adiciona suporte à descriptografia de mensagens editadas protegidas por secret,
incluindo flags/configuração e documentação. A adaptação utiliza o mutex já
existente para acesso aos clientes.

## Lote 3 — ciclo de vida e reconexão

O PR #145 não foi aplicado integralmente. Seus conceitos foram consolidados
sobre as correções locais #117, #126 e #127, preservando o container de autenticação
compartilhado e a proteção dos mapas de clientes.

Principais mudanças:

- reserva exclusiva de runtime por instância;
- canais de parada bufferizados e sinalização não bloqueante;
- teardown idempotente, sem fechamento concorrente de canais;
- remoção da reinicialização recursiva de `StartClient`;
- espera pela finalização da goroutine proprietária antes de iniciar outra;
- deduplicação de eventos simultâneos de reconexão;
- backoff exponencial com jitter, limitado a 30 segundos;
- reconexão por `Disconnected` e por três timeouts consecutivos de keepalive;
- centralização do teardown usado por `Disconnect`, `Logout`, `Delete` e
  `ForceReconnect`;
- receivers convertidos para ponteiro para que os mutexes não sejam copiados.

Os testes de ciclo de vida cobrem reserva concorrente, token de propriedade,
teardown repetido, deduplicação de reconexões e cálculo do backoff.

## Validação executada

Após a integração foram executados:

```text
go test ./...
go test -race ./pkg/whatsmeow/service ./pkg/instance/service
git diff --check
```

Todos finalizaram com sucesso. A imagem Docker de teste também foi construída e
publicada no Docker Hub com o digest indicado no início deste documento.

## Pontos conhecidos para acompanhamento

### PIX isolado

O botão PIX deve ser o único botão da mensagem e requer uma chave PIX válida,
com `keyType` compatível (`phone`, `email`, `cpf`, `cnpj` ou `random`). Nos testes
de 2026-08-01, o servidor aceitou e entregou os stanzas interativos; uma falha de
renderização no aparelho deve ser investigada com o payload utilizado e logs em
`WADEBUG=DEBUG`.

### Lista com seções

Em 2026-08-01, `/send/list` chegou ao servidor WhatsApp, mas o formato legado
`ListMessage` foi rejeitado com erro 405. Esse comportamento já existia na base
anterior à integração dos PRs e não foi causado pelos três lotes. Deve ser
reavaliado separadamente, comparando o envelope e os nodes adicionais com uma
implementação atualmente aceita pelo WhatsApp.

## Orientação para atualizações futuras

Ao sincronizar novamente com o upstream, verificar primeiro se os PRs listados
acima já foram incorporados à base escolhida. Para mudanças no ciclo de vida,
manter a regra de um único runtime e uma única tentativa de reconexão por
instância. Para mensagens interativas, validar tanto a resposta da API quanto a
renderização real em Android, iOS e WhatsApp Web.

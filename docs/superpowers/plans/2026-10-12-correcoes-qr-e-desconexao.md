# Correções de QR e desconexão — Plano de implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fazer a Evolution GO 0.8.0 sair com o "Não foi possível conectar" do QR e as falhas de reconexão corrigidos, e alinhar o evoproxy e o frontend a esse comportamento.

**Architecture:** Na EvGo, o laço que emite os QRs sai de `handleQRCodes` para uma função pura (`runQREmitter`) que só conversa com o runtime por uma interface. Isso permite prendê-la à vida do runtime, tratar a queda do socket e a troca da chave ADV, e testar tudo sem WhatsApp. O resto são mudanças pequenas e locais: o QR passa a ser guardado com validade, o teardown sinaliza o próprio runtime, o supervisor respeita ban e sessão substituída, e há uma verificação periódica. O proxy e o frontend só consomem os campos novos (`needsQr`, `expiresAt`, `disconnect_reason`).

**Tech Stack:** Go 1.26 + whatsmeow `c386243a72ba` (EvGo); Node 22 + Express + `node:test` (evoproxy-backend); React + TypeScript + Vitest (Mobilium-Frontend-V2).

**Spec:** [`docs/DIAGNOSTICO-QR-E-DESCONEXAO.md`](../../DIAGNOSTICO-QR-E-DESCONEXAO.md) — causas, evidências e o plano numerado (Q1–Q8, D1–D7, P1–P2, F1–F4, A1–A6) de onde vêm estas tarefas.

## Global Constraints

- **Produção (0.7.6, branch `main`) não é alterada.** Todas as correções da EvGo entram na branch `feat/upstream-sync-2026-10` e saem na 0.8.0.
- **EvGo sem Go local:** testes com `bash scripts/go-test.sh <pacote> -run <Teste> -v` (roda `go test` num container `golang:1.26.8-alpine`); compilação com `docker build --target build -t evo-go-check .`. O Docker Desktop não sobe sozinho: abra-o e espere `docker info` responder antes.
- **Git Bash:** `cd` dispara um hook do `fnm` que engole a saída. Rode os comandos pelo PowerShell (`Set-Location C:\Users\Maxsuel\Projetos\evo-go-v2; bash scripts/go-test.sh ...`) ou com caminhos absolutos.
- **Muitos arquivos Go estão em CRLF.** Edite com a ferramenta de edição (que preserva o fim de linha), não com `sed` cego.
- **Comentários e logs:** comentários em português, como no código existente; mensagens de log em inglês, como as vizinhas.
- **Commits:** Conventional Commits em inglês (`fix(qr): ...`), terminando com a linha `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`. Não fazer push sem pedido.
- **evoproxy-backend:** branch `fix/alinhamento-evogo-0.8.0`, testes com `npm test`, comentários e mensagens em português.
- **Mobilium-Frontend-V2:** criar a branch `fix/whatsapp-qr-tempos` a partir de `development`; seguir o `CLAUDE.md` do repositório; `npm test`, `npx tsc -b` e `npm run lint` verdes antes do commit.
- **Compatibilidade:** nenhum campo existente de resposta ou webhook é removido nem muda de tipo. Campos novos são opcionais para quem consome.

## Review Focus

1. **Passkey durante o QR:** com uma cerimônia de passkey ativa, o socket tem de continuar vivo, inclusive na checagem periódica do socket. Teste em Task 2 (`TestQREmitterKeepsSocketForPasskey`).
2. **Reconexão 515 logo após o pareamento:** o socket cai por um instante depois do `PairSuccess`. Isso não pode virar teardown do QR. Teste em Task 2 (`TestQREmitterLeavesPairedSessionAlone`).
3. **QR gravado antes da atualização:** valores antigos da coluna `qrcode` (`imagem|código`, sem validade) precisam continuar sendo servidos depois do deploy. Teste em Task 1 (`TestParseStoredQRLegacyTwoFields`).
4. **Verificação periódica contra runtime que ainda conecta:** a verificação não pode reiniciar um runtime recém-criado (conectando ou mostrando QR). Teste em Task 10 (`TestWatchdogTargetsSkipsYoungRuntime`).
5. **Ban sem prazo informado** (`Expire == 0`): o supervisor também precisa esperar. Teste em Task 8 (`TestTemporaryBanUntilDefaultsWhenExpiryUnknown`).

## Mapa: itens do diagnóstico → tarefas

| Item | Correção | Tarefas |
|---|---|---|
| 1 | Troca da chave do QR (`RotateADVSecret`) | 3 |
| 2 | Rotina de QR presa ao runtime; QR antigo apagado | 1, 2, 4, 5 |
| 3 | `GET /qr` não reinicia sessão que ainda conecta | 6 |
| 4 | Registrar ban, sessão substituída e cliente desatualizado; supervisor respeita o ban | 8, 9 |
| 5 | Proxy: "precisa parear" pelo motivo da desconexão | 11, 13 |
| 6 | Frontend: tempos do QR | 15 (documento para as equipes de frontend, não implementado aqui) |
| 7 | `PairError` por webhook; sem `LoggedOut` no fim do QR | 4, 7 |
| 8 | Verificação periódica + espera longa | 10 |
| 9 | Alertas | 14, 17, 18 |
| 10 | Janela de 160s (`QRCODE_MAX_COUNT`) | 12 |

**Decisões do item 9 (alertas):** o alerta vive no status da instância (`alertas`, Task 14), que os frontends já consultam, com a hora em que a instância caiu (`desconectadaDesde`, Task 17, guardada no Redis do proxy — sem coluna nova na EvGo), e numa visão de plataforma `GET /admin/alertas` (Task 18) para o backoffice. Não há canal novo de notificação (e-mail/WhatsApp) nesta etapa. A heurística dos "14 dias sem usar o celular" não é implementada: o WhatsApp não documenta o critério, e um aviso impreciso acostuma a equipe a ignorar avisos.

**Execução:** Partes A, B e D são implementadas. A Parte C (frontend) é entregue como documento de instruções para as equipes do Mobilium-Frontend-V2 e do mobpainel, que aplicam a correção nos seus repositórios.

**Ordem obrigatória:** 1 → 2 → 3 (a 3 muda o laço da 2). As tarefas 4–12 dependem só da 2 quando indicado em **Interfaces**. As tarefas 13–14 (proxy) funcionam com qualquer EvGo, mas aproveitam o `needsQr` da 11. A 15 (frontend) funciona com qualquer EvGo e aproveita o `expiresAt` da 1.

## Mapa de arquivos

| Arquivo | Responsabilidade | Tarefas |
|---|---|---|
| `pkg/utils/qr_store.go` (novo) | Formato do QR guardado na coluna `qrcode`, com validade | 1 |
| `pkg/instance/service/instance_service.go` | `GetQr` lê o QR com validade; carência de conexão; `needsQr` em `GetAll`/`Info` | 1, 6, 11 |
| `pkg/whatsmeow/service/qr_emitter.go` (novo) | Laço puro de emissão dos QRs | 2, 3 |
| `pkg/whatsmeow/service/qr_env.go` (novo) | Liga o laço ao `MyClient` real | 2, 3 |
| `pkg/whatsmeow/service/whatsmeow.go` | Campos do `MyClient`, `handleQRCodes`, `teardownQR`, `StartClient`, eventos | 2–5, 7, 8, 10 |
| `pkg/whatsmeow/service/runtime_lifecycle.go` | Idade do runtime, espera do ban, espera longa, verificação periódica | 6, 9, 10 |
| `pkg/whatsmeow/service/event_groups.go` | Grupo de assinatura dos eventos novos | 7, 8 |
| `pkg/instance/model/instance_model.go` + `disconnect_reason.go` (novo) | Motivos de desconexão e `RequiresQR` | 2, 8, 11 |
| `pkg/config/config.go` | Padrão de `QRCODE_MAX_COUNT` | 12 |
| `docs/CHANGELOG-0.8.0-E-CATALOGO-DE-ROTAS.md`, `docs/wiki/...` | Documentação para integradores | 1, 7, 8, 11, 12 |
| evoproxy `src/lib/instanceLive.js` | `precisaParear`, `motivoDesconexao`, `alertas` | 13, 14 |
| frontend `src/lib/whatsapp-qr.ts` (novo), `whattsapp-card.tsx`, `timing.ts`, `api/integrations/whattsapp/index.ts` | Validade do QR na tela | 15 |

---

## Parte A — Evolution GO (branch `feat/upstream-sync-2026-10`)

### Task 1: QR guardado com validade

Hoje a coluna guarda `"<imagem>|<código>"` sem validade, e o `GET /instance/qr` devolve o que estiver lá, mesmo vencido. Passa a guardar `"<imagem>|<código>|<expira em unix>"`, e o `GetQr` recusa código vencido e informa a validade.

**Files:**
- Create: `pkg/utils/qr_store.go`
- Create: `pkg/utils/qr_store_test.go`
- Modify: `pkg/instance/service/instance_service.go` (struct `QrcodeStruct`, linhas ~92-105; final de `GetQr`, do `code := instance.Qrcode` até o `return qr, nil`)
- Create: `pkg/instance/service/qr_fresh_test.go`
- Modify: `docs/CHANGELOG-0.8.0-E-CATALOGO-DE-ROTAS.md`

**Interfaces:**
- Produces: `utils.StoredQR{Image, Code string; ExpiresAt time.Time}`, `utils.EncodeStoredQR(image, code string, expiresAt time.Time) string`, `utils.ParseStoredQR(raw string) (utils.StoredQR, bool)`, `(utils.StoredQR).Expired(now time.Time) bool`. Usados pela Task 2.

- [ ] **Step 1: Escrever os testes que falham**

`pkg/utils/qr_store_test.go`:

```go
package utils

import (
	"testing"
	"time"
)

func TestStoredQRRoundTrip(t *testing.T) {
	expires := time.Unix(1_800_000_000, 0)
	raw := EncodeStoredQR("data:image/png;base64,AAA", "2@ref,pub,id,adv", expires)

	q, ok := ParseStoredQR(raw)
	if !ok {
		t.Fatalf("ParseStoredQR(%q) rejected a value written by EncodeStoredQR", raw)
	}
	if q.Image != "data:image/png;base64,AAA" || q.Code != "2@ref,pub,id,adv" || !q.ExpiresAt.Equal(expires) {
		t.Fatalf("round trip mismatch: %+v", q)
	}
}

// Valores gravados antes desta versão (sem validade) continuam servidos.
func TestParseStoredQRLegacyTwoFields(t *testing.T) {
	q, ok := ParseStoredQR("img|code")
	if !ok || q.Image != "img" || q.Code != "code" {
		t.Fatalf("legacy value rejected: %+v ok=%v", q, ok)
	}
	if !q.ExpiresAt.IsZero() || q.Expired(time.Now()) {
		t.Fatalf("legacy value must have unknown validity and never expire, got %+v", q)
	}
}

func TestParseStoredQRRejectsMalformed(t *testing.T) {
	for _, raw := range []string{"", "img", "img|", "|code", "img|code|not-a-number"} {
		if _, ok := ParseStoredQR(raw); ok {
			t.Errorf("ParseStoredQR(%q) accepted a malformed value", raw)
		}
	}
}

func TestStoredQRExpired(t *testing.T) {
	q := StoredQR{Image: "i", Code: "c", ExpiresAt: time.Unix(100, 0)}
	if q.Expired(time.Unix(99, 0)) {
		t.Fatal("expired one second before its deadline")
	}
	if !q.Expired(time.Unix(100, 0)) {
		t.Fatal("still valid at its deadline")
	}
}
```

`pkg/instance/service/qr_fresh_test.go`:

```go
package instance_service

import (
	"testing"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/utils"
)

func TestReadFreshQR(t *testing.T) {
	fresh := utils.EncodeStoredQR("img", "code", time.Now().Add(time.Minute))
	stale := utils.EncodeStoredQR("img", "code", time.Now().Add(-time.Second))

	if q, ok := readFreshQR(fresh); !ok || q.Code != "code" {
		t.Fatalf("fresh QR rejected: %+v ok=%v", q, ok)
	}
	if _, ok := readFreshQR(stale); ok {
		t.Fatal("expired QR was served")
	}
	if _, ok := readFreshQR(""); ok {
		t.Fatal("empty column was served")
	}
	if q, ok := readFreshQR("img|code"); !ok || q.Code != "code" {
		t.Fatalf("legacy QR rejected: %+v ok=%v", q, ok)
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `bash scripts/go-test.sh ./pkg/utils/ ./pkg/instance/service/ -run "StoredQR|ReadFreshQR" -v`
Expected: FAIL na compilação — `undefined: EncodeStoredQR`, `undefined: readFreshQR`.

- [ ] **Step 3: Implementar**

`pkg/utils/qr_store.go`:

```go
package utils

import (
	"strconv"
	"strings"
	"time"
)

// StoredQR é o QR guardado na coluna instance.qrcode, no formato
// "<imagem>|<código>|<expira em unix>". Valores gravados antes da validade
// existir têm só dois campos: ExpiresAt fica zero (validade desconhecida).
type StoredQR struct {
	Image     string
	Code      string
	ExpiresAt time.Time
}

// EncodeStoredQR monta o valor da coluna. Nem a imagem (base64) nem o código
// do WhatsApp (campos separados por vírgula) contêm "|".
func EncodeStoredQR(image, code string, expiresAt time.Time) string {
	return image + "|" + code + "|" + strconv.FormatInt(expiresAt.Unix(), 10)
}

// ParseStoredQR lê o valor da coluna; false se estiver vazio ou malformado.
func ParseStoredQR(raw string) (StoredQR, bool) {
	parts := strings.SplitN(raw, "|", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return StoredQR{}, false
	}
	q := StoredQR{Image: parts[0], Code: parts[1]}
	if len(parts) == 3 {
		unix, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return StoredQR{}, false
		}
		q.ExpiresAt = time.Unix(unix, 0)
	}
	return q, true
}

// Expired diz se o código já venceu. Validade desconhecida nunca vence.
func (q StoredQR) Expired(now time.Time) bool {
	return !q.ExpiresAt.IsZero() && !now.Before(q.ExpiresAt)
}
```

Em `pkg/instance/service/instance_service.go`, acrescente ao `QrcodeStruct`, logo depois de `Code string \`json:"code"\``:

```go
	// Validade do código (unix, segundos) e quantos segundos faltam, quando a
	// EvGo sabe. O primeiro código vale ~60 s e os seguintes ~20 s.
	ExpiresAt int64 `json:"expiresAt,omitempty"`
	ExpiresIn int   `json:"expiresIn,omitempty"`
```

Substitua o final de `GetQr`, de `code := instance.Qrcode` até `return qr, nil` (inclusive), por:

```go
	stored, ok := readFreshQR(instance.Qrcode)
	if !ok {
		// Sem QR válido ainda (sessão subindo ou código recém-vencido): espera
		// a próxima rotação antes de desistir.
		logger.LogInfo("[%s] No valid QR code yet, waiting a bit more...", instance.Id)
		time.Sleep(2 * time.Second)

		instance, err = i.instanceRepository.GetInstanceByID(instance.Id)
		if err != nil {
			return nil, err
		}
		if stored, ok = readFreshQR(instance.Qrcode); !ok {
			return nil, fmt.Errorf("no QR code available. Please wait a moment and try again")
		}
	}

	qr := &QrcodeStruct{
		Qrcode: stored.Image,
		Code:   stored.Code,
	}
	if !stored.ExpiresAt.IsZero() {
		qr.ExpiresAt = stored.ExpiresAt.Unix()
		qr.ExpiresIn = int(time.Until(stored.ExpiresAt).Seconds())
	}

	return qr, nil
}

// readFreshQR devolve o QR guardado se ele existir e ainda não tiver vencido.
func readFreshQR(raw string) (utils.StoredQR, bool) {
	q, ok := utils.ParseStoredQR(raw)
	if !ok || q.Expired(time.Now()) {
		return utils.StoredQR{}, false
	}
	return q, true
```

(O `}` que fechava `GetQr` passa a fechar `readFreshQR`. `utils`, `time` e `fmt` já são importados no arquivo.)

- [ ] **Step 4: Rodar e ver passar**

Run: `bash scripts/go-test.sh ./pkg/utils/ ./pkg/instance/service/ -v -run "StoredQR|ReadFreshQR"`
Expected: PASS nos 5 testes.

- [ ] **Step 5: Documentar**

Em `docs/CHANGELOG-0.8.0-E-CATALOGO-DE-ROTAS.md`, na tabela "Changelog — campos e comportamento das rotas existentes", acrescente a linha:

```markdown
| `GET /instance/qr` | Campo novo na resposta | `expiresAt` (unix), `expiresIn` (segundos) | Quando o código vence. O primeiro vale ~60 s e os seguintes ~20 s. Código vencido não é mais devolvido: a rota espera a próxima rotação ou responde `no QR code available`. |
```

E, na seção `### GET /instance/qr`, troque o exemplo de resposta por:

```markdown
{ "message": "success", "data": { "qrcode": "data:image/png;base64,iVBOR...", "code": "2@AbCd...,...", "expiresAt": 1792000000, "expiresIn": 54 } }
```

- [ ] **Step 6: Commit**

```bash
git add pkg/utils/qr_store.go pkg/utils/qr_store_test.go pkg/instance/service/instance_service.go pkg/instance/service/qr_fresh_test.go docs/CHANGELOG-0.8.0-E-CATALOGO-DE-ROTAS.md
git commit -m "fix(qr): store the QR with its expiry and never serve an expired code"
```

---

### Task 2: Emissor de QR preso ao runtime e ao socket

Causas A e B do diagnóstico. A rotina de `handleQRCodes` dormia 60s/20s sem olhar se o runtime ainda existia ou se o socket estava vivo. Ela passa a ser `runQREmitter`, que sai quando o runtime termina, derruba a sessão quando o socket cai e grava a validade de cada código.

**Files:**
- Create: `pkg/whatsmeow/service/qr_emitter.go`
- Create: `pkg/whatsmeow/service/qr_emitter_test.go`
- Create: `pkg/whatsmeow/service/qr_env.go`
- Create: `pkg/instance/model/disconnect_reason.go`
- Modify: `pkg/whatsmeow/service/whatsmeow.go` (struct `MyClient` ~134-183; literal `mycli = &MyClient{...}` ~559-590; `defer` do `StartClient` ~368-389; `handleQRCodes` ~781-878)

**Interfaces:**
- Consumes: `utils.EncodeStoredQR` (Task 1).
- Produces: `qrEmitterEnv` (interface), `runQREmitter(env qrEmitterEnv, codes []string, count int, timing qrTiming) (qrEmitterEnd, int)`, `qrWait`, `qrCheck`, `qrTiming{First, Next, Poll}`, `defaultQRTiming`, os valores `qrEndPaired/qrEndStopped/qrEndSocketLost/qrEndMaxCount/qrEndExhausted`, o tipo `qrEnv{mycli *MyClient}`, o campo `MyClient.runtimeDone chan struct{}` e a constante `instance_model.DisconnectReasonQRSocketLost`. A Task 3 estende a interface e o laço; a Task 11 usa a constante.

- [ ] **Step 1: Escrever os testes que falham**

`pkg/whatsmeow/service/qr_emitter_test.go`:

```go
package whatsmeow_service

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
)

type publishedQR struct {
	code      string
	count     int
	expiresAt time.Time
}

type teardownCall struct {
	reason      string
	forceLogout bool
}

// fakeQREnv simula o runtime para o emissor, sem WhatsApp.
type fakeQREnv struct {
	mu        sync.Mutex
	stopped   chan struct{}
	connected atomic.Bool
	paired    atomic.Bool
	passkey   atomic.Bool
	maxCount  int
	now       time.Time
	published []publishedQR
	teardowns []teardownCall
}

func newFakeQREnv() *fakeQREnv {
	env := &fakeQREnv{stopped: make(chan struct{}), now: time.Unix(1000, 0)}
	env.connected.Store(true)
	return env
}

func (e *fakeQREnv) Stopped() <-chan struct{} { return e.stopped }
func (e *fakeQREnv) Connected() bool          { return e.connected.Load() }
func (e *fakeQREnv) Paired() bool             { return e.paired.Load() }
func (e *fakeQREnv) PasskeyActive() bool      { return e.passkey.Load() }
func (e *fakeQREnv) MaxCount() int            { return e.maxCount }
func (e *fakeQREnv) Now() time.Time           { return e.now }

func (e *fakeQREnv) Publish(code string, count int, expiresAt time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.published = append(e.published, publishedQR{code, count, expiresAt})
}

func (e *fakeQREnv) Teardown(reason string, forceLogout bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.teardowns = append(e.teardowns, teardownCall{reason, forceLogout})
}

func (e *fakeQREnv) snapshot() ([]publishedQR, []teardownCall) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]publishedQR(nil), e.published...), append([]teardownCall(nil), e.teardowns...)
}

// waitPublished espera o emissor publicar n códigos (ou falha o teste).
func (e *fakeQREnv) waitPublished(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if published, _ := e.snapshot(); len(published) >= n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("emitter did not publish %d codes in time", n)
}

var fastQRTiming = qrTiming{First: 60 * time.Millisecond, Next: 30 * time.Millisecond, Poll: 2 * time.Millisecond}

type emitterResult struct {
	end   qrEmitterEnd
	count int
}

func runAsync(env *fakeQREnv, codes []string, count int) <-chan emitterResult {
	out := make(chan emitterResult, 1)
	go func() {
		end, n := runQREmitter(env, codes, count, fastQRTiming)
		out <- emitterResult{end, n}
	}()
	return out
}

func waitResult(t *testing.T, ch <-chan emitterResult) emitterResult {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(2 * time.Second):
		t.Fatal("emitter did not return in time")
		return emitterResult{}
	}
}

func TestQREmitterPublishesAllCodesThenTearsDown(t *testing.T) {
	env := newFakeQREnv()
	r := waitResult(t, runAsync(env, []string{"a", "b", "c"}, 0))

	if r.end != qrEndExhausted || r.count != 3 {
		t.Fatalf("end=%v count=%d, want exhausted/3", r.end, r.count)
	}
	published, teardowns := env.snapshot()
	if len(published) != 3 || published[0].code != "a" || published[2].code != "c" || published[2].count != 3 {
		t.Fatalf("published = %+v", published)
	}
	if !published[0].expiresAt.Equal(env.now.Add(fastQRTiming.First)) || !published[1].expiresAt.Equal(env.now.Add(fastQRTiming.Next)) {
		t.Fatalf("first code must live First and the next ones Next: %+v", published)
	}
	if len(teardowns) != 1 || teardowns[0] != (teardownCall{"", false}) {
		t.Fatalf("teardowns = %+v, want one plain QR timeout", teardowns)
	}
}

// Runtime encerrado (restart, logout, reconnect): a rotina antiga para em
// silêncio — não sobrescreve o QR do runtime novo nem o derruba.
func TestQREmitterStopsWithRuntimeWithoutTeardown(t *testing.T) {
	env := newFakeQREnv()
	result := runAsync(env, []string{"a", "b", "c"}, 0)
	env.waitPublished(t, 1)
	close(env.stopped)

	r := waitResult(t, result)
	published, teardowns := env.snapshot()
	if r.end != qrEndStopped || len(published) != 1 || len(teardowns) != 0 {
		t.Fatalf("end=%v published=%d teardowns=%+v, want stopped/1/none", r.end, len(published), teardowns)
	}
}

// Socket do QR caiu: o código na tela morreu; derruba a sessão para o próximo
// GET /qr subir uma nova em vez de servir códigos de um socket morto.
func TestQREmitterTearsDownWhenSocketDrops(t *testing.T) {
	env := newFakeQREnv()
	result := runAsync(env, []string{"a", "b", "c"}, 0)
	env.waitPublished(t, 1)
	env.connected.Store(false)

	r := waitResult(t, result)
	published, teardowns := env.snapshot()
	want := teardownCall{instance_model.DisconnectReasonQRSocketLost, false}
	if r.end != qrEndSocketLost || len(published) != 1 || len(teardowns) != 1 || teardowns[0] != want {
		t.Fatalf("end=%v published=%d teardowns=%+v", r.end, len(published), teardowns)
	}
}

// Depois do PairSuccess o WhatsApp derruba o socket (515) e reconecta: isso
// não é queda do QR.
func TestQREmitterLeavesPairedSessionAlone(t *testing.T) {
	env := newFakeQREnv()
	result := runAsync(env, []string{"a", "b"}, 0)
	env.waitPublished(t, 1)
	env.paired.Store(true)
	env.connected.Store(false)

	r := waitResult(t, result)
	if _, teardowns := env.snapshot(); r.end != qrEndPaired || len(teardowns) != 0 {
		t.Fatalf("end=%v teardowns=%+v, want paired/none", r.end, teardowns)
	}
}

func TestQREmitterKeepsSocketForPasskey(t *testing.T) {
	env := newFakeQREnv()
	result := runAsync(env, []string{"a", "b"}, 0)
	env.waitPublished(t, 1)
	env.passkey.Store(true)

	r := waitResult(t, result)
	if _, teardowns := env.snapshot(); r.end != qrEndPaired || len(teardowns) != 0 {
		t.Fatalf("end=%v teardowns=%+v, want paired/none", r.end, teardowns)
	}
}

func TestQREmitterMaxCount(t *testing.T) {
	env := newFakeQREnv()
	env.maxCount = 2
	r := waitResult(t, runAsync(env, []string{"a", "b", "c"}, 0))

	published, teardowns := env.snapshot()
	want := teardownCall{"Maximum QR code count (2) reached", true}
	if r.end != qrEndMaxCount || len(published) != 1 || len(teardowns) != 1 || teardowns[0] != want {
		t.Fatalf("end=%v published=%+v teardowns=%+v", r.end, published, teardowns)
	}
}

// A contagem continua entre lotes de códigos do mesmo runtime.
func TestQREmitterCountContinuesAcrossBatches(t *testing.T) {
	env := newFakeQREnv()
	env.maxCount = 5
	r := waitResult(t, runAsync(env, []string{"a", "b"}, 3))

	published, _ := env.snapshot()
	if r.end != qrEndMaxCount || r.count != 5 || len(published) != 1 || published[0].count != 4 {
		t.Fatalf("end=%v count=%d published=%+v", r.end, r.count, published)
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `bash scripts/go-test.sh ./pkg/whatsmeow/service/ -run TestQREmitter -v`
Expected: FAIL na compilação — `undefined: runQREmitter`, `undefined: qrTiming`, `undefined: instance_model.DisconnectReasonQRSocketLost`.

- [ ] **Step 3: Implementar o motivo e o emissor**

`pkg/instance/model/disconnect_reason.go`:

```go
package instance_model

// DisconnectReasonQRSocketLost: o socket da sessão de QR caiu antes do scan. O
// código na tela morreu junto; é preciso gerar outro.
const DisconnectReasonQRSocketLost = "QR socket closed before pairing"
```

`pkg/whatsmeow/service/qr_emitter.go`:

```go
package whatsmeow_service

import (
	"fmt"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
)

// qrEmitterEnv é o que o emissor de QR precisa do runtime. Existe para o laço
// ser testado sem um cliente do WhatsApp (ver qr_emitter_test.go).
type qrEmitterEnv interface {
	// Stopped fecha quando o runtime que recebeu os códigos termina.
	Stopped() <-chan struct{}
	Connected() bool
	Paired() bool
	PasskeyActive() bool
	// Publish grava o código (com a validade) e avisa por webhook.
	Publish(code string, count int, expiresAt time.Time)
	// MaxCount é o QRCODE_MAX_COUNT (0 = sem limite).
	MaxCount() int
	// Teardown limpa o QR, emite QRTimeout e encerra ESTE runtime.
	Teardown(reason string, forceLogout bool)
	Now() time.Time
}

type qrTiming struct {
	First time.Duration // validade do 1º código do lote
	Next  time.Duration // validade dos seguintes
	Poll  time.Duration // intervalo da checagem do socket enquanto espera
}

// Mesma cadência do whatsmeow: o 1º código vive ~60 s e os demais ~20 s.
var defaultQRTiming = qrTiming{First: 60 * time.Second, Next: 20 * time.Second, Poll: 2 * time.Second}

type qrEmitterEnd int

const (
	qrEndPaired     qrEmitterEnd = iota // pareou ou há passkey em andamento: sai sem derrubar
	qrEndStopped                        // o runtime terminou por outro motivo: sai sem derrubar
	qrEndSocketLost                     // o socket do QR caiu: derruba
	qrEndMaxCount                       // atingiu QRCODE_MAX_COUNT: derruba com logout
	qrEndExhausted                      // acabaram os códigos sem scan: derruba
)

// runQREmitter publica os códigos com a cadência do WhatsApp até o scan, o fim
// do runtime, a queda do socket ou o fim dos códigos. count é a contagem já
// usada neste runtime (QRCODE_MAX_COUNT vale para o runtime, não para o lote);
// a nova contagem volta junto com o motivo do fim.
func runQREmitter(env qrEmitterEnv, codes []string, count int, timing qrTiming) (qrEmitterEnd, int) {
	for i := 0; i < len(codes); i++ {
		if end, done := qrCheck(env); done {
			return end, count
		}

		count++
		if max := env.MaxCount(); max > 0 && count >= max {
			env.Teardown(fmt.Sprintf("Maximum QR code count (%d) reached", max), true)
			return qrEndMaxCount, count
		}

		lifetime := timing.Next
		if i == 0 {
			lifetime = timing.First
		}
		env.Publish(codes[i], count, env.Now().Add(lifetime))

		if end, done := qrWait(env, lifetime, timing.Poll); done {
			return end, count
		}
	}

	if env.Paired() || env.PasskeyActive() {
		return qrEndPaired, count
	}
	env.Teardown("", false)
	return qrEndExhausted, count
}

// qrCheck vê se algo encerra a emissão agora.
func qrCheck(env qrEmitterEnv) (qrEmitterEnd, bool) {
	select {
	case <-env.Stopped():
		return qrEndStopped, true
	default:
	}
	// Pareado vem antes do socket: depois do PairSuccess o WhatsApp derruba o
	// socket (515) para reconectar já logado, e isso não é queda do QR.
	if env.Paired() || env.PasskeyActive() {
		return qrEndPaired, true
	}
	if !env.Connected() {
		env.Teardown(instance_model.DisconnectReasonQRSocketLost, false)
		return qrEndSocketLost, true
	}
	return 0, false
}

// qrWait espera a validade do código atual, conferindo o runtime e o socket a
// cada poll. done=true quando a emissão acabou durante a espera.
func qrWait(env qrEmitterEnv, lifetime, poll time.Duration) (qrEmitterEnd, bool) {
	deadline := time.NewTimer(lifetime)
	defer deadline.Stop()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	for {
		select {
		case <-deadline.C:
			return 0, false
		case <-env.Stopped():
			return qrEndStopped, true
		case <-ticker.C:
			if end, done := qrCheck(env); done {
				return end, true
			}
		}
	}
}
```

- [ ] **Step 4: Rodar e ver passar**

Run: `bash scripts/go-test.sh ./pkg/whatsmeow/service/ -run TestQREmitter -v -race`
Expected: PASS nos 7 testes, sem data race.

- [ ] **Step 5: Ligar o emissor ao runtime real**

`pkg/whatsmeow/service/qr_env.go`:

```go
package whatsmeow_service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"

	"github.com/evolution-foundation/evolution-go/pkg/utils"
)

// qrEnv liga o emissor de QR (qr_emitter.go) a um runtime real.
type qrEnv struct {
	mycli *MyClient
}

func (e qrEnv) Stopped() <-chan struct{} { return e.mycli.runtimeDone }

func (e qrEnv) Connected() bool {
	client := e.mycli.WAClient
	return client != nil && client.IsConnected()
}

// Paired também é true sem cliente: não há mais o que emitir.
func (e qrEnv) Paired() bool {
	client := e.mycli.WAClient
	return client == nil || client.Store.ID != nil
}

func (e qrEnv) PasskeyActive() bool {
	ceremony := e.mycli.passkeyCeremony
	return ceremony != nil && ceremony.HasActiveByInstance(e.mycli.userID)
}

func (e qrEnv) MaxCount() int  { return e.mycli.config.QrcodeMaxCount }
func (e qrEnv) Now() time.Time { return time.Now() }

func (e qrEnv) Teardown(reason string, forceLogout bool) {
	mycli := e.mycli
	if forceLogout && mycli.WAClient != nil && mycli.WAClient.IsConnected() {
		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] %s, forcing logout and QRTimeout", mycli.userID, reason)
		if err := mycli.WAClient.Logout(context.Background()); err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Error during forced logout: %v", mycli.userID, err)
		}
	}
	mycli.teardownQR(reason, forceLogout)
}

func (e qrEnv) Publish(code string, count int, expiresAt time.Time) {
	mycli := e.mycli
	instanceID := mycli.userID

	if mycli.config.QrcodeMaxCount > 0 {
		mycli.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] QR code generated #%d (max: %d)", instanceID, count, mycli.config.QrcodeMaxCount)
	} else {
		mycli.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] QR code generated #%d (limit disabled)", instanceID, count)
	}
	if mycli.config.LogType != "json" {
		fmt.Println("QR code:\n", code)
	}

	image, _ := qrcode.Encode(code, qrcode.Medium, 256)
	base64qrcode := "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)
	if err := mycli.instanceRepository.UpdateQrcode(instanceID, utils.EncodeStoredQR(base64qrcode, code, expiresAt)); err != nil {
		mycli.loggerWrapper.GetLogger(instanceID).LogError("[%s] Error updating instance: %s", instanceID, err)
	}

	postMap := map[string]interface{}{
		"event": "QRCode",
		"data": map[string]interface{}{
			"qrcode":    base64qrcode,
			"code":      code,
			"count":     count,
			"maxCount":  mycli.config.QrcodeMaxCount,
			"expiresAt": expiresAt.Unix(),
		},
		"instanceToken": mycli.token,
		"instanceId":    instanceID,
		"instanceName":  mycli.currentInstance().Name,
	}
	queueName := strings.ToLower(fmt.Sprintf("%s.%s", instanceID, "QRCode"))
	values, err := json.Marshal(postMap)
	if err != nil {
		mycli.loggerWrapper.GetLogger(instanceID).LogError("[%s] Failed to marshal JSON for queue", instanceID)
		return
	}
	go mycli.service.CallWebhook(mycli.currentInstance(), queueName, values)
	if mycli.config.AmqpGlobalEnabled || mycli.config.NatsGlobalEnabled {
		go mycli.service.SendToGlobalQueues("QRCode", values, instanceID)
	}
}
```

Em `pkg/whatsmeow/service/whatsmeow.go`:

1. No struct `MyClient`, depois de `qrcodeCount int`, acrescente:

```go
	// runtimeDone fecha quando o StartClient deste runtime termina; o emissor
	// de QR usa para não sobreviver ao runtime que o criou.
	runtimeDone chan struct{}
```

2. No literal `mycli = &MyClient{...}` do `StartClient`, depois de `qrcodeCount: 0,`, acrescente `runtimeDone: make(chan struct{}),`.

3. No `defer func() {` do início do `StartClient`, como primeira linha dentro dele, acrescente:

```go
		if mycli != nil {
			close(mycli.runtimeDone)
		}
```

4. Substitua a função `handleQRCodes` inteira (e o comentário de documentação acima dela) por:

```go
// handleQRCodes emite um lote de QR (events.QR.Codes) sem usar o
// GetQRChannel: o qrChannel do whatsmeow confirma a passkey sozinho
// (SkipHandoffUX) e derruba o socket no fim dos códigos, o que quebra o
// pareamento por passkey (DOC2 §4.3/§4.4). O laço fica em runQREmitter, preso
// à vida deste runtime e ao estado do socket (qr_emitter.go).
func (mycli *MyClient) handleQRCodes(codes []string) {
	go func() {
		defer utils.Recover("handleQRCodes")
		_, count := runQREmitter(qrEnv{mycli: mycli}, append([]string(nil), codes...), mycli.qrcodeCount, defaultQRTiming)
		mycli.qrcodeCount = count
	}()
}
```

5. Se, depois disso, `qrcode` e `base64` não forem mais usados em `whatsmeow.go`, o compilador acusa import não usado: remova-os do bloco de imports.

- [ ] **Step 6: Compilar**

Run: `docker build --target build -t evo-go-check .`
Expected: build conclui sem erro.

Run: `bash scripts/go-test.sh ./pkg/whatsmeow/service/ -v -race`
Expected: PASS em todo o pacote.

- [ ] **Step 7: Commit**

```bash
git add pkg/whatsmeow/service/qr_emitter.go pkg/whatsmeow/service/qr_emitter_test.go pkg/whatsmeow/service/qr_env.go pkg/whatsmeow/service/whatsmeow.go pkg/instance/model/disconnect_reason.go
git commit -m "fix(qr): stop the QR emitter with its runtime and tear down when the QR socket drops"
```

---

### Task 3: Troca da chave ADV no meio do pareamento

Item 1 / causa D. Com o whatsmeow da 0.8.0, o WhatsApp pode trocar a chave ADV durante o QR (`events.RotateADVSecret`). Os códigos que já estão no banco e no webhook ficam com a chave antiga, e todo scan falha. O `qrchan` nativo reescreve os códigos (`wm/qrchan.go:117-124`); o nosso emissor passa a fazer o mesmo.

**Files:**
- Modify: `pkg/whatsmeow/service/qr_emitter.go`
- Modify: `pkg/whatsmeow/service/qr_emitter_test.go`
- Modify: `pkg/whatsmeow/service/qr_env.go`
- Modify: `pkg/whatsmeow/service/whatsmeow.go` (struct `MyClient`, literal do `StartClient`, `switch` do `myEventHandler`)

**Interfaces:**
- Consumes: `runQREmitter`, `qrWait`, `qrEmitterEnv`, `fakeQREnv` (Task 2).
- Produces: `rotateADVSecret(codes []string, from int, oldSecret, newSecret string) []string`, método `Rotations() <-chan *events.RotateADVSecret` na interface, campo `MyClient.qrRotate`.

- [ ] **Step 1: Escrever os testes que falham**

Em `qr_emitter_test.go`, acrescente o import `"go.mau.fi/whatsmeow/types/events"`, o campo `rotations chan *events.RotateADVSecret` em `fakeQREnv`, e em `newFakeQREnv` inicialize `rotations: make(chan *events.RotateADVSecret, 4)`. Acrescente o método e os testes:

```go
func (e *fakeQREnv) Rotations() <-chan *events.RotateADVSecret { return e.rotations }

func TestRotateADVSecretRewritesFromCurrentCode(t *testing.T) {
	codes := []string{"a,OLD", "b,OLD", "c,OLD"}
	got := rotateADVSecret(codes, 1, "OLD", "NEW")

	want := []string{"a,OLD", "b,NEW", "c,NEW"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if codes[1] != "b,OLD" {
		t.Fatal("rotateADVSecret must not modify its input")
	}
}

// O código na tela é republicado com a chave nova (mesma validade) e os
// próximos já saem trocados.
func TestQREmitterRepublishesOnADVRotation(t *testing.T) {
	env := newFakeQREnv()
	result := runAsync(env, []string{"a,OLD", "b,OLD"}, 0)
	env.waitPublished(t, 1)
	env.rotations <- &events.RotateADVSecret{OldSecret: "OLD", NewSecret: "NEW"}

	waitResult(t, result)
	published, _ := env.snapshot()
	if len(published) != 3 {
		t.Fatalf("published = %+v, want original + republished + next", published)
	}
	if published[1].code != "a,NEW" || published[1].count != 1 || !published[1].expiresAt.Equal(published[0].expiresAt) {
		t.Fatalf("republished = %+v, want a,NEW with the same count and expiry", published[1])
	}
	if published[2].code != "b,NEW" {
		t.Fatalf("next code = %q, want b,NEW", published[2].code)
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `bash scripts/go-test.sh ./pkg/whatsmeow/service/ -run "TestQREmitter|TestRotateADVSecret" -v`
Expected: FAIL — `undefined: rotateADVSecret`.

- [ ] **Step 3: Implementar**

Em `qr_emitter.go`, acrescente `"strings"` e `"go.mau.fi/whatsmeow/types/events"` aos imports. Na interface `qrEmitterEnv`, acrescente:

```go
	// Rotations entrega as trocas da chave ADV feitas durante o pareamento.
	Rotations() <-chan *events.RotateADVSecret
```

Acrescente a função:

```go
// rotateADVSecret troca a chave ADV antiga pela nova do código atual em diante,
// como o qrchan nativo do whatsmeow faz. Os códigos já exibidos não importam.
func rotateADVSecret(codes []string, from int, oldSecret, newSecret string) []string {
	out := append([]string(nil), codes...)
	for i := from; i < len(out); i++ {
		out[i] = strings.Replace(out[i], oldSecret, newSecret, 1)
	}
	return out
}
```

Em `runQREmitter`, troque:

```go
		env.Publish(codes[i], count, env.Now().Add(lifetime))

		if end, done := qrWait(env, lifetime, timing.Poll); done {
			return end, count
		}
```

por:

```go
		expiresAt := env.Now().Add(lifetime)
		env.Publish(codes[i], count, expiresAt)

		current := i
		onRotate := func(rot *events.RotateADVSecret) {
			codes = rotateADVSecret(codes, current, rot.OldSecret, rot.NewSecret)
			env.Publish(codes[current], count, expiresAt)
		}
		if end, done := qrWait(env, lifetime, timing.Poll, onRotate); done {
			return end, count
		}
```

Em `qrWait`, mude a assinatura para `func qrWait(env qrEmitterEnv, lifetime, poll time.Duration, onRotate func(*events.RotateADVSecret)) (qrEmitterEnd, bool)` e acrescente ao `select`:

```go
		case rot := <-env.Rotations():
			if rot != nil {
				onRotate(rot)
			}
```

Em `qr_env.go`, acrescente o import `"go.mau.fi/whatsmeow/types/events"` e:

```go
func (e qrEnv) Rotations() <-chan *events.RotateADVSecret { return e.mycli.qrRotate }
```

Em `whatsmeow.go`:

1. No struct `MyClient`, depois de `runtimeDone chan struct{}`:

```go
	// qrRotate leva as trocas da chave ADV (events.RotateADVSecret) ao emissor
	// de QR deste runtime.
	qrRotate chan *events.RotateADVSecret
```

2. No literal do `StartClient`, depois de `runtimeDone: make(chan struct{}),`: `qrRotate: make(chan *events.RotateADVSecret, 4),`.

3. No `switch evt := rawEvt.(type)` do `myEventHandler`, logo depois do `case *events.QR:` (que termina em `return`), acrescente:

```go
	case *events.RotateADVSecret:
		// O WhatsApp trocou a chave ADV no meio do pareamento: os QRs já
		// emitidos levam a antiga e todo scan falharia. O emissor reescreve.
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] ADV secret rotated during pairing — rewriting QR codes", mycli.userID)
		select {
		case mycli.qrRotate <- evt:
		default:
			mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] ADV secret rotation dropped: QR emitter is not reading", mycli.userID)
		}
		return
```

- [ ] **Step 4: Rodar e ver passar**

Run: `bash scripts/go-test.sh ./pkg/whatsmeow/service/ -run "TestQREmitter|TestRotateADVSecret" -v -race`
Expected: PASS em todos (os 7 da Task 2 + 2 novos).

Run: `docker build --target build -t evo-go-check .`
Expected: build conclui.

- [ ] **Step 5: Commit**

```bash
git add pkg/whatsmeow/service/qr_emitter.go pkg/whatsmeow/service/qr_emitter_test.go pkg/whatsmeow/service/qr_env.go pkg/whatsmeow/service/whatsmeow.go
git commit -m "fix(qr): rewrite pending QR codes when WhatsApp rotates the ADV secret"
```

---

### Task 4: Teardown do QR encerra só o próprio runtime e não anuncia LoggedOut

Causa B e item 7. O `teardownQR` sinalizava `killChannel[ID da instância]`, um mapa que, depois de um restart, aponta para o runtime novo: derrubava a sessão que a pessoa estava escaneando. Além disso, o fim de uma janela de QR sem scan saía como `LoggedOut`.

**Files:**
- Modify: `pkg/whatsmeow/service/whatsmeow.go` (imports; struct `MyClient`; literal do `StartClient`; `select` do laço do `StartClient` ~687-700; bloco `if w.runtimeRestarting(...)` ~724-727; final de `teardownQR` ~926-938)
- Create: `pkg/whatsmeow/service/qr_teardown_test.go`

**Interfaces:**
- Consumes: `teardownQR` (chamado pelo `qrEnv.Teardown` da Task 2).
- Produces: `(*MyClient).signalRuntimeKill()`, `shouldAnnounceLoggedOut(restarting, qrEnded bool) bool`, campos `MyClient.runtimeKill chan bool` e `MyClient.qrEnded atomic.Bool`.

- [ ] **Step 1: Escrever os testes que falham**

`pkg/whatsmeow/service/qr_teardown_test.go`:

```go
package whatsmeow_service

import "testing"

// O teardown do QR encerra o runtime DESTE MyClient, nunca o que estiver no
// mapa killChannel (depois de um restart, ele é o runtime novo).
func TestSignalRuntimeKillUsesOwnChannel(t *testing.T) {
	own := make(chan bool, 1)
	newer := make(chan bool, 1)
	mycli := &MyClient{
		userID:      "inst-1",
		runtimeKill: own,
		killChannel: map[string](chan bool){"inst-1": newer},
	}

	mycli.signalRuntimeKill()
	mycli.signalRuntimeKill() // canal cheio: não pode bloquear

	if len(own) != 1 {
		t.Fatalf("own kill channel has %d signals, want 1", len(own))
	}
	if len(newer) != 0 {
		t.Fatal("teardown signalled the runtime registered in the shared map")
	}
}

func TestSignalRuntimeKillWithoutChannel(t *testing.T) {
	(&MyClient{}).signalRuntimeKill() // não pode entrar em pânico nem bloquear
}

func TestShouldAnnounceLoggedOut(t *testing.T) {
	tests := []struct {
		restarting, qrEnded, want bool
	}{
		{false, false, true},
		{true, false, false},
		{false, true, false},
		{true, true, false},
	}
	for _, tt := range tests {
		if got := shouldAnnounceLoggedOut(tt.restarting, tt.qrEnded); got != tt.want {
			t.Errorf("shouldAnnounceLoggedOut(%v, %v) = %v, want %v", tt.restarting, tt.qrEnded, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `bash scripts/go-test.sh ./pkg/whatsmeow/service/ -run "SignalRuntimeKill|ShouldAnnounceLoggedOut" -v`
Expected: FAIL — `unknown field runtimeKill`, `undefined: shouldAnnounceLoggedOut`.

- [ ] **Step 3: Implementar**

Em `whatsmeow.go`:

1. Acrescente `"sync/atomic"` aos imports.

2. No struct `MyClient`, depois de `qrRotate`:

```go
	// runtimeKill é o kill channel deste runtime. O mapa killChannel é por
	// instância e, depois de um restart, aponta para o runtime novo.
	runtimeKill chan bool
	// qrEnded marca que o runtime está parando porque a janela de QR acabou sem
	// scan: não é logout e não pode sair como LoggedOut.
	qrEnded atomic.Bool
```

3. No literal do `StartClient`, depois de `qrRotate: ...,`: `runtimeKill: killChan,` (o `killChan` criado no início do `StartClient`, `make(chan bool, 1)`).

4. Acrescente, logo antes de `func (mycli *MyClient) teardownQR`:

```go
// signalRuntimeKill encerra o runtime deste MyClient sem bloquear.
func (mycli *MyClient) signalRuntimeKill() {
	if mycli.runtimeKill == nil {
		return
	}
	select {
	case mycli.runtimeKill <- true:
	default:
	}
}

// shouldAnnounceLoggedOut: o kill do runtime só vira LoggedOut num encerramento
// de verdade — não num restart do supervisor nem no fim de uma janela de QR
// sem scan (a instância nem chegou a estar logada).
func shouldAnnounceLoggedOut(restarting, qrEnded bool) bool {
	return !restarting && !qrEnded
}
```

5. No final de `teardownQR`, substitua o bloco que começa em `ClientMapsMu.RLock()` / `killChan, exists := mycli.killChannel[instanceID]` até o `}` do `if exists` por:

```go
	mycli.qrEnded.Store(true)
	mycli.signalRuntimeKill()
```

(mantenha a linha `LogWarn("[%s] QR timeout — signaling kill channel", ...)` antes delas).

6. No laço do `StartClient`, troque:

```go
		select {
		case <-killChan:
```

por:

```go
		select {
		case <-killChan:
		case <-mycli.runtimeKill:
		}
		{
```

e feche o bloco `{` novo onde antes fechava o `select` (o `}` que vinha depois do último `return` do ramo de kill). O corpo do ramo não muda de lugar nem de conteúdo; só passa a rodar para qualquer um dos dois sinais.

7. No mesmo corpo, troque:

```go
			if w.runtimeRestarting(cd.Instance.Id) {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] Runtime stopped for restart; LoggedOut not emitted", cd.Instance.Id)
				return
			}
```

por:

```go
			if !shouldAnnounceLoggedOut(w.runtimeRestarting(cd.Instance.Id), mycli.qrEnded.Load()) {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] Runtime stopped for restart or QR end; LoggedOut not emitted", cd.Instance.Id)
				return
			}
```

- [ ] **Step 4: Rodar e ver passar**

Run: `bash scripts/go-test.sh ./pkg/whatsmeow/service/ -v -race`
Expected: PASS em todo o pacote.

Run: `docker build --target build -t evo-go-check .`
Expected: build conclui.

- [ ] **Step 5: Commit**

```bash
git add pkg/whatsmeow/service/whatsmeow.go pkg/whatsmeow/service/qr_teardown_test.go
git commit -m "fix(qr): QR teardown stops only its own runtime and no longer emits LoggedOut"
```

---

### Task 5: Apagar o QR guardado quando a sessão de QR termina ou recomeça

Causa C. O encerramento do runtime e o `ClearInstanceCache` não limpavam a coluna `qrcode`: o `GET /qr` logo depois devolvia o código da sessão anterior. (A validade da Task 1 já impede servir código vencido; isto cobre o código ainda "dentro do prazo" de um socket que não existe mais.)

**Files:**
- Modify: `pkg/whatsmeow/service/whatsmeow.go` (`defer` do `StartClient`; ramo `else` de pareamento novo, antes do `client.Connect()` ~654)

**Interfaces:**
- Consumes: nada novo.
- Produces: comportamento — coluna `qrcode` vazia sempre que não há runtime de QR vivo.

- [ ] **Step 1: Implementar**

No `defer func() {` do `StartClient`, dentro do `if client != nil {`, antes do `if mycli != nil && mycli.eventHandlerID != 0`, acrescente:

```go
			// Runtime que termina sem pareamento: o QR guardado é de um socket
			// que deixou de existir e não pode ser servido pelo GET /instance/qr.
			if client.Store.ID == nil {
				if err := w.instanceRepository.UpdateQrcode(cd.Instance.Id, ""); err != nil {
					w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Error clearing stored QR: %v", cd.Instance.Id, err)
				}
			}
```

No ramo `} else {` de pareamento de device novo, logo depois do bloco `if w.passkeyCeremony != nil { w.passkeyCeremony.Clear(cd.Instance.Id) }`, acrescente:

```go
		// Sessão de QR nova: um código que tenha sobrado de um runtime anterior
		// (ex.: restart do container) não pode ser servido antes do primeiro
		// código desta sessão chegar.
		if err := w.instanceRepository.UpdateQrcode(cd.Instance.Id, ""); err != nil {
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Error clearing stored QR: %v", cd.Instance.Id, err)
		}
```

- [ ] **Step 2: Compilar e rodar o pacote**

Run: `docker build --target build -t evo-go-check .`
Expected: build conclui.

Run: `bash scripts/go-test.sh ./pkg/whatsmeow/service/ -v`
Expected: PASS (nada existente quebra; este caminho não tem teste unitário porque depende do store do whatsmeow — ele é coberto pela validação manual da Task 16).

- [ ] **Step 3: Commit**

```bash
git add pkg/whatsmeow/service/whatsmeow.go
git commit -m "fix(qr): clear the stored QR when a pairing runtime ends or starts over"
```

---

### Task 6: `GET /qr` não reinicia a sessão que ainda está conectando

Item 3 / causa E. A 0.8.0 derruba e recria o runtime no `GET /qr` quando ele "existe, mas não está conectado nem pareado". Logo depois do `/connect`, o socket ainda está abrindo e cai exatamente nessa condição: cada recriação gera um aparelho novo e pode deixar uma rotina órfã.

**Files:**
- Modify: `pkg/whatsmeow/service/runtime_lifecycle.go` (`instanceRuntime`, `reserveRuntime`)
- Modify: `pkg/whatsmeow/service/runtime_lifecycle_test.go`
- Modify: `pkg/whatsmeow/service/whatsmeow.go` (interface `WhatsmeowService`)
- Modify: `pkg/instance/service/instance_service.go` (`GetQr`, bloco `if client == nil || (!client.IsConnected() && !client.IsLoggedIn()) {`)
- Create: `pkg/instance/service/qr_grace_test.go`

**Interfaces:**
- Produces: `(*whatsmeowService).runtimeAge(instanceID string) (time.Duration, bool)`, `RuntimeAge(instanceId string) (time.Duration, bool)` na interface `WhatsmeowService`, `qrConnectGrace = 15 * time.Second`, `qrRestartAllowed(age time.Duration, known bool) bool`. A Task 10 usa `runtimeAge`.

- [ ] **Step 1: Escrever os testes que falham**

Em `runtime_lifecycle_test.go`, acrescente:

```go
func TestRuntimeAgeTracksReservedRuntime(t *testing.T) {
	service := &whatsmeowService{}
	if _, known := service.runtimeAge("inst"); known {
		t.Fatal("age known before any runtime was reserved")
	}

	token, _, ok := service.reserveRuntime("inst")
	if !ok {
		t.Fatal("reserveRuntime failed")
	}
	age, known := service.runtimeAge("inst")
	if !known || age < 0 || age > time.Second {
		t.Fatalf("age=%s known=%v right after reserve", age, known)
	}

	service.finishRuntime("inst", token)
	if _, known := service.runtimeAge("inst"); known {
		t.Fatal("age still known after the runtime finished")
	}
}
```

`pkg/instance/service/qr_grace_test.go`:

```go
package instance_service

import (
	"testing"
	"time"
)

func TestQRRestartAllowed(t *testing.T) {
	tests := []struct {
		name  string
		age   time.Duration
		known bool
		want  bool
	}{
		{"no runtime age (nothing registered)", 0, false, true},
		{"runtime just created, still connecting", 2 * time.Second, true, false},
		{"right at the grace limit", qrConnectGrace, true, true},
		{"old runtime with a dead socket", time.Minute, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := qrRestartAllowed(tt.age, tt.known); got != tt.want {
				t.Fatalf("qrRestartAllowed(%s, %v) = %v, want %v", tt.age, tt.known, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `bash scripts/go-test.sh ./pkg/whatsmeow/service/ ./pkg/instance/service/ -run "RuntimeAge|QRRestartAllowed" -v`
Expected: FAIL — `undefined: runtimeAge`, `undefined: qrRestartAllowed`.

- [ ] **Step 3: Implementar**

Em `runtime_lifecycle.go`, no struct `instanceRuntime`, acrescente `startedAt time.Time`. Em `reserveRuntime`, troque a criação por:

```go
	runtime := &instanceRuntime{token: w.runtimeSequence, done: make(chan struct{}), startedAt: time.Now()}
```

Acrescente depois de `runtimeDone`:

```go
// runtimeAge diz há quanto tempo o runtime atual da instância foi reservado.
func (w *whatsmeowService) runtimeAge(instanceID string) (time.Duration, bool) {
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	runtime := w.runtimes[instanceID]
	if runtime == nil {
		return 0, false
	}
	return time.Since(runtime.startedAt), true
}

// RuntimeAge expõe runtimeAge ao serviço de instâncias.
func (w *whatsmeowService) RuntimeAge(instanceID string) (time.Duration, bool) {
	return w.runtimeAge(instanceID)
}
```

Na interface `WhatsmeowService` (`whatsmeow.go`), depois de `ClearInstanceCache(...)`:

```go
	// RuntimeAge diz há quanto tempo o runtime da instância subiu (false se
	// não há runtime). O GET /qr usa para não derrubar um socket que ainda abre.
	RuntimeAge(instanceId string) (time.Duration, bool)
```

Em `instance_service.go`, acrescente perto de `readFreshQR`:

```go
// qrConnectGrace: tempo que um runtime recém-criado tem para abrir o socket
// antes de o GET /qr considerá-lo morto. Sem isso, um GET logo depois do
// /connect derrubava a sessão que ainda conectava e gerava um aparelho novo.
const qrConnectGrace = 15 * time.Second

func qrRestartAllowed(age time.Duration, known bool) bool {
	return !known || age >= qrConnectGrace
}
```

Em `GetQr`, substitua o bloco `if client == nil || (!client.IsConnected() && !client.IsLoggedIn()) {` inteiro (até o `} else if !client.IsConnected() {`, exclusive) por:

```go
	if client == nil || (!client.IsConnected() && !client.IsLoggedIn()) {
		start := true
		if client == nil {
			logger.LogInfo("[%s] No client found, starting new instance for QR code", instance.Id)
		} else {
			// Pareada mas fora do ar (conectando ou após stream:error): QR não
			// se aplica e derrubar o runtime atrapalharia a reconexão.
			if client.Store != nil && client.Store.ID != nil {
				logger.LogInfo("[%s] Paired client is reconnecting — no QR to return", instance.Id)
				return nil, fmt.Errorf("session already logged in")
			}
			if age, known := i.whatsmeowService.RuntimeAge(instance.Id); !qrRestartAllowed(age, known) {
				logger.LogInfo("[%s] Runtime started %s ago and is still connecting — waiting for its QR", instance.Id, age.Round(time.Second))
				start = false
			} else {
				logger.LogInfo("[%s] Client exists but not connected/logged, restarting for QR code", instance.Id)
				// O runtime antigo continua reservado mesmo com o socket morto, e o
				// StartInstance abaixo viraria no-op ("already running"). Para ele primeiro.
				if err := i.whatsmeowService.ClearInstanceCache(instance.Id, instance.Token); err != nil {
					logger.LogError("[%s] Failed to stop stale runtime: %v", instance.Id, err)
					return nil, fmt.Errorf("failed to stop stale runtime: %w", err)
				}
			}
		}

		if start {
			err := i.whatsmeowService.StartInstance(instance.Id)
			if err != nil {
				logger.LogError("[%s] Failed to start instance: %v", instance.Id, err)
				return nil, fmt.Errorf("failed to start instance: %w", err)
			}

			// Aguardar um pouco para o cliente iniciar e gerar QR code
			logger.LogInfo("[%s] Waiting for QR code generation...", instance.Id)
			time.Sleep(3 * time.Second)

			whatsmeow_service.ClientMapsMu.RLock()
			client = i.clientPointer[instance.Id]
			whatsmeow_service.ClientMapsMu.RUnlock()
			if client != nil && client.IsLoggedIn() {
				return nil, fmt.Errorf("session already logged in")
			}
		}
```

- [ ] **Step 4: Rodar e ver passar**

Run: `bash scripts/go-test.sh ./pkg/whatsmeow/service/ ./pkg/instance/service/ -v`
Expected: PASS nos dois pacotes.

Run: `docker build --target build -t evo-go-check .`
Expected: build conclui.

- [ ] **Step 5: Commit**

```bash
git add pkg/whatsmeow/service/runtime_lifecycle.go pkg/whatsmeow/service/runtime_lifecycle_test.go pkg/whatsmeow/service/whatsmeow.go pkg/instance/service/instance_service.go pkg/instance/service/qr_grace_test.go
git commit -m "fix(qr): GET /instance/qr no longer restarts a runtime that is still connecting"
```

---

### Task 7: Webhook `PairError`

Item 7. Quando o pareamento falha depois do scan (chave inválida, erro ao gravar o aparelho), o whatsmeow emite `events.PairError`, que a EvGo descartava como "Unhandled event". Passa a sair no log e por webhook, no grupo `QRCODE`.

**Files:**
- Modify: `pkg/whatsmeow/service/whatsmeow.go` (`switch` do `myEventHandler`; lista do grupo `QRCODE` no `CallWebhook`, ~2494)
- Modify: `pkg/whatsmeow/service/event_groups.go`
- Modify: `pkg/whatsmeow/service/event_groups_test.go`
- Modify: `docs/CHANGELOG-0.8.0-E-CATALOGO-DE-ROTAS.md`

**Interfaces:**
- Produces: evento de webhook `PairError` com `data: {error, jid, platform}`.

- [ ] **Step 1: Escrever o teste que falha**

Em `event_groups_test.go`, acrescente ao mapa `tests`:

```go
		"PairError":            event_types.QRCODE,
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `bash scripts/go-test.sh ./pkg/whatsmeow/service/ -run TestGlobalEventGroup -v`
Expected: FAIL — `globalEventGroup("PairError") = "", want "QRCODE"`.

- [ ] **Step 3: Implementar**

Em `event_groups.go`, troque:

```go
	case "QRCode", "QRTimeout", "QRSuccess", "PasskeyRequest", "PasskeyConfirmation", "PasskeyError":
```

por:

```go
	case "QRCode", "QRTimeout", "QRSuccess", "PairError", "PasskeyRequest", "PasskeyConfirmation", "PasskeyError":
```

Faça a mesma troca na lista do `CallWebhook` em `whatsmeow.go` (a linha `case "QRCode", "QRTimeout", "QRSuccess", "PasskeyRequest", "PasskeyConfirmation", "PasskeyError":` seguida de `if contains(subscriptions, "QRCODE")`).

No `switch` do `myEventHandler`, logo antes de `case *events.PairSuccess:`, acrescente:

```go
	case *events.PairError:
		// O scan chegou, mas o pareamento falhou (chave inválida, erro ao gravar
		// o aparelho): é o "Não foi possível conectar" que o celular mostra.
		errText := ""
		if evt.Error != nil {
			errText = evt.Error.Error()
		}
		mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Pairing failed after scan: %s", mycli.userID, errText)
		doWebhook = true
		postMap["event"] = "PairError"
		postMap["data"] = map[string]interface{}{
			"error":    errText,
			"jid":      evt.ID.String(),
			"platform": evt.Platform,
		}
```

- [ ] **Step 4: Rodar e ver passar**

Run: `bash scripts/go-test.sh ./pkg/whatsmeow/service/ -v`
Expected: PASS.

Run: `docker build --target build -t evo-go-check .`
Expected: build conclui.

- [ ] **Step 5: Documentar**

Em `docs/CHANGELOG-0.8.0-E-CATALOGO-DE-ROTAS.md`, na tabela "Changelog — webhooks, eventos e variáveis de ambiente", acrescente:

```markdown
| `PairError` | Novo | Grupo `QRCODE`: o QR foi escaneado, mas o pareamento falhou (o celular mostra "Não foi possível conectar"). `data.error` traz o motivo. Gere um QR novo. |
| `QRTimeout` / `LoggedOut` | Menos disparos | O fim de uma janela de QR sem scan emite só `QRTimeout`; antes também saía um `LoggedOut` enganoso. |
| `QRCode` | Campo novo | `data.expiresAt` (unix): quando o código vence. Se o WhatsApp trocar a chave durante o pareamento, o código atual é reemitido com a chave nova. |
| `QRTimeout` | Novo motivo | `data.reason = "QR socket closed before pairing"`: a conexão do QR caiu antes do scan; peça um QR novo. |
```

- [ ] **Step 6: Commit**

```bash
git add pkg/whatsmeow/service/whatsmeow.go pkg/whatsmeow/service/event_groups.go pkg/whatsmeow/service/event_groups_test.go docs/CHANGELOG-0.8.0-E-CATALOGO-DE-ROTAS.md
git commit -m "feat(qr): emit a PairError webhook when pairing fails after the scan"
```

---

### Task 8: Registrar ban, sessão substituída e cliente desatualizado

Item 4 (parte 1). `TemporaryBan`, `StreamReplaced` e `ClientOutdated` não gravavam nada no banco, e os dois últimos nem saíam por webhook. Passam a gravar `disconnect_reason` e a sair no grupo `CONNECTION`. O motivo do ban carrega a data em que ele termina, que a Task 9 usa.

**Files:**
- Modify: `pkg/instance/model/disconnect_reason.go`
- Create: `pkg/instance/model/disconnect_reason_test.go`
- Create: `pkg/whatsmeow/service/temp_ban.go`
- Create: `pkg/whatsmeow/service/temp_ban_test.go`
- Modify: `pkg/whatsmeow/service/whatsmeow.go` (`case *events.StreamReplaced`, `case *events.TemporaryBan`; novo `case *events.ClientOutdated`; lista `CONNECTION` do `CallWebhook` ~2459)
- Modify: `pkg/whatsmeow/service/event_groups.go`, `event_groups_test.go`
- Modify: `docs/CHANGELOG-0.8.0-E-CATALOGO-DE-ROTAS.md`

**Interfaces:**
- Produces: `instance_model.DisconnectReasonStreamReplaced`, `instance_model.DisconnectReasonClientOutdated`, `instance_model.TemporaryBanReason(until time.Time, code string) string`, `instance_model.TemporaryBanUntil(reason string) (time.Time, bool)`, `temporaryBanUntil(now time.Time, expire time.Duration) time.Time`, `(*MyClient).recordDisconnect(reason string)`. A Task 9 usa `TemporaryBanUntil` e `DisconnectReasonStreamReplaced`; a Task 10 usa `DisconnectReasonStreamReplaced`; a Task 13 reconhece os textos no proxy.

- [ ] **Step 1: Escrever os testes que falham**

`pkg/instance/model/disconnect_reason_test.go`:

```go
package instance_model

import (
	"testing"
	"time"
)

func TestTemporaryBanReasonRoundTrip(t *testing.T) {
	until := time.Date(2026, 10, 12, 15, 4, 5, 0, time.UTC)
	reason := TemporaryBanReason(until, "sent too many messages")

	got, ok := TemporaryBanUntil(reason)
	if !ok || !got.Equal(until) {
		t.Fatalf("TemporaryBanUntil(%q) = %s, %v", reason, got, ok)
	}
}

func TestTemporaryBanUntilIgnoresOtherReasons(t *testing.T) {
	for _, reason := range []string{"", "Reconnecting", "401: logged out from another device", "Temporary ban until garbage: x"} {
		if _, ok := TemporaryBanUntil(reason); ok {
			t.Errorf("TemporaryBanUntil(%q) parsed a non-ban reason", reason)
		}
	}
}
```

`pkg/whatsmeow/service/temp_ban_test.go`:

```go
package whatsmeow_service

import (
	"testing"
	"time"
)

func TestTemporaryBanUntilUsesExpiry(t *testing.T) {
	now := time.Unix(1000, 0)
	if got := temporaryBanUntil(now, 2*time.Hour); !got.Equal(now.Add(2 * time.Hour)) {
		t.Fatalf("got %s", got)
	}
}

// Sem prazo informado, o supervisor também precisa esperar.
func TestTemporaryBanUntilDefaultsWhenExpiryUnknown(t *testing.T) {
	now := time.Unix(1000, 0)
	if got := temporaryBanUntil(now, 0); !got.Equal(now.Add(tempBanUnknownWait)) {
		t.Fatalf("got %s, want now + %s", got, tempBanUnknownWait)
	}
}
```

Em `event_groups_test.go`, acrescente ao mapa:

```go
		"StreamReplaced":       event_types.CONNECTION,
		"ClientOutdated":       event_types.CONNECTION,
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `bash scripts/go-test.sh ./pkg/instance/model/ ./pkg/whatsmeow/service/ -run "TemporaryBan|GlobalEventGroup" -v`
Expected: FAIL — `undefined: TemporaryBanReason`, `undefined: temporaryBanUntil`, e `globalEventGroup("StreamReplaced") = ""`.

- [ ] **Step 3: Implementar**

Acrescente a `pkg/instance/model/disconnect_reason.go` (troque o import implícito por um bloco com `"strings"` e `"time"`):

```go
const (
	// DisconnectReasonStreamReplaced: outro processo conectou com as mesmas
	// chaves. Religar sozinho brigaria com ele.
	DisconnectReasonStreamReplaced = "Stream replaced by another connection"
	// DisconnectReasonClientOutdated: o WhatsApp recusou a versão do cliente (405).
	DisconnectReasonClientOutdated = "405: client outdated"

	temporaryBanPrefix = "Temporary ban until "
)

// TemporaryBanReason monta o disconnect_reason de um ban temporário, com a data
// em que ele termina (o supervisor espera até lá antes de religar).
func TemporaryBanReason(until time.Time, code string) string {
	return temporaryBanPrefix + until.UTC().Format(time.RFC3339) + ": " + code
}

// TemporaryBanUntil lê a data de fim de um motivo montado por TemporaryBanReason.
func TemporaryBanUntil(reason string) (time.Time, bool) {
	if !strings.HasPrefix(reason, temporaryBanPrefix) {
		return time.Time{}, false
	}
	rest := reason[len(temporaryBanPrefix):]
	end := strings.Index(rest, ": ")
	if end < 0 {
		return time.Time{}, false
	}
	until, err := time.Parse(time.RFC3339, rest[:end])
	if err != nil {
		return time.Time{}, false
	}
	return until, true
}
```

`pkg/whatsmeow/service/temp_ban.go`:

```go
package whatsmeow_service

import "time"

// tempBanUnknownWait: espera usada quando o WhatsApp não informa o prazo do ban.
const tempBanUnknownWait = time.Hour

// temporaryBanUntil calcula quando o ban termina.
func temporaryBanUntil(now time.Time, expire time.Duration) time.Time {
	if expire <= 0 {
		expire = tempBanUnknownWait
	}
	return now.Add(expire)
}

// recordDisconnect grava a instância como desconectada com o motivo dado.
func (mycli *MyClient) recordDisconnect(reason string) {
	instance := mycli.currentInstance()
	instance.Connected = false
	instance.DisconnectReason = reason
	if err := mycli.instanceRepository.UpdateConnected(instance.Id, false, reason); err != nil {
		mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Error updating instance: %s", instance.Id, err)
	}
}
```

Em `whatsmeow.go`, troque o `case *events.StreamReplaced:` (3 linhas, termina em `return`) por:

```go
	case *events.StreamReplaced:
		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Session replaced by another connection using the same keys", mycli.userID)
		mycli.recordDisconnect(instance_model.DisconnectReasonStreamReplaced)
		doWebhook = true
		postMap["event"] = "StreamReplaced"
		postMap["data"] = map[string]interface{}{"reason": instance_model.DisconnectReasonStreamReplaced}
	case *events.ClientOutdated:
		mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] WhatsApp rejected the client version (405)", mycli.userID)
		mycli.recordDisconnect(instance_model.DisconnectReasonClientOutdated)
		doWebhook = true
		postMap["event"] = "ClientOutdated"
		postMap["data"] = map[string]interface{}{"reason": instance_model.DisconnectReasonClientOutdated}
```

No `case *events.TemporaryBan:`, logo depois de `dataMap["expire"] = evt.Expire`, acrescente:

```go
		until := temporaryBanUntil(time.Now(), evt.Expire)
		mycli.recordDisconnect(instance_model.TemporaryBanReason(until, evt.Code.String()))
		dataMap["expiresAt"] = until.Unix()
```

Em `event_groups.go` e na lista equivalente do `CallWebhook`, acrescente `"StreamReplaced", "ClientOutdated"` ao `case` do grupo `CONNECTION` (a linha que começa com `case "Connected", "PairSuccess", "TemporaryBan", "LoggedOut", "ConnectFailure",`).

- [ ] **Step 4: Rodar e ver passar**

Run: `bash scripts/go-test.sh ./pkg/instance/model/ ./pkg/whatsmeow/service/ -v`
Expected: PASS.

Run: `docker build --target build -t evo-go-check .`
Expected: build conclui.

- [ ] **Step 5: Documentar**

Em `docs/CHANGELOG-0.8.0-E-CATALOGO-DE-ROTAS.md`, na tabela de webhooks, acrescente:

```markdown
| `StreamReplaced` | Novo | Grupo `CONNECTION`: outro processo conectou com a mesma sessão. A instância não religa sozinha; `disconnect_reason = "Stream replaced by another connection"`. |
| `ClientOutdated` | Novo | Grupo `CONNECTION`: o WhatsApp recusou a versão do cliente (405). `disconnect_reason = "405: client outdated"`. |
| `TemporaryBan` | Campo novo e registro | `data.expiresAt` (unix). O ban agora fica em `disconnect_reason` (`"Temporary ban until <data>: <motivo>"`) e a reconexão espera o fim do prazo. |
```

- [ ] **Step 6: Commit**

```bash
git add pkg/instance/model/disconnect_reason.go pkg/instance/model/disconnect_reason_test.go pkg/whatsmeow/service/temp_ban.go pkg/whatsmeow/service/temp_ban_test.go pkg/whatsmeow/service/whatsmeow.go pkg/whatsmeow/service/event_groups.go pkg/whatsmeow/service/event_groups_test.go docs/CHANGELOG-0.8.0-E-CATALOGO-DE-ROTAS.md
git commit -m "feat(connection): record and announce temporary bans, replaced sessions and outdated clients"
```

---

### Task 9: Supervisor espera o ban e não briga com sessão substituída

Item 4 (parte 2). Durante um ban o supervisor tentava logar a cada ~30-70s, o que pode agravar o ban. E ele religava por cima de outro processo que tinha assumido a sessão.

**Files:**
- Modify: `pkg/whatsmeow/service/runtime_lifecycle.go` (`decideReconnect`, `ScheduleReconnect`)
- Modify: `pkg/whatsmeow/service/reconnect_decision_test.go`
- Modify: `pkg/whatsmeow/service/temp_ban.go`, `temp_ban_test.go`
- Modify: `pkg/whatsmeow/service/whatsmeow.go` (`case *events.TemporaryBan`)

**Interfaces:**
- Consumes: `instance_model.TemporaryBanUntil`, `instance_model.DisconnectReasonStreamReplaced` (Task 8).
- Produces: `banWait(reason string, now time.Time) time.Duration`, `(*whatsmeowService).pendingBanWait(instanceID string) time.Duration`.

- [ ] **Step 1: Escrever os testes que falham**

Em `reconnect_decision_test.go`, acrescente ao slice `tests`:

```go
		{
			name:      "session replaced by another process",
			instance:  &instance_model.Instance{Jid: paired.Jid, DisconnectReason: instance_model.DisconnectReasonStreamReplaced},
			hasDevice: true,
			want:      reconnectStop,
		},
```

Em `temp_ban_test.go`, acrescente o import `instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"` e:

```go
func TestBanWait(t *testing.T) {
	now := time.Unix(10_000, 0)
	banned := instance_model.TemporaryBanReason(now.Add(90*time.Minute), "spam")
	expired := instance_model.TemporaryBanReason(now.Add(-time.Minute), "spam")

	if got := banWait(banned, now); got != 90*time.Minute {
		t.Fatalf("banWait(active) = %s, want 1h30m", got)
	}
	if got := banWait(expired, now); got != 0 {
		t.Fatalf("banWait(expired) = %s, want 0", got)
	}
	if got := banWait("Reconnecting", now); got != 0 {
		t.Fatalf("banWait(not a ban) = %s, want 0", got)
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `bash scripts/go-test.sh ./pkg/whatsmeow/service/ -run "DecideReconnect|BanWait" -v`
Expected: FAIL — `undefined: banWait` e o caso "session replaced" retornando `proceed`.

- [ ] **Step 3: Implementar**

Em `temp_ban.go`, acrescente o import `instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"` e:

```go
// banWait diz quanto falta para o ban gravado no motivo terminar (0 se não há
// ban ou se ele já passou).
func banWait(reason string, now time.Time) time.Duration {
	until, ok := instance_model.TemporaryBanUntil(reason)
	if !ok || !now.Before(until) {
		return 0
	}
	return until.Sub(now)
}

// pendingBanWait lê o motivo gravado da instância e devolve a espera do ban.
func (w *whatsmeowService) pendingBanWait(instanceID string) time.Duration {
	instance, err := w.instanceRepository.GetInstanceByID(instanceID)
	if err != nil || instance == nil {
		return 0
	}
	return banWait(instance.DisconnectReason, time.Now())
}
```

Em `decideReconnect` (`runtime_lifecycle.go`), troque:

```go
	case instance_model.DisconnectReasonByAPI, instance_model.DisconnectReasonLogoutByAPI:
```

por:

```go
	case instance_model.DisconnectReasonByAPI, instance_model.DisconnectReasonLogoutByAPI,
		instance_model.DisconnectReasonStreamReplaced:
```

Em `ScheduleReconnect`, logo depois de `delay := reconnectDelay(failures, rand.Float64())`, acrescente:

```go
		// Ban temporário: tentar antes do prazo só renova o ban.
		if wait := w.pendingBanWait(instanceID); wait > delay {
			delay = wait + reconnectDelay(0, rand.Float64())
		}
```

No `case *events.TemporaryBan:` (`whatsmeow.go`), logo depois das linhas acrescentadas na Task 8 (`dataMap["expiresAt"] = until.Unix()`), acrescente:

```go
		// Ban fora do supervisor (boot, /connect) ninguém reagendava: agenda a
		// volta para depois do prazo (ScheduleReconnect espera o ban).
		mycli.service.ScheduleReconnect(mycli.userID)
```

- [ ] **Step 4: Rodar e ver passar**

Run: `bash scripts/go-test.sh ./pkg/whatsmeow/service/ -v -race`
Expected: PASS.

Run: `docker build --target build -t evo-go-check .`
Expected: build conclui.

- [ ] **Step 5: Commit**

```bash
git add pkg/whatsmeow/service/runtime_lifecycle.go pkg/whatsmeow/service/reconnect_decision_test.go pkg/whatsmeow/service/temp_ban.go pkg/whatsmeow/service/temp_ban_test.go pkg/whatsmeow/service/whatsmeow.go
git commit -m "fix(reconnect): wait for temporary bans to expire and leave replaced sessions alone"
```

---

### Task 10: Verificação periódica e espera longa depois de muitas falhas

Item 8. Instâncias paradas fora do supervisor (falha ao iniciar o runtime, cliente desatualizado, fim de ban sem reagendamento) ficavam paradas até alguém agir. Uma verificação a cada 90s religa quem deveria estar no ar. Para não martelar o WhatsApp, a espera entre tentativas cresce até 5 minutos depois de 10 falhas seguidas.

**Files:**
- Modify: `pkg/whatsmeow/service/runtime_lifecycle.go` (constantes, `reconnectDelay`)
- Modify: `pkg/whatsmeow/service/runtime_lifecycle_test.go` (`TestReconnectDelayBackoffAndCap`)
- Create: `pkg/whatsmeow/service/watchdog.go`
- Create: `pkg/whatsmeow/service/watchdog_test.go`
- Modify: `pkg/whatsmeow/service/whatsmeow.go` (final de `ConnectOnStartup`; `case *events.ClientOutdated`)

**Interfaces:**
- Consumes: `runtimeAge` (Task 6), `instance_model.DisconnectReasonStreamReplaced` (Task 8), `runtimeConnected`, `ScheduleReconnect`, `GetInstancesToRestore` (existentes).
- Produces: `watchdogTargets(...) []string`, `(*whatsmeowService).startReconnectWatchdog(clientName string)`, constantes `reconnectLongAfter`, `reconnectLongMaxDelay`, `watchdogInterval`, `watchdogRuntimeGrace`.

- [ ] **Step 1: Escrever os testes que falham**

Em `runtime_lifecycle_test.go`, troque o slice de `TestReconnectDelayBackoffAndCap` por:

```go
	tests := []struct {
		failures int
		want     time.Duration
	}{
		{failures: 0, want: time.Second},
		{failures: 1, want: 2 * time.Second},
		{failures: 4, want: 16 * time.Second},
		{failures: 5, want: 30 * time.Second},
		{failures: 9, want: 30 * time.Second},
		{failures: 10, want: 5 * time.Minute},
		{failures: 40, want: 5 * time.Minute},
	}
```

`pkg/whatsmeow/service/watchdog_test.go`:

```go
package whatsmeow_service

import (
	"reflect"
	"testing"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
)

func ageOf(ages map[string]time.Duration) func(string) (time.Duration, bool) {
	return func(id string) (time.Duration, bool) {
		age, ok := ages[id]
		return age, ok
	}
}

func TestWatchdogTargets(t *testing.T) {
	instances := []*instance_model.Instance{
		{Id: "down", Jid: "1@s.whatsapp.net", DisconnectReason: "Reconnecting"},
		{Id: "online", Jid: "2@s.whatsapp.net"},
		{Id: "never-paired"},
		{Id: "by-api", Jid: "3@s.whatsapp.net", DisconnectReason: instance_model.DisconnectReasonByAPI},
		{Id: "replaced", Jid: "4@s.whatsapp.net", DisconnectReason: instance_model.DisconnectReasonStreamReplaced},
		nil,
	}
	connected := func(id string) bool { return id == "online" }

	got := watchdogTargets(instances, connected, ageOf(nil))
	if want := []string{"down"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("watchdogTargets = %v, want %v", got, want)
	}
}

// Um runtime recém-criado (conectando ou mostrando QR) não pode ser derrubado.
func TestWatchdogTargetsSkipsYoungRuntime(t *testing.T) {
	instances := []*instance_model.Instance{
		{Id: "young", Jid: "1@s.whatsapp.net"},
		{Id: "old", Jid: "2@s.whatsapp.net"},
	}
	ages := map[string]time.Duration{"young": 10 * time.Second, "old": 10 * time.Minute}

	got := watchdogTargets(instances, func(string) bool { return false }, ageOf(ages))
	if want := []string{"old"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("watchdogTargets = %v, want %v", got, want)
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `bash scripts/go-test.sh ./pkg/whatsmeow/service/ -run "ReconnectDelay|WatchdogTargets" -v`
Expected: FAIL — `failures=10: got 30s, want 5m0s` e `undefined: watchdogTargets`.

- [ ] **Step 3: Implementar**

Em `runtime_lifecycle.go`, acrescente ao bloco `const`:

```go
	// Depois de reconnectLongAfter falhas seguidas, a espera passa a crescer até
	// reconnectLongMaxDelay: uma instância que não volta não pode bater no
	// WhatsApp a cada 30 s para sempre.
	reconnectLongAfter    = 10
	reconnectLongMaxDelay = 5 * time.Minute
```

E troque o corpo de `reconnectDelay` por:

```go
func reconnectDelay(failures int, jitter float64) time.Duration {
	if failures < 0 {
		failures = 0
	}
	maxDelay := reconnectMaxDelay
	if failures >= reconnectLongAfter {
		maxDelay = reconnectLongMaxDelay
	}
	delay := reconnectBaseDelay
	for attempt := 0; attempt < failures && delay < maxDelay; attempt++ {
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
	if jitter < 0 {
		jitter = 0
	}
	if jitter > 1 {
		jitter = 1
	}
	return delay + time.Duration(float64(delay)*0.25*jitter)
}
```

`pkg/whatsmeow/service/watchdog.go`:

```go
package whatsmeow_service

import (
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
)

const (
	// watchdogInterval: a cada quanto tempo a verificação procura instâncias
	// que deveriam estar no ar e não estão.
	watchdogInterval = 90 * time.Second
	// watchdogRuntimeGrace: runtime mais novo que isso ainda está conectando ou
	// mostrando QR, e não pode ser derrubado pela verificação.
	watchdogRuntimeGrace = 2 * time.Minute
)

// watchdogTargets escolhe as instâncias pareadas que não estão conectadas. O
// ScheduleReconnect ainda confere o aparelho, o ban e a deduplicação.
func watchdogTargets(
	instances []*instance_model.Instance,
	connected func(id string) bool,
	runtimeAge func(id string) (time.Duration, bool),
) []string {
	var ids []string
	for _, instance := range instances {
		if instance == nil || instance.Jid == "" {
			continue
		}
		switch instance.DisconnectReason {
		case instance_model.DisconnectReasonByAPI, instance_model.DisconnectReasonLogoutByAPI,
			instance_model.DisconnectReasonStreamReplaced:
			continue
		}
		if connected(instance.Id) {
			continue
		}
		if age, ok := runtimeAge(instance.Id); ok && age < watchdogRuntimeGrace {
			continue
		}
		ids = append(ids, instance.Id)
	}
	return ids
}

// startReconnectWatchdog religa, periodicamente, as instâncias que deveriam
// estar no ar e ficaram paradas fora do supervisor.
func (w *whatsmeowService) startReconnectWatchdog(clientName string) {
	go func() {
		defer utils.Recover("reconnectWatchdog")
		ticker := time.NewTicker(watchdogInterval)
		defer ticker.Stop()
		for range ticker.C {
			instances, err := w.instanceRepository.GetInstancesToRestore(clientName)
			if err != nil {
				w.loggerWrapper.GetLogger(clientName).LogWarn("[%s] Watchdog could not list instances: %v", clientName, err)
				continue
			}
			for _, id := range watchdogTargets(instances, w.runtimeConnected, w.runtimeAge) {
				w.loggerWrapper.GetLogger(id).LogInfo("[%s] Watchdog: instance should be online but is not — scheduling reconnection", id)
				w.ScheduleReconnect(id)
			}
		}
	}()
}
```

Em `whatsmeow.go`, no final de `ConnectOnStartup` (depois do `for` que inicia as instâncias), acrescente:

```go
	w.startReconnectWatchdog(clientName)
```

E no `case *events.ClientOutdated:` (Task 8), depois de `postMap["data"] = ...`, acrescente:

```go
		// Um restart busca de novo a versão do WhatsApp Web; a espera longa do
		// supervisor evita insistir a cada 30 s se a recusa continuar.
		mycli.service.ScheduleReconnect(mycli.userID)
```

- [ ] **Step 4: Rodar e ver passar**

Run: `bash scripts/go-test.sh ./pkg/whatsmeow/service/ -v -race`
Expected: PASS.

Run: `docker build --target build -t evo-go-check .`
Expected: build conclui.

- [ ] **Step 5: Commit**

```bash
git add pkg/whatsmeow/service/runtime_lifecycle.go pkg/whatsmeow/service/runtime_lifecycle_test.go pkg/whatsmeow/service/watchdog.go pkg/whatsmeow/service/watchdog_test.go pkg/whatsmeow/service/whatsmeow.go
git commit -m "feat(reconnect): periodic watchdog and longer backoff after repeated failures"
```

---

### Task 11: Campo `needsQr` em `/instance/all` e `/instance/info`

Base do item 5. Hoje não há como saber, de fora, se a instância precisa de QR novo: o `jid` continua gravado depois do logout pelo celular. O campo novo resolve isso para o proxy e para quem integra direto.

**Files:**
- Modify: `pkg/instance/model/disconnect_reason.go`, `disconnect_reason_test.go`
- Modify: `pkg/instance/model/instance_model.go` (struct `Instance`)
- Modify: `pkg/instance/service/instance_service.go` (`GetAll`, `Info`)
- Modify: `docs/CHANGELOG-0.8.0-E-CATALOGO-DE-ROTAS.md`

**Interfaces:**
- Consumes: `DisconnectReasonQRSocketLost` (Task 2), `DisconnectReasonLogoutByAPI` (existente).
- Produces: `instance_model.RequiresQR(jid, reason string) bool`, campo `Instance.NeedsQR` (`json:"needsQr"`). A Task 13 lê `needsQr`.

- [ ] **Step 1: Escrever o teste que falha**

Em `disconnect_reason_test.go`, acrescente:

```go
func TestRequiresQR(t *testing.T) {
	const jid = "5511999999999@s.whatsapp.net"
	tests := []struct {
		name   string
		jid    string
		reason string
		want   bool
	}{
		{"never paired", "", "", true},
		{"logged out from the phone", jid, "401: logged out from another device", true},
		{"primary device logged out", jid, "403: primary device was logged out", true},
		{"unknown logout", jid, "406: logged out for unknown reason", true},
		{"logged out through the API", jid, DisconnectReasonLogoutByAPI, true},
		{"QR window ended", jid, "Maximum QR code count (7) reached", true},
		{"QR socket dropped", jid, DisconnectReasonQRSocketLost, true},
		{"reconnecting", jid, "Reconnecting", false},
		{"disconnected through the API", jid, DisconnectReasonByAPI, false},
		{"banned", jid, TemporaryBanReason(time.Now(), "spam"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequiresQR(tt.jid, tt.reason); got != tt.want {
				t.Fatalf("RequiresQR(%q, %q) = %v, want %v", tt.jid, tt.reason, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `bash scripts/go-test.sh ./pkg/instance/model/ -run TestRequiresQR -v`
Expected: FAIL — `undefined: RequiresQR`.

- [ ] **Step 3: Implementar**

Em `disconnect_reason.go`, acrescente:

```go
// requiresQRPrefixes: motivos em que o aparelho foi desvinculado (ou nunca
// chegou a ser) e só um QR novo resolve. Nesses casos o jid continua gravado,
// então "tem número" não basta para saber se dá para só reconectar.
var requiresQRPrefixes = []string{
	"401:", "403:", "406:",
	DisconnectReasonLogoutByAPI,
	"Maximum QR code count",
	DisconnectReasonQRSocketLost,
}

// RequiresQR diz se a instância (desconectada) precisa de um QR novo.
func RequiresQR(jid, reason string) bool {
	if jid == "" {
		return true
	}
	for _, prefix := range requiresQRPrefixes {
		if strings.HasPrefix(reason, prefix) {
			return true
		}
	}
	return false
}
```

Em `instance_model.go`, no struct `Instance`, depois de `DisconnectReason string ...`, acrescente:

```go
	// NeedsQR é calculado na leitura (não é coluna): desconectada e sem sessão
	// para retomar — só um QR novo resolve.
	NeedsQR bool `json:"needsQr" gorm:"-"`
```

Em `instance_service.go`, em `GetAll`, logo depois do `if client != nil { ... } else { ... }` que calcula `instance.Connected`, acrescente:

```go
		instance.NeedsQR = !instance.Connected && instance_model.RequiresQR(instance.Jid, instance.DisconnectReason)
```

E em `Info`, no mesmo ponto (depois do cálculo de `instance.Connected`):

```go
	instance.NeedsQR = !instance.Connected && instance_model.RequiresQR(instance.Jid, instance.DisconnectReason)
```

- [ ] **Step 4: Rodar e ver passar**

Run: `bash scripts/go-test.sh ./pkg/instance/... -v`
Expected: PASS.

Run: `docker build --target build -t evo-go-check .`
Expected: build conclui.

- [ ] **Step 5: Documentar**

Em `docs/CHANGELOG-0.8.0-E-CATALOGO-DE-ROTAS.md`, na tabela de campos e comportamento, acrescente:

```markdown
| `GET /instance/all`, `GET /instance/info/:instanceId` | Campo novo | `needsQr` (bool) | `true` quando a instância está desconectada e só um QR novo resolve (logout pelo celular, logout pela API, janela de QR encerrada, nunca pareada). `false` quando ela volta sozinha ou só precisa de `/instance/connect`. |
```

- [ ] **Step 6: Commit**

```bash
git add pkg/instance/model/disconnect_reason.go pkg/instance/model/disconnect_reason_test.go pkg/instance/model/instance_model.go pkg/instance/service/instance_service.go docs/CHANGELOG-0.8.0-E-CATALOGO-DE-ROTAS.md
git commit -m "feat(instance): expose needsQr on instance listing and info"
```

---

### Task 12: Janela de QR de 160 segundos

Item 10. Com `QRCODE_MAX_COUNT=5`, a 5ª emissão encerra a sessão: só 4 códigos aparecem (~120s), e não os ~160s que o WhatsApp oferece. O padrão passa a 7 (6 códigos: 60 + 5×20 = 160s). Além disso, um valor inválido na variável hoje vira 0 (sem limite); passa a cair no padrão.

**Files:**
- Modify: `pkg/config/config.go` (bloco `qrMaxCount := 5`, ~304-308)
- Create: `pkg/config/qr_max_count_test.go`
- Modify: `docs/wiki/fundamentos/configuration.md` (seção `QRCODE_MAX_COUNT`)
- Modify: `docs/wiki/referencia/faq.md` (seção "Validade do QR Code", linha ~62)
- Modify: `docs/CHANGELOG-0.8.0-E-CATALOGO-DE-ROTAS.md`

**Interfaces:**
- Produces: `defaultQRMaxCount = 7`, `parseQRMaxCount(raw string) int`. A Task 15 (frontend) dimensiona a janela de polling para 160s.

- [ ] **Step 1: Escrever o teste que falha**

`pkg/config/qr_max_count_test.go`:

```go
package config

import "testing"

func TestParseQRMaxCount(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{"", defaultQRMaxCount},
		{"0", 0},
		{"3", 3},
		{" 9 ", 9},
		{"abc", defaultQRMaxCount},
		{"-1", defaultQRMaxCount},
	}
	for _, tt := range tests {
		if got := parseQRMaxCount(tt.raw); got != tt.want {
			t.Errorf("parseQRMaxCount(%q) = %d, want %d", tt.raw, got, tt.want)
		}
	}
	if defaultQRMaxCount != 7 {
		t.Fatalf("defaultQRMaxCount = %d, want 7 (6 codes, ~160 s)", defaultQRMaxCount)
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `bash scripts/go-test.sh ./pkg/config/ -run TestParseQRMaxCount -v`
Expected: FAIL — `undefined: parseQRMaxCount`.

- [ ] **Step 3: Implementar**

Em `config.go`, troque:

```go
	qrMaxCount := 5 // Valor padrão
	if qrcodeMaxCount != "" {
		qrMaxCount, _ = strconv.Atoi(qrcodeMaxCount)
	}
```

por:

```go
	qrMaxCount := parseQRMaxCount(qrcodeMaxCount)
```

E acrescente ao arquivo (fora de `Load`):

```go
// defaultQRMaxCount: a 7ª emissão encerra a sessão, então 6 códigos aparecem
// (60 s + 5 × 20 s = ~160 s), a janela inteira que o WhatsApp oferece.
const defaultQRMaxCount = 7

// parseQRMaxCount lê QRCODE_MAX_COUNT. 0 desliga o limite; vazio ou inválido
// usa o padrão (antes, um valor inválido virava 0 e desligava o limite).
func parseQRMaxCount(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultQRMaxCount
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return defaultQRMaxCount
	}
	return n
}
```

(`strings` e `strconv` já são importados em `config.go`.)

- [ ] **Step 4: Rodar e ver passar**

Run: `bash scripts/go-test.sh ./pkg/config/ -v`
Expected: PASS.

- [ ] **Step 5: Documentar**

Em `docs/wiki/fundamentos/configuration.md`, na seção `### QRCODE_MAX_COUNT`, troque o padrão `5` por `7` e explique: "com 7, aparecem 6 códigos — o primeiro vale ~60 s e os seguintes ~20 s, ~160 s no total. `0` desliga o limite."

Em `docs/wiki/referencia/faq.md`, troque a linha 62 por:

```markdown
O primeiro QR vale cerca de 60 segundos e cada um dos seguintes cerca de 20 segundos. Com o padrão `QRCODE_MAX_COUNT=7`, a janela total é de ~160 segundos (6 códigos); depois disso é preciso pedir um QR novo. O `GET /instance/qr` informa a validade em `expiresAt`/`expiresIn`.
```

Em `docs/CHANGELOG-0.8.0-E-CATALOGO-DE-ROTAS.md`, na tabela de variáveis de ambiente, acrescente:

```markdown
| `QRCODE_MAX_COUNT` | `7` (antes `5`) | Quantas emissões de QR antes de encerrar a sessão de pareamento. Com 7, a janela vai a ~160 s (antes ~120 s). Valor inválido agora usa o padrão em vez de desligar o limite. |
```

- [ ] **Step 6: Commit**

```bash
git add pkg/config/config.go pkg/config/qr_max_count_test.go docs/wiki/fundamentos/configuration.md docs/wiki/referencia/faq.md docs/CHANGELOG-0.8.0-E-CATALOGO-DE-ROTAS.md
git commit -m "fix(qr): default QRCODE_MAX_COUNT to 7 to use the full 160 s pairing window"
```

---

## Parte B — evoproxy-backend (branch `fix/alinhamento-evogo-0.8.0`)

Diretório: `C:\Users\Maxsuel\Projetos\evproxy\evoproxy-backend`.

### Task 13: "Precisa parear" pelo motivo da desconexão

Item 5. O proxy marca `precisaParear` só quando a instância nunca teve número. Depois de um logout pelo celular o número continua, o proxy responde `temDevice: true, precisaParear: false`, o frontend mostra "Reconectar" e o usuário fica preso. Passa a usar o `needsQr` da EvGo 0.8.0 (Task 11) e, com uma EvGo mais antiga (0.7.6), o `disconnect_reason`.

**Files:**
- Modify: `src/lib/instanceLive.js` (`fetchEvoLive`, `connectionStatus`, `module.exports`)
- Create: `test/status.test.js`
- Modify: `guia-evoproxy.md` (seção "O objeto Instância")

**Interfaces:**
- Consumes: `needsQr` e `disconnect_reason` de cada item do `GET /instance/all` (Task 11; funciona sem ela).
- Produces: `evoNeedsQr(it)`, `liveRecord(it)`; `connectionStatus` passa a devolver também `motivoDesconexao`. A Task 14 estende `connectionStatus`.

- [ ] **Step 1: Escrever os testes que falham**

`test/status.test.js`:

```js
const test = require("node:test");
const assert = require("node:assert");

process.env.INSTANCE_JWT_SECRET = process.env.INSTANCE_JWT_SECRET || "segredo-de-teste";

const { liveRecord, connectionStatus } = require("../src/lib/instanceLive");

const JID = "5511999999999:12@s.whatsapp.net";

function statusOf(item) {
  const live = { ok: true, map: new Map([["evo-1", liveRecord(item)]]) };
  return connectionStatus({ evoInstanceId: "evo-1", evoInstanceName: "x" }, live);
}

test("logout pelo celular (jid mantido) → precisa parear, sem device", () => {
  const s = statusOf({ id: "evo-1", jid: JID, connected: false, disconnect_reason: "401: logged out from another device" });
  assert.equal(s.precisaParear, true);
  assert.equal(s.temDevice, false);
  assert.equal(s.motivoDesconexao, "401: logged out from another device");
});

test("queda comum (jid mantido) → só reconectar", () => {
  const s = statusOf({ id: "evo-1", jid: JID, connected: false, disconnect_reason: "Reconnecting" });
  assert.equal(s.precisaParear, false);
  assert.equal(s.temDevice, true);
});

test("needsQr da EvGo 0.8.0 tem prioridade sobre o motivo", () => {
  const s = statusOf({ id: "evo-1", jid: JID, connected: false, disconnect_reason: "Reconnecting", needsQr: true });
  assert.equal(s.precisaParear, true);
});

test("nunca pareada → precisa parear", () => {
  const s = statusOf({ id: "evo-1", jid: "", connected: false, disconnect_reason: "" });
  assert.equal(s.precisaParear, true);
  assert.equal(s.temDevice, false);
});

test("conectada → não precisa parear", () => {
  const s = statusOf({ id: "evo-1", jid: JID, connected: true, disconnect_reason: "" });
  assert.equal(s.statusConexao, "CONNECTED");
  assert.equal(s.precisaParear, false);
  assert.equal(s.motivoDesconexao, null);
});

test("instância que sumiu da EvGo → precisa parear", () => {
  const s = connectionStatus({ evoInstanceId: "outra" }, { ok: true, map: new Map() });
  assert.equal(s.precisaParear, true);
});

test("EvGo fora do ar → UNKNOWN sem afirmar nada", () => {
  const s = connectionStatus({ evoInstanceId: "evo-1" }, { ok: false, map: new Map() });
  assert.equal(s.statusConexao, "UNKNOWN");
  assert.equal(s.precisaParear, false);
});
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `npm test`
Expected: FAIL — `liveRecord is not a function`.

- [ ] **Step 3: Implementar**

Em `src/lib/instanceLive.js`, acima de `fetchEvoLive`, acrescente:

```js
// Motivos de desconexão (disconnect_reason da EvGo) em que o aparelho foi
// desvinculado e só um QR novo resolve. O jid continua gravado nesses casos,
// então "tem número" não basta para decidir entre reconectar e parear.
const NEEDS_QR_PREFIXES = [
  "401:",
  "403:",
  "406:",
  "Logged out by API",
  "Maximum QR code count",
  "QR socket closed",
];

// Precisa de QR novo? Usa o needsQr da EvGo 0.8.0+; em versões anteriores,
// deduz pelo jid e pelo motivo.
function evoNeedsQr(it) {
  if (it.connected) return false;
  if (typeof it.needsQr === "boolean") return it.needsQr;
  if (!it.jid) return true;
  const motivo = String(it.disconnect_reason || "");
  return NEEDS_QR_PREFIXES.some((p) => motivo.startsWith(p));
}

// Registro de conexão de uma instância do /instance/all.
function liveRecord(it) {
  const numero = it.jid ? String(it.jid).split(/[:@]/)[0] || null : null;
  const needsQr = evoNeedsQr(it);
  return {
    connected: !!it.connected,
    numero,
    // device só conta se ainda dá para retomar a sessão sem QR.
    temDevice: !!it.jid && !needsQr,
    needsQr,
    motivo: it.disconnect_reason || null,
  };
}
```

Em `fetchEvoLive`, troque:

```js
      for (const it of arr) {
        const numero = it.jid ? String(it.jid).split(/[:@]/)[0] || null : null;
        // jid presente = já existe um device/número pareado nessa instância.
        const rec = { connected: !!it.connected, numero, temDevice: !!it.jid };
        if (it.id) map.set(it.id, rec);
        if (it.name) map.set(it.name, rec);
      }
```

por:

```js
      for (const it of arr) {
        const rec = liveRecord(it);
        if (it.id) map.set(it.id, rec);
        if (it.name) map.set(it.name, rec);
      }
```

Troque a função `connectionStatus` inteira por:

```js
// Status de conexão ENXUTO (para polling /status — sem foto, sem metadados).
function connectionStatus(i, live) {
  const info = live.map.get(i.evoInstanceId) || live.map.get(i.evoInstanceName) || null;
  const connected = info ? info.connected : false;
  const numero = info ? info.numero : null;
  const temDevice = info ? info.temDevice : false;
  const statusConexao = !live.ok ? "UNKNOWN" : connected ? "CONNECTED" : "DISCONNECTED";
  // Instância que sumiu da EvGo também precisa de QR (a sessão não existe mais).
  const needsQr = info ? info.needsQr : true;
  const precisaParear = live.ok && !connected && needsQr;
  const motivoDesconexao = info && !connected ? info.motivo : null;
  return { statusConexao, numero, temDevice, precisaParear, motivoDesconexao };
}
```

E acrescente `evoNeedsQr` e `liveRecord` ao `module.exports`.

- [ ] **Step 4: Rodar e ver passar**

Run: `npm test`
Expected: PASS em todos os testes (os 18 existentes + 7 novos).

- [ ] **Step 5: Documentar**

Em `guia-evoproxy.md`, na tabela de campos de "O objeto Instância", troque as linhas de `temDevice` e `precisaParear` e acrescente `motivoDesconexao`:

```markdown
| `temDevice` | `true` se há uma sessão do WhatsApp que pode ser retomada sem QR | Desconectada com `temDevice: true` = só reconectar |
| `precisaParear` | `true` se está desconectada e só um QR novo resolve: nunca pareada, ou o aparelho foi desvinculado (logout pelo celular, logout pela API, janela de QR encerrada) | Mostrar o fluxo de QR/código |
| `motivoDesconexao` | Motivo informado pela Evolution GO quando desconectada (ex.: `"401: logged out from another device"`, `"Reconnecting"`), ou `null` | Diagnóstico e mensagens para o operador |
```

Faça o mesmo no exemplo de resposta de `GET /instances/:id/status` (acrescente `"motivoDesconexao": null`).

- [ ] **Step 6: Commit**

```bash
git add src/lib/instanceLive.js test/status.test.js guia-evoproxy.md
git commit -m "fix(status): precisaParear also when the phone unlinked the device (jid kept)"
```

---

### Task 14: Campo `alertas` no status das instâncias

Item 9 (parte do proxy). O status passa a dizer, em códigos estáveis, o que precisa de atenção. O painel e as notificações consomem isso depois (ver "Fora deste plano").

**Files:**
- Modify: `src/lib/instanceLive.js` (`connectionStatus`, `module.exports`)
- Modify: `test/status.test.js`
- Modify: `guia-evoproxy.md`

**Interfaces:**
- Consumes: `connectionStatus` e `motivoDesconexao` (Task 13); textos de motivo das Tasks 8 e 11.
- Produces: `alertasDaInstancia(status) string[]`; `connectionStatus` devolve `alertas`. Códigos: `PRECISA_QR`, `BANIDA`, `SESSAO_SUBSTITUIDA`, `DESCONECTADA`.

- [ ] **Step 1: Escrever os testes que falham**

Em `test/status.test.js`, acrescente:

```js
const { alertasDaInstancia } = require("../src/lib/instanceLive");

test("alertas: precisa de QR", () => {
  const s = statusOf({ id: "evo-1", jid: JID, connected: false, disconnect_reason: "401: logged out from another device" });
  assert.deepEqual(s.alertas, ["PRECISA_QR"]);
});

test("alertas: ban temporário", () => {
  const s = statusOf({ id: "evo-1", jid: JID, connected: false, disconnect_reason: "Temporary ban until 2026-10-12T15:00:00Z: spam" });
  assert.deepEqual(s.alertas, ["BANIDA"]);
});

test("alertas: sessão assumida por outro processo", () => {
  const s = statusOf({ id: "evo-1", jid: JID, connected: false, disconnect_reason: "Stream replaced by another connection" });
  assert.deepEqual(s.alertas, ["SESSAO_SUBSTITUIDA"]);
});

test("alertas: desconectada por outro motivo", () => {
  const s = statusOf({ id: "evo-1", jid: JID, connected: false, disconnect_reason: "Reconnecting" });
  assert.deepEqual(s.alertas, ["DESCONECTADA"]);
});

test("alertas: desligada pela API não alerta (foi de propósito)", () => {
  const s = statusOf({ id: "evo-1", jid: JID, connected: false, disconnect_reason: "Disconnected by API" });
  assert.deepEqual(s.alertas, []);
});

test("alertas: conectada ou EvGo fora do ar não alertam", () => {
  assert.deepEqual(statusOf({ id: "evo-1", jid: JID, connected: true }).alertas, []);
  assert.deepEqual(alertasDaInstancia({ statusConexao: "UNKNOWN", precisaParear: false, motivoDesconexao: null }), []);
});
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `npm test`
Expected: FAIL — `alertasDaInstancia is not a function` / `s.alertas` indefinido.

- [ ] **Step 3: Implementar**

Em `src/lib/instanceLive.js`, acima de `connectionStatus`, acrescente:

```js
// O que precisa de atenção numa instância desconectada, em códigos estáveis
// para o painel e as notificações. Desligada pela API não alerta: foi pedido.
function alertasDaInstancia(status) {
  if (status.statusConexao !== "DISCONNECTED") return [];
  const motivo = String(status.motivoDesconexao || "");
  if (status.precisaParear) return ["PRECISA_QR"];
  if (motivo.startsWith("Temporary ban until")) return ["BANIDA"];
  if (motivo === "Stream replaced by another connection") return ["SESSAO_SUBSTITUIDA"];
  if (motivo === "Disconnected by API") return [];
  return ["DESCONECTADA"];
}
```

No final de `connectionStatus`, troque o `return` por:

```js
  const status = { statusConexao, numero, temDevice, precisaParear, motivoDesconexao };
  return { ...status, alertas: alertasDaInstancia(status) };
```

E acrescente `alertasDaInstancia` ao `module.exports`.

- [ ] **Step 4: Rodar e ver passar**

Run: `npm test`
Expected: PASS.

- [ ] **Step 5: Documentar**

Em `guia-evoproxy.md`, na tabela de "O objeto Instância", acrescente:

```markdown
| `alertas` | Lista do que precisa de atenção: `PRECISA_QR` (gerar QR novo), `BANIDA` (ban temporário do WhatsApp; aguarde o prazo), `SESSAO_SUBSTITUIDA` (outra conexão assumiu a sessão), `DESCONECTADA` (caiu e ainda não voltou). Vazia quando conectada ou desligada de propósito. | Destacar a instância no painel e avisar o responsável |
```

E acrescente `"alertas": []` ao exemplo de resposta de `GET /instances/:id/status`.

- [ ] **Step 6: Commit**

```bash
git add src/lib/instanceLive.js test/status.test.js guia-evoproxy.md
git commit -m "feat(status): alertas on instance status (needs QR, banned, replaced, disconnected)"
```

---

### Task 17: Hora em que a instância caiu (`desconectadaDesde`)

Item 9. A EvGo não guarda quando a instância caiu. O proxy anota no Redis o primeiro instante em que viu cada instância desconectada (na leitura do `/instance/all`, que o polling dos painéis já faz) e apaga a anotação quando ela volta. O status passa a trazer `desconectadaDesde`.

**Files:**
- Create: `src/lib/offlineSince.js`
- Create: `test/offlineSince.test.js`
- Modify: `src/lib/instanceLive.js` (`fetchEvoLive`, `connectionStatus`)
- Modify: `test/status.test.js`
- Modify: `guia-evoproxy.md`

**Interfaces:**
- Consumes: `liveRecord`, `connectionStatus` (Tasks 13 e 14); cliente Redis `src/lib/redis.js`.
- Produces: `planOfflineSince(known, current, now) => { set, clear }` (pura), `syncOfflineSince(records) => Promise<Map<id, epochMs>>`; `connectionStatus` devolve `desconectadaDesde` (ISO 8601 ou `null`). A Task 18 usa o campo.

- [ ] **Step 1: Escrever os testes que falham**

`test/offlineSince.test.js`:

```js
const test = require("node:test");
const assert = require("node:assert");

const { planOfflineSince } = require("../src/lib/offlineSince");

test("anota só a primeira vez que a instância aparece desconectada", () => {
  const plan = planOfflineSince({ a: 1000 }, [
    { id: "a", connected: false },
    { id: "b", connected: false },
  ], 5000);
  assert.deepEqual(plan.set, { b: 5000 });
  assert.deepEqual(plan.clear, []);
});

test("apaga a anotação quando a instância volta", () => {
  const plan = planOfflineSince({ a: 1000 }, [{ id: "a", connected: true }], 5000);
  assert.deepEqual(plan.set, {});
  assert.deepEqual(plan.clear, ["a"]);
});

test("apaga a anotação de instância que sumiu da EvGo", () => {
  const plan = planOfflineSince({ gone: 1000 }, [], 5000);
  assert.deepEqual(plan.clear, ["gone"]);
});
```

Em `test/status.test.js`, acrescente:

```js
test("desconectadaDesde vem em ISO quando há anotação, e null conectada", () => {
  const rec = liveRecord({ id: "evo-1", jid: JID, connected: false, disconnect_reason: "Reconnecting" });
  rec.offlineSince = Date.UTC(2026, 9, 12, 10, 0, 0);
  const s = connectionStatus({ evoInstanceId: "evo-1" }, { ok: true, map: new Map([["evo-1", rec]]) });
  assert.equal(s.desconectadaDesde, "2026-10-12T10:00:00.000Z");
  assert.equal(statusOf({ id: "evo-1", jid: JID, connected: true }).desconectadaDesde, null);
});
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `npm test`
Expected: FAIL — `Cannot find module '../src/lib/offlineSince'`.

- [ ] **Step 3: Implementar**

`src/lib/offlineSince.js`:

```js
// ---------------------------------------------------------------
// Hora em que cada instância foi vista desconectada pela primeira vez.
//
// A EvGo não guarda quando a instância caiu. O proxy anota no Redis (hash
// evo:offline-since, evoInstanceId → epoch ms) na leitura do /instance/all e
// apaga quando ela volta. É "visto pela primeira vez": a precisão é a do
// intervalo entre leituras (polling dos painéis, cache de 2.5 s).
// ---------------------------------------------------------------
const redis = require("./redis");

const KEY = "evo:offline-since";

// Decide o que anotar e o que apagar. known: { id: epochMs } já no Redis;
// current: [{ id, connected }] da leitura atual.
function planOfflineSince(known, current, now) {
  const set = {};
  const vistos = new Set();
  for (const { id, connected } of current) {
    vistos.add(id);
    if (!connected && known[id] === undefined) set[id] = now;
  }
  const clear = Object.keys(known).filter((id) => {
    const atual = current.find((c) => c.id === id);
    return !vistos.has(id) || (atual && atual.connected);
  });
  return { set, clear };
}

// Aplica o plano no Redis e devolve Map(id → epochMs) das desconectadas.
// Best-effort: sem Redis, devolve vazio e o status sai sem a hora.
async function syncOfflineSince(current, now = Date.now()) {
  try {
    const raw = await redis.hgetall(KEY);
    const known = Object.fromEntries(Object.entries(raw || {}).map(([k, v]) => [k, Number(v)]));
    const { set, clear } = planOfflineSince(known, current, now);
    if (Object.keys(set).length) await redis.hset(KEY, set);
    if (clear.length) await redis.hdel(KEY, ...clear);
    for (const id of clear) delete known[id];
    return new Map(Object.entries({ ...known, ...set }));
  } catch {
    return new Map();
  }
}

module.exports = { planOfflineSince, syncOfflineSince };
```

Em `src/lib/instanceLive.js`, importe `const { syncOfflineSince } = require("./offlineSince");`. Em `fetchEvoLive`, no ramo de sucesso, depois do `for (const it of arr) { ... }`, acrescente:

```js
      // Hora em que cada instância foi vista caindo (Redis; best-effort).
      const desde = await syncOfflineSince(
        arr.filter((it) => it.id).map((it) => ({ id: it.id, connected: !!it.connected }))
      );
      for (const it of arr) {
        if (it.id && desde.has(it.id)) map.get(it.id).offlineSince = desde.get(it.id);
      }
```

Em `connectionStatus`, troque a montagem de `status` por:

```js
  const desconectadaDesde =
    info && !connected && info.offlineSince ? new Date(info.offlineSince).toISOString() : null;
  const status = { statusConexao, numero, temDevice, precisaParear, motivoDesconexao, desconectadaDesde };
```

- [ ] **Step 4: Rodar e ver passar**

Run: `npm test`
Expected: PASS.

- [ ] **Step 5: Documentar**

Em `guia-evoproxy.md`, na tabela de "O objeto Instância", acrescente:

```markdown
| `desconectadaDesde` | Quando o proxy viu a instância desconectada pela primeira vez (ISO 8601), ou `null` se conectada | Mostrar "desconectada há X min" e decidir quando escalar o alerta |
```

- [ ] **Step 6: Commit**

```bash
git add src/lib/offlineSince.js test/offlineSince.test.js src/lib/instanceLive.js test/status.test.js guia-evoproxy.md
git commit -m "feat(status): desconectadaDesde, tracked in Redis when the proxy first sees an instance down"
```

---

### Task 18: `GET /admin/alertas` — visão de plataforma

Item 9. O backoffice precisa ver, de uma vez, todas as instâncias de todas as contas que pedem ação.

**Files:**
- Modify: `src/lib/instanceLive.js` (nova função pura + export)
- Modify: `src/routes/admin.js` (rota nova, antes de `POST /scan`)
- Modify: `test/status.test.js`
- Modify: `guia-evoproxy.md` (seção de admin)

**Interfaces:**
- Consumes: `connectionStatus` com `alertas` e `desconectadaDesde` (Tasks 14 e 17), `fetchEvoLive`.
- Produces: `instanciasComAlerta(instances, live) => Array<{ clientId, cliente, codeTenant, instanceId, name, department, statusConexao, alertas, motivoDesconexao, desconectadaDesde }>`, ordenada da queda mais antiga para a mais recente.

- [ ] **Step 1: Escrever o teste que falha**

Em `test/status.test.js`, acrescente:

```js
const { instanciasComAlerta } = require("../src/lib/instanceLive");

test("instanciasComAlerta: só as que pedem ação, da queda mais antiga para a mais nova", () => {
  const recA = liveRecord({ id: "a", jid: JID, connected: false, disconnect_reason: "Reconnecting" });
  recA.offlineSince = 2000;
  const recB = liveRecord({ id: "b", jid: JID, connected: false, disconnect_reason: "401: logged out from another device" });
  recB.offlineSince = 1000;
  const recC = liveRecord({ id: "c", jid: JID, connected: true });
  const live = { ok: true, map: new Map([["a", recA], ["b", recB], ["c", recC]]) };
  const client = { id: "cli", name: "Domarco", codeTenant: "abc123" };
  const instances = ["a", "b", "c"].map((id) => ({
    id: `inst-${id}`, evoInstanceId: id, name: id, department: "COMERCIAL", clientId: "cli", client,
  }));

  const got = instanciasComAlerta(instances, live);
  assert.deepEqual(got.map((i) => [i.instanceId, i.alertas]), [
    ["inst-b", ["PRECISA_QR"]],
    ["inst-a", ["DESCONECTADA"]],
  ]);
  assert.equal(got[0].cliente, "Domarco");
});
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `npm test`
Expected: FAIL — `instanciasComAlerta is not a function`.

- [ ] **Step 3: Implementar**

Em `src/lib/instanceLive.js`, acrescente e exporte:

```js
// Instâncias (de qualquer conta) que pedem ação, para a visão de plataforma.
// `instances` vem do Prisma com `client: { select: { name, codeTenant } }`.
function instanciasComAlerta(instances, live) {
  return instances
    .map((i) => ({ i, s: connectionStatus(i, live) }))
    .filter(({ s }) => s.alertas.length > 0)
    .sort((x, y) => (x.s.desconectadaDesde || "").localeCompare(y.s.desconectadaDesde || ""))
    .map(({ i, s }) => ({
      clientId: i.clientId,
      cliente: i.client ? i.client.name : null,
      codeTenant: i.client ? i.client.codeTenant : null,
      instanceId: i.id,
      name: i.name,
      department: i.department,
      statusConexao: s.statusConexao,
      alertas: s.alertas,
      motivoDesconexao: s.motivoDesconexao,
      desconectadaDesde: s.desconectadaDesde,
    }));
}
```

Em `src/routes/admin.js`, importe `instanciasComAlerta` junto dos outros helpers de `../lib/instanceLive` e acrescente, antes de `router.post("/scan", ...)`:

```js
// GET /admin/alertas — instâncias de TODAS as contas que pedem ação (precisa
// de QR, ban, sessão substituída, desconectada), da queda mais antiga para a
// mais recente. Lista vazia quando está tudo no ar.
router.get("/alertas", async (req, res) => {
  try {
    const instances = await prisma.instance.findMany({
      include: { client: { select: { name: true, codeTenant: true } } },
    });
    const live = await fetchEvoLive();
    if (!live.ok) {
      return res.status(502).json({ message: "Não foi possível ler o estado da Evolution GO." });
    }
    res.status(200).json(instanciasComAlerta(instances, live));
  } catch (err) {
    handleError(res, err, "admin alertas");
  }
});
```

- [ ] **Step 4: Rodar e ver passar**

Run: `npm test`
Expected: PASS.

Run (smoke de carga das rotas): `node -e "process.env.INSTANCE_JWT_SECRET='x';process.env.EVOLUTION_API_URL='http://127.0.0.1:1';process.env.EVOLUTION_API_KEY='k';const r=require('./src/routes/admin');console.log(r.stack.some(l=>l.route&&l.route.path==='/alertas'));process.exit(0)"`
Expected: `true`.

- [ ] **Step 5: Documentar**

Em `guia-evoproxy.md`, na seção "Plataforma" do admin, acrescente:

```markdown
| `GET /admin/alertas` | Instâncias de todas as contas que pedem ação, da queda mais antiga para a mais recente: `[{ clientId, cliente, codeTenant, instanceId, name, department, statusConexao, alertas, motivoDesconexao, desconectadaDesde }]`. `502` se a Evolution GO não responder. |
```

- [ ] **Step 6: Commit**

```bash
git add src/lib/instanceLive.js src/routes/admin.js test/status.test.js guia-evoproxy.md
git commit -m "feat(admin): GET /admin/alertas lists instances that need attention across accounts"
```

---

## Parte C — Frontends (entregue como documento; não implementado neste plano)

> As equipes do **Mobilium-Frontend-V2** e do **mobpainel** recebem um documento de instruções com o porquê, as rotas e as mudanças. A Task 15 abaixo fica como referência técnica do que foi recomendado para o Mobilium-Frontend-V2.

## Parte C (referência) — Mobilium-Frontend-V2 (branch nova `fix/whatsapp-qr-tempos`, a partir de `development`)

Diretório: `C:\Users\Maxsuel\Projetos\frontendMobilium\Mobilium-Frontend-V2`. Antes da tarefa: `git switch development; git pull; git switch -c fix/whatsapp-qr-tempos`.

### Task 15: Validade real do QR na tela

Item 6. O card trata todo QR como de 20s, inclusive o primeiro (que vale 60s): aos ~30s mostra "QR expirado" com o código ainda bom. Também pede o QR no mesmo instante do `connect`, o que na 0.8.0 (sem a Task 6) derrubava a sessão que abria. Passa a usar a validade que a EvGo informa (`expiresAt`, Task 1) ou, sem ela, 60s para o primeiro e 20s para os seguintes; espera 1,5s antes do primeiro `GET /qr`; e a janela de polling cobre os 160s da Task 12.

**Files:**
- Create: `src/lib/whatsapp-qr.ts`
- Create: `src/lib/whatsapp-qr.test.ts`
- Modify: `src/constants/timing.ts` (`WHATSAPP_MAX_POLLS`, `WHATSAPP_QR_TTL_MS`)
- Modify: `src/api/integrations/whattsapp/index.ts` (`getInstanceQr`, ~448-464)
- Modify: `src/pages/app/settings/components/whattsapp-card.tsx` (imports ~58-64; refs/estado ~118-158; `applyQr` ~171-178; `refreshQr` ~194-207; polling ~258-289; `handlePairQr` ~337-354; contador ~466-468)

**Interfaces:**
- Consumes: `data.expiresAt` (unix, segundos) do `GET /instances/:id/qr` (EvGo 0.8.0, Task 1). Sem ele, usa o padrão.
- Produces: `qrValidUntil(receivedAt: number, index: number, expiresAt?: number | null): number`, `EvoQr = { qrcode: string; expiresAt: number | null }`, `getInstanceQr(id): Promise<EvoQr | null>`.

- [ ] **Step 1: Escrever os testes que falham**

`src/lib/whatsapp-qr.test.ts`:

```ts
import { describe, expect, it } from 'vitest'

import {
  QR_FIRST_LIFETIME_MS,
  QR_NEXT_LIFETIME_MS,
  qrValidUntil,
} from './whatsapp-qr'

describe('qrValidUntil', () => {
  const recebidoEm = 1_000_000

  it('o primeiro QR da sessão vale 60 s', () => {
    expect(qrValidUntil(recebidoEm, 0)).toBe(recebidoEm + QR_FIRST_LIFETIME_MS)
    expect(QR_FIRST_LIFETIME_MS).toBe(60_000)
  })

  it('os QRs seguintes valem 20 s', () => {
    expect(qrValidUntil(recebidoEm, 1)).toBe(recebidoEm + QR_NEXT_LIFETIME_MS)
    expect(qrValidUntil(recebidoEm, 4)).toBe(recebidoEm + QR_NEXT_LIFETIME_MS)
    expect(QR_NEXT_LIFETIME_MS).toBe(20_000)
  })

  it('a validade informada pela EvGo (unix em segundos) tem prioridade', () => {
    expect(qrValidUntil(recebidoEm, 0, 2_000)).toBe(2_000_000)
  })

  it('validade ausente ou zerada cai no padrão', () => {
    expect(qrValidUntil(recebidoEm, 0, null)).toBe(recebidoEm + QR_FIRST_LIFETIME_MS)
    expect(qrValidUntil(recebidoEm, 1, 0)).toBe(recebidoEm + QR_NEXT_LIFETIME_MS)
  })
})
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `npx vitest run src/lib/whatsapp-qr.test.ts`
Expected: FAIL — `Failed to resolve import "./whatsapp-qr"`.

- [ ] **Step 3: Implementar a regra**

`src/lib/whatsapp-qr.ts`:

```ts
// Validade do QR de pareamento do WhatsApp. O primeiro código de uma sessão
// vale ~60 s e cada um dos seguintes ~20 s; a Evolution GO 0.8.0+ informa a
// validade exata em `expiresAt` (unix, segundos), que tem prioridade.
export const QR_FIRST_LIFETIME_MS = 60_000
export const QR_NEXT_LIFETIME_MS = 20_000

/**
 * Até quando (epoch em ms) o QR na tela vale.
 * @param receivedAt quando o código chegou (Date.now()).
 * @param index posição do código na sessão (0 = primeiro).
 * @param expiresAt validade informada pela EvGo, em segundos; ausente = padrão.
 */
export function qrValidUntil(
  receivedAt: number,
  index: number,
  expiresAt?: number | null,
): number {
  if (expiresAt && expiresAt > 0) return expiresAt * 1_000
  return receivedAt + (index === 0 ? QR_FIRST_LIFETIME_MS : QR_NEXT_LIFETIME_MS)
}
```

- [ ] **Step 4: Rodar e ver passar**

Run: `npx vitest run src/lib/whatsapp-qr.test.ts`
Expected: PASS nos 4 testes.

- [ ] **Step 5: API devolve a validade junto com o QR**

Em `src/api/integrations/whattsapp/index.ts`, troque `getInstanceQr` (e o comentário acima dele) por:

```ts
/** QR de pareamento e até quando ele vale (unix em segundos, se a EvGo informar). */
export interface EvoQr {
  qrcode: string
  expiresAt: number | null
}

/**
 * Via QR (passo 2): devolve o QR pronto (`data:image/png;base64,…`) e a
 * validade, ou null se ainda não há QR / já pareada.
 *
 * O proxy responde `{ data: { qrcode, code, expiresAt? }, message }`. Builds
 * antigas do proxy usavam `Qrcode` (maiúsculo); os dois nomes são aceitos
 * porque os tenants não rodam todos a mesma versão. `expiresAt` só vem da
 * Evolution GO 0.8.0+.
 */
export async function getInstanceQr(id: string): Promise<EvoQr | null> {
  const raw = await evoRequest<{
    data?: { qrcode?: string; Qrcode?: string; expiresAt?: number }
    error?: string
  }>(`/instances/${id}/qr`, 'GET', undefined, EVO_POLL_TIMEOUT_MS)
  const qrcode = raw?.data?.qrcode ?? raw?.data?.Qrcode ?? null
  if (!qrcode) return null
  return { qrcode, expiresAt: raw?.data?.expiresAt ?? null }
}
```

- [ ] **Step 6: Constantes**

Em `src/constants/timing.ts`, troque as linhas de `WHATSAPP_MAX_POLLS` e de `WHATSAPP_QR_TTL_MS` (com seus comentários) por:

```ts
// Backend caches /status responses every 2.5 s. 66 polls ≈ 165 s: cobre a
// janela de pareamento da Evolution GO (~160 s: 1º QR 60 s + 5 × 20 s).
export const WHATSAPP_POLL_INTERVAL_MS = 2_500
export const WHATSAPP_MAX_POLLS = 66
// Espera entre iniciar a sessão (connect/repair) e o primeiro GET /qr: o socket
// ainda está abrindo, e pedir o QR no mesmo instante fazia a Evolution GO 0.8.0
// recriar a sessão.
export const WHATSAPP_QR_FIRST_FETCH_DELAY_MS = 1_500
```

(Remova a declaração duplicada de `WHATSAPP_POLL_INTERVAL_MS` se ela ficar repetida. A validade do QR passa a vir de `qrValidUntil` em `src/lib/whatsapp-qr.ts`; `WHATSAPP_QR_TTL_MS` deixa de existir.)

- [ ] **Step 7: Card usa a validade real**

Em `whattsapp-card.tsx`:

1. Nos imports de `@/constants/timing`, troque `WHATSAPP_QR_TTL_MS` por `WHATSAPP_QR_FIRST_FETCH_DELAY_MS`. Acrescente `import { qrValidUntil } from '@/lib/whatsapp-qr'` e acrescente `type EvoQr` ao import de `@/api/integrations/whattsapp`. Rode `npm run lint` depois para ordenar os imports.

2. Logo abaixo de `const [qrAt, setQrAt] = useState(0)`, acrescente:

```tsx
  // Até quando o QR atual vale (epoch ms). Vem da EvGo (`expiresAt`) ou do
  // padrão do WhatsApp: 60 s para o primeiro código da sessão, 20 s depois.
  const [qrUntil, setQrUntil] = useState(0)
```

3. Junto das refs (`qrAtRef`, `qrRef`), acrescente:

```tsx
  const qrUntilRef = useRef(0)
  // Quantos códigos já chegaram nesta sessão de pareamento (0 = nenhum).
  const qrIndexRef = useRef(0)
```

4. Troque `applyQr` por:

```tsx
  // Único ponto que grava o QR — mantém estado e refs em sincronia.
  const applyQr = useCallback((next: EvoQr | null) => {
    const now = Date.now()
    const until = next ? qrValidUntil(now, qrIndexRef.current, next.expiresAt) : 0
    qrIndexRef.current = next ? qrIndexRef.current + 1 : 0
    qrAtRef.current = next ? now : 0
    qrUntilRef.current = until
    qrRef.current = next?.qrcode ?? null
    setQrAt(qrAtRef.current)
    setQrUntil(until)
    setQr(qrRef.current)
  }, [])
```

5. Em `refreshQr`, troque:

```tsx
      const code = await getInstanceQr(instance.id)
      if (!code || code === qrRef.current) return false
      applyQr(code)
      return true
```

por:

```tsx
      const next = await getInstanceQr(instance.id)
      if (!next || next.qrcode === qrRef.current) return false
      applyQr(next)
      return true
```

6. No intervalo do `startPolling`, troque:

```tsx
          const idadeQr = qrAtRef.current
            ? Date.now() - qrAtRef.current
            : Number.POSITIVE_INFINITY
          if (mode === 'qr' && idadeQr >= WHATSAPP_QR_TTL_MS) {
            if (!qrAtRef.current) {
              // Sessão recém-aberta, ainda sem QR nenhum: só aguarda o primeiro
              // código chegar — reconectar aqui reiniciaria o socket e atrasaria.
              const code = await getInstanceQr(instance.id)
              if (code) applyQr(code)
```

por:

```tsx
          // O QR só é re-buscado depois de vencer (60 s o primeiro, 20 s os
          // seguintes, ou o que a EvGo informar). Sem QR ainda, busca a cada tick.
          const qrVenceu = !qrAtRef.current || Date.now() >= qrUntilRef.current
          if (mode === 'qr' && qrVenceu) {
            if (!qrAtRef.current) {
              // Sessão recém-aberta, ainda sem QR nenhum: só aguarda o primeiro
              // código chegar — reconectar aqui reiniciaria o socket e atrasaria.
              const next = await getInstanceQr(instance.id)
              if (next) applyQr(next)
```

7. Em `handlePairQr`, troque:

```tsx
      await startSession()
      const code = await getInstanceQr(instance.id)
      if (code) applyQr(code)
      startPolling('qr')
```

por:

```tsx
      await startSession()
      // O socket ainda está abrindo: pedir o QR no mesmo instante fazia a EvGo
      // recriar a sessão (e o código na tela morria).
      await new Promise((resolve) =>
        setTimeout(resolve, WHATSAPP_QR_FIRST_FETCH_DELAY_MS),
      )
      const next = await getInstanceQr(instance.id)
      if (next) applyQr(next)
      startPolling('qr')
```

8. No contador, troque:

```tsx
  const qrSegundosRestantes = qrAt
    ? Math.max(0, Math.ceil((WHATSAPP_QR_TTL_MS - (agora - qrAt)) / 1_000))
    : 0
```

por:

```tsx
  const qrSegundosRestantes = qrAt
    ? Math.max(0, Math.ceil((qrUntil - agora) / 1_000))
    : 0
```

9. Atualize o comentário de `refreshQr` que diz "Um GET /qr sozinho devolve o último código capturado pela sessão atual — quem faz o WhatsApp emitir outro é o `connect`" para: "A EvGo troca o código guardado a cada rotação; o `connect` só importa quando a sessão de QR já acabou (aí ele abre outra). O código só é aceito quando difere do que está na tela."

- [ ] **Step 8: Verificar**

Run: `npm test`
Expected: PASS (inclusive `src/lib/whatsapp-qr.test.ts`).

Run: `npx tsc -b`
Expected: sem erros (nenhum uso restante de `WHATSAPP_QR_TTL_MS` nem de `getInstanceQr` como `string`).

Run: `npm run lint`
Expected: sem erros.

- [ ] **Step 9: Commit**

```bash
git add src/lib/whatsapp-qr.ts src/lib/whatsapp-qr.test.ts src/constants/timing.ts src/api/integrations/whattsapp/index.ts src/pages/app/settings/components/whattsapp-card.tsx
git commit -m "fix(whatsapp): honor the real QR lifetime (60 s first, 20 s next) and wait before the first fetch"
```

---

## Parte D — Fechamento

### Task 16: Verificação completa e validação manual da EvGo

**Files:** nenhum arquivo novo; atualiza o status no diagnóstico.

- [ ] **Step 1: Suítes completas**

Run (evo-go-v2): `bash scripts/go-test.sh ./pkg/... -race`
Expected: PASS em todos os pacotes.

Run (evo-go-v2): `docker build --target build -t evo-go-check .`
Expected: build conclui.

Run (evoproxy-backend): `npm test`
Expected: PASS.

Run (Mobilium-Frontend-V2): `npm test; npx tsc -b; npm run lint`
Expected: tudo verde.

- [ ] **Step 2: Validação manual (precisa de um número de teste de WhatsApp)**

Suba a EvGo da branch (imagem de desenvolvimento) com uma instância de teste e confira, um por um:

1. **QR normal:** `POST /instance/connect` → `GET /instance/qr` traz `expiresAt`; o primeiro código fica válido ~60s; escanear conecta.
2. **Socket cai no meio do QR (causa A):** durante o QR, `docker network disconnect <rede> <container-evogo>` por ~10s e reconecte. O log mostra `QR socket closed before pairing`, chega um `QRTimeout` com esse `reason`, e o próximo `GET /instance/qr` traz um código novo que conecta.
3. **Restart no meio do QR (causa B):** com um QR na tela, `POST /instance/reconnect` (ou `DELETE /instance/logout`) e depois `GET /instance/qr`. Não aparecem duas sequências `QR code generated #1` intercaladas e o novo QR não cai sozinho no fim da janela antiga.
4. **GET /qr junto do connect (causa E):** chame `POST /instance/connect` e `GET /instance/qr` sem espera, 10 vezes seguidas. O log mostra `still connecting — waiting for its QR`, sem `restarting for QR code`.
5. **Janela:** cronometre do primeiro QR ao `QRTimeout`: ~160s.
6. **Logout pelo celular:** desvincule o aparelho no celular. `GET /instance/all` traz `needsQr: true`; no proxy, `GET /instances/:id/status` traz `precisaParear: true` e `alertas: ["PRECISA_QR"]`; o card do frontend mostra o fluxo de QR, não "Reconectar".
7. **Frontend:** o primeiro QR fica na tela até ~60s sem "QR expirado"; o contador mostra o tempo real.

- [ ] **Step 3: Registrar o resultado**

No topo de `docs/DIAGNOSTICO-QR-E-DESCONEXAO.md`, acrescente uma linha de status com a data, as tarefas concluídas e o resultado da validação manual (item a item). Commit:

```bash
git add docs/DIAGNOSTICO-QR-E-DESCONEXAO.md
git commit -m "docs: record the QR and reconnection fixes validation"
```

---

## Fora deste plano (precisa de decisão antes)

- **Notificação ativa (e-mail/WhatsApp ao responsável):** nesta etapa o alerta aparece nos painéis (`alertas`, `desconectadaDesde`) e no backoffice (`GET /admin/alertas`). Avisar por fora dos painéis exige escolher o canal e o destinatário; o `GET /admin/alertas` já serve de fonte para um job do serviço de notificações quando isso for decidido.
- **Heurística dos 14 dias (celular principal sem uso):** não será implementada. O WhatsApp não documenta o critério e o aviso seria impreciso. A orientação aos clientes é usar o WhatsApp no celular principal com regularidade.
- **Pulso de presença que zera a cada reconexão (D6 do diagnóstico)** e **`LoggedOut` enviado até 3 vezes (D7):** problemas reais, mas sem relação direta com o QR ou com a volta das instâncias; ficam para um próximo ciclo.
- **Produção 0.7.6:** não recebe estas correções; elas chegam com a 0.8.0.

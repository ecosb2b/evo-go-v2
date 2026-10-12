package whatsmeow_service

import (
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types/events"

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
	// Rotations entrega as trocas da chave ADV feitas durante o pareamento.
	Rotations() <-chan *events.RotateADVSecret
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
func qrWait(env qrEmitterEnv, lifetime, poll time.Duration, onRotate func(*events.RotateADVSecret)) (qrEmitterEnd, bool) {
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
		case rot := <-env.Rotations():
			if rot != nil {
				onRotate(rot)
			}
		case <-ticker.C:
			if end, done := qrCheck(env); done {
				return end, true
			}
		}
	}
}

// rotateADVSecret troca a chave ADV antiga pela nova do código atual em diante,
// como o qrchan nativo do whatsmeow faz. Os códigos já exibidos não importam.
func rotateADVSecret(codes []string, from int, oldSecret, newSecret string) []string {
	out := append([]string(nil), codes...)
	for i := from; i < len(out); i++ {
		out[i] = strings.Replace(out[i], oldSecret, newSecret, 1)
	}
	return out
}

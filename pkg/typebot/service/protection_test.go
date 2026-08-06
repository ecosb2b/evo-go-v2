package typebot_service

import (
	"sync"
	"testing"
	"time"
)

// TestNormalizeSelfJid é a peça que faz a guarda de auto-laço funcionar: o JID
// gravado na instância traz o número do aparelho vinculado, e o remetente de
// uma mensagem chega sem ele. Comparar as strings cruas deixaria o laço passar.
func TestNormalizeSelfJid(t *testing.T) {
	cases := []struct {
		jid  string
		want string
	}{
		{"5588999999999:7@s.whatsapp.net", "5588999999999@s.whatsapp.net"},
		{"5588999999999@s.whatsapp.net", "5588999999999@s.whatsapp.net"},
		{"216754450071619:3@lid", "216754450071619@lid"},
		{"216754450071619@lid", "216754450071619@lid"},
		{"", ""},
		// Sem servidor não há o que normalizar; devolve como veio.
		{"5588999999999", "5588999999999"},
	}

	for _, tc := range cases {
		t.Run(tc.jid, func(t *testing.T) {
			if got := normalizeSelfJid(tc.jid); got != tc.want {
				t.Errorf("normalizeSelfJid(%q) = %q, want %q", tc.jid, got, tc.want)
			}
		})
	}
}

// TestSendBucketAllowsBurst confirma que o balde entrega a rajada configurada
// sem espera. Um pico normal de atendimento não pode ser penalizado.
func TestSendBucketAllowsBurst(t *testing.T) {
	registry := newBucketRegistry(20, 20)

	for i := 0; i < 20; i++ {
		if delay := registry.wait("instancia"); delay != 0 {
			t.Fatalf("envio %d esperou %s, deveria estar dentro da rajada", i+1, delay)
		}
	}
}

// TestSendBucketThrottlesBeyondBurst confirma que o envio seguinte à rajada
// espera — e por um tempo compatível com a taxa, não um valor arbitrário.
func TestSendBucketThrottlesBeyondBurst(t *testing.T) {
	// 60/min = 1/s, rajada de 1: o segundo envio deve esperar perto de 1s.
	registry := newBucketRegistry(60, 1)

	if delay := registry.wait("instancia"); delay != 0 {
		t.Fatalf("primeiro envio esperou %s", delay)
	}

	start := time.Now()
	delay := registry.wait("instancia")
	elapsed := time.Since(start)

	if delay == 0 {
		t.Fatal("segundo envio não esperou apesar de a rajada estar esgotada")
	}
	if delay > 2*time.Second {
		t.Errorf("espera de %s é longa demais para 60/min", delay)
	}
	// wait() dorme de verdade, então o tempo decorrido acompanha o atraso.
	if elapsed < delay/2 {
		t.Errorf("wait() devolveu %s mas só bloqueou por %s", delay, elapsed)
	}
}

// TestSendBucketDisabled garante que o padrão desligado não introduz atraso —
// é o que está em produção até alguém configurar TYPEBOT_SEND_RATE_LIMIT.
func TestSendBucketDisabled(t *testing.T) {
	registry := newBucketRegistry(0, 20)

	if registry.enabled() {
		t.Fatal("registry deveria estar desligado com limite 0")
	}
	for i := 0; i < 100; i++ {
		if delay := registry.wait("instancia"); delay != 0 {
			t.Fatalf("envio %d esperou %s com o teto desligado", i+1, delay)
		}
	}
}

// TestSendBucketIsolatesInstances confirma que o teto é por instância: o
// consumo de uma não pode atrasar outra.
func TestSendBucketIsolatesInstances(t *testing.T) {
	registry := newBucketRegistry(60, 1)

	registry.wait("instancia-a")

	if delay := registry.wait("instancia-b"); delay != 0 {
		t.Errorf("instancia-b esperou %s por consumo da instancia-a", delay)
	}
}

// TestSendBucketConcurrent exercita o caminho que realmente ocorre em produção:
// várias goroutines de resposta disputando o mesmo balde. Rodar com -race.
func TestSendBucketConcurrent(t *testing.T) {
	registry := newBucketRegistry(600, 50)

	var wg sync.WaitGroup
	wg.Add(50)
	for i := 0; i < 50; i++ {
		go func() {
			defer wg.Done()
			registry.wait("instancia")
		}()
	}
	wg.Wait()
}

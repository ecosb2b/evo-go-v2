package typebot_service

import (
	"strings"
	"sync"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_repository "github.com/evolution-foundation/evolution-go/pkg/instance/repository"
)

// Motivos de pausa automática. Vão no campo PausedReason da sessão e no evento
// TypebotAutoPaused, então mudar estes valores quebra automações de quem já
// consome o webhook.
const (
	ReasonRateLimit = "rate_limit"
	ReasonSelfLoop  = "self_loop"
)

// AlertEmitter entrega eventos operacionais — pausas automáticas — pelo webhook
// da instância.
//
// É uma interface declarada aqui, no consumidor, em vez de uma dependência
// direta do serviço do whatsmeow: mantém este pacote testável sem subir um
// cliente de WhatsApp, e deixa explícito que a única coisa que precisamos dele
// é publicar um evento.
type AlertEmitter interface {
	SendOperationalEvent(instance *instance_model.Instance, event string, data map[string]any)
}

// selfJidCache guarda os JIDs das instâncias do próprio servidor.
//
// Serve à proteção mais importante do conjunto: impedir que duas instâncias
// nossas conversem entre si. Um laço desses não tem freio natural — cada
// resposta gera outra, nas duas pontas, no ritmo máximo — e é o caminho mais
// rápido para o número ser bloqueado.
//
// Diferente das outras proteções, esta não é heurística: ou o JID é nosso, ou
// não é.
type selfJidCache struct {
	repository instance_repository.InstanceRepository
	ttl        time.Duration

	mu        sync.RWMutex
	jids      map[string]struct{}
	refreshed time.Time
}

func newSelfJidCache(repository instance_repository.InstanceRepository) *selfJidCache {
	return &selfJidCache{
		repository: repository,
		// Instâncias novas aparecem em no máximo um minuto. Consultar o banco a
		// cada mensagem seria desperdício para um dado que quase nunca muda.
		ttl:  time.Minute,
		jids: map[string]struct{}{},
	}
}

// contains informa se o JID pertence a alguma instância deste servidor.
//
// Em caso de falha ao consultar o banco, responde false: bloquear conversas
// legítimas por causa de uma indisponibilidade momentânea seria pior que perder
// a proteção por um minuto, e a próxima mensagem tenta de novo.
func (c *selfJidCache) contains(remoteJid string) bool {
	if remoteJid == "" {
		return false
	}

	c.mu.RLock()
	fresh := time.Since(c.refreshed) < c.ttl
	_, found := c.jids[normalizeSelfJid(remoteJid)]
	c.mu.RUnlock()

	if fresh {
		return found
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Outra goroutine pode ter recarregado enquanto esperávamos o lock.
	if time.Since(c.refreshed) < c.ttl {
		_, found := c.jids[normalizeSelfJid(remoteJid)]
		return found
	}

	instances, err := c.repository.GetAll("")
	if err != nil {
		c.refreshed = time.Now()
		return false
	}

	jids := make(map[string]struct{}, len(instances))
	for _, instance := range instances {
		if instance == nil || instance.Jid == "" {
			continue
		}
		jids[normalizeSelfJid(instance.Jid)] = struct{}{}
	}

	c.jids = jids
	c.refreshed = time.Now()

	_, found = c.jids[normalizeSelfJid(remoteJid)]
	return found
}

// normalizeSelfJid descarta o sufixo de dispositivo para comparar JIDs.
//
// O JID gravado na instância costuma trazer o número do aparelho vinculado
// ("5588999999999:7@s.whatsapp.net"), enquanto o remetente de uma mensagem
// chega sem ele. Comparar as strings cruas deixaria a proteção passar batido.
func normalizeSelfJid(jid string) string {
	at := strings.Index(jid, "@")
	if at < 0 {
		return jid
	}

	user, server := jid[:at], jid[at:]
	if colon := strings.Index(user, ":"); colon >= 0 {
		user = user[:colon]
	}
	return user + server
}

// sendBucket limita o volume de envio de uma instância — o total somado de
// todos os contatos, que é a métrica que o WhatsApp observa.
//
// O limite por contato não cobre este caso: cem contatos com nove mensagens
// cada não estouram nenhum limite individual, mas produzem novecentos envios em
// rajada. É o padrão que derruba número.
//
// Vem desligado por padrão (limit = 0), porque um valor mal calibrado atrasaria
// respostas legítimas num pico normal de atendimento.
type sendBucket struct {
	mu sync.Mutex

	// tokens é fracionário para que a reposição seja contínua em vez de dar
	// saltos a cada minuto cheio.
	tokens     float64
	capacity   float64
	perSecond  float64
	lastRefill time.Time
}

type bucketRegistry struct {
	mu        sync.Mutex
	buckets   map[string]*sendBucket
	perMinute int
	burst     int
}

func newBucketRegistry(perMinute, burst int) *bucketRegistry {
	if burst <= 0 {
		burst = perMinute
	}
	return &bucketRegistry{
		buckets:   map[string]*sendBucket{},
		perMinute: perMinute,
		burst:     burst,
	}
}

// enabled informa se o teto de envio está configurado.
func (r *bucketRegistry) enabled() bool {
	return r != nil && r.perMinute > 0
}

// wait segura a goroutine até haver ficha disponível para a instância.
//
// Bloquear em vez de descartar é deliberado: a resposta do bot atrasada ainda é
// útil, enquanto uma resposta perdida deixa o contato sem retorno. Quem chama
// já está numa goroutine própria, então a espera não trava o processamento de
// eventos.
func (r *bucketRegistry) wait(instanceID string) time.Duration {
	if !r.enabled() {
		return 0
	}

	r.mu.Lock()
	bucket, ok := r.buckets[instanceID]
	if !ok {
		bucket = &sendBucket{
			tokens:     float64(r.burst),
			capacity:   float64(r.burst),
			perSecond:  float64(r.perMinute) / 60,
			lastRefill: time.Now(),
		}
		r.buckets[instanceID] = bucket
	}
	r.mu.Unlock()

	delay := bucket.take()
	if delay > 0 {
		time.Sleep(delay)
	}
	return delay
}

// take consome uma ficha e devolve quanto é preciso esperar por ela.
func (b *sendBucket) take() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	b.tokens += now.Sub(b.lastRefill).Seconds() * b.perSecond
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	b.lastRefill = now

	if b.tokens >= 1 {
		b.tokens--
		return 0
	}

	// Sem ficha: o débito é registrado agora e a espera é o tempo até repor a
	// fração que falta. Assim chamadas concorrentes se enfileiram em vez de
	// acordarem todas no mesmo instante.
	missing := 1 - b.tokens
	b.tokens--
	return time.Duration(missing / b.perSecond * float64(time.Second))
}

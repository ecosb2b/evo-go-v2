package websocket_producer

import (
	"net/http"
	"strings"
	"sync"
	"time"

	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/gomessguii/logger"
	"github.com/gorilla/websocket"

	"github.com/evolution-foundation/evolution-go/pkg/utils"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		logger.LogInfo("Verificando origem da conexão WebSocket")
		return true
	},
}

// wsConn serializa writes: gorilla/websocket permite apenas um writer concorrente.
type wsConn struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

// wsWriteTimeout limita quanto um cliente que parou de ler segura a entrega:
// sem prazo, o write bloqueava a goroutine do evento indefinidamente.
const wsWriteTimeout = 10 * time.Second

func (w *wsConn) writeJSON(v any) error {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	_ = w.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
	return w.conn.WriteJSON(v)
}

type websocketProducer struct {
	// Várias conexões por instância: antes uma segunda aba substituía a
	// primeira, e fechar qualquer uma removia a instância inteira. [upstream#181]
	clients       map[string][]*wsConn
	broadcast     []*wsConn // conexões que recebem todos os eventos
	clientsMux    sync.RWMutex
	loggerWrapper *logger_wrapper.LoggerManager
}

func NewWebsocketProducer(loggerWrapper *logger_wrapper.LoggerManager) *websocketProducer {
	return &websocketProducer{
		clients:       make(map[string][]*wsConn),
		broadcast:     make([]*wsConn, 0),
		loggerWrapper: loggerWrapper,
	}
}

// ServeWs lida com as requisições de upgrade para websocket. instanceId vazio
// assina os eventos de todas as instâncias.
func ServeWs(w http.ResponseWriter, r *http.Request, instanceId string, producer *websocketProducer) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.LogError("Erro ao fazer upgrade da conexão websocket: %v", err)
		return
	}

	if instanceId == "" {
		producer.AddBroadcastClient(conn)
	} else {
		producer.AddClient(instanceId, conn)
	}

	// O loop de leitura só serve para notar o fechamento. A remoção é desta
	// conexão, não da instância: as outras abas seguem recebendo.
	go func() {
		defer utils.Recover("websocketReadLoop")
		defer func() {
			if instanceId == "" {
				producer.RemoveBroadcastClient(conn)
			} else {
				producer.RemoveClient(instanceId, conn)
			}
			_ = conn.Close()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}

func (p *websocketProducer) AddBroadcastClient(conn *websocket.Conn) {
	p.clientsMux.Lock()
	p.broadcast = append(p.broadcast, &wsConn{conn: conn})
	n := len(p.broadcast)
	p.clientsMux.Unlock()
	logger.LogInfo("Cliente broadcast websocket adicionado (total: %d)", n)
}

func (p *websocketProducer) RemoveBroadcastClient(conn *websocket.Conn) {
	p.clientsMux.Lock()
	p.broadcast = withoutConn(p.broadcast, conn)
	n := len(p.broadcast)
	p.clientsMux.Unlock()
	logger.LogInfo("Cliente broadcast websocket removido (total: %d)", n)
}

func (p *websocketProducer) AddClient(instanceID string, conn *websocket.Conn) {
	p.clientsMux.Lock()
	p.clients[instanceID] = append(p.clients[instanceID], &wsConn{conn: conn})
	n := len(p.clients[instanceID])
	p.clientsMux.Unlock()
	p.loggerWrapper.GetLogger(instanceID).LogInfo("Cliente websocket adicionado para instância: %s (total: %d)", instanceID, n)
}

// RemoveClient remove só esta conexão da instância.
func (p *websocketProducer) RemoveClient(instanceID string, conn *websocket.Conn) {
	p.clientsMux.Lock()
	n := p.removeInstanceConnLocked(instanceID, conn)
	p.clientsMux.Unlock()
	p.loggerWrapper.GetLogger(instanceID).LogInfo("Cliente websocket removido para instância: %s (restantes: %d)", instanceID, n)
}

func (p *websocketProducer) removeInstanceConnLocked(instanceID string, conn *websocket.Conn) int {
	remaining := withoutConn(p.clients[instanceID], conn)
	if len(remaining) == 0 {
		delete(p.clients, instanceID)
	} else {
		p.clients[instanceID] = remaining
	}
	return len(remaining)
}

// withoutConn devolve uma fatia nova sem conn. Não filtra no lugar: um Produce
// concorrente pode estar iterando a fatia antiga.
func withoutConn(list []*wsConn, conn *websocket.Conn) []*wsConn {
	out := make([]*wsConn, 0, len(list))
	for _, c := range list {
		if c.conn != conn {
			out = append(out, c)
		}
	}
	return out
}

func (p *websocketProducer) Produce(queueName string, payload []byte, instanceID string, _ string) error {
	message := map[string]interface{}{
		"queue":   strings.ToLower(queueName),
		"payload": string(payload),
	}

	// Copia os destinos sob o lock e escreve fora dele: um socket lento ou
	// meio aberto não pode travar a entrega das outras instâncias.
	p.clientsMux.RLock()
	instanceTargets := append([]*wsConn(nil), p.clients[instanceID]...)
	broadcastTargets := append([]*wsConn(nil), p.broadcast...)
	p.clientsMux.RUnlock()

	var firstErr error
	var deadInstance, deadBroadcast []*websocket.Conn
	for _, c := range instanceTargets {
		if err := c.writeJSON(message); err != nil {
			p.loggerWrapper.GetLogger(instanceID).LogError("Erro ao enviar mensagem websocket para %s: %v", instanceID, err)
			deadInstance = append(deadInstance, c.conn)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	for _, c := range broadcastTargets {
		if err := c.writeJSON(message); err != nil {
			p.loggerWrapper.GetLogger(instanceID).LogError("Erro ao enviar mensagem broadcast websocket: %v", err)
			deadBroadcast = append(deadBroadcast, c.conn)
		}
	}

	// Conexão que falhou na escrita está morta; o loop de leitura dela também
	// a removeria, mas só quando notar o fechamento.
	if len(deadInstance) > 0 || len(deadBroadcast) > 0 {
		p.clientsMux.Lock()
		for _, conn := range deadInstance {
			p.removeInstanceConnLocked(instanceID, conn)
		}
		for _, conn := range deadBroadcast {
			p.broadcast = withoutConn(p.broadcast, conn)
		}
		p.clientsMux.Unlock()
		// Fechar encerra também o loop de leitura dessas conexões.
		for _, conn := range append(deadInstance, deadBroadcast...) {
			_ = conn.Close()
		}
	}

	return firstErr
}

// CreateGlobalQueues não faz nada para websocket producer
func (p *websocketProducer) CreateGlobalQueues() error {
	return nil
}

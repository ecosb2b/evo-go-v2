package websocket_producer

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// dialPair devolve a conexão do lado do servidor e a do cliente, para o teste
// ler o que o producer escreveu.
func dialPair(t *testing.T) (server, client *websocket.Conn) {
	t.Helper()
	upgraded := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		upgraded <- conn
	}))
	t.Cleanup(srv.Close)

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	select {
	case server = <-upgraded:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for upgrade")
	}
	t.Cleanup(func() { server.Close() })
	return server, client
}

func readQueue(t *testing.T, c *websocket.Conn) string {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg map[string]any
	if err := c.ReadJSON(&msg); err != nil {
		t.Fatalf("read: %v", err)
	}
	q, _ := msg["queue"].(string)
	return q
}

// Duas abas da mesma instância: antes a segunda substituía a primeira, e
// fechar qualquer uma removia a instância inteira.
func TestProduceDeliversToEverySubscriberOfAnInstance(t *testing.T) {
	producer := NewWebsocketProducer(newTestLoggerManager(t))
	serverA, clientA := dialPair(t)
	serverB, clientB := dialPair(t)
	producer.AddClient("inst-a", serverA)
	producer.AddClient("inst-a", serverB)

	if err := producer.Produce("inst-a.message", []byte(`{}`), "inst-a", ""); err != nil {
		t.Fatalf("produce: %v", err)
	}
	if q := readQueue(t, clientA); q != "inst-a.message" {
		t.Fatalf("subscriber A got %q", q)
	}
	if q := readQueue(t, clientB); q != "inst-a.message" {
		t.Fatalf("subscriber B got %q", q)
	}

	producer.RemoveClient("inst-a", serverA)
	if err := producer.Produce("inst-a.receipt", []byte(`{}`), "inst-a", ""); err != nil {
		t.Fatalf("produce after removal: %v", err)
	}
	if q := readQueue(t, clientB); q != "inst-a.receipt" {
		t.Fatalf("remaining subscriber got %q", q)
	}
}

// Uma conexão morta é descartada e não impede a entrega às outras.
func TestProduceDropsDeadSubscriber(t *testing.T) {
	producer := NewWebsocketProducer(newTestLoggerManager(t))
	dead, _ := dialPair(t)
	alive, aliveClient := dialPair(t)
	producer.AddClient("inst-a", dead)
	producer.AddClient("inst-a", alive)
	dead.Close()

	_ = producer.Produce("inst-a.message", []byte(`{}`), "inst-a", "")
	if q := readQueue(t, aliveClient); q != "inst-a.message" {
		t.Fatalf("live subscriber got %q", q)
	}

	producer.clientsMux.RLock()
	n := len(producer.clients["inst-a"])
	producer.clientsMux.RUnlock()
	if n != 1 {
		t.Fatalf("subscribers after dead write = %d, want 1", n)
	}
}

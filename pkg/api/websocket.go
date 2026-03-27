package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true // tightened in production via CORS middleware
	},
}

type WSGateway struct {
	natsConn *nats.Conn
	js       nats.JetStreamContext
	stream   string
	mu       sync.RWMutex
	clients  map[*wsClient]bool
}

type wsClient struct {
	conn     *websocket.Conn
	tenantID string
	send     chan []byte
	done     chan struct{}
}

func NewWSGateway(nc *nats.Conn, stream string) (*WSGateway, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, err
	}
	return &WSGateway{
		natsConn: nc,
		js:       js,
		stream:   stream,
		clients:  make(map[*wsClient]bool),
	}, nil
}

func (gw *WSGateway) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("websocket upgrade failed")
		return
	}

	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "missing tenant_id"))
		conn.Close()
		return
	}

	client := &wsClient{
		conn:     conn,
		tenantID: tenantID,
		send:     make(chan []byte, 256),
		done:     make(chan struct{}),
	}

	gw.mu.Lock()
	gw.clients[client] = true
	gw.mu.Unlock()

	go gw.writePump(client)
	go gw.readPump(client)
	go gw.subscribeForClient(client)

	log.Info().Str("tenant", tenantID).Msg("WebSocket client connected")
}

func (gw *WSGateway) writePump(c *wsClient) {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
		gw.removeClient(c)
	}()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-c.done:
			return
		}
	}
}

func (gw *WSGateway) readPump(c *wsClient) {
	defer func() {
		close(c.done)
		c.conn.Close()
		gw.removeClient(c)
	}()

	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
	}
}

func (gw *WSGateway) subscribeForClient(c *wsClient) {
	subjects := []string{
		gw.stream + "." + c.tenantID + ".flows",
		gw.stream + "." + c.tenantID + ".metrics",
		gw.stream + "." + c.tenantID + ".training",
	}

	var subs []*nats.Subscription
	for _, subj := range subjects {
		sub, err := gw.natsConn.Subscribe(subj, func(msg *nats.Msg) {
			envelope := map[string]interface{}{
				"subject": msg.Subject,
			}
			var payload interface{}
			if json.Unmarshal(msg.Data, &payload) == nil {
				envelope["data"] = payload
			}
			data, _ := json.Marshal(envelope)
			select {
			case c.send <- data:
			default:
				// drop if client is slow
			}
		})
		if err != nil {
			log.Warn().Err(err).Str("subject", subj).Msg("NATS subscribe failed")
			continue
		}
		subs = append(subs, sub)
	}

	<-c.done

	for _, sub := range subs {
		sub.Unsubscribe()
	}
}

func (gw *WSGateway) removeClient(c *wsClient) {
	gw.mu.Lock()
	delete(gw.clients, c)
	gw.mu.Unlock()
}

func (gw *WSGateway) Shutdown(_ context.Context) {
	gw.mu.Lock()
	defer gw.mu.Unlock()
	for c := range gw.clients {
		close(c.send)
	}
}

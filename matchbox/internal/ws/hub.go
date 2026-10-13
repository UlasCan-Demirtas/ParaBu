package ws

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	
	"matchbox/pkg/metrics"
)

// Message is an envelope for broadcasted data over WebSocket.
type Message struct {
	Topic   string      `json:"topic"`
	Payload interface{} `json:"payload"`
}

// ClientRequest represents a message sent from the frontend to manage subscriptions.
type ClientRequest struct {
	Action string   `json:"action"` // "subscribe" or "unsubscribe"
	Topics []string `json:"topics"`
}

// Hub maintains active WebSocket clients and routes messages based on topics.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan Message
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex // Protects the clients map
	rdb        *redis.Client
}

// NewHub creates a new WebSocket hub.
func NewHub(rdb *redis.Client) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan Message, 1024),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		rdb:        rdb,
	}
}

// Run starts the hub's main select loop.
func (h *Hub) Run() {
	if h.rdb != nil {
		go h.redisSubscriber()
	}

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			metrics.WsActiveConnections.Inc()
			log.Printf("New WS client connected. Total clients: %d", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				metrics.WsActiveConnections.Dec()
				close(client.send)
				log.Printf("WS client disconnected. Total clients: %d", len(h.clients))
			}
			h.mu.Unlock()

		case msg := <-h.broadcast:
			// Marshal once for all subscribed clients
			payload, err := json.Marshal(msg)
			if err != nil {
				log.Printf("Error marshaling broadcast message: %v", err)
				continue
			}

			h.mu.RLock()
			for client := range h.clients {
				// Topic-based routing
				if client.hasTopic(msg.Topic) {
					select {
					case client.send <- payload:
					default:
						// Buffer full, client is too slow.
						// We don't delete here to avoid map mutation during RLock.
						// The writePump will detect the closed channel and unregister.
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// redisSubscriber listens to Redis Pub/Sub and pushes messages to the local broadcast channel.
func (h *Hub) redisSubscriber() {
	ctx := context.Background()
	// Subscribe to all ws_events topics
	pubsub := h.rdb.PSubscribe(ctx, "ws_events:*")
	defer pubsub.Close()
	
	ch := pubsub.Channel()
	for msg := range ch {
		var m Message
		if err := json.Unmarshal([]byte(msg.Payload), &m); err == nil {
			select {
			case h.broadcast <- m:
			default:
				log.Println("Warning: Local WS broadcast buffer full, dropping redis message")
			}
		}
	}
}

// Broadcast accepts a message and queues it for distribution.
// If Redis is configured, it publishes to Redis instead of local broadcast.
func (h *Hub) Broadcast(topic string, payload interface{}) {
	msg := Message{Topic: topic, Payload: payload}
	
	if h.rdb != nil {
		data, err := json.Marshal(msg)
		if err == nil {
			// Publish to a specific Redis channel for this topic
			h.rdb.Publish(context.Background(), "ws_events:"+topic, data)
		}
	} else {
		select {
		case h.broadcast <- msg:
		default:
			log.Println("Warning: WS broadcast buffer full, dropping message on topic:", topic)
		}
	}
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	topics map[string]bool
	mu     sync.RWMutex // Protects topics map
}

func (c *Client) hasTopic(topic string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.topics[topic]
}

func (c *Client) addTopics(topics []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range topics {
		c.topics[t] = true
	}
}

func (c *Client) removeTopics(topics []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range topics {
		delete(c.topics, t)
	}
}

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 1024 * 4
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins
	},
}

// writePump pumps messages from the hub to the websocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Drain any remaining messages in the channel to send as a batch
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump pumps messages from the websocket connection to the hub.
// This is essential for detecting disconnects and processing client subscriptions.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WS read error: %v", err)
			}
			break
		}

		var req ClientRequest
		if err := json.Unmarshal(message, &req); err != nil {
			log.Printf("Invalid message from client: %v", err)
			continue
		}

		switch req.Action {
		case "subscribe":
			c.addTopics(req.Topics)
		case "unsubscribe":
			c.removeTopics(req.Topics)
		case "ping":
			// Respond with pong for client-side latency measurement
			pong, _ := json.Marshal(map[string]string{"action": "pong"})
			c.send <- pong
		default:
			log.Printf("Unknown WS action: %s", req.Action)
		}
	}
}

// ServeWs handles websocket requests from the peer.
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	client := &Client{
		hub:    hub,
		conn:   conn,
		send:   make(chan []byte, 256),
		topics: make(map[string]bool),
	}

	client.hub.register <- client

	go client.writePump()
	go client.readPump()
}

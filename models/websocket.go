package models

import (
	"github.com/gorilla/websocket"
	"log"
	"sync"
)

type WSMessage struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

type Client struct {
	Conn *websocket.Conn
	Send chan WSMessage
}

type Hub struct {
	Clients      map[*Client]bool
	Broadcast    chan WSMessage
	Register     chan *Client
	RoundManager *RoundManager
	Unregister   chan *Client
	Mutex        sync.Mutex
}

// NewHub initializes and returns a new Hub
func NewHub() *Hub {
	return &Hub{
		Clients:    make(map[*Client]bool),
		Broadcast:  make(chan WSMessage, 256),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.Mutex.Lock()
			h.Clients[client] = true
			h.Mutex.Unlock()
			log.Println("Client registered")
		case client := <-h.Unregister:
			h.Mutex.Lock()
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.Send)
				log.Println("Client unregistered")
			}
			h.Mutex.Unlock()
		case message := <-h.Broadcast:
			h.Mutex.Lock()
			for client := range h.Clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.Clients, client)
				}
			}
			h.Mutex.Unlock()
		}
	}
}

// ReadPump handles incoming messages from a client (optional, as we only send from server)
func (c *Client) ReadPump(h *Hub) {
	defer func() {
		h.Unregister <- c
		c.Conn.Close()
	}()
	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		// You can handle incoming messages from clients here if needed
	}
}

// WritePump sends messages from the Send channel to the WebSocket connection
func (c *Client) WritePump() {
	defer func() {
		c.Conn.Close()
	}()
	for message := range c.Send {
		if err := c.Conn.WriteJSON(message); err != nil {
			log.Println("Write error:", err)
			break
		}
	}
}

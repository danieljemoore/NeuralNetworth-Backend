package websocket

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"midnight-trader/controllers"
	"midnight-trader/models"
)

var (
	Clients   = make(map[*websocket.Conn]bool)
	Broadcast = make(chan models.WSMessage)
	WSMutex   sync.Mutex
	Upgrader  = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

func WebSocketBroadcaster() {
	for msg := range Broadcast {
		WSMutex.Lock()
		for client := range Clients {
			if err := client.WriteJSON(msg); err != nil {
				client.Close()
				delete(Clients, client)
			}
		}
		WSMutex.Unlock()
	}
}

// ServeWs handles WebSocket requests from clients
func ServeWs(h *models.Hub, w http.ResponseWriter, r *http.Request) {
	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		// Allow all origins for simplicity; adjust in production
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}

	client := &models.Client{Conn: conn, Send: make(chan models.WSMessage, 256)}
	h.Register <- client

	// Start read and write pumps
	go client.WritePump()
	go client.ReadPump(h)
	// Send the current round state to the newly connected client
	go func() {
		currentRound := controllers.GetCurrentRound()
		if currentRound != nil {
			roundData, err := json.Marshal(currentRound)
			if err != nil {
				log.Println("Error marshalling currentRound:", err)
				return
			}

			wsMessage := models.WSMessage{
				Event: "current_round",
				Data:  json.RawMessage(roundData),
			}
			client.Send <- wsMessage
		} else {
			wsMessage := models.WSMessage{
				Event: "no_active_round",
				Data:  json.RawMessage(`{}`),
			}
			client.Send <- wsMessage
		}
	}()

	// Fetch and send all portfolios upon connection
	go func() {
		// Create a context with a timeout to avoid hanging
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Fetch portfolios from the database
		portfolios, err := controllers.GetPortfolios(ctx)
		if err != nil {
			log.Println("Failed to get portfolios:", err)

			// Optionally, send an error message to the client
			errorMsg := models.WSMessage{
				Event: "error",
				Data:  "Failed to fetch portfolios",
			}
			if err := conn.WriteJSON(errorMsg); err != nil {
				log.Println("WriteJSON error:", err)
				// Unregister and close the connection if unable to send error
				h.Unregister <- client
				conn.Close()
			}
			return
		}

		// Create the all_portfolios message
		portfoliosMsg := models.WSMessage{
			Event: "all_portfolios",
			Data:  portfolios,
		}

		// Send the message to the client
		if err := conn.WriteJSON(portfoliosMsg); err != nil {
			log.Println("WriteJSON error:", err)
			// Unregister and close the connection if unable to send portfolios
			h.Unregister <- client
			conn.Close()
			return
		}
	}()
}

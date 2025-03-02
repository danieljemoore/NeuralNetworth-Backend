package controllers

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"midnight-trader/models"
	"midnight-trader/websocket"
)

func WebSocketHandler(c *gin.Context) {
	conn, err := websocket.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// send a welcome message
	welcome := models.WSMessage{Event: "welcome", Data: "connected to server"}
	if err := conn.WriteJSON(welcome); err != nil {
		return
	}

	websocket.WSMutex.Lock()
	websocket.Clients[conn] = true
	websocket.WSMutex.Unlock()

	// Fetch and send all portfolios upon connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	portfolios, err := GetPortfolios(ctx)
	if err == nil {
		portfoliosMsg := models.WSMessage{Event: "all_portfolios", Data: portfolios}
		if err := conn.WriteJSON(portfoliosMsg); err != nil {
			websocket.WSMutex.Lock()
			delete(websocket.Clients, conn)
			websocket.WSMutex.Unlock()
			return
		}
	}
	// Listen for messages from the client
	for {
		var msg models.WSMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			websocket.WSMutex.Lock()
			delete(websocket.Clients, conn)
			websocket.WSMutex.Unlock()
			break
		}
		// optional: filter or validate msg.event here
		websocket.Broadcast <- msg
	}
}

func SendRoundUpdate(event string, data interface{}) {
	msg := models.WSMessage{Event: event, Data: data}
	websocket.Broadcast <- msg
}

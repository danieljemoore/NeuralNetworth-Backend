package websocket

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"

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

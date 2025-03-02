package models

type WSMessage struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

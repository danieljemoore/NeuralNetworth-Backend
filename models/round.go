package models

import (
	"time"
)

type RoundState struct {
	ID        int       `json:"id" bson:"id"`
	Status    string    `json:"status" bson:"status"` // "lobby", "active", "ended"
	StartTime time.Time `json:"startTime,omitempty"`
	EndTime   time.Time `json:"endTime,omitempty"`
}

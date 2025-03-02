package models

import (
	"sync"
	"time"
)

type RoundState struct {
	ID           int         `json:"id" bson:"id"`
	Status       string      `json:"status" bson:"status"` // "lobby", "active", "ended"
	StartTime    time.Time   `json:"startTime,omitempty"`
	EndTime      time.Time   `json:"endTime,omitempty"`
	Participants []Portfolio `json:"players,omitempty"`
	Winner       *Portfolio  `json:"winner,omitempty"`
}

// Global variables (ensure proper initialization and synchronization)
var (
	CurrentRound  *RoundState
	RoundLock     sync.Mutex
	RoundDuration = 2 * time.Minute // Example duration; adjust as needed
	RoundTimer    *time.Timer
	TimerTicker   *time.Ticker
	TimerStopChan chan struct{}
)

type RoundManager struct {
	Hub             *Hub
	CurrentRound    *RoundState
	RoundLock       sync.Mutex
	RoundDuration   time.Duration
	TotalRounds     int // Total number of rounds to play; 0 for infinite
	CompletedRounds int // Number of rounds completed
	Timer           *time.Timer
	TimerTicker     *time.Ticker
	TimerStopChan   chan struct{}
}

